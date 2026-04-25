"""Public Classifier composing all internal modules."""

from __future__ import annotations

import random
import secrets
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from .answers import HumanAnswer, RequestHumanInput
from .audit import AuditContext, AuditLog
from .batcher import Batcher
from .constitution import load_constitution
from .decisions import ProposedDecision, RoutingInputs, Stage2Verdict
from .errors import (
    ClassifierOutageError,
    ConstitutionConflictError,
    EscalationRequired,
)
from .gate import Stage1Gate
from .llm_classifier import Stage2Classifier
from .overrides import OverrideCapture
from .policy import load_policy
from .routing import RoutingClassifier
from .spot_check import SpotChecker


@dataclass
class _PendingRequest:
    proposed_decision: ProposedDecision
    verdict: Stage2Verdict
    request: RequestHumanInput


@dataclass
class Classifier:
    """Public surface: `decide` and `resume`."""

    constitution_path: Path
    policy_path: Path
    audit_log: AuditLog
    llm_client: Any
    routing_client: Any | None = None
    spend_provider: Callable[[], float] | None = None
    clock: Callable[[], datetime] | None = None
    rng: random.Random | None = None

    _gate: Stage1Gate = field(init=False)
    _stage2: Stage2Classifier = field(init=False)
    _routing: RoutingClassifier = field(init=False)
    _batcher: Batcher = field(init=False)
    _spot: SpotChecker = field(init=False)
    _override: OverrideCapture = field(init=False)
    _pending: dict[str, _PendingRequest] = field(default_factory=dict)

    def __post_init__(self) -> None:
        self._constitution = load_constitution(self.constitution_path)
        self._policy = load_policy(self.policy_path)
        self._spend = self.spend_provider or (lambda: 0.0)
        self._clock_fn = self.clock or (lambda: datetime.now(UTC))
        self._rng_inst = self.rng or random.Random()

        self._gate = Stage1Gate(
            policy=self._policy,
            audit_log=self.audit_log,
            spend_provider=self._spend,
        )
        self._stage2 = Stage2Classifier(
            client=self.llm_client,
            audit_log=self.audit_log,
            constitution=self._constitution,
            policy=self._policy,
        )
        self._routing = RoutingClassifier(
            client=self.routing_client if self.routing_client is not None else self.llm_client,
            audit_log=self.audit_log,
            rubric_overrides=dict(self._policy.routing_rubric),
        )
        self._batcher = Batcher(
            batch_size=self._policy.thresholds.batch_size,
            max_latency_min=self._policy.thresholds.max_queue_latency_min,
            clock=self._clock_fn,
            audit_log=self.audit_log,
        )
        self._spot = SpotChecker(
            rate=self._policy.spot_check_rate,
            audit_log=self.audit_log,
            rng=self._rng_inst,
        )
        self._override = OverrideCapture(audit_log=self.audit_log)

    def _audit_ctx(self, decision_id: str) -> AuditContext:
        return AuditContext(
            decision_id=decision_id,
            constitution_hash=self._constitution.hash,
            policy_hash=self._policy.hash,
        )

    # ----- public surface -----

    def decide(self, proposed_decision: ProposedDecision) -> HumanAnswer | RequestHumanInput:
        ctx = self._audit_ctx(proposed_decision.decision_id)

        if proposed_decision.is_routing_request:
            return self._handle_routing(proposed_decision, ctx)

        # Stage 1: deterministic gate (LLM is never passed in).
        try:
            self._gate.evaluate(proposed_decision, audit_ctx=ctx)
        except EscalationRequired as exc:
            return self._build_request(
                proposed_decision,
                verdict=Stage2Verdict(
                    escalate=True,
                    categories=exc.categories,  # type: ignore[arg-type]
                    confidence=1.0,
                    reasoning=f"stage_1: {exc.trigger}",
                    produced_by="gate",
                ),
                question=f"Reversibility trigger fired: {exc.trigger}. Approve?",
                audit_ctx=ctx,
            )

        # Constitution structural check.
        try:
            self._constitution.assert_no_conflict(
                decision_text=proposed_decision.description,
                files_touched=list(proposed_decision.files_touched),
            )
        except ConstitutionConflictError as exc:
            payload = ctx.envelope()
            payload.update({"section": exc.section, "invariant": exc.invariant})
            self.audit_log.emit("constitution_conflict", payload)
            return self._build_request(
                proposed_decision,
                verdict=Stage2Verdict(
                    escalate=True,
                    categories=[],
                    confidence=1.0,
                    reasoning=f"constitution_conflict: {exc.section}/{exc.invariant}",
                    produced_by="gate",
                ),
                question=f"Constitution conflict: {exc.invariant}. Approve?",
                audit_ctx=ctx,
            )

        # Stage 2: adversarial LLM.
        try:
            verdict = self._stage2.classify(proposed_decision, audit_ctx=ctx)
        except ClassifierOutageError as exc:
            return self._build_request(
                proposed_decision,
                verdict=Stage2Verdict(
                    escalate=True,
                    categories=[],
                    confidence=0.0,
                    reasoning=f"classifier_outage: {exc}",
                    produced_by="gate",
                ),
                question="Classifier outage; please review.",
                audit_ctx=ctx,
            )

        if not verdict.escalate:
            for cat in verdict.categories or ["taste"]:
                self._spot.maybe_sample(category=cat, audit_ctx=ctx)
            return HumanAnswer(
                kind="ratify",
                raw_text="auto-ratified",
                interpretation="ratify",
            )

        return self._build_request(
            proposed_decision,
            verdict=verdict,
            question=f"Stage-2 escalation: {', '.join(verdict.categories) or 'unspecified'}",
            audit_ctx=ctx,
        )

    def resume(self, token: str, operator_answer: HumanAnswer) -> HumanAnswer:
        # Implemented in Task 14.
        raise NotImplementedError

    # ----- internals -----

    def _handle_routing(self, pd: ProposedDecision, ctx: AuditContext) -> HumanAnswer:
        inputs = pd.routing_inputs or RoutingInputs()
        label = self._routing.classify(inputs=inputs, audit_ctx=ctx)
        return HumanAnswer(
            kind="choose",
            choice=label,
            raw_text=label,
            interpretation=label,
        )

    def _build_request(
        self,
        pd: ProposedDecision,
        *,
        verdict: Stage2Verdict,
        question: str,
        audit_ctx: AuditContext,
    ) -> RequestHumanInput:
        token = secrets.token_urlsafe(16)
        request = RequestHumanInput(
            question=question,
            context=pd.description,
            urgency=pd.urgency,
            format="yes_no",
            choices=["yes", "no"],
            recommendation="no" if verdict.escalate else "yes",
            reasoning=verdict.reasoning,
            categories=verdict.categories,
            worktree_ref=pd.worktree_ref,
            resume_token=token,
        )
        self._pending[token] = _PendingRequest(
            proposed_decision=pd,
            verdict=verdict,
            request=request,
        )
        flushed = self._batcher.enqueue(request, audit_ctx=audit_ctx)
        del flushed  # batcher already emitted the event
        return request


__all__ = ["Classifier"]
