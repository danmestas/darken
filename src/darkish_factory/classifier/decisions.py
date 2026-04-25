"""Internal decision/verdict models."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

from .answers import Axis, Urgency


class ProposedToolCall(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    name: str
    arguments: dict[str, object] = Field(default_factory=dict)


class ProposedDecision(BaseModel):
    """Caller-supplied decision under evaluation."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    decision_id: str
    title: str
    description: str
    files_touched: list[str] = Field(default_factory=list)
    modules: list[str] = Field(default_factory=list)
    diff_stats: dict[str, int] = Field(default_factory=dict)
    urgency: Urgency = "medium"
    spend_delta_usd: float = 0.0
    worktree_ref: str
    is_routing_request: bool = False
    tool_calls: list[ProposedToolCall] = Field(default_factory=list)
    routing_inputs: RoutingInputs | None = None


class RoutingInputs(BaseModel):
    """Rubric inputs for the routing classifier."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    loc_affected: int = 0
    modules_touched: list[str] = Field(default_factory=list)
    external_deps_added: int = 0
    user_visible_surface: bool = False
    data_model_changes: bool = False
    security_concerns: bool = False


class Stage2Verdict(BaseModel):
    """Resolved Stage-2 verdict (confidence_floor already applied)."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    escalate: bool
    categories: list[Axis] = Field(default_factory=list)
    confidence: float = Field(ge=0.0, le=1.0)
    reasoning: str
    produced_by: Literal["gate", "classifier"]


ProposedDecision.model_rebuild()
