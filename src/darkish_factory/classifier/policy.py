"""Policy loader, validator, and hasher.

Accepts the README's flat-or-nested `regulated_domain` form. Refuses to load
if reversibility triggers are weakened below the safe default set
(library-pinned floor) — surfaced as `PolicyDriftError`.
"""

from __future__ import annotations

import hashlib
import re
from pathlib import Path
from typing import Any

import yaml
from pydantic import BaseModel, ConfigDict, Field, field_validator

from .errors import PolicyDriftError

_SAFE_REVERSIBILITY_FLOOR = {"data_deletion", "destructive_fs_op_outside_worktree"}
_SHA256_HEX_LEN = 64


def _empty_regulated_domain() -> dict[str, list[str]]:
    return {"kinds": []}


class TastePolicy(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    triggers: list[str] = Field(default_factory=list)


class ArchitecturePolicy(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    triggers: list[str] = Field(default_factory=list)


class EthicsPolicy(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    triggers: list[str] = Field(default_factory=list)
    regulated_domain: dict[str, list[str]] = Field(default_factory=_empty_regulated_domain)


class ReversibilityPolicy(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    triggers: list[str] = Field(default_factory=list)
    spend_above_usd: float = 50.0


class EscalateOn(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    taste: TastePolicy
    architecture: ArchitecturePolicy
    ethics: EthicsPolicy
    reversibility: ReversibilityPolicy


class Thresholds(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    confidence_floor: float = Field(ge=0.0, le=1.0)
    batch_size: int = Field(ge=1)
    max_queue_latency_min: float = Field(gt=0.0)
    protected_branches: list[str] = Field(default_factory=lambda: ["main", "master"])


class Policy(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    escalate_on: EscalateOn
    thresholds: Thresholds
    spot_check_rate: float = Field(default=0.05, ge=0.0, le=1.0)
    routing_rubric: dict[str, str] = Field(default_factory=dict)
    tool_wrapper_matchers: dict[str, str] = Field(default_factory=dict)
    hash: str

    @field_validator("hash")
    @classmethod
    def _hex_hash(cls, value: str) -> str:
        if len(value) != _SHA256_HEX_LEN or not all(ch in "0123456789abcdef" for ch in value):
            raise ValueError("hash must be 64-char lowercase sha256 hex")
        return value


def _normalize_ethics_triggers(triggers: list[Any]) -> tuple[list[str], dict[str, list[str]]]:
    flat: list[str] = []
    regulated: dict[str, list[str]] = {"kinds": []}
    for entry in triggers:
        if isinstance(entry, str):
            flat.append(entry)
        elif isinstance(entry, dict) and "regulated_domain" in entry:
            regulated["kinds"] = list(entry["regulated_domain"])
        else:
            raise ValueError(f"unrecognized ethics trigger entry: {entry!r}")
    return flat, regulated


def _parse_spend_above(token: str | float | int) -> float:
    if isinstance(token, (int, float)):
        return float(token)
    match = re.fullmatch(r"(\d+(?:\.\d+)?)_?usd", str(token).strip(), re.IGNORECASE)
    if not match:
        raise ValueError(f"unparseable spend_above value: {token!r}")
    return float(match.group(1))


def _normalize_reversibility_triggers(
    triggers: list[Any],
) -> tuple[list[str], float]:
    flat: list[str] = []
    spend = 50.0
    for entry in triggers:
        if isinstance(entry, str):
            flat.append(entry)
        elif isinstance(entry, dict) and "spend_above" in entry:
            spend = _parse_spend_above(entry["spend_above"])
            flat.append("spend_above")
        else:
            raise ValueError(f"unrecognized reversibility trigger entry: {entry!r}")
    return flat, spend


def load_policy(path: Path) -> Policy:
    """Load + validate + hash."""
    raw_bytes = path.read_bytes()
    digest = hashlib.sha256(raw_bytes).hexdigest()
    data = yaml.safe_load(raw_bytes.decode("utf-8")) or {}

    if "escalate_on" not in data or "thresholds" not in data:
        raise ValueError("policy file must declare both `escalate_on` and `thresholds`")

    eo = data["escalate_on"]
    for required in ("taste", "architecture", "ethics", "reversibility"):
        if required not in eo:
            raise ValueError(f"escalate_on must declare `{required}`")

    ethics_block = eo["ethics"]
    raw_eth_triggers = list(ethics_block.get("triggers", []))
    flat_ethics, regulated = _normalize_ethics_triggers(raw_eth_triggers)
    if "regulated_domain" in ethics_block and regulated["kinds"] == []:
        nested = ethics_block["regulated_domain"]
        if isinstance(nested, dict) and "kinds" in nested:
            regulated = {"kinds": list(nested["kinds"])}
        elif isinstance(nested, list):
            regulated = {"kinds": list(nested)}

    rev_block = eo["reversibility"]
    flat_rev, spend_usd = _normalize_reversibility_triggers(list(rev_block.get("triggers", [])))

    if not (set(flat_rev) & _SAFE_REVERSIBILITY_FLOOR):
        raise PolicyDriftError(
            f"reversibility triggers {flat_rev!r} weaken safe floor {_SAFE_REVERSIBILITY_FLOOR!r}"
        )

    thresholds = data["thresholds"]
    if "protected_branches" not in thresholds:
        thresholds["protected_branches"] = ["main", "master"]

    policy = Policy(
        escalate_on=EscalateOn(
            taste=TastePolicy(triggers=list(eo["taste"].get("triggers", []))),
            architecture=ArchitecturePolicy(triggers=list(eo["architecture"].get("triggers", []))),
            ethics=EthicsPolicy(triggers=flat_ethics, regulated_domain=regulated),
            reversibility=ReversibilityPolicy(triggers=flat_rev, spend_above_usd=spend_usd),
        ),
        thresholds=Thresholds(**thresholds),
        spot_check_rate=float(data.get("spot_check_rate", 0.05)),
        routing_rubric={str(k): str(v) for k, v in dict(data.get("routing_rubric", {})).items()},
        tool_wrapper_matchers={
            str(k): str(v) for k, v in dict(data.get("tool_wrapper_matchers", {})).items()
        },
        hash=digest,
    )
    return policy


__all__ = ["Policy", "PolicyDriftError", "load_policy"]
