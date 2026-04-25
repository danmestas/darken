"""Routing classifier: light | heavy | ambiguous → heavy.

The default rubric questions are baked in. Callers can extend or override
them by passing `rubric_overrides` (a `dict[str, str]` of name → prompt
fragment), which the `Classifier` sources from `policy.routing_rubric`.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any, Literal, Protocol

from .audit import AuditContext, AuditLog
from .decisions import RoutingInputs

ResolvedLabel = Literal["light", "heavy"]
RawLabel = Literal["light", "heavy", "ambiguous"]


_BASE_PROMPT = """You are a routing classifier.
Given the rubric inputs in the user message, label the work as
"light", "heavy", or "ambiguous". Reply with strict JSON only:
{"label": "light"|"heavy"|"ambiguous", "reasoning": "..."}.

Definitions:
- light: small change in one module, no user-visible surface, no data-model
  change, no new external deps, no security implications.
- heavy: anything that crosses modules, changes data models, adds external
  deps, touches user-visible surface, or has security implications.
- ambiguous: pick this only if you genuinely can't decide. Ambiguous will be
  resolved to heavy by the caller.
"""


class _MessagesAPI(Protocol):
    def create(self, **kwargs: Any) -> Any:
        ...


class _AnthropicLike(Protocol):
    @property
    def messages(self) -> _MessagesAPI:
        ...


@dataclass
class RoutingClassifier:
    client: _AnthropicLike
    audit_log: AuditLog
    rubric_overrides: dict[str, str] = field(default_factory=dict)
    model: str = "claude-opus-4-7"
    max_tokens: int = 256

    def _system_prompt(self) -> str:
        if not self.rubric_overrides:
            return _BASE_PROMPT
        addendum_lines = ["Additional rubric questions to weigh:"]
        for name, question in self.rubric_overrides.items():
            addendum_lines.append(f"- {name}: {question}")
        return _BASE_PROMPT + "\n" + "\n".join(addendum_lines) + "\n"

    def classify(self, inputs: RoutingInputs, audit_ctx: AuditContext) -> ResolvedLabel:
        prompt_payload = {
            "loc_affected": inputs.loc_affected,
            "modules_touched": inputs.modules_touched,
            "external_deps_added": inputs.external_deps_added,
            "user_visible_surface": inputs.user_visible_surface,
            "data_model_changes": inputs.data_model_changes,
            "security_concerns": inputs.security_concerns,
        }
        msg = self.client.messages.create(
            model=self.model,
            max_tokens=self.max_tokens,
            system=self._system_prompt(),
            messages=[{"role": "user", "content": json.dumps(prompt_payload)}],
        )
        text_block = next(b for b in msg.content if getattr(b, "type", None) == "text")
        parsed = json.loads(text_block.text)
        raw = parsed.get("label")
        if raw not in {"light", "heavy", "ambiguous"}:
            raise ValueError(f"router returned invalid label: {raw!r}")
        resolved: ResolvedLabel = "heavy" if raw == "ambiguous" else raw

        payload = audit_ctx.envelope()
        payload.update(
            {
                "label": resolved,
                "raw_label": raw,
                "reasoning": parsed.get("reasoning", ""),
            }
        )
        self.audit_log.emit("routing_verdict", payload)
        return resolved


__all__ = ["RawLabel", "ResolvedLabel", "RoutingClassifier"]
