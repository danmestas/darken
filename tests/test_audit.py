"""AuditLog Protocol contract + concrete impls."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from darkish_factory.classifier.audit import (
    AuditLog,
    JSONLAuditLog,
    NullAuditLog,
)


def test_protocol_runtime_checkable() -> None:
    class Stub:
        def emit(self, event_type: str, payload: dict[str, object]) -> None:
            return None

    assert isinstance(Stub(), AuditLog)


def test_null_audit_log_is_a_noop() -> None:
    log = NullAuditLog()
    log.emit("stage_1_pass", {"decision_id": "d1"})


def test_jsonl_audit_log_appends_one_line_per_event(tmp_path: Path) -> None:
    target = tmp_path / "audit.jsonl"
    log = JSONLAuditLog(target)
    log.emit("stage_1_pass", {"decision_id": "d1", "constitution_hash": "abc"})
    log.emit("stage_2_verdict", {"decision_id": "d1", "escalate": False})

    raw = target.read_text(encoding="utf-8").splitlines()
    assert len(raw) == 2
    first = json.loads(raw[0])
    second = json.loads(raw[1])
    assert first["event_type"] == "stage_1_pass"
    assert first["payload"]["decision_id"] == "d1"
    assert second["event_type"] == "stage_2_verdict"


def test_jsonl_creates_parent_directory(tmp_path: Path) -> None:
    target = tmp_path / "nested" / "deep" / "audit.jsonl"
    log = JSONLAuditLog(target)
    log.emit("batch_flush", {"batch_size": 3})
    assert target.exists()


def test_jsonl_rejects_non_serializable_payload(tmp_path: Path) -> None:
    log = JSONLAuditLog(tmp_path / "audit.jsonl")
    with pytest.raises(TypeError):
        log.emit("stage_2_verdict", {"client": object()})
