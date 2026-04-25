"""Pydantic shape tests for ProposedDecision, RoutingInputs, Verdict."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from darkish_factory.classifier.decisions import (
    ProposedDecision,
    RoutingInputs,
    Stage2Verdict,
)


def test_proposed_decision_roundtrip() -> None:
    pd = ProposedDecision(
        decision_id="dec-1",
        title="Add /admin endpoint",
        description="Adds the /admin endpoint with role checks.",
        files_touched=["src/api.py"],
        modules=["api"],
        diff_stats={"added": 40, "removed": 2},
        urgency="medium",
        spend_delta_usd=0.0,
        worktree_ref="api@deadbeef",
        is_routing_request=False,
        tool_calls=[],
    )
    assert pd.urgency == "medium"


def test_routing_inputs_minimum() -> None:
    ri = RoutingInputs(
        loc_affected=120,
        modules_touched=["api", "db"],
        external_deps_added=0,
        user_visible_surface=False,
        data_model_changes=False,
        security_concerns=False,
    )
    assert ri.loc_affected == 120


def test_stage2_verdict_categories_subset() -> None:
    v = Stage2Verdict(
        escalate=True,
        categories=["taste", "architecture"],
        confidence=0.6,
        reasoning="Naming choice impacts public API.",
        produced_by="classifier",
    )
    assert v.escalate is True


def test_stage2_verdict_rejects_bad_axis() -> None:
    with pytest.raises(ValidationError):
        Stage2Verdict(
            escalate=False,
            categories=["security"],  # type: ignore[list-item]
            confidence=0.9,
            reasoning="ok",
            produced_by="classifier",
        )


def test_stage2_verdict_confidence_bounds() -> None:
    with pytest.raises(ValidationError):
        Stage2Verdict(
            escalate=False,
            categories=[],
            confidence=1.5,
            reasoning="impossible",
            produced_by="classifier",
        )
