"""End-to-end tests for Classifier.decide."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path
from typing import Any

import httpx
from anthropic import APIError

from darkish_factory import Classifier, RequestHumanInput
from darkish_factory.classifier.audit import NullAuditLog
from darkish_factory.classifier.decisions import ProposedDecision, ProposedToolCall

from .conftest import FakeClient


# Stub subclass to satisfy APIError's required `request` parameter (SDK v0.40+).
class _StubAPIError(APIError):
    def __init__(self) -> None:
        super().__init__(
            message="upstream 503",
            request=httpx.Request("GET", "https://api.anthropic.com"),
            body=None,
        )


def _decision(**overrides: Any) -> ProposedDecision:
    base: dict[str, Any] = {
        "decision_id": "dec-e2e",
        "title": "Refactor helper",
        "description": "Convert helper to pure.",
        "files_touched": ["src/util.py"],
        "modules": ["util"],
        "diff_stats": {"added": 30, "removed": 5},
        "urgency": "low",
        "spend_delta_usd": 0.0,
        "worktree_ref": "wt@abc",
    }
    base.update(overrides)
    return ProposedDecision(**base)


def test_auto_ratify_path_returns_human_answer_ratify(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    s2_client = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.95, "reasoning": "private refactor."}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2_client,
        clock=lambda: fixed_clock,
    )
    out = c.decide(_decision())
    assert out.__class__.__name__ == "HumanAnswer"
    assert out.kind == "ratify"  # type: ignore[union-attr]


def test_reversibility_trigger_escalates_without_calling_llm(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    class ExplodingClient:
        def __getattr__(self, name: str) -> object:
            raise AssertionError("LLM must not be called on Stage-1 escalation")

    pd = _decision(
        description="DELETE FROM accounts WHERE inactive=true;",
        tool_calls=[ProposedToolCall(name="run_sql", arguments={"sql": "DELETE FROM accounts"})],
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=ExplodingClient(),
        clock=lambda: fixed_clock,
    )
    out = c.decide(pd)
    assert isinstance(out, RequestHumanInput)
    assert "reversibility" in out.categories


def test_stage2_escalate_path_returns_request(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    s2_client = FakeClient(
        {"escalate": True, "categories": ["taste"], "confidence": 0.95, "reasoning": "renames API."}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2_client,
        clock=lambda: fixed_clock,
    )
    out = c.decide(_decision(urgency="high"))  # high urgency to bypass batcher
    assert isinstance(out, RequestHumanInput)
    assert out.categories == ["taste"]
    assert out.resume_token


def test_constitution_conflict_via_structural_check(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    s2_client = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "ok."}
    )
    pd = _decision(
        description="Add top-level module: admin/handlers.py",
        files_touched=["admin/handlers.py"],
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2_client,
        clock=lambda: fixed_clock,
    )
    out = c.decide(pd)
    assert isinstance(out, RequestHumanInput)
    # Conflict event records the matched invariant
    # (tested directly via audit-log assertion below)


def test_constitution_conflict_event_emitted(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    s2_client = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "ok."}
    )
    pd = _decision(
        description="Add top-level module: admin/handlers.py",
        files_touched=["admin/handlers.py"],
        urgency="high",
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2_client,
        clock=lambda: fixed_clock,
    )
    c.decide(pd)
    types = [t for t, _ in captured]
    assert "constitution_conflict" in types


def test_routing_request_returns_routing_label_as_ratified_answer(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    s2_client = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "ok."}
    )
    routing_client = FakeClient({"label": "heavy", "reasoning": "cross-module."})
    pd = _decision(is_routing_request=True)
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2_client,
        routing_client=routing_client,
        clock=lambda: fixed_clock,
    )
    out = c.decide(pd)
    assert out.__class__.__name__ == "HumanAnswer"
    assert out.kind == "choose"  # type: ignore[union-attr]
    assert out.choice == "heavy"  # type: ignore[union-attr]


def test_classifier_outage_escalates(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    err = _StubAPIError()
    s2_client = FakeClient(err)
    pd = _decision(urgency="high")
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2_client,
        clock=lambda: fixed_clock,
    )
    out = c.decide(pd)
    assert isinstance(out, RequestHumanInput)
    assert "classifier_outage" in out.reasoning
