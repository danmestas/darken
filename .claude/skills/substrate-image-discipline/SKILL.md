---
name: substrate-image-discipline
description: Use when canonical image source files (images/<harness>/Dockerfile or darkish-prelude.sh) have been edited; the local image tag must be rebuilt before any subharness dispatched into that harness will pick up the change
---

# Substrate Image Discipline

## Overview

The local Docker image tags (`local/darkish-claude:latest`, `local/darkish-codex:latest`, `local/darkish-gemini:latest`, `local/darkish-pi:latest`) are NOT automatically rebuilt by goreleaser at release time. They are local artifacts of `make -C images <harness>`. If the canonical source under `images/<harness>/` changes and the operator forgets to rebuild, every subsequent subharness dispatch into that harness uses the STALE image — even after a brew upgrade of `darken` itself.

Core rule: **after editing any file under `images/<harness>/`, rebuild that harness's image before dispatching the affected role.**

## When to Use

- After editing `images/<harness>/darkish-prelude.sh`
- After editing `images/<harness>/Dockerfile`
- After modifying `images/Makefile` or anything under `images/<harness>/bin/`
- After `git checkout` of a branch that has substrate-source changes you have not yet built
- Before dispatching the FIRST subharness of a session — verify the image tag is fresh

## The Rebuild

```
make -C images claude          # darkish-claude:latest
make -C images codex           # darkish-codex:latest
make -C images gemini          # darkish-gemini:latest
make -C images pi              # darkish-pi:latest
```

Each target is independent. Build only the ones whose source changed.

## Verifying the Image is Current

Before dispatching a role, sanity-check the image has the expected fix baked in. Pattern: pull a known string out of the prelude inside the image:

```
docker run --rm --entrypoint cat local/darkish-codex:latest \
  /usr/local/bin/darkish-prelude.sh | grep -c "<expected-marker>"
```

If the count is wrong, rebuild.

## Recorded Failures

- **rev-ousterhout (v0.1.17)**: dispatched against stale `local/darkish-codex:latest`. The codex prelude on `feat/v0.1.17` had the pre-clone pre-pop fix from B4, but the local image was last built on main (which lacked it). Container errored at `Git clone failed: git init failed:`. Required `scion stop`, branch checkout, `make -C images codex`, re-spawn.
- **General pattern**: brew upgrade of darken does NOT touch local image tags. The brew bottle ships the darken binary; images are built locally per-machine. Confusing because `darken setup` reports "OK darken images built" if any tag exists at all (regardless of how stale).

## Quick Reference

| Edited | Rebuild | Verify |
|---|---|---|
| `images/claude/darkish-prelude.sh` | `make -C images claude` | `docker run --rm --entrypoint cat local/darkish-claude:latest /usr/local/bin/darkish-prelude.sh \| grep <marker>` |
| `images/codex/Dockerfile` | `make -C images codex` | image exists with recent CreatedAt |
| Worktree out of sync (e.g. on main, code lives on feat branch) | `git checkout feat/<branch>` first | run rebuild from that branch |

## Anti-Patterns

- Rebuilding the wrong harness — check the actual file path you edited
- Forgetting that `make -C images all` may not exist; build per-harness
- Trusting `darken doctor` to detect stale images — it currently checks existence, not freshness
- Dispatching into a harness whose source you changed minutes ago without rebuilding — this happens often
