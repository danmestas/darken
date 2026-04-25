"""Stage-1 deterministic gate.

One matcher per reversibility trigger from §6.3 of the spec. Each takes the
proposed decision, the policy, and the live spend counter; returns a `Match`
or `None`. The gate composes them and emits `stage_1_pass` or
`stage_1_escalate` audit events. Stage 1 never sees an LLM client — the
`evaluate` signature deliberately omits it.
"""

from __future__ import annotations

import os
import re
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from .audit import AuditContext, AuditLog
from .decisions import ProposedDecision, ProposedToolCall
from .errors import EscalationRequired
from .policy import Policy


@dataclass(frozen=True)
class Match:
    trigger: str
    detail: str


_DELETE_SQL = re.compile(r"\bDELETE\s+FROM\b|\bTRUNCATE\b|\bDROP\s+TABLE\b", re.IGNORECASE)
_ALTER_SQL = re.compile(r"\bALTER\s+TABLE\b|\bADD\s+COLUMN\b|\bDROP\s+COLUMN\b", re.IGNORECASE)
_PUBLISH_TOOLS = {"publish_package", "release_artifact", "deploy_production"}
_EMAIL_TOOLS = {"send_email", "post_tweet", "post_slack_external", "send_sms"}
_DESTRUCTIVE_FS = re.compile(r"\b(rm\s+-r[fF]?|shred|dd\s+if=)")
_GIT_PUSH = re.compile(r"\bgit\s+push\b\s+\S+\s+(\S+)")


def _aggregate_text(pd: ProposedDecision) -> str:
    parts = [pd.title, pd.description]
    for call in pd.tool_calls:
        parts.append(call.name)
        for value in call.arguments.values():
            parts.append(str(value))
    return "\n".join(parts)


def _path_inside(target: str, worktree: str) -> bool:
    """Return True iff `target` is the worktree itself or a path inside it.

    Robust against symlinks, trailing slashes, and `../` traversals: we
    canonicalize both paths via `os.path.realpath` and compare via
    `os.path.commonpath`. If commonpath raises (different drives on Windows,
    empty string), we fall back to "outside" — fail closed.
    """
    if not target or not worktree:
        return False
    try:
        canonical_target = os.path.realpath(target)
        canonical_worktree = os.path.realpath(worktree)
        return os.path.commonpath([canonical_target, canonical_worktree]) == canonical_worktree
    except ValueError:
        return False


def match_schema_migration_on_populated_table(
    pd: ProposedDecision, policy: Policy, spend_total: float
) -> Match | None:
    if "schema_migration_on_populated_table" not in policy.escalate_on.reversibility.triggers:
        return None
    for call in pd.tool_calls:
        sql = str(call.arguments.get("sql", ""))
        row_count = call.arguments.get("row_count")
        if _ALTER_SQL.search(sql) and isinstance(row_count, int) and row_count > 0:
            return Match("schema_migration_on_populated_table", sql)
    return None


def match_data_deletion(pd: ProposedDecision, policy: Policy, spend_total: float) -> Match | None:
    if "data_deletion" not in policy.escalate_on.reversibility.triggers:
        return None
    text = _aggregate_text(pd)
    if _DELETE_SQL.search(text):
        return Match("data_deletion", "destructive SQL detected")
    return None


def match_public_release(pd: ProposedDecision, policy: Policy, spend_total: float) -> Match | None:
    if "public_release" not in policy.escalate_on.reversibility.triggers:
        return None
    for call in pd.tool_calls:
        if call.name in _PUBLISH_TOOLS:
            return Match("public_release", call.name)
    return None


def match_external_communication(
    pd: ProposedDecision, policy: Policy, spend_total: float
) -> Match | None:
    if "external_communication" not in policy.escalate_on.reversibility.triggers:
        return None
    for call in pd.tool_calls:
        if call.name in _EMAIL_TOOLS:
            return Match("external_communication", call.name)
    return None


def match_destructive_fs_op_outside_worktree(
    pd: ProposedDecision, policy: Policy, spend_total: float
) -> Match | None:
    if "destructive_fs_op_outside_worktree" not in policy.escalate_on.reversibility.triggers:
        return None
    worktree = pd.worktree_ref
    for call in pd.tool_calls:
        cmd = str(call.arguments.get("command", ""))
        if not _DESTRUCTIVE_FS.search(cmd):
            continue
        tokens = cmd.split()
        target = tokens[-1] if tokens else ""
        if _path_inside(target, worktree):
            continue
        return Match("destructive_fs_op_outside_worktree", cmd)
    return None


def match_git_push_protected_branch(
    pd: ProposedDecision, policy: Policy, spend_total: float
) -> Match | None:
    if "git_push_protected_branch" not in policy.escalate_on.reversibility.triggers:
        return None
    protected = set(policy.thresholds.protected_branches)
    for call in pd.tool_calls:
        cmd = str(call.arguments.get("command", ""))
        m = _GIT_PUSH.search(cmd)
        if m and m.group(1) in protected:
            return Match("git_push_protected_branch", m.group(1))
    return None


def match_spend_above(pd: ProposedDecision, policy: Policy, spend_total: float) -> Match | None:
    if "spend_above" not in policy.escalate_on.reversibility.triggers:
        return None
    floor = policy.escalate_on.reversibility.spend_above_usd
    if (spend_total + pd.spend_delta_usd) >= floor:
        return Match("spend_above", f"total>={floor}")
    return None


_BUILTIN_MATCHERS: tuple[Callable[[ProposedDecision, Policy, float], Match | None], ...] = (
    match_schema_migration_on_populated_table,
    match_data_deletion,
    match_public_release,
    match_external_communication,
    match_destructive_fs_op_outside_worktree,
    match_git_push_protected_branch,
    match_spend_above,
)


def _make_policy_matcher(
    name: str, pattern: str
) -> Callable[[ProposedDecision, Policy, float], Match | None]:
    """Compile a policy-defined regex matcher into the matcher contract."""
    compiled = re.compile(pattern)

    def _matcher(pd: ProposedDecision, policy: Policy, spend_total: float) -> Match | None:
        text = _aggregate_text(pd)
        if compiled.search(text):
            return Match(name, f"custom matcher fired: {name}")
        return None

    return _matcher


@dataclass
class Stage1Gate:
    policy: Policy
    audit_log: AuditLog
    spend_provider: Callable[[], float]

    def evaluate(self, pd: ProposedDecision, audit_ctx: AuditContext) -> None:
        """Raise `EscalationRequired` on the first matched trigger; else emit pass.

        Stage 1 deliberately does not accept an LLM client — that contract is
        enforced by `test_evaluate_signature_does_not_accept_llm_client`.
        """
        spend_total = float(self.spend_provider())
        all_matchers = list(_BUILTIN_MATCHERS) + [
            _make_policy_matcher(name, pat)
            for name, pat in self.policy.tool_wrapper_matchers.items()
        ]
        for matcher in all_matchers:
            match = matcher(pd, self.policy, spend_total)
            if match is None:
                continue
            payload = audit_ctx.envelope()
            payload.update({"trigger": match.trigger, "detail": match.detail})
            self.audit_log.emit("stage_1_escalate", payload)
            raise EscalationRequired(trigger=match.trigger, categories=["reversibility"])

        self.audit_log.emit("stage_1_pass", audit_ctx.envelope())


def wrap_tool(
    tool: Callable[..., Any],
    *,
    gate: Stage1Gate,
    audit_ctx: AuditContext,
    tool_name: str,
    argument_builder: Callable[..., dict[str, Any]],
    worktree_ref: str,
    decision_id: str,
) -> Callable[..., Any]:
    """Convenience wrapper. Constructs a ProposedDecision from the tool call,
    runs the gate; raises EscalationRequired if any trigger matches.
    Otherwise forwards to the tool.
    """

    def _wrapped(*args: Any, **kwargs: Any) -> Any:
        recorded_args = argument_builder(*args, **kwargs)
        synthetic = ProposedDecision(
            decision_id=decision_id,
            title=f"tool_call:{tool_name}",
            description=str(recorded_args),
            files_touched=[],
            modules=[],
            diff_stats={},
            urgency="medium",
            spend_delta_usd=0.0,
            worktree_ref=worktree_ref,
            tool_calls=[ProposedToolCall(name=tool_name, arguments=recorded_args)],
        )
        gate.evaluate(synthetic, audit_ctx=audit_ctx)
        return tool(*args, **kwargs)

    return _wrapped


__all__ = ["Match", "ProposedToolCall", "Stage1Gate", "wrap_tool"]
