"""Override capture tests."""

from __future__ import annotations

from typing import Any

from darkish_factory.classifier.answers import HumanAnswer
from darkish_factory.classifier.audit import AuditContext
from darkish_factory.classifier.decisions import Stage2Verdict
from darkish_factory.classifier.overrides import OverrideCapture

CTX = AuditContext(decision_id="d?", constitution_hash="0" * 64, policy_hash="1" * 64)


def _ctx(decision_id: str) -> AuditContext:
    return AuditContext(
        decision_id=decision_id, constitution_hash="0" * 64, policy_hash="1" * 64
    )


class _CaptureLog:
    def __init__(self) -> None:
        self.events: list[tuple[str, dict[str, Any]]] = []

    def emit(self, event_type: str, payload: dict[str, Any]) -> None:
        self.events.append((event_type, payload))


def _verdict(escalate: bool, cats: list[str] | None = None) -> Stage2Verdict:
    return Stage2Verdict(
        escalate=escalate,
        categories=cats or [],  # type: ignore[arg-type]
        confidence=0.9,
        reasoning="r",
        produced_by="classifier",
    )


def test_agreement_does_not_record() -> None:
    log = _CaptureLog()
    cap = OverrideCapture(audit_log=log)
    verdict = _verdict(escalate=False)
    answer = HumanAnswer(kind="ratify", raw_text="ok", interpretation="ratify")
    cap.capture(verdict=verdict, answer=answer, audit_ctx=_ctx("d1"))
    assert log.events == []


def test_disagreement_records_override_with_full_envelope() -> None:
    log = _CaptureLog()
    cap = OverrideCapture(audit_log=log)
    verdict = _verdict(escalate=False)
    answer = HumanAnswer(
        kind="rework", direction="redo with snake_case", raw_text="redo", interpretation="redo"
    )
    cap.capture(verdict=verdict, answer=answer, audit_ctx=_ctx("d2"))
    types = [t for t, _ in log.events]
    assert "override_recorded" in types
    payload = next(p for t, p in log.events if t == "override_recorded")
    assert payload["decision_id"] == "d2"
    assert payload["operator_kind"] == "rework"
    assert payload["against_committed"] is False
    for required in ("decision_id", "timestamp", "constitution_hash", "policy_hash"):
        assert required in payload


def test_abort_against_committed_flag_passes_through() -> None:
    log = _CaptureLog()
    cap = OverrideCapture(audit_log=log)
    verdict = _verdict(escalate=False)
    answer = HumanAnswer(
        kind="abort",
        raw_text="kill",
        interpretation="abort",
        against_committed=True,
    )
    cap.capture(verdict=verdict, answer=answer, audit_ctx=_ctx("d3"))
    payload = next(p for t, p in log.events if t == "override_recorded")
    assert payload["against_committed"] is True


def test_choose_against_escalation_records_when_choice_differs_from_recommendation() -> None:
    log = _CaptureLog()
    cap = OverrideCapture(audit_log=log)
    verdict = _verdict(escalate=True, cats=["taste"])
    answer = HumanAnswer(
        kind="choose",
        choice="option_b",
        raw_text="b",
        interpretation="option_b",
    )
    cap.capture(
        verdict=verdict,
        answer=answer,
        audit_ctx=_ctx("d4"),
        recommendation="option_a",
    )
    payload = next(p for t, p in log.events if t == "override_recorded")
    assert payload["operator_kind"] == "choose"
    assert payload["matched_recommendation"] is False
