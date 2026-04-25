"""Spot-check sampler tests."""

from __future__ import annotations

import random
from typing import Any

import pytest

from darkish_factory.classifier.audit import AuditContext
from darkish_factory.classifier.errors import PolicyDriftError
from darkish_factory.classifier.spot_check import SpotChecker

CTX = AuditContext(decision_id="d?", constitution_hash="0" * 64, policy_hash="1" * 64)


def _ctx(decision_id: str) -> AuditContext:
    return AuditContext(decision_id=decision_id, constitution_hash="0" * 64, policy_hash="1" * 64)


class _CaptureLog:
    def __init__(self) -> None:
        self.events: list[tuple[str, dict[str, Any]]] = []

    def emit(self, event_type: str, payload: dict[str, Any]) -> None:
        self.events.append((event_type, payload))


def test_sample_rate_respected_within_tolerance() -> None:
    log = _CaptureLog()
    rng = random.Random(0xC0FFEE)
    checker = SpotChecker(rate=0.05, audit_log=log, rng=rng, drift_threshold=999)
    samples = sum(
        1 for i in range(10000) if checker.maybe_sample(category="taste", audit_ctx=_ctx(f"d{i}"))
    )
    # Expect ~500; tolerate +/- 100
    assert 400 <= samples <= 600


def test_emits_spot_check_sample_event() -> None:
    log = _CaptureLog()
    rng = random.Random(0)
    checker = SpotChecker(rate=1.0, audit_log=log, rng=rng, drift_threshold=999)
    assert checker.maybe_sample(category="taste", audit_ctx=_ctx("d1")) is True
    types = [t for t, _ in log.events]
    assert "spot_check_sample" in types


def test_record_disagreement_below_threshold_does_not_drift() -> None:
    log = _CaptureLog()
    checker = SpotChecker(rate=0.0, audit_log=log, rng=random.Random(), drift_threshold=3)
    checker.record_disagreement("ethics", audit_ctx=CTX)
    checker.record_disagreement("ethics", audit_ctx=CTX)  # 2 < 3


def test_record_disagreement_at_threshold_resets_and_raises_and_emits() -> None:
    log = _CaptureLog()
    checker = SpotChecker(rate=0.0, audit_log=log, rng=random.Random(), drift_threshold=3)
    checker.record_disagreement("architecture", audit_ctx=CTX)
    checker.record_disagreement("architecture", audit_ctx=CTX)
    with pytest.raises(PolicyDriftError):
        checker.record_disagreement("architecture", audit_ctx=CTX)
    # Counter is reset after raising so the next call starts fresh
    assert checker._counts["architecture"] == 0
    # And `policy_drift_flagged` is emitted at the moment of the breach.
    types = [t for t, _ in log.events]
    assert "policy_drift_flagged" in types
    payload = next(p for t, p in log.events if t == "policy_drift_flagged")
    assert payload["category"] == "architecture"


def test_disagreement_counts_are_per_category() -> None:
    log = _CaptureLog()
    checker = SpotChecker(rate=0.0, audit_log=log, rng=random.Random(), drift_threshold=2)
    # Two taste disagreements: the second one breaches and raises.
    checker.record_disagreement("taste", audit_ctx=CTX)
    with pytest.raises(PolicyDriftError):
        checker.record_disagreement("taste", audit_ctx=CTX)
    # One ethics disagreement: must NOT raise — taste counts do not bleed.
    checker.record_disagreement("ethics", audit_ctx=CTX)
    assert checker._counts["ethics"] == 1
