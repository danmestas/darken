"""Pydantic shape tests for the public Answer/Request types."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from darkish_factory.classifier.answers import HumanAnswer, RequestHumanInput


def test_request_human_input_round_trips_minimum_fields() -> None:
    rhi = RequestHumanInput(
        question="Ship public copy?",
        context="The new homepage hero text mentions launch.",
        urgency="high",
        format="yes_no",
        choices=["yes", "no"],
        recommendation="yes",
        reasoning="Within taste guardrails.",
        categories=["taste"],
        worktree_ref="hero-copy@abcd123",
        resume_token="tok-1",
    )
    assert rhi.format == "yes_no"
    assert rhi.categories == ["taste"]


def test_request_human_input_rejects_unknown_axis() -> None:
    with pytest.raises(ValidationError):
        RequestHumanInput(
            question="?",
            context="?",
            urgency="low",
            format="yes_no",
            choices=[],
            recommendation="",
            reasoning="",
            categories=["aesthetics"],  # type: ignore[list-item]
            worktree_ref="ref",
            resume_token="t",
        )


def test_request_human_input_rejects_bad_format() -> None:
    with pytest.raises(ValidationError):
        RequestHumanInput(
            question="?",
            context="?",
            urgency="low",
            format="dropdown",  # type: ignore[arg-type]
            choices=[],
            recommendation="",
            reasoning="",
            categories=[],
            worktree_ref="ref",
            resume_token="t",
        )


def test_human_answer_ratify_minimum() -> None:
    ans = HumanAnswer(kind="ratify", raw_text="ok", interpretation="ratify")
    assert ans.kind == "ratify"
    assert ans.choice is None


def test_human_answer_choose_requires_choice() -> None:
    with pytest.raises(ValidationError):
        HumanAnswer(kind="choose", raw_text="b", interpretation="b")


def test_human_answer_rework_requires_direction() -> None:
    with pytest.raises(ValidationError):
        HumanAnswer(kind="rework", raw_text="redo", interpretation="redo")


def test_human_answer_abort_against_committed_flag() -> None:
    ans = HumanAnswer(
        kind="abort",
        raw_text="kill it",
        interpretation="abort",
        against_committed=True,
    )
    assert ans.against_committed is True


def test_human_answer_abort_default_against_committed_is_false() -> None:
    ans = HumanAnswer(kind="abort", raw_text="kill", interpretation="abort")
    assert ans.against_committed is False


def test_human_answer_choose_with_choice() -> None:
    ans = HumanAnswer(
        kind="choose",
        choice="option_b",
        raw_text="b",
        interpretation="option_b",
    )
    assert ans.choice == "option_b"


def test_human_answer_rejects_extra_field() -> None:
    with pytest.raises(ValidationError):
        HumanAnswer(  # type: ignore[call-arg]
            kind="ratify",
            raw_text="ok",
            interpretation="ratify",
            extra_unknown_key="boom",
        )
