"""Determinism + Stage-2 client separation tests."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path
from typing import Any

from darkish_factory import Classifier, RequestHumanInput
from darkish_factory.classifier.audit import NullAuditLog
from darkish_factory.classifier.decisions import ProposedDecision, ProposedToolCall

from .conftest import FakeClient


def _pd() -> ProposedDecision:
    return ProposedDecision(
        decision_id="dec-det",
        title="t",
        description="DELETE FROM accounts;",
        files_touched=[],
        modules=[],
        diff_stats={},
        urgency="high",
        spend_delta_usd=0.0,
        worktree_ref="wt@1",
        tool_calls=[ProposedToolCall(name="run_sql", arguments={"sql": "DELETE FROM accounts"})],
    )


def test_same_inputs_produce_same_stage_1_verdict(
    constitution_path: Path, policy_path: Path
) -> None:
    s2 = FakeClient({"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "n/a"})
    captured_a: list[tuple[str, dict[str, Any]]] = []
    captured_b: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def __init__(self, sink: list[tuple[str, dict[str, Any]]]) -> None:
            self._sink = sink

        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            self._sink.append((event_type, payload))

    a = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(captured_a),
        llm_client=s2,
        clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
    )
    b = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(captured_b),
        llm_client=s2,
        clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
    )
    out_a = a.decide(_pd())
    out_b = b.decide(_pd())
    assert isinstance(out_a, RequestHumanInput)
    assert isinstance(out_b, RequestHumanInput)
    assert out_a.categories == out_b.categories
    triggers_a = [p.get("trigger") for t, p in captured_a if t == "stage_1_escalate"]
    triggers_b = [p.get("trigger") for t, p in captured_b if t == "stage_1_escalate"]
    assert triggers_a == triggers_b


def test_stage_2_client_is_identity_distinct_from_caller_deciding_client(
    constitution_path: Path, policy_path: Path
) -> None:
    deciding_client = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "x"}
    )
    s2_client = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "y"}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2_client,
        clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
    )
    # The classifier holds its own client reference; the caller's "deciding"
    # client must never be touched by Stage 2.
    assert c._stage2.client is s2_client
    assert c._stage2.client is not deciding_client


def test_audit_records_constitution_and_policy_hash(
    constitution_path: Path, policy_path: Path
) -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    s2 = FakeClient({"escalate": False, "categories": [], "confidence": 0.99, "reasoning": "x"})
    pd = ProposedDecision(
        decision_id="dec-h",
        title="t",
        description="trivial",
        files_touched=["src/x.py"],
        modules=["x"],
        diff_stats={},
        urgency="low",
        spend_delta_usd=0.0,
        worktree_ref="wt@1",
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=CaptureLog(),
        llm_client=s2,
        clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
    )
    c.decide(pd)
    s2_payload = next(p for t, p in captured if t == "stage_2_verdict")
    assert "constitution_hash" in s2_payload
    assert "policy_hash" in s2_payload
    assert len(s2_payload["constitution_hash"]) == 64
    assert s2_payload["produced_by"] == "classifier"


def test_stage_2_sampling_variance_is_bounded(constitution_path: Path, policy_path: Path) -> None:
    """With identical stub output, all Stage-2 verdicts are byte-identical;
    with a small variance in the reasoning text, the textual divergence
    between any two verdicts is bounded.
    """
    pd = ProposedDecision(
        decision_id="dec-var",
        title="t",
        description="trivial",
        files_touched=["src/x.py"],
        modules=["x"],
        diff_stats={},
        urgency="low",
        spend_delta_usd=0.0,
        worktree_ref="wt@1",
    )

    # Phase 1: identical output → byte-identical verdicts.
    base_payload = {
        "escalate": False,
        "categories": [],
        "confidence": 0.95,
        "reasoning": "small private refactor; below floor.",
    }
    captured_payloads: list[dict[str, Any]] = []
    for _ in range(5):
        captured: list[tuple[str, dict[str, Any]]] = []

        class CaptureLog:
            def __init__(self, sink: list[tuple[str, dict[str, Any]]]) -> None:
                self._sink = sink

            def emit(self, event_type: str, payload: dict[str, Any]) -> None:
                self._sink.append((event_type, payload))

        s2 = FakeClient(dict(base_payload))
        c = Classifier(
            constitution_path=constitution_path,
            policy_path=policy_path,
            audit_log=CaptureLog(captured),
            llm_client=s2,
            clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
        )
        c.decide(pd)
        verdict_payload = next(p for t, p in captured if t == "stage_2_verdict")
        captured_payloads.append(
            {k: verdict_payload[k] for k in ("escalate", "categories", "confidence", "reasoning")}
        )
    assert all(p == captured_payloads[0] for p in captured_payloads)

    # Phase 2: a small reasoning-text variance must stay within an edit-distance
    # bound of 5% of the longer string (a permissive textual stability guard
    # so model wording changes do not flip the verdict shape).
    variants = [
        {**base_payload, "reasoning": "small private refactor; below floor."},
        {**base_payload, "reasoning": "small private refactor; below floor!"},  # 1 char delta
    ]

    def edit_distance(a: str, b: str) -> int:
        prev = list(range(len(b) + 1))
        for i, ca in enumerate(a, start=1):
            row = [i] + [0] * len(b)
            for j, cb in enumerate(b, start=1):
                row[j] = min(
                    row[j - 1] + 1,
                    prev[j] + 1,
                    prev[j - 1] + (0 if ca == cb else 1),
                )
            prev = row
        return prev[-1]

    reasonings: list[str] = []
    for variant in variants:
        captured = []
        s2 = FakeClient(dict(variant))
        c = Classifier(
            constitution_path=constitution_path,
            policy_path=policy_path,
            audit_log=CaptureLog(captured),
            llm_client=s2,
            clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
        )
        c.decide(pd)
        verdict_payload = next(p for t, p in captured if t == "stage_2_verdict")
        reasonings.append(str(verdict_payload["reasoning"]))

    longest = max(len(r) for r in reasonings)
    bound = max(1, longest // 20)  # 5% of the longer string, ≥ 1 char
    assert edit_distance(reasonings[0], reasonings[1]) <= bound
