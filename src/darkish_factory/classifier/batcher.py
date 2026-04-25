"""In-process escalation batcher."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import datetime, timedelta

from .answers import RequestHumanInput
from .audit import AuditContext, AuditLog


@dataclass
class _Entry:
    request: RequestHumanInput
    enqueued_at: datetime


@dataclass
class Batcher:
    """Internal batcher; the orchestrator drains it via `decide`'s return."""

    batch_size: int
    max_latency_min: float
    clock: Callable[[], datetime]
    audit_log: AuditLog
    _queue: list[_Entry] = field(default_factory=list)

    def enqueue(
        self, request: RequestHumanInput, audit_ctx: AuditContext
    ) -> list[RequestHumanInput] | None:
        if request.urgency == "high":
            payload = audit_ctx.envelope()
            payload.update({"reason": "urgency_high", "size": 1, "tokens": [request.resume_token]})
            self.audit_log.emit("batch_flush", payload)
            return [request]

        self._queue.append(_Entry(request=request, enqueued_at=self.clock()))
        if len(self._queue) >= self.batch_size:
            return self._drain(reason="size", audit_ctx=audit_ctx)
        return None

    def flush_due(self, audit_ctx: AuditContext) -> list[RequestHumanInput] | None:
        if not self._queue:
            return None
        oldest = self._queue[0].enqueued_at
        if (self.clock() - oldest) >= timedelta(minutes=self.max_latency_min):
            return self._drain(reason="latency", audit_ctx=audit_ctx)
        return None

    def _drain(self, reason: str, audit_ctx: AuditContext) -> list[RequestHumanInput]:
        out = [e.request for e in self._queue]
        self._queue.clear()
        payload = audit_ctx.envelope()
        payload.update(
            {"reason": reason, "size": len(out), "tokens": [r.resume_token for r in out]}
        )
        self.audit_log.emit("batch_flush", payload)
        return out


__all__ = ["Batcher"]
