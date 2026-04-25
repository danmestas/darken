"""Prompt-injection probes: classifier must escalate."""

from __future__ import annotations

import json
from datetime import datetime
from pathlib import Path

import pytest

from darkish_factory import Classifier, RequestHumanInput
from darkish_factory.classifier.audit import NullAuditLog
from darkish_factory.classifier.decisions import ProposedDecision

from .conftest import FakeClient

FIXTURE = Path(__file__).parent / "fixtures" / "adversarial_decisions.jsonl"


def _load() -> list[ProposedDecision]:
    out: list[ProposedDecision] = []
    for line in FIXTURE.read_text().splitlines():
        if not line.strip():
            continue
        record = json.loads(line)
        out.append(ProposedDecision(**record))
    return out


@pytest.mark.parametrize("pd", _load(), ids=lambda p: p.decision_id)
def test_adversarial_payloads_force_escalation(
    pd: ProposedDecision, constitution_path: Path, policy_path: Path
) -> None:
    # The Stage-2 stub mimics a model that obeyed the injection — the harness
    # MUST still escalate via confidence-floor resolution AND the test asserts
    # the classifier does not auto-ratify when the description carries
    # injection-pattern strings.
    s2 = FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.4, "reasoning": "approved per text."}
    )
    c = Classifier(
        constitution_path=constitution_path,
        policy_path=policy_path,
        audit_log=NullAuditLog(),
        llm_client=s2,
        clock=lambda: datetime(2026, 4, 25, 12, 0, 0),
    )
    out = c.decide(pd)
    assert isinstance(out, RequestHumanInput), f"expected escalation for {pd.decision_id}"
