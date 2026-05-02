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

**Q-A. Does scion offer a command to copy a template body from a path into its own local store?**

Verified 2026-05-01: **yes**, via `scion --global templates import <path>` (single) or `scion --global templates import --all <dir>` (batch from a directory of templates). The body is copied to `~/.scion/templates/<role>` and survives source-dir deletion. Probe used:

```bash
tmp=$(mktemp -d); cp -r .scion/templates/admin "$tmp/admin"
scion --global templates import "$tmp/admin"
rm -rf "$tmp"
scion --global templates list | grep admin   # admin still appears; body at ~/.scion/templates/admin
```

(Note: `templates create` — referenced earlier as a guess — actually creates an empty template, not what we want. `import` is the correct command.)

**Q-B. Does `scion templates push <role>` accept a `--from <path>` flag?**

Verified 2026-05-01: **no**. `templates push` accepts only `--all` and `--name`. It looks up the template by name in scion's own local store; the caller cannot pass a path at push time.

The implementer records both answers in the PR description before writing code. The design branch follows.

## Design — two conditional branches

The two branches are scored against Ousterhout principles. Pick whichever the verification step rules in.

### Branch A — register-at-extract (verification-confirmed)

Verification ruled in this branch (Q-A yes, Q-B no).

Single change site: `ensureAllSkillsStaged` (bootstrap.go:83-105) gains one extra step inside its existing extract→stage→cleanup window. After staging, before cleanup, call `scion --global templates import --all <dir>` once. Scion copies all 14 canonical role bodies into its own template store at `~/.scion/templates/<role>`. The deferred cleanup runs and removes the tmpdir. The `uploadAllTemplatesToHub` step (setup.go:50-58) then runs unchanged — `scion --global templates push <role>` succeeds because scion has the bodies.

Pseudocode:

```go
func ensureAllSkillsStaged(...) error {
    dir, cleanup, err := resolveTemplatesDir()
    if err != nil { return err }
    defer cleanup()

    if err := stageSkillsFrom(dir); err != nil { return err }

    if err := defaultScionClient.ImportAllTemplates(dir); err != nil {
        return fmt.Errorf("import templates: %w", err)
    }
    return nil
}
```

Files changed: bootstrap.go (one new call), scion_client.go (new `ImportAllTemplates(dir)` method), scion_client_test.go (mock gains the new method).

`uploadAllTemplatesToHub`, `PushTemplate`, `runUp`, `up.go`, `setup.go`: untouched.

**Why this is the deeper module.**

- `runUp` doesn't learn about templates resolution. The `runBootstrap()` post-condition becomes "scion has all 14 canonical templates registered locally." That precondition is what `uploadAllTemplatesToHub` already implicitly assumes.
- The error class "tmpdir gone before push" is defined out of existence — push never reads the tmpdir.
- One file changed, one new method. No signature breakage on `PushTemplate`.

### Branch B — tmpdir-lifetime-spans-up-flow (fallback)

Applies when **Q-A is no** (scion `create` is reference-only) **AND** Q-B answers either yes or no — both push variants need the dir alive for the same reason.

`runUp` resolves the templates dir, defers cleanup, and constructs a `ScionClient` that holds the dir as hidden state. The dir-lifetime concern lives in two scopes only: `runUp` (owner) and `ScionClient` (consumer). Bootstrap and upload-templates are dir-blind — they call `client.PushTemplate(role)` and don't know where the bytes live.

```go
func runUp(...) error {
    dir, cleanup, err := resolveTemplatesDir()
    if err != nil { return err }
    defer cleanup()

    client := NewScionClient(WithTemplatesDir(dir))

    if err := runBootstrap(client, ...); err != nil { return err }
    if err := uploadAllTemplatesToHub(client); err != nil { return err }
    return nil
}
```

`ScionClient.PushTemplate(role)` signature stays as today. Internally, when scion's CLI requires a path (either `--from` or two-step `create`), the client reads its own `templatesDir` field and assembles the per-role path. Callers ask for "push template admin" without knowing about dirs.

| File | Function | Before | After |
|---|---|---|---|
| cmd/darken/scion_client.go | `ScionClient` constructor | implicit zero-value | `NewScionClient(opts ...Option)` with `WithTemplatesDir(dir)` functional option |
| cmd/darken/scion_client.go:83 | `PushTemplate(role)` | shells `scion --global templates push <role>` | shells `scion --global templates push <role> --from <c.templatesDir>/<role>` (Q-B yes) OR two-step `create <role> <c.templatesDir>/<role>` then `push <role>` (Q-B no) |
| cmd/darken/setup.go:50 | `uploadAllTemplatesToHub` | uses `defaultScionClient` | takes `client ScionClient`; iterates roles; calls `client.PushTemplate(role)` |
| cmd/darken/bootstrap.go:83 | `ensureAllSkillsStaged` | resolves internally | takes the resolved `dir string` |

`PushTemplate` signature unchanged. Test mocks at scion_client_test.go:43 unchanged.

### Decision rule

```
Q-A yes → Branch A.
Q-A no  → Branch B (with Q-B picking one-step vs two-step exec form).
```

The implementer picks the branch in the PR after running step 0, and the rest of the implementation follows.

## Error handling

Existing fail-fast preserved. On any error from `ImportAllTemplates` (Branch A) or `PushTemplate` (Branch B), `runBootstrap` or `uploadAllTemplatesToHub` returns the wrapped error. The deferred cleanup runs. The operator's recovery is `darken up` again.

```go
return fmt.Errorf("import templates: %w", err)
// or
return fmt.Errorf("upload template %s: %w", role, err)
```

No partial-progress retry. Out of scope.

## Test strategy

The test asserts the **observable outcome**, not the implementation invariant. This makes the test correct regardless of which branch ships.

### Primary test — observable outcome

`TestRunUp_AllTemplatesRegisteredAfterUp` in cmd/darken/up_test.go.

1. Fresh tmpdir as the consuming project (no `.scion/templates/`).
2. PATH-stub scion that records every invocation AND maintains an in-memory map of registered roles. The stub mimics scion's own behavior: `templates import <path>` and `templates import --all <dir>` register the role(s) in the map; `templates push <role>` requires the role be in the map (returns "not found locally" otherwise — the exact production error); `templates list` returns the map keys.
3. Run `runUp(...)` end-to-end. Docker/Hub calls remain stubbed.
4. After return, invoke the PATH-stub `scion --global templates list` and assert all 14 canonical roles appear.

This test fails on `main` (no role ever gets registered before push). It passes under either Branch A or Branch B because both end with scion's local store containing the 14 roles. The assertion is design-agnostic.

### Secondary test — branch-specific lifecycle

Once the implementer picks a branch:

- **Branch A:** assert `templates import --all <dir>` was invoked exactly once during bootstrap, with `<dir>` containing all 14 canonical role subdirectories at call time (stub records call sequence and inspects argument).
- **Branch B:** assert the source dir exists on disk at the moment every `templates push <role>` (or the two-step form) is recorded by the stub.

Both are invariant tests guarding against regressions in the chosen design.

### Existing tests

`scion_client_test.go:119` (mocked `PushTemplate` call-count) keeps catching client-API regressions under either branch — `PushTemplate(role)` signature is unchanged.

`setup_test.go:276-303` (verifies the `--global templates push <role>` invocation shape) keeps catching invocation-shape regressions.

## Backward-compatibility

- **darkish-factory.** `.scion/templates/<role>/` exists at repo root. `resolveTemplatesDir` returns repo-root path with a no-op cleanup. No extraction. No behavior change under either branch.
- **All other projects.** Branch A: one `templates import --all <dir>` exec during bootstrap; all 14 templates registered to scion's local store. Branch B: client holds the dir; push reads its per-role path internally. Either way: zero successful uploads → fourteen.
- **Public function signatures.** Branch A: `ScionClient` gains a method (`ImportAllTemplates`); no existing method changes. Branch B: `ScionClient` gains an optional constructor (`NewScionClient(WithTemplatesDir(...))`) and an internal field; `PushTemplate(role)` signature unchanged. No breaking change to existing callers in either branch.

## Migration

Single PR. No staged rollout. The fix is internal to `cmd/darken` and the test stub.

## Open questions for operator

1. **Branch / PR shape.** Single PR for verification + fix + test, or split (verification commit reporting Q-A/Q-B, then design-branch commit, then test commit)? Spec defaults to single PR; PR description records the verification answers and selected branch.
2. **Verification ruled in Branch A.** Branch B is documented as a fallback for completeness; not pursued by the implementation.

## File-paths cited

- cmd/darken/up.go:45
- cmd/darken/setup.go:50-58
- cmd/darken/bootstrap.go:83-105, 129-181
- cmd/darken/scion_client.go:83-88
- cmd/darken/roles.go:6-21
- cmd/darken/scion_client_test.go:43, 117-135
- cmd/darken/setup_test.go:192-203, 276-303
- README.md:22
