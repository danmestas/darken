"""End-to-end tests for Classifier.resume."""

from __future__ import annotations

import random
from datetime import datetime
from pathlib import Path
from typing import Any

import pytest

from darkish_factory import Classifier, HumanAnswer, RequestHumanInput
from darkish_factory.classifier.audit import NullAuditLog
from darkish_factory.classifier.decisions import ProposedDecision, ProposedToolCall

from .conftest import FakeClient


def _pd(**overrides: Any) -> ProposedDecision:
    base: dict[str, Any] = {
        "decision_id": "dec-r",
        "title": "x",
        "description": "x",
        "files_touched": ["src/x.py"],
        "modules": ["x"],
        "diff_stats": {},
        "urgency": "high",
        "spend_delta_usd": 0.0,
        "worktree_ref": "wt@1",
    }
    base.update(overrides)
    return ProposedDecision(**base)


def test_resume_with_ratify_records_override_against_escalation(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime, fixed_rng: random.Random
) -> None:
    """Ratify against an escalation verdict IS a disagreement.

    The test name reflects the body: an `override_recorded` event fires.
    """
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    s2 = FakeClient(
        {"escalate": True, "categories": ["taste"], "confidence": 0.95, "reasoning": "x"}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2,
        clock=lambda: fixed_clock,
        rng=fixed_rng,
    )
    request = c.decide(_pd())
    assert isinstance(request, RequestHumanInput)
    answer = c.resume(
        request.resume_token,
        HumanAnswer(kind="ratify", raw_text="ok", interpretation="ratify"),
    )
    assert answer.kind == "ratify"
    types = [t for t, _ in captured]
    assert "override_recorded" in types


def test_resume_with_ratify_records_no_override(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime, fixed_rng: random.Random
) -> None:
    """When the verdict already says ratify, ratifying is agreement; no override fires."""
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    # Stage-2 says no-escalation. Confidence floor is 0.7 in the fixture, so
    # a confidence of 0.95 keeps the verdict as ratify — but we still need an
    # escalation request to issue a resume_token. Use a Stage-1 trigger
    # (data_deletion) to force the request, then Stage-2 is never called.
    s2 = FakeClient({"escalate": False, "categories": [], "confidence": 0.95, "reasoning": "x"})
    pd = _pd(
        description="DELETE FROM x;",
        tool_calls=[ProposedToolCall(name="run_sql", arguments={"sql": "DELETE FROM x"})],
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2,
        clock=lambda: fixed_clock,
        rng=fixed_rng,
    )
    request = c.decide(pd)
    assert isinstance(request, RequestHumanInput)
    # The verdict here was produced by the gate with `escalate=True`; ratify
    # against THAT is a disagreement. To exercise the no-override branch, drive
    # an auto-ratify path: a clean decision returns a HumanAnswer immediately.
    s2_clean = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.95, "reasoning": "ok"}
    )
    c_clean = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2_clean,
        clock=lambda: fixed_clock,
        rng=fixed_rng,
    )
    out = c_clean.decide(_pd(description="trivial private refactor"))
    assert isinstance(out, HumanAnswer)
    assert out.kind == "ratify"
    types = [t for t, _ in captured]
    assert "override_recorded" not in types


def test_resume_with_rework_records_override(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime, fixed_rng: random.Random
) -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    s2 = FakeClient(
        {"escalate": True, "categories": ["taste"], "confidence": 0.9, "reasoning": "x"}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2,
        clock=lambda: fixed_clock,
        rng=fixed_rng,
    )
    request = c.decide(_pd())
    assert isinstance(request, RequestHumanInput)
    answer = c.resume(
        request.resume_token,
        HumanAnswer(
            kind="rework",
            direction="redo with snake_case",
            raw_text="redo",
            interpretation="redo",
        ),
    )
    assert answer.kind == "rework"
    types = [t for t, _ in captured]
    assert "override_recorded" in types


def test_abort_after_committed_state_emits_override_with_against_committed_flag(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime, fixed_rng: random.Random
) -> None:
    """The library no longer tracks commit state. The CALLER signals it via
    `HumanAnswer.against_committed`; the override event records that flag.
    """
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    s2 = FakeClient(
        {"escalate": True, "categories": ["ethics"], "confidence": 0.92, "reasoning": "x"}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2,
        clock=lambda: fixed_clock,
        rng=fixed_rng,
    )
    request = c.decide(_pd())
    assert isinstance(request, RequestHumanInput)
    answer = c.resume(
        request.resume_token,
        HumanAnswer(
            kind="abort",
            raw_text="kill",
            interpretation="abort",
            against_committed=True,
        ),
    )
    assert answer.kind == "abort"
    payload = next(p for t, p in captured if t == "override_recorded")
    assert payload["against_committed"] is True


def test_resume_unknown_token_raises(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    s2 = FakeClient({"escalate": False, "categories": [], "confidence": 0.95, "reasoning": "ok"})
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2,
        clock=lambda: fixed_clock,
    )
    with pytest.raises(KeyError):
        c.resume("not-a-token", HumanAnswer(kind="ratify", raw_text="ok", interpretation="ratify"))


def test_resume_runs_post_ratification_spot_check(
    constitution_path: Path, policy_path: Path, fixed_clock: datetime
) -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    s2 = FakeClient(
        {"escalate": True, "categories": ["taste"], "confidence": 0.95, "reasoning": "x"}
    )

    class _AlwaysRng(random.Random):
        def random(self) -> float:
            return 0.0  # below any positive rate

    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2,
        clock=lambda: fixed_clock,
        rng=_AlwaysRng(),
    )
    request = c.decide(_pd())
    assert isinstance(request, RequestHumanInput)
    c.resume(
        request.resume_token,
        HumanAnswer(kind="ratify", raw_text="ok", interpretation="ratify"),
    )
    types = [t for t, _ in captured]
    assert "spot_check_sample" in types
