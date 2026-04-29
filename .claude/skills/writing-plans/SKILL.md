---
name: writing-plans
version: 0.1.0
description: >-
  Skill for plan authoring agents. Writes structured implementation plans
  to the worktree. When BONES_REPO is set, routes plan output through
  bones repo CI for review before finalizing. Backward compatible: when
  BONES_REPO is unset, writes plans directly to the local worktree.
type: skill
targets:
  - claude-code
category:
  primary: workflow
---

# writing-plans

You are authoring an implementation plan. Follow this protocol precisely.

## Output target

**Default (BONES_REPO unset):** Write the plan to `docs/superpowers/plans/<date>-<slug>.md` in your current worktree. Commit with message `plan: <slug>`.

**Bones repo CI (BONES_REPO set):** When the `BONES_REPO` environment variable is set to a path, route plan output through that repo for review:

1. Resolve the target directory: `${BONES_REPO}/plans/pending/`
2. Write the plan file there as `<date>-<slug>.md`
3. Commit to `${BONES_REPO}` with message `plan(pending): <slug>`
4. Signal CI by pushing the commit to the bones repo remote (if a remote is configured)
5. After CI approval (bones repo CI moves the file from `plans/pending/` to `plans/approved/`), copy the approved plan back to your worktree at `docs/superpowers/plans/<date>-<slug>.md` and commit

If `BONES_REPO` points to a path that does not exist or is not a git repo, fall back to the default output target and emit a warning: `writing-plans: BONES_REPO=${BONES_REPO} is not a git repo, falling back to local output`.

## Plan structure

Every plan MUST contain these sections in this order:

1. **Intent** -- one paragraph. What problem does this solve? What is out of scope?
2. **Constraints** -- bullet list. Non-negotiable technical and process limits.
3. **Design decisions** -- table. Decision | Rationale | Alternatives considered.
4. **Units** -- numbered list. Each unit is one atomic TDD implementer task:
   - Name (short slug)
   - Files touched
   - Failing test description
   - Acceptance criterion (one verifiable sentence)
   - Dependencies (prior unit names or "none")
5. **Risk register** -- table. Risk | Likelihood | Impact | Mitigation.
6. **Open questions** -- bullet list of unresolved items requiring operator input.

## Unit sizing

Each unit must be implementable in a single TDD cycle (write test, write impl, green, commit). A unit that requires more than ~150 LOC of production code is too large -- split it.

## Escalation

If you cannot write a unit spec because the interface is undefined or the acceptance criterion is unverifiable, add the unit to **Open questions** instead of guessing. Do not invent interfaces.

## Backward compatibility

Plans authored without `BONES_REPO` are fully valid. The bones repo CI path is opt-in. Existing tooling that reads `docs/superpowers/plans/` is not affected.
