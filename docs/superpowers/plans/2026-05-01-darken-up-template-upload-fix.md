# `darken up` template-upload fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `darken up` succeed in any project (not just darkish-factory) by registering the 14 canonical templates with scion's local store during bootstrap, before the Hub-push step runs.

**Architecture:** Branch A from `docs/superpowers/specs/2026-05-01-darken-up-template-upload-fix.md`. Verification confirmed `scion --global templates import --all <dir>` copies template bodies into scion's permanent store at `~/.scion/templates/<role>`. Wire one new call into `ensureAllSkillsStaged` so scion has the templates before `uploadAllTemplatesToHub` later calls `scion templates push <role>`.

**Tech Stack:** Go (standard library + scion CLI). Tests use the existing PATH-stub harness in `cmd/darken/setup_test.go`.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `cmd/darken/scion_client.go` | Production scion CLI wrapper | Add `ImportAllTemplates(dir string) error` to the `ScionClient` interface and to `execScionClient` |
| `cmd/darken/scion_client_test.go` | Mock scion client | Extend `mockScionClient` with `importAllTemplatesCalls []string` plus the interface method |
| `cmd/darken/bootstrap.go` | Bootstrap step orchestration | Call `defaultScionClient.ImportAllTemplates(templatesDir)` inside `ensureAllSkillsStaged` after stage-skills loop, before deferred cleanup runs |
| `cmd/darken/setup_test.go` | PATH-stub integration test | Tighten the scion stub to model "import-before-push" semantics so the existing `TestSetup_UploadsAllTemplatesToHub` catches the bug |
| `cmd/darken/bootstrap_test.go` (new file) | Unit test for the new wiring | Assert `ensureAllSkillsStaged` calls `ImportAllTemplates` with the resolved templatesDir |

Files NOT changed: `cmd/darken/up.go`, `cmd/darken/setup.go`, `cmd/darken/roles.go`. Branch A's whole point is that runUp and uploadAllTemplatesToHub are untouched.

---

## Task 1: Tighten scion PATH-stub to model import-before-push (RED)

The current stub at `cmd/darken/setup_test.go:27-45` always exits 0 on `templates push`. That's why `TestSetup_UploadsAllTemplatesToHub` passes on `main` even though `darken up` fails in the real world. Make the stub model scion's actual behavior: `templates push <role>` requires that `<role>` was previously imported. This test is RED until Task 3 is complete.

**Files:**
- Modify: `cmd/darken/setup_test.go:18-80` (the `stubAllBinariesForSetup` function — specifically the `scionStub` heredoc at lines 27-45)

- [ ] **Step 1: Read the current stub**

```bash
sed -n '11,80p' cmd/darken/setup_test.go
```

Confirm the scion stub exits 0 unconditionally and only special-cases `list` and `hub`.

- [ ] **Step 2: Replace `scionStub` heredoc with import-aware version**

Replace lines 27-45 of `cmd/darken/setup_test.go` (the `scionStub` heredoc) with:

```go
	scionStub := `#!/bin/sh
echo "scion $@" >> ` + logPath + `
# Path of the import-state file. Each role registered via 'templates import'
# adds a line to this file; 'templates push' then requires the role be present.
state="` + filepath.Join(stubDir, "scion-import-state") + `"

# Strip global flags to find the subcommand.
while [ "$1" = "--global" ] || [ "$1" = "--no-hub" ]; do shift; done

case "$1" in
  list)
    case "$2" in
      "--format") echo "[]" ;;
      *) echo "" ;;
    esac
    ;;
  hub)
    if [ "$3" = "list" ]; then
      echo "claude_auth"
      echo "codex_auth"
      echo "gemini_auth"
    fi
    ;;
  templates)
    sub="$2"
    case "$sub" in
      import)
        # 'import --all <dir>' or 'import <path>'
        if [ "$3" = "--all" ]; then
          dir="$4"
          for d in "$dir"/*/; do
            role=$(basename "$d")
            echo "$role" >> "$state"
          done
        else
          role=$(basename "$3")
          echo "$role" >> "$state"
        fi
        ;;
      push)
        role="$3"
        if [ ! -f "$state" ] || ! grep -qx "$role" "$state"; then
          echo "Error: template '$role' not found locally: template '$role' not found" >&2
          exit 1
        fi
        ;;
      list)
        if [ -f "$state" ]; then cat "$state"; fi
        ;;
    esac
    ;;
esac
exit 0
`
```

Note the use of `filepath.Join(stubDir, "scion-import-state")` — Go string interpolation, not shell. The heredoc must be assembled in Go code; the import-state file path is a Go expression.

- [ ] **Step 3: Run the existing test to confirm it now fails**

Run: `go test ./cmd/darken/ -run TestSetup_UploadsAllTemplatesToHub -v`

Expected: FAIL with output like `expected scion "--global templates push admin" to be called` — actually the push IS called but exits non-zero because no import preceded it. Look for the runSetup return error: `bootstrap returned error: upload template admin: ...`. The exact failure message depends on how runSetup propagates the push error; the key observation is the test no longer passes.

If the test still passes, the stub change wasn't applied — re-check Step 2.

- [ ] **Step 4: Commit**

```bash
git add cmd/darken/setup_test.go
git commit -m "test: model scion import-before-push semantics in PATH stub

The existing scion stub exited 0 unconditionally, which is why
TestSetup_UploadsAllTemplatesToHub passed despite darken up
failing in real usage. Now the stub tracks imported roles in a
state file and requires push targets to have been imported first.
Test is RED until ImportAllTemplates is wired into bootstrap."
```

---

## Task 2: Add `ImportAllTemplates` to the `ScionClient` interface and implementations

**Files:**
- Modify: `cmd/darken/scion_client.go:16-45` (interface declaration); `cmd/darken/scion_client.go:50-108` (execScionClient methods)
- Modify: `cmd/darken/scion_client_test.go:10-55` (mockScionClient struct + methods)

- [ ] **Step 1: Add interface method to `ScionClient`**

In `cmd/darken/scion_client.go`, inside the `ScionClient` interface block (lines 16-45), add this method between `PushTemplate` and `GroveInit`:

```go
	// ImportAllTemplates copies every template subdirectory under dir into
	// scion's local store at user (global) scope. Idempotent: re-importing
	// the same template overwrites the prior copy. Bodies survive deletion
	// of the source dir, so the caller can clean up an extracted tmpdir
	// immediately after this returns.
	ImportAllTemplates(dir string) error
```

- [ ] **Step 2: Implement `ImportAllTemplates` on `execScionClient`**

After `PushTemplate` in `cmd/darken/scion_client.go` (insert after line 88), add:

```go
func (c *execScionClient) ImportAllTemplates(dir string) error {
	cmd := scionCmdWithEnv([]string{"--global", "templates", "import", "--all", dir})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 3: Add mock method**

In `cmd/darken/scion_client_test.go`, extend the `mockScionClient` struct (lines 11-28) by adding two fields:

```go
	importAllTemplatesErr   error
	importAllTemplatesCalls []string
```

Insert them next to the other call-tracking fields (around line 24, near `pushTemplateCalls`).

Then add the method after `PushTemplate` (line 46):

```go
func (m *mockScionClient) ImportAllTemplates(dir string) error {
	m.importAllTemplatesCalls = append(m.importAllTemplatesCalls, dir)
	return m.importAllTemplatesErr
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./cmd/darken/`

Expected: success (no output). Confirms the interface and both implementations satisfy the contract.

- [ ] **Step 5: Run all existing cmd/darken tests**

Run: `go test ./cmd/darken/ -count=1`

Expected: `TestSetup_UploadsAllTemplatesToHub` still FAILs (Task 1's RED state). All other tests should still PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/scion_client.go cmd/darken/scion_client_test.go
git commit -m "feat(scion-client): add ImportAllTemplates method

Wraps 'scion --global templates import --all <dir>'. Copies all
template subdirectories under dir into scion's permanent local
store. Used by bootstrap to register canonical roles before the
Hub-push step. Mock and execScionClient both implement the new
interface method; not yet called from bootstrap."
```

---

## Task 3: Call `ImportAllTemplates` from `ensureAllSkillsStaged` (GREEN)

**Files:**
- Modify: `cmd/darken/bootstrap.go:83-105` (the `ensureAllSkillsStaged` function)

- [ ] **Step 1: Read the current function**

```bash
sed -n '80,106p' cmd/darken/bootstrap.go
```

Note the structure: resolve dir, defer cleanup, set DARKEN_TEMPLATES_DIR env, iterate dirs running stage-skills.sh per harness.

- [ ] **Step 2: Add the import call after the stage-skills loop, before cleanup runs**

Replace `cmd/darken/bootstrap.go:83-105` with:

```go
func ensureAllSkillsStaged() error {
	templatesDir, cleanup, err := resolveTemplatesDir()
	if err != nil {
		return err
	}
	defer cleanup()

	if err := withTemplatesDirEnv(templatesDir, func() error {
		dirs, err := os.ReadDir(templatesDir)
		if err != nil {
			return fmt.Errorf("read templates dir %s: %w", templatesDir, err)
		}
		for _, d := range dirs {
			if !d.IsDir() || d.Name() == "base" {
				continue
			}
			if err := runSubstrateScript("scripts/stage-skills.sh", []string{d.Name()}); err != nil {
				fmt.Fprintf(os.Stderr, "bootstrap: stage-skills %s failed: %v\n", d.Name(), err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Register every canonical role with scion's local template store so
	// uploadAllTemplatesToHub can push them. The deferred cleanup above
	// removes the source dir; scion's import copies bodies into its own
	// store, so post-cleanup push works.
	if err := defaultScionClient.ImportAllTemplates(templatesDir); err != nil {
		return fmt.Errorf("import templates: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Run the integration test**

Run: `go test ./cmd/darken/ -run TestSetup_UploadsAllTemplatesToHub -v`

Expected: PASS. Bootstrap now imports before upload; the stub recognizes the imported roles when push runs.

- [ ] **Step 4: Run the full cmd/darken test suite**

Run: `go test ./cmd/darken/ -count=1`

Expected: all tests PASS. No regressions.

- [ ] **Step 5: Commit**

```bash
git add cmd/darken/bootstrap.go
git commit -m "fix(bootstrap): import templates into scion before Hub push

darken up extracts embedded templates to a tmpdir, stages skills,
then deferred-cleanup removes the dir. Subsequent
uploadAllTemplatesToHub then called scion templates push <role>
with nothing in scion's local store and got a 404. Call
ImportAllTemplates inside the same scope as the extraction so
scion has the bodies before cleanup runs and before push fires.

Fixes 'darken up' in any project that does not have
.scion/templates/ committed at the repo root (e.g. fresh
non-darkish-factory checkouts)."
```

---

## Task 4: Add unit test guarding the wiring (regression guard)

The integration test in Task 1/3 is observable-outcome (push succeeds). Add a unit test that pins the implementation invariant: `ensureAllSkillsStaged` calls `ImportAllTemplates` exactly once with the templatesDir.

**Files:**
- Create: `cmd/darken/bootstrap_test.go`

- [ ] **Step 1: Check whether bootstrap_test.go already exists**

```bash
ls cmd/darken/bootstrap_test.go 2>&1 || echo "missing"
```

If the file exists, the new test goes inside it. If missing, create it. The instructions below assume creation; if it exists, splice the new test in next to other tests that share the package and skip the package declaration.

- [ ] **Step 2: Write the test**

Create `cmd/darken/bootstrap_test.go` with this content (or append the function if the file exists):

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureAllSkillsStaged_ImportsTemplatesForLocalStore asserts that
// ensureAllSkillsStaged calls ImportAllTemplates with the resolved
// templatesDir. This is the regression guard for the darken-up
// template-upload bug: without the import, scion's local store is empty
// when uploadAllTemplatesToHub later calls templates push.
func TestEnsureAllSkillsStaged_ImportsTemplatesForLocalStore(t *testing.T) {
	// Prepare a fake project root with .scion/templates/ so resolveTemplatesDir
	// returns a known, no-cleanup path. resolveTemplatesDir's first preference
	// is repoRoot()/.scion/templates/.
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	templatesDir := filepath.Join(root, ".scion", "templates")
	for _, role := range []string{"admin", "researcher"} {
		dir := filepath.Join(templatesDir, role)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Plant a minimal manifest so stage-skills doesn't barf on missing yaml.
		manifest := filepath.Join(dir, "scion-agent.yaml")
		if err := os.WriteFile(manifest, []byte("schema_version: \"1\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Stub bash on PATH so runSubstrateScript no-ops.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "bash"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := ensureAllSkillsStaged(); err != nil {
		t.Fatalf("ensureAllSkillsStaged: %v", err)
	}

	if got := len(mc.importAllTemplatesCalls); got != 1 {
		t.Fatalf("expected exactly 1 ImportAllTemplates call; got %d: %v",
			got, mc.importAllTemplatesCalls)
	}
	if got := mc.importAllTemplatesCalls[0]; got != templatesDir {
		t.Errorf("ImportAllTemplates called with %q; want %q", got, templatesDir)
	}
}
```

- [ ] **Step 3: Run the new test**

Run: `go test ./cmd/darken/ -run TestEnsureAllSkillsStaged_ImportsTemplatesForLocalStore -v`

Expected: PASS.

- [ ] **Step 4: Run the full cmd/darken test suite once more**

Run: `go test ./cmd/darken/ -count=1`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/darken/bootstrap_test.go
git commit -m "test(bootstrap): pin ImportAllTemplates wiring as regression guard

The integration test in setup_test.go covers the observable outcome
(push succeeds). This unit test pins the implementation invariant:
ensureAllSkillsStaged calls ImportAllTemplates exactly once with the
resolved templatesDir. Catches future refactors that drop or move
the call."
```

---

## Task 5: Document the precondition on `uploadAllTemplatesToHub`

**Files:**
- Modify: `cmd/darken/setup.go:47-58` (the `uploadAllTemplatesToHub` doc comment)

- [ ] **Step 1: Update the comment**

In `cmd/darken/setup.go`, replace the doc comment above `uploadAllTemplatesToHub` (currently lines 47-49):

```go
// uploadAllTemplatesToHub pushes all 14 canonical templates to the Hub at
// user (global) scope via ScionClient.PushTemplate.
// Runs after bootstrap so the scion server is guaranteed to be running.
```

with:

```go
// uploadAllTemplatesToHub pushes all 14 canonical templates to the Hub at
// user (global) scope via ScionClient.PushTemplate.
//
// Precondition: scion's local template store contains every canonical
// role. ensureAllSkillsStaged satisfies this by calling
// ImportAllTemplates during bootstrap. Without that import, push fails
// with "template not found locally" because scion looks up templates by
// name in its own store and refuses to push what it has never seen.
```

- [ ] **Step 2: Verify build still works**

Run: `go build ./cmd/darken/`

Expected: success.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./cmd/darken/ -count=1`

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/darken/setup.go
git commit -m "docs: record the import-before-push precondition

Comment-only change. Makes the implicit dependency between bootstrap
and upload explicit at the call site, so future readers don't reorder
the bootstrap pipeline and re-introduce the silent-failure mode."
```

---

## Self-review notes (post-plan)

Spec coverage check:

- Verification step 0 → done in the spec edit (commit 590766a). The plan reflects Q-A=yes / Q-B=no.
- Branch A design → Tasks 2, 3 (interface method, wiring).
- Test strategy primary (observable outcome) → Tasks 1 + 3 (stub tightening + the existing TestSetup_UploadsAllTemplatesToHub now actually catches the bug).
- Test strategy secondary (Branch A invariant) → Task 4 (unit-level call assertion).
- Test strategy existing-tests preservation → Tasks 2-4 keep the full suite green.
- Error handling → Task 3's `fmt.Errorf("import templates: %w", err)`.
- Backward-compat → Task 3's behavior in darkish-factory: `resolveTemplatesDir` returns the repo-root path, no-op cleanup, `ImportAllTemplates` runs against that path. The repo-root templates re-import to scion's global store every `darken up` — harmless (idempotent, scion overwrites).

No placeholders. Each step shows the exact code or command. Method names (`ImportAllTemplates`, `importAllTemplatesCalls`, `importAllTemplatesErr`) consistent across tasks.
