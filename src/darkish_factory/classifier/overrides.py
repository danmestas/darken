"""Override capture: write `override_recorded` events when operator disagrees.

The library does not own commit-state. The caller signals that an answer
was given against committed work by setting `HumanAnswer.against_committed`;
that flag flows straight through into the audit payload. Rollback itself is
the orchestrator's concern.
"""

from __future__ import annotations

from dataclasses import dataclass

from .answers import HumanAnswer
from .audit import AuditContext, AuditLog
from .decisions import Stage2Verdict


@dataclass
class OverrideCapture:
    audit_log: AuditLog

    def capture(
        self,
        *,
        verdict: Stage2Verdict,
        answer: HumanAnswer,
        audit_ctx: AuditContext,
        recommendation: str | None = None,
    ) -> None:
        disagreement = self._is_disagreement(verdict, answer, recommendation)
        if not disagreement:
            return

        payload = audit_ctx.envelope()
        payload.update(
            {
                "verdict_escalate": verdict.escalate,
                "verdict_categories": verdict.categories,
                "operator_kind": answer.kind,
                "operator_choice": answer.choice,
                "operator_direction": answer.direction,
                "matched_recommendation": (
                    recommendation is not None and answer.choice == recommendation
                ),
                "against_committed": answer.against_committed,
                "raw_text": answer.raw_text,
                "interpretation": answer.interpretation,
            }
        )
        self.audit_log.emit("override_recorded", payload)

    @staticmethod
    def _is_disagreement(
        verdict: Stage2Verdict, answer: HumanAnswer, recommendation: str | None
    ) -> bool:
        if not verdict.escalate and answer.kind == "ratify":
            return False
        if answer.kind in {"rework", "abort"}:
            return True
        if answer.kind == "choose" and recommendation is not None:
            return answer.choice != recommendation
        # ratify against an escalation verdict is a disagreement
        return bool(verdict.escalate and answer.kind == "ratify")


__all__ = ["OverrideCapture"]
