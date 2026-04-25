"""Golden-set recall/precision per axis."""

from __future__ import annotations

import json
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import cast

from darkish_factory import Classifier, RequestHumanInput
from darkish_factory.classifier.audit import NullAuditLog
from darkish_factory.classifier.decisions import ProposedDecision

from .conftest import FakeClient

FIXTURE = Path(__file__).parent / "fixtures" / "golden_set.jsonl"


def _load() -> list[tuple[ProposedDecision, dict[str, object]]]:
    out: list[tuple[ProposedDecision, dict[str, object]]] = []
    for line in FIXTURE.read_text().splitlines():
        if not line.strip():
            continue
        record = json.loads(line)
        expected = record.pop("expected")
        out.append((ProposedDecision(**record), expected))
    return out


def _stub_for(expected_escalate: bool, expected_categories: list[str]) -> FakeClient:
    if expected_escalate:
        return FakeClient(
            {
                "escalate": True,
                "categories": expected_categories,
                "confidence": 0.95,
                "reasoning": "matches axis trigger.",
            }
        )
    return FakeClient(
        {
            "escalate": False,
            "categories": [],
            "confidence": 0.95,
            "reasoning": "private; below floor.",
        }
    )


def test_golden_set_recall_per_axis(constitution_path: Path, policy_path: Path) -> None:
    tp: dict[str, int] = defaultdict(int)
    fn: dict[str, int] = defaultdict(int)
    fp: dict[str, int] = defaultdict(int)
    items = _load()
    for pd, expected in items:
        cats = cast(list[str], expected.get("categories") or [])
        s2 = _stub_for(bool(expected["escalate"]), cats)
        c = Classifier(
            constitution_path=constitution_path,
            policy_path=policy_path,
            audit_log=NullAuditLog(),
            llm_client=s2,
            clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
        )
        out = c.decide(pd)
        actual_escalate = isinstance(out, RequestHumanInput)
        for axis in ("taste", "architecture", "ethics", "reversibility"):
            in_expected = axis in (expected.get("categories") or [])  # type: ignore[operator]
            in_actual = actual_escalate and (
                axis in (out.categories if isinstance(out, RequestHumanInput) else [])
            )
            if in_expected and in_actual:
                tp[axis] += 1
            elif in_expected and not in_actual:
                fn[axis] += 1
            elif not in_expected and in_actual:
                fp[axis] += 1

    # Calibration is recall-first; for the curated golden set we require
    # zero false negatives.
    assert sum(fn.values()) == 0, f"recall miss: {dict(fn)}"
