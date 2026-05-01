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

## Design

### Tmpdir ownership moves up to `runUp`

`runUp` (cmd/darken/up.go:45) becomes the single owner of the resolved templates dir. Both bootstrap stage-skills and upload-templates borrow the path.

Pseudocode:

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

### Function signature changes

| File | Function | Before | After |
|---|---|---|---|
| cmd/darken/bootstrap.go:83 | `ensureAllSkillsStaged` | resolves internally | takes `dir string` |
| cmd/darken/bootstrap.go:129 | `resolveTemplatesDir` | unchanged (still callable from `runUp`) | unchanged |
| cmd/darken/setup.go:50 | `uploadAllTemplatesToHub` | iterates roles, calls `client.PushTemplate(role)` | takes `dir string`, calls `client.PushTemplate(role, dir)` |
| cmd/darken/scion_client.go:83 | `ScionClient.PushTemplate` | `PushTemplate(role string)` | `PushTemplate(role, dir string)` |
| cmd/darken/scion_client.go:84 | exec form | `scion --global templates push <role>` | `scion --global templates push <role> --from <dir>/<role>` (preferred) OR two-step (fallback) |

### Verification gap (implementation step 0)

Before coding, verify what `scion templates push` accepts:

```bash
scion templates push --help
```

- If `--from <path>` is supported: one-step form.
- If not: two-step — `scion templates create <role> <dir>/<role>` followed by `scion templates push <role>`. The `create` step makes scion aware of the local template; the subsequent `push` ships it to the Hub.

The spec records this as a verification gap, not a guess. The implementer reports back which form scion supports and the test/code follow that path.

### Error handling

Existing fail-fast preserved (setup.go:53-55). On any push error:

```go
return fmt.Errorf("upload template %s: %w", role, err)
```

This bubbles to `runUp`, which returns. The deferred cleanup runs. The operator's recovery is `darken up` again. No partial-progress state to manage.

## Test strategy

PATH-stub scion. The pattern at setup_test.go:194 already substitutes a fake `scion` binary on `PATH` that logs invocations. Extend it:

1. **Stub records dir-state at invocation time.** When the stub is called for `templates push <role> [--from <dir>]` (or the two-step form), it asserts the source directory exists on the filesystem AND contains the role's `template.yaml`. Records pass/fail per call.

2. **New test: `TestRunUp_TemplateDirSurvivesUntilPush`** in cmd/darken/up_test.go. Setup:
   - Fresh tmpdir as the consuming project (no `.scion/templates/`).
   - PATH-stub scion as above.
   - Run `runUp(...)` end-to-end (excluding actual Docker/Hub calls — those are separately mocked or stubbed).
   - Assert: every one of the 14 canonical roles produced a successful push call (stub recorded a live source dir).

3. **Failing-state coverage.** The new test fails on `main` today (push happens after cleanup). Verifies the bug. Passes after the fix.

4. **Existing coverage stays.** `scion_client_test.go:119` (mocked `PushTemplate` records call count) keeps catching client-API regressions; `setup_test.go:276-303` keeps verifying the `--global templates push <role>` invocation shape.

## Backward-compatibility

- **darkish-factory.** `.scion/templates/<role>/` exists at repo root. `resolveTemplatesDir` returns repo-root path with a no-op cleanup. No extraction. Behavior identical to today.
- **All other projects.** One extraction per `darken up` (was: zero successful uploads). Net win.
- **Public function signatures.** `ScionClient.PushTemplate` gains a parameter — breaking change for any external caller. There are none in the repo. Mock at scion_client_test.go:43 must update to match. No external Go-import surface to consider.

## Migration

Single PR. No staged rollout needed. The fix is internal to `cmd/darken` and the test stub.

## Open questions for operator

1. **Branch / PR shape.** Single PR for fix + test, or split (test-first as a failing test on its own commit, then fix as a second commit on the same PR)? Spec defaults to single PR with both commits.
2. **Verification gap escalation.** If `scion templates push --from` is NOT supported, the two-step form adds a second exec per role (28 exec calls instead of 14). Acceptable, or worth a scion-side feature request to add `--from`? Spec defaults to "use two-step, file scion issue separately."

## File-paths cited

- cmd/darken/up.go:45
- cmd/darken/setup.go:50-58
- cmd/darken/bootstrap.go:83-105, 129-181
- cmd/darken/scion_client.go:83-88
- cmd/darken/roles.go:6-21
- cmd/darken/scion_client_test.go:43, 117-135
- cmd/darken/setup_test.go:192-203, 276-303
- README.md:22
