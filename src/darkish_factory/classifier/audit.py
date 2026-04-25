"""Audit-log writer surface for Slice 1.

Slice 2 will own the canonical implementation. Here we ship a Protocol and
two simple concrete impls — `JSONLAuditLog` for dev/integration tests and
`NullAuditLog` for unit tests that don't care.

`AuditContext` is the small bundle every emitter needs in order to satisfy
spec §6.4 — every event payload must carry `decision_id`, `timestamp`,
`constitution_hash`, `policy_hash`. Components accept it per-call rather than
holding it as state, so a single `Classifier` can fan out to many decisions
with their own ids without leaking.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Protocol, runtime_checkable


@dataclass(frozen=True)
class AuditContext:
    """Per-decision audit envelope passed to every component-level emitter."""

    decision_id: str
    constitution_hash: str
    policy_hash: str

    def envelope(self) -> dict[str, Any]:
        """Render the four mandatory §6.4 fields as a fresh dict.

        `timestamp` is computed at call time so each event records when the
        emitter actually fired, not when the context was built.
        """
        return {
            "decision_id": self.decision_id,
            "timestamp": datetime.now(UTC).isoformat(),
            "constitution_hash": self.constitution_hash,
            "policy_hash": self.policy_hash,
        }


@runtime_checkable
class AuditLog(Protocol):
    """Single-method writer surface."""

    def emit(self, event_type: str, payload: dict[str, Any]) -> None:
        ...


class NullAuditLog:
    """Drops every event. Used by unit tests that don't assert on the log."""

    def emit(self, event_type: str, payload: dict[str, Any]) -> None:
        return None


class JSONLAuditLog:
    """Newline-delimited JSON appender."""

    def __init__(self, path: Path) -> None:
        self._path = path
        self._path.parent.mkdir(parents=True, exist_ok=True)

    def emit(self, event_type: str, payload: dict[str, Any]) -> None:
        record = {"event_type": event_type, "payload": payload}
        line = json.dumps(record, sort_keys=True)
        with self._path.open("a", encoding="utf-8") as fh:
            fh.write(line + "\n")


__all__ = ["AuditContext", "AuditLog", "JSONLAuditLog", "NullAuditLog"]
