"""Stage-1 deterministic gate. Every matcher tested; the gate never sees an LLM."""

from __future__ import annotations

import inspect
from pathlib import Path
from typing import Any

import pytest

from darkish_factory.classifier.audit import AuditContext, NullAuditLog
from darkish_factory.classifier.decisions import ProposedDecision, ProposedToolCall
from darkish_factory.classifier.errors import EscalationRequired
from darkish_factory.classifier.gate import Stage1Gate, wrap_tool
from darkish_factory.classifier.policy import load_policy

POLICY = load_policy(Path(__file__).parent / "fixtures" / "policy.yaml")
CTX = AuditContext(
    decision_id="dec-1",
    constitution_hash="0" * 64,
    policy_hash=POLICY.hash,
)


def _decision(**overrides: Any) -> ProposedDecision:
    base: dict[str, Any] = {
        "decision_id": "dec-1",
        "title": "x",
        "description": "x",
        "files_touched": [],
        "modules": [],
        "diff_stats": {},
        "urgency": "medium",
        "spend_delta_usd": 0.0,
        "worktree_ref": "wt@123",
    }
    base.update(overrides)
    return ProposedDecision(**base)


def _gate() -> Stage1Gate:
    return Stage1Gate(policy=POLICY, audit_log=NullAuditLog(), spend_provider=lambda: 0.0)


def test_evaluate_signature_does_not_accept_llm_client() -> None:
    sig = inspect.signature(Stage1Gate.evaluate)
    assert "llm_client" not in sig.parameters
    assert set(sig.parameters) == {"self", "pd", "audit_ctx"}


def test_schema_migration_on_populated_table_escalates() -> None:
    pd = _decision(
        description="ALTER TABLE users ADD COLUMN locale TEXT;",
        files_touched=["migrations/0007.sql"],
        tool_calls=[
            ProposedToolCall(
                name="run_migration",
                arguments={
                    "sql": "ALTER TABLE users ADD COLUMN locale TEXT;",
                    "table": "users",
                    "row_count": 1200,
                },
            ),
        ],
    )
    with pytest.raises(EscalationRequired) as info:
        _gate().evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "schema_migration_on_populated_table"


def test_data_deletion_escalates() -> None:
    pd = _decision(
        description="DELETE FROM accounts WHERE inactive=true;",
        tool_calls=[
            ProposedToolCall(
                name="run_sql",
                arguments={"sql": "DELETE FROM accounts WHERE inactive=true"},
            )
        ],
    )
    with pytest.raises(EscalationRequired) as info:
        _gate().evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "data_deletion"


def test_public_release_escalates() -> None:
    pd = _decision(
        description="Cut v2.0.0 and publish to PyPI.",
        tool_calls=[
            ProposedToolCall(name="publish_package", arguments={"index": "pypi", "tag": "v2.0.0"})
        ],
    )
    with pytest.raises(EscalationRequired) as info:
        _gate().evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "public_release"


def test_external_communication_escalates() -> None:
    pd = _decision(
        description="Send the launch email to all opted-in customers.",
        tool_calls=[
            ProposedToolCall(
                name="send_email", arguments={"to": "customers@list", "subject": "Launch"}
            )
        ],
    )
    with pytest.raises(EscalationRequired) as info:
        _gate().evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "external_communication"


def test_destructive_fs_op_outside_worktree_escalates(tmp_path: Path) -> None:
    pd = _decision(
        description="rm -rf /var/log/old to clean disk.",
        tool_calls=[ProposedToolCall(name="bash", arguments={"command": "rm -rf /var/log/old"})],
        worktree_ref=str(tmp_path),
    )
    with pytest.raises(EscalationRequired) as info:
        _gate().evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "destructive_fs_op_outside_worktree"


def test_destructive_fs_op_inside_worktree_does_not_escalate(tmp_path: Path) -> None:
    target = tmp_path / "build"
    pd = _decision(
        description=f"rm -rf {target} to refresh outputs.",
        tool_calls=[ProposedToolCall(name="bash", arguments={"command": f"rm -rf {target}"})],
        worktree_ref=str(tmp_path),
    )
    _gate().evaluate(pd, audit_ctx=CTX)


def test_destructive_fs_op_traversal_escapes_worktree(tmp_path: Path) -> None:
    pd = _decision(
        description="rm -rf via traversal",
        tool_calls=[
            ProposedToolCall(
                name="bash",
                arguments={"command": f"rm -rf {tmp_path}/build/../../etc"},
            )
        ],
        worktree_ref=str(tmp_path),
    )
    with pytest.raises(EscalationRequired):
        _gate().evaluate(pd, audit_ctx=CTX)


def test_git_push_protected_branch_escalates() -> None:
    pd = _decision(
        description="git push origin main",
        tool_calls=[ProposedToolCall(name="bash", arguments={"command": "git push origin main"})],
    )
    with pytest.raises(EscalationRequired) as info:
        _gate().evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "git_push_protected_branch"


def test_spend_above_threshold_escalates() -> None:
    pd = _decision(description="One-shot embedding rebuild.", spend_delta_usd=60.0)
    gate = Stage1Gate(policy=POLICY, audit_log=NullAuditLog(), spend_provider=lambda: 0.0)
    with pytest.raises(EscalationRequired) as info:
        gate.evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "spend_above"


def test_spend_above_total_with_running_counter_escalates() -> None:
    pd = _decision(description="incremental.", spend_delta_usd=10.0)
    gate = Stage1Gate(policy=POLICY, audit_log=NullAuditLog(), spend_provider=lambda: 49.0)
    with pytest.raises(EscalationRequired):
        gate.evaluate(pd, audit_ctx=CTX)


def test_clean_decision_passes() -> None:
    pd = _decision(description="Refactor private helper to be pure.", files_touched=["src/util.py"])
    _gate().evaluate(pd, audit_ctx=CTX)


def test_gate_emits_stage_1_pass_with_full_envelope() -> None:
    captured: list[tuple[str, dict[str, object]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, object]) -> None:
            captured.append((event_type, payload))

    gate = Stage1Gate(policy=POLICY, audit_log=CaptureLog(), spend_provider=lambda: 0.0)
    pd = _decision(description="trivial.")
    gate.evaluate(pd, audit_ctx=CTX)
    pass_events = [(t, p) for t, p in captured if t == "stage_1_pass"]
    assert pass_events
    payload = pass_events[0][1]
    for required in ("decision_id", "timestamp", "constitution_hash", "policy_hash"):
        assert required in payload


def test_gate_emits_stage_1_escalate_event() -> None:
    captured: list[tuple[str, dict[str, object]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, object]) -> None:
            captured.append((event_type, payload))

    gate = Stage1Gate(policy=POLICY, audit_log=CaptureLog(), spend_provider=lambda: 0.0)
    pd = _decision(
        description="DELETE FROM x;",
        tool_calls=[ProposedToolCall(name="run_sql", arguments={"sql": "DELETE FROM x"})],
    )
    with pytest.raises(EscalationRequired):
        gate.evaluate(pd, audit_ctx=CTX)
    types = [t for t, _ in captured]
    assert "stage_1_escalate" in types
    assert "stage_1_pass" not in types


def test_custom_policy_matcher_fires(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics: {triggers: []}
  reversibility:
    triggers: [data_deletion]
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
tool_wrapper_matchers:
  pager_duty_trigger: "\\\\bpd_trigger\\\\b"
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    policy = load_policy(target)
    gate = Stage1Gate(policy=policy, audit_log=NullAuditLog(), spend_provider=lambda: 0.0)
    pd = _decision(
        description="run pd_trigger to wake on-call",
        tool_calls=[ProposedToolCall(name="webhook", arguments={"body": "pd_trigger"})],
    )
    with pytest.raises(EscalationRequired) as info:
        gate.evaluate(pd, audit_ctx=CTX)
    assert info.value.trigger == "pager_duty_trigger"


def test_wrap_tool_raises_before_inner_runs(tmp_path: Path) -> None:
    inner_called = False

    def destructive_rm(target: str) -> None:
        nonlocal inner_called
        inner_called = True

    gate = _gate()
    wrapped = wrap_tool(
        destructive_rm,
        gate=gate,
        audit_ctx=CTX,
        tool_name="bash",
        argument_builder=lambda *args, **kwargs: {"command": f"rm -rf {args[0]}"},
        worktree_ref=str(tmp_path),
        decision_id="dec-wrap",
    )
    with pytest.raises(EscalationRequired):
        wrapped("/var/log/old")
    assert inner_called is False
