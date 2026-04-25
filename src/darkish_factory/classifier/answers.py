"""Public data models for operator interaction."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator

Axis = Literal["taste", "architecture", "ethics", "reversibility"]
Urgency = Literal["low", "medium", "high"]
Format = Literal["yes_no", "multiple_choice", "free_text"]
AnswerKind = Literal["ratify", "choose", "rework", "abort"]


class RequestHumanInput(BaseModel):
    """Operator-facing escalation request payload."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    question: str
    context: str
    urgency: Urgency
    format: Format
    choices: list[str] = Field(default_factory=list)
    recommendation: str
    reasoning: str
    categories: list[Axis] = Field(default_factory=list)
    worktree_ref: str
    resume_token: str


class HumanAnswer(BaseModel):
    """Operator-supplied answer to a `RequestHumanInput`."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    kind: AnswerKind
    choice: str | None = None
    direction: str | None = None
    raw_text: str
    interpretation: str
    against_committed: bool = False

    @model_validator(mode="after")
    def _validate_kind_specifics(self) -> HumanAnswer:
        if self.kind == "choose" and not self.choice:
            raise ValueError("HumanAnswer(kind='choose') requires `choice`")
        if self.kind == "rework" and not self.direction:
            raise ValueError("HumanAnswer(kind='rework') requires `direction`")
        return self
