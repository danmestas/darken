"""5% spot-check sampler with per-category drift detection."""

from __future__ import annotations

import random
from collections import defaultdict
from dataclasses import dataclass, field

from .audit import AuditContext, AuditLog
from .errors import PolicyDriftError


@dataclass
class SpotChecker:
    rate: float
    audit_log: AuditLog
    rng: random.Random
    drift_threshold: int = 5
    _counts: dict[str, int] = field(default_factory=lambda: defaultdict(int))

    def maybe_sample(self, category: str, audit_ctx: AuditContext) -> bool:
        if self.rate <= 0.0:
            return False
        if self.rng.random() >= self.rate:
            return False
        payload = audit_ctx.envelope()
        payload.update({"category": category, "rate": self.rate})
        self.audit_log.emit("spot_check_sample", payload)
        return True

    def record_disagreement(self, category: str, audit_ctx: AuditContext) -> None:
        self._counts[category] += 1
        if self._counts[category] >= self.drift_threshold:
            self._counts[category] = 0
            payload = audit_ctx.envelope()
            payload.update({"category": category, "threshold": self.drift_threshold})
            self.audit_log.emit("policy_drift_flagged", payload)
            raise PolicyDriftError(
                f"systematic miss on {category}: thresholds reset, policy flagged drifted"
            )


__all__ = ["SpotChecker"]
