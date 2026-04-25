"""Batcher tests: size flush, latency flush, urgency bypass."""

from __future__ import annotations

from datetime import datetime, timedelta
from typing import Any

from darkish_factory.classifier.answers import RequestHumanInput
from darkish_factory.classifier.audit import AuditContext
from darkish_factory.classifier.batcher import Batcher

CTX = AuditContext(decision_id="dec-batch", constitution_hash="0" * 64, policy_hash="1" * 64)


def _rhi(token: str, urgency: str = "low") -> RequestHumanInput:
    return RequestHumanInput(
        question="Q",
        context="ctx",
        urgency=urgency,  # type: ignore[arg-type]
        format="yes_no",
        choices=["yes", "no"],
        recommendation="yes",
        reasoning="r",
        categories=["taste"],
        worktree_ref="wt@1",
        resume_token=token,
    )


class _Clock:
    def __init__(self, now: datetime) -> None:
        self._now = now

    def __call__(self) -> datetime:
        return self._now

    def advance(self, minutes: float) -> None:
        self._now += timedelta(minutes=minutes)


def test_size_flush_releases_when_threshold_hit() -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    clock = _Clock(datetime(2026, 4, 25, 12, 0, 0))
    b = Batcher(batch_size=3, max_latency_min=30.0, clock=clock, audit_log=CaptureLog())
    assert b.enqueue(_rhi("t1"), audit_ctx=CTX) is None
    assert b.enqueue(_rhi("t2"), audit_ctx=CTX) is None
    flushed = b.enqueue(_rhi("t3"), audit_ctx=CTX)
    assert flushed is not None
    assert [r.resume_token for r in flushed] == ["t1", "t2", "t3"]
    flush_events = [(t, p) for t, p in captured if t == "batch_flush"]
    assert flush_events
    payload = flush_events[0][1]
    for required in ("decision_id", "timestamp", "constitution_hash", "policy_hash"):
        assert required in payload


def test_latency_flush_releases_after_max_age() -> None:
    clock = _Clock(datetime(2026, 4, 25, 12, 0, 0))

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            return None

    b = Batcher(batch_size=10, max_latency_min=5.0, clock=clock, audit_log=CaptureLog())
    assert b.enqueue(_rhi("t1"), audit_ctx=CTX) is None
    clock.advance(6.0)
    flushed = b.flush_due(audit_ctx=CTX)
    assert flushed is not None
    assert [r.resume_token for r in flushed] == ["t1"]


def test_high_urgency_bypasses_batching() -> None:
    clock = _Clock(datetime(2026, 4, 25, 12, 0, 0))

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            return None

    b = Batcher(batch_size=10, max_latency_min=30.0, clock=clock, audit_log=CaptureLog())
    flushed = b.enqueue(_rhi("urgent", urgency="high"), audit_ctx=CTX)
    assert flushed is not None
    assert [r.resume_token for r in flushed] == ["urgent"]


def test_flush_due_returns_none_when_nothing_aged() -> None:
    clock = _Clock(datetime(2026, 4, 25, 12, 0, 0))

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            return None

    b = Batcher(batch_size=10, max_latency_min=30.0, clock=clock, audit_log=CaptureLog())
    b.enqueue(_rhi("t1"), audit_ctx=CTX)
    clock.advance(1.0)
    assert b.flush_due(audit_ctx=CTX) is None


def test_flush_clears_queue() -> None:
    clock = _Clock(datetime(2026, 4, 25, 12, 0, 0))

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            return None

    b = Batcher(batch_size=2, max_latency_min=30.0, clock=clock, audit_log=CaptureLog())
    b.enqueue(_rhi("t1"), audit_ctx=CTX)
    b.enqueue(_rhi("t2"), audit_ctx=CTX)  # triggers flush
    assert b.enqueue(_rhi("t3"), audit_ctx=CTX) is None
