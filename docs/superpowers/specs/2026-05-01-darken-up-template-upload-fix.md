# Spec: `darken up` template-upload fix

**Date:** 2026-05-01
**Status:** Draft, awaiting operator approval of written spec
**Topic:** Plug the leak in `uploadAllTemplatesToHub` so `darken up` succeeds in any project, not just the darkish-factory checkout.

## Problem

`darken up` fails at the template-upload step in any project that does not have `.scion/templates/<role>/` committed at the repo root. Symptom (observed in `~/base4m`):

```
uploading template admin to Hub (user scope) ...
Error: template 'admin' not found locally: template 'admin' not found
darken: upload template admin: exit status 1
```

The 8-stage bootstrap completes cleanly. The failure is in the post-bootstrap Hub-push step.

### Root cause

1. `runUp` (cmd/darken/up.go:45) calls `runBootstrap()` → `uploadAllTemplatesToHub()` in sequence.
2. `runBootstrap` step 7 (`ensureAllSkillsStaged`, cmd/darken/bootstrap.go:83-105) calls `resolveTemplatesDir()` (bootstrap.go:129-140). When `repoRoot()/.scion/templates/` is absent, it falls through to `extractEmbeddedTemplates()` (bootstrap.go:142-181) which copies the embedded set into `os.MkdirTemp`.
3. `ensureAllSkillsStaged` `defer`s the cleanup func from `extractEmbeddedTemplates`. The tmpdir is removed when bootstrap returns.
4. `uploadAllTemplatesToHub` (cmd/darken/setup.go:50-58) then iterates `canonicalRoles` (roles.go:6-21) and shells `scion --global templates push <role>` (scion_client.go:84). Scion looks in its own local store; nothing is registered → 404.

In darkish-factory, `.scion/templates/<role>/` exists in the repo root, `resolveTemplatesDir` returns that path with no extraction, no cleanup ever runs, and scion finds the templates via its file-system convention. The bug is masked.

### Why tests miss it

`scion_client_test.go:119` and `setup_test.go:276-303` mock `ScionClient.PushTemplate`. The mock never observes the directory state at the moment `PushTemplate` would have been called. README.md:22 ("the `darken` binary is self-contained — templates, scripts, Dockerfiles, and the host-mode skills are embedded") is partly true: the bootstrap-stage path honors it; the upload-templates path silently does not.

## Scope

**In scope.**

- Fix `darken up` so all 14 canonical templates are registered with the Hub when run in any project.
- Test coverage that fails today and passes after the fix.
- Honor README.md:22's "self-contained" claim.

**Out of scope.**

- Per-project template overrides. `resolveTemplatesDir()` already prefers `repoRoot()/.scion/templates/` when present, so a fork or custom checkout still works. A formal override design (merge semantics, conflict rules, custom-role support) is a separate spec triggered by a real user request.
- Partial-progress retry. Current fail-fast on first push error stays. Recovery stays "re-run `darken up`".
- The existing dual-source resolver (`repoRoot()/.scion/templates/` vs. embedded fallback). The fix preserves that precedence.

## Verification step 0 — scion CLI semantics

The right design depends on a fact about scion that the spec cannot assume. Before writing any code, the implementer establishes the scion CLI surface by running:

```bash
scion templates push --help
scion templates create --help
scion templates list --help     # for the test assertion
```

Two questions to answer:

**Q-A. Does `scion templates create <role> <path>` intern (copy) the template body into scion's own store?** Run a one-line probe:

```bash
tmp=$(mktemp -d); cp -r .scion/templates/admin "$tmp/admin"; \
  scion --global templates create admin "$tmp/admin"; \
  rm -rf "$tmp"; \
  scion --global templates list | grep admin
```

If `admin` still appears after `rm -rf`, scion interns the body. If `templates list` errors or `admin` is gone, scion stored a path reference and the dir must outlive every later operation.

**Q-B. Does `scion templates push <role>` accept a `--from <path>` flag** that lets the caller pass the dir at push time? Read `--help` output; presence/absence is a binary fact.

The implementer records both answers in the PR description before writing code. The design branch follows.

## Design — two conditional branches

The two branches are scored against Ousterhout principles. Pick whichever the verification step rules in.

### Branch A — register-at-extract (preferred)

Applies when **Q-A is yes** (`create` interns the body).

Single change site: `ensureAllSkillsStaged` (bootstrap.go:83-105) gains one extra step inside its existing extract→stage→cleanup window. After staging, before cleanup, call `scion --global templates create <role> <dir>/<role>` for each role. Scion now has the bodies internalized in its own template store. The deferred cleanup runs and removes the tmpdir. The `uploadAllTemplatesToHub` step (setup.go:50-58) then runs unchanged — `scion --global templates push <role>` succeeds because scion already has the body.

Pseudocode:

```go
func ensureAllSkillsStaged(...) error {
    dir, cleanup, err := resolveTemplatesDir()
    if err != nil { return err }
    defer cleanup()

    if err := stageSkillsFrom(dir); err != nil { return err }

    for _, role := range canonicalRoles {
        if err := defaultScionClient.CreateTemplate(role, filepath.Join(dir, role)); err != nil {
            return fmt.Errorf("create template %s: %w", role, err)
        }
    }
    return nil
}
```

Files changed: bootstrap.go (one new loop), scion_client.go (new `CreateTemplate(role, dir)` method), scion_client_test.go (mock gains the new method).

`uploadAllTemplatesToHub`, `PushTemplate`, `runUp`, `up.go`, `setup.go`: untouched.

**Why this is the deeper module.**

- `runUp` doesn't learn about templates resolution. The `runBootstrap()` post-condition becomes "scion has all 14 canonical templates registered locally." That precondition is what `uploadAllTemplatesToHub` already implicitly assumes.
- The error class "tmpdir gone before push" is defined out of existence — push never reads the tmpdir.
- One file changed, one new method. No signature breakage on `PushTemplate`.

### Branch B — tmpdir-lifetime-spans-up-flow (fallback)

Applies when **Q-A is no** (scion `create` is reference-only) **AND** Q-B answers either yes or no — both push variants need the dir alive for the same reason.

`runUp` (cmd/darken/up.go:45) becomes the single owner of the resolved templates dir. Both bootstrap stage-skills and upload-templates borrow the path.

```go
func runUp(...) error {
    dir, cleanup, err := resolveTemplatesDir()
    if err != nil { return err }
    defer cleanup()

    if err := runBootstrap(dir, ...); err != nil { return err }
    if err := uploadAllTemplatesToHub(dir); err != nil { return err }
    return nil
}
```

| File | Function | Before | After |
|---|---|---|---|
| cmd/darken/bootstrap.go:83 | `ensureAllSkillsStaged` | resolves internally | takes `dir string` |
| cmd/darken/setup.go:50 | `uploadAllTemplatesToHub` | iterates roles, `client.PushTemplate(role)` | takes `dir string`, calls `client.PushTemplate(role, dir)` |
| cmd/darken/scion_client.go:83 | `PushTemplate` | `PushTemplate(role)` | `PushTemplate(role, dir)` |
| cmd/darken/scion_client.go:84 | exec form | `scion --global templates push <role>` | `scion --global templates push <role> --from <dir>/<role>` (Q-B yes) OR two-step `create <role> <dir>/<role>` then `push <role>` (Q-B no) |

Branch B accepts the cost of widening `PushTemplate`'s interface and wiring the dir through three layers because no other path is available given Q-A no.

### Decision rule

```
Q-A yes → Branch A.
Q-A no  → Branch B (with Q-B picking one-step vs two-step exec form).
```

The implementer picks the branch in the PR after running step 0, and the rest of the implementation follows.

## Error handling

Existing fail-fast preserved. On any error from `CreateTemplate` (Branch A) or `PushTemplate` (Branch B), `runBootstrap` or `uploadAllTemplatesToHub` returns the wrapped error. The deferred cleanup runs. The operator's recovery is `darken up` again.

```go
return fmt.Errorf("create template %s: %w", role, err)
// or
return fmt.Errorf("upload template %s: %w", role, err)
```

No partial-progress retry. Out of scope.

## Test strategy

The test asserts the **observable outcome**, not the implementation invariant. This makes the test correct regardless of which branch ships.

### Primary test — observable outcome

`TestRunUp_AllTemplatesRegisteredAfterUp` in cmd/darken/up_test.go.

1. Fresh tmpdir as the consuming project (no `.scion/templates/`).
2. PATH-stub scion that records every invocation AND maintains an in-memory map of registered roles. The stub mimics scion's own behavior: `templates create <role> <path>` records the role in the map; `templates push <role>` requires the role be in the map (returns "not found locally" otherwise — the exact production error); `templates list` returns the map keys.
3. Run `runUp(...)` end-to-end. Docker/Hub calls remain stubbed.
4. After return, invoke the PATH-stub `scion --global templates list` and assert all 14 canonical roles appear.

This test fails on `main` (no role ever gets registered before push). It passes under either Branch A or Branch B because both end with scion's local store containing the 14 roles. The assertion is design-agnostic.

### Secondary test — branch-specific lifecycle

Once the implementer picks a branch:

- **Branch A:** assert `templates create` was invoked once per role during bootstrap (stub records call sequence).
- **Branch B:** assert the source dir exists on disk at the moment every `templates push <role>` (or the two-step form) is recorded by the stub.

Both are invariant tests guarding against regressions in the chosen design.

### Existing tests

`scion_client_test.go:119` (mocked `PushTemplate` call-count) keeps catching client-API regressions. Under Branch A it stays as-is; under Branch B the mock signature updates to match.

`setup_test.go:276-303` (verifies the `--global templates push <role>` invocation shape) keeps catching invocation-shape regressions.

## Backward-compatibility

- **darkish-factory.** `.scion/templates/<role>/` exists at repo root. `resolveTemplatesDir` returns repo-root path with a no-op cleanup. No extraction. No behavior change under either branch.
- **All other projects.** Branch A: 14 extra `templates create` execs during bootstrap; templates registered. Branch B: the dir survives until push (one-step) or until create+push (two-step). Either way: zero successful uploads → fourteen.
- **Public function signatures.** Branch A: `ScionClient` gains a method (`CreateTemplate`); no existing method changes. Branch B: `ScionClient.PushTemplate` gains a parameter — breaking change for any external caller. There are none in the repo. No external Go-import surface in either case.

## Migration

Single PR. No staged rollout. The fix is internal to `cmd/darken` and the test stub.

## Open questions for operator

1. **Branch / PR shape.** Single PR for verification + fix + test, or split (verification commit reporting Q-A/Q-B, then design-branch commit, then test commit)? Spec defaults to single PR; PR description records the verification answers and selected branch.
2. **If Branch B with Q-B no (two-step).** The fallback path adds 14 extra exec calls (`templates create` per role). Acceptable, or worth a scion-side feature request for `templates push --from`? Spec defaults to "ship two-step, file scion issue separately."

## File-paths cited

- cmd/darken/up.go:45
- cmd/darken/setup.go:50-58
- cmd/darken/bootstrap.go:83-105, 129-181
- cmd/darken/scion_client.go:83-88
- cmd/darken/roles.go:6-21
- cmd/darken/scion_client_test.go:43, 117-135
- cmd/darken/setup_test.go:192-203, 276-303
- README.md:22
