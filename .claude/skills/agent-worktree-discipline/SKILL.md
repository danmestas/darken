---
name: agent-worktree-discipline
description: Use when dispatching darken spawn subharnesses or reading their committed work back from .scion/agents/<name>/workspace
---

# Agent Worktree Discipline

## Overview

Agent worktrees at `.scion/agents/<name>/workspace/` are **destroyed** when the agent is deleted via `scion delete <name>`. Any commit that exists only in the worktree (not pushed to a remote branch) is permanently lost.

Core rule: **push every commit immediately. The worktree is not durable storage.**

## When to Use

- Dispatching any tdd-implementer, planner, or other subharness that will commit
- Reading committed work back from an agent worktree
- Cleaning up agents after a pipeline completes
- Investigating a session's history retroactively

## Rules

1. **Brief every dispatched implementer with: push to a feature branch after every commit.** Single-commit-per-task with `git push origin feat/<branch>` between commits. Never accumulate uncommitted or unpushed work.

2. **Never `scion delete <agent>` until you have verified the agent's commits exist on origin.** Run `git fetch origin <branch>; git log --oneline main..origin/<branch>` and confirm the SHAs match what you expect before deletion.

3. **Use `git -C <absolute-path>` instead of `cd` when inspecting agent worktrees.** A `cd` into the worktree shifts the orchestrator's cwd into a different git repo (the worktree has a token-embedded clone URL different from the host repo), and subsequent commands target the wrong repo. Lost an entire PR cycle to this in the v0.1.16 push attempt.

4. **Cherry-pick from worktree before delete if not pushed.** `git fetch /absolute/path/to/.scion/agents/<name>/workspace <branch>; git cherry-pick FETCH_HEAD` preserves authorship.

## Recorded Failures

- **impl-1 (v0.1.16 cycle)**: 3 commits committed in worktree, never pushed, then `scion delete impl-1 -y` was run — all 3 commits irrecoverable. Bug #18 in v0.1.18 backlog (preserve-on-delete) addresses this at substrate level; until landed, push-as-you-go is mandatory.
- **PR #25 path confusion**: `cd .scion/agents/.../workspace` followed by `git push -u origin <branch>` wrote to the worktree's remote (which had a different URL); subsequent `gh pr create` from the orchestrator dir picked the wrong default repo, returning a confusing error. `git -C <path>` would have prevented it.

## Brief Template (paste into every implementer dispatch)

```
After EACH commit, run git push origin <feature-branch> immediately.
Never accumulate uncommitted work or unpushed commits.
If budget runs out, push what you have before terminating.
```
