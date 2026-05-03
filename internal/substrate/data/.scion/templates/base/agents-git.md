# Git Worktree Protocol

Every darken sub-harness owns exactly one git worktree. This file defines the rules. Role-specific behavior does not override these rules.

---

## Worktree Ownership

- You work in your assigned worktree only. You do not write to any other worktree.
- You do not check out `main` or any other branch that may be checked out in another worktree. Doing so will fail or corrupt state.
- Comparisons to main: `git diff main...HEAD`
- File inspection on main: `git show main:path/to/file`
- Rebasing: `git rebase main` from your current branch

---

## Commit Discipline

- Every commit is atomic: one logical unit of work, one commit. Do not bundle unrelated changes.
- Every commit message is present-tense imperative: `Add failing test for auth edge case`, not `Added tests`.
- Never commit work-in-progress. If you need to save state mid-task, use `git stash`.
- Never amend a commit that has been handed off to the orchestrator. If a fix is needed, make a new commit.
- Never rebase shared history. You may rebase your own branch against `main` before handoff; you may not rebase after the orchestrator has cherry-picked from you.

---

## Handoffs

Handoffs are the orchestrator's responsibility, not yours. Your job is to produce well-formed commits the orchestrator can cherry-pick.

What the orchestrator does:
```bash
git cherry-pick <your-commit-sha>
```

What you must provide:
- A clean commit SHA that represents your deliverable.
- No merge commits. No squashed history that loses intermediate refs.
- A `docs/` subdirectory in your worktree with any structured output (briefs, specs, plans) as committed files, not loose files.

---

## Non-interactive Environment

You are running in a non-interactive sandbox. Configure git to avoid editor prompts:

```bash
git config core.editor true
```

Commit with explicit messages:

```bash
git commit -m "Your message"
```

Rebase without interactive mode:

```bash
git rebase main
GIT_EDITOR=true git rebase --continue  # after resolving conflicts
```

Merge without editor:

```bash
git merge main --no-edit
```

---

## Conflict Resolution

If a rebase or merge produces conflicts:

1. `git status` to identify conflicted files.
2. Resolve conflicts in the source files.
3. `git add <resolved-files>`
4. `GIT_EDITOR=true git rebase --continue`

Do not abort a rebase without logging the reason and notifying the orchestrator.

---

## What Never Happens

- No `git push` from a sub-harness worktree. The orchestrator owns pushes to protected branches, and they are a reversibility trigger (README §2).
- No `git fetch` or `git pull`. The environment is air-gapped by default; assume `main` is the local source of truth.
- No cross-worktree writes. Your filesystem scope is your worktree.

---

Implements README §5.3, §2 (reversibility axis).
