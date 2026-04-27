# Darken Phase 1 — Substrate Resolver + Rename

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple the `darkish` CLI from CWD-bound substrate access via a resolver chain, AND rename the binary to `darken`. Backward-compatible — project-local `.scion/templates/` continues to work in this repo.

**Architecture:** New `internal/substrate` package with a `Resolver` that layers (1) flag → (2) env → (3) `~/.config/darken/overrides/` → (4) project-local CWD `.scion/templates/` (templates only) → (5) **embedded fallback (added in Phase 2; Phase 1 returns "not found" at this layer)**. Each subcommand reads files via the resolver instead of `filepath.Join(repoRoot, ...)`.

**Tech Stack:** Go 1.23+, stdlib only. New package `internal/substrate`. No dependency changes.

---

## File structure

### Created

- `internal/substrate/resolver.go` — `Resolver` struct + `Open(name string) (fs.File, error)` + `Stat`
- `internal/substrate/resolver_test.go` — covers each precedence layer and miss behavior
- `cmd/darken/` — renamed from `cmd/darkish/`

### Modified

- `Makefile` — `darkish` target → `darken`; build path `./cmd/darken`
- `.gitignore` — `bin/` (already correct, just verify)
- `cmd/darken/{main,bootstrap,spawn,creds,skills,images,apply,doctor,create_harness,list,orchestrate}.go` — add resolver wiring
- `CLAUDE.md` — `darkish` → `darken` references
- `.claude/skills/{orchestrator-mode,subagent-to-subharness}/SKILL.md` — `darkish` → `darken` references
- `images/README.md`, `.design/harness-roster.md`, `.design/pipeline-mechanics.md` — `darkish` → `darken` in prose
- `scripts/bootstrap.sh` — `darkish bootstrap` → `darken bootstrap`
- `cmd/darken/orchestrate.go` — lookup paths use `darken`

### Removed

- `cmd/darkish/` (replaced by `cmd/darken/`)

---

## Tasks

### Task 1: Rename cmd/darkish → cmd/darken (mechanical)

**Files:**
- Move: `cmd/darkish/` → `cmd/darken/`
- Modify: `Makefile`

- [ ] **Step 1: Move the directory**

```bash
git mv cmd/darkish cmd/darken
```

- [ ] **Step 2: Update Makefile target**

Replace the `darkish` target with `darken`:

```makefile
.PHONY: darken
darken:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w" -o bin/darken ./cmd/darken

# Sync host-mode skills from the canonical agent-skills repo into the
# project-local .claude/skills/ tree so Claude Code in this repo
# auto-discovers them. Canonical source: ~/projects/agent-skills.
SKILLS_CANONICAL ?= $(HOME)/projects/agent-skills/skills
HOST_SKILLS      := orchestrator-mode subagent-to-subharness

.PHONY: sync-skills
sync-skills:
	@for s in $(HOST_SKILLS); do \
		mkdir -p .claude/skills/$$s && \
		cp -f $(SKILLS_CANONICAL)/$$s/SKILL.md .claude/skills/$$s/SKILL.md && \
		echo "synced .claude/skills/$$s/SKILL.md ← $(SKILLS_CANONICAL)/$$s/SKILL.md"; \
	done
```

- [ ] **Step 3: Verify build still works**

Run: `make darken && ls -la bin/darken`
Expected: `bin/darken` exists, ~2 MB.

- [ ] **Step 4: Commit**

```
refactor(cli): rename binary from darkish to darken

Avoids conflict with the unix `df` builtin that was the alternative
short name. Repo name stays darkish-factory; only the produced binary
and its CLI surface change.
```

---

### Task 2: Update symlink + docs to `darken`

**Files:**
- Modify: `CLAUDE.md`, `.claude/skills/orchestrator-mode/SKILL.md`, `.claude/skills/subagent-to-subharness/SKILL.md`, `images/README.md`, `.design/harness-roster.md`, `.design/pipeline-mechanics.md`, `scripts/bootstrap.sh`

- [ ] **Step 1: Replace `darkish` invocations with `darken` in all docs and skills**

Replace `bin/darkish` → `bin/darken`, `darkish spawn` → `darken spawn`, etc. across all the listed files. Also replace bare prose `\`darkish\`` with `\`darken\``.

Use `git grep -l '\bdarkish\b'` to find remaining occurrences. Filter out: `darkish-factory` (repo name, KEEP), `darkish-claude/codex/pi/gemini` (image names, KEEP), `darkish-prelude.sh` (filename, KEEP), `local/darkish-` (registry path, KEEP).

- [ ] **Step 2: Update `~/projects/agent-skills/skills/{orchestrator-mode,subagent-to-subharness}/SKILL.md` too**

This is the canonical source. After the project-local copy is updated, run `make sync-skills` to verify the project copy matches. Then commit the canonical update in a separate agent-skills branch.

(For Phase 1 PR: only the project-local copies need to be updated. The canonical update can land in a parallel agent-skills PR.)

- [ ] **Step 3: Update scripts/bootstrap.sh**

Replace:
```bash
if [[ -x "${ROOT}/bin/darkish" ]]; then
  exec "${ROOT}/bin/darkish" bootstrap "$@"
fi
if command -v darkish >/dev/null 2>&1; then
  exec darkish bootstrap "$@"
fi
echo "bootstrap: bin/darkish not built; run 'make darkish' first" >&2
```

With:
```bash
if [[ -x "${ROOT}/bin/darken" ]]; then
  exec "${ROOT}/bin/darken" bootstrap "$@"
fi
if command -v darken >/dev/null 2>&1; then
  exec darken bootstrap "$@"
fi
echo "bootstrap: bin/darken not built; run 'make darken' first" >&2
```

- [ ] **Step 4: Run `bash scripts/test-bootstrap-wrapper.sh`**

Expected: PASS. The test grep'd `darkish bootstrap`; update the test to look for `darken bootstrap`. File: `scripts/test-bootstrap-wrapper.sh`.

- [ ] **Step 5: Verify nothing important still references `darkish` as a CLI invocation**

```bash
git grep -nE '\bdarkish\b(\s|$)' | grep -v "darkish-factory" | grep -v "darkish-claude\|darkish-codex\|darkish-pi\|darkish-gemini\|darkish-prelude" | grep -v "local/darkish-"
```

Expected: empty (or only false-positive matches in audit-log examples).

- [ ] **Step 6: Commit**

```
refactor(docs): rename darkish CLI invocations to darken

Updates CLAUDE.md, the two skills (project copies), .design/* docs,
images/README.md, scripts/bootstrap.sh, and the bootstrap-wrapper
test. Keeps darkish-factory (repo name), darkish-{claude,codex,pi,
gemini} (image names), and darkish-prelude.sh (filename) unchanged.
```

---

### Task 3: Add internal/substrate package

**Files:**
- Create: `internal/substrate/resolver.go`
- Create: `internal/substrate/resolver_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/substrate/resolver_test.go`:

```go
package substrate

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestResolver_FlagOverrideWins(t *testing.T) {
	tmp := t.TempDir()
	flagDir := filepath.Join(tmp, "flag")
	envDir := filepath.Join(tmp, "env")
	userDir := filepath.Join(tmp, "user")

	for _, d := range []string{flagDir, envDir, userDir} {
		os.MkdirAll(filepath.Join(d, ".scion", "templates", "researcher"), 0o755)
	}
	os.WriteFile(filepath.Join(flagDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("FLAG"), 0o644)
	os.WriteFile(filepath.Join(envDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("ENV"), 0o644)
	os.WriteFile(filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("USER"), 0o644)

	t.Setenv("DARKEN_SUBSTRATE_OVERRIDES", envDir)

	r := New(Config{
		FlagOverride:    flagDir,
		UserOverrideDir: userDir,
		ProjectRoot:     "",
	})

	body, err := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "FLAG" {
		t.Fatalf("expected FLAG to win, got %q", string(body))
	}
}

func TestResolver_EnvBeatsUserBeatsProject(t *testing.T) {
	tmp := t.TempDir()
	envDir := filepath.Join(tmp, "env")
	userDir := filepath.Join(tmp, "user")
	projectDir := filepath.Join(tmp, "project")

	for _, d := range []string{envDir, userDir, projectDir} {
		os.MkdirAll(filepath.Join(d, ".scion", "templates", "researcher"), 0o755)
	}
	os.WriteFile(filepath.Join(envDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("ENV"), 0o644)
	os.WriteFile(filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("USER"), 0o644)
	os.WriteFile(filepath.Join(projectDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("PROJECT"), 0o644)

	t.Setenv("DARKEN_SUBSTRATE_OVERRIDES", envDir)

	r := New(Config{
		UserOverrideDir: userDir,
		ProjectRoot:     projectDir,
	})

	body, _ := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if string(body) != "ENV" {
		t.Fatalf("expected ENV to beat user/project, got %q", string(body))
	}
}

func TestResolver_ProjectOnlyForTemplates(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	os.MkdirAll(filepath.Join(projectDir, "scripts"), 0o755)
	os.WriteFile(filepath.Join(projectDir, "scripts", "stage-creds.sh"), []byte("PROJECT"), 0o644)

	r := New(Config{ProjectRoot: projectDir})

	// Non-template files do NOT resolve from project root.
	_, err := r.ReadFile("scripts/stage-creds.sh")
	if err == nil {
		t.Fatalf("expected miss for scripts/* in project root (templates-only), got hit")
	}
}

func TestResolver_MissesAreErrFsExtMissing(t *testing.T) {
	r := New(Config{})
	_, err := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if err == nil {
		t.Fatal("expected error on miss")
	}
	// Don't pin the exact error type yet; Phase 2 adds embedded fallback that
	// should always succeed for embedded paths.
}

func TestResolver_OpenAndStat(t *testing.T) {
	tmp := t.TempDir()
	userDir := filepath.Join(tmp, "user")
	os.MkdirAll(filepath.Join(userDir, ".scion", "templates", "researcher"), 0o755)
	os.WriteFile(filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("USER"), 0o644)

	r := New(Config{UserOverrideDir: userDir})
	f, err := r.Open(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	body, _ := io.ReadAll(f)
	if string(body) != "USER" {
		t.Fatalf("Open returned %q", string(body))
	}

	info, err := r.Stat(".scion/templates/researcher/scion-agent.yaml")
	if err != nil || info.Size() != 4 {
		t.Fatalf("Stat returned size=%d err=%v", info.Size(), err)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
cd internal/substrate && go test ./... 2>&1 | head -20
```

Expected: package not found / undefined: `New`, `Config`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/substrate/resolver.go`:

```go
// Package substrate resolves substrate files (harness templates, stage
// scripts, Dockerfiles, orchestrator-side skills) through a layered
// override chain. Used by every darken subcommand that needs to read
// substrate state.
//
// Resolution order (first match wins):
//
//   1. Config.FlagOverride        (--substrate-overrides)
//   2. $DARKEN_SUBSTRATE_OVERRIDES (Config.envDir, set in New)
//   3. Config.UserOverrideDir     (~/.config/darken/overrides/)
//   4. Config.ProjectRoot         (CWD; templates only — see comment)
//   5. embedded                   (added in Phase 2; Phase 1 fails through)
//
// Layer 4 is special: only paths starting with ".scion/templates/"
// resolve here. This lets a working repo version-control its own role
// overrides without polluting other override scopes.
package substrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config configures a Resolver. All fields are optional; unset layers
// are skipped during resolution.
type Config struct {
	FlagOverride    string // --substrate-overrides flag value
	UserOverrideDir string // typically ~/.config/darken/overrides/
	ProjectRoot     string // typically CWD (only used for .scion/templates/* paths)
}

// Resolver resolves substrate-relative paths to filesystem files via
// the layered chain documented on the package.
type Resolver struct {
	flagDir    string
	envDir     string
	userDir    string
	projectDir string
}

// New builds a Resolver from the given Config. The DARKEN_SUBSTRATE_OVERRIDES
// env var is captured at construction time.
func New(cfg Config) *Resolver {
	return &Resolver{
		flagDir:    cfg.FlagOverride,
		envDir:     os.Getenv("DARKEN_SUBSTRATE_OVERRIDES"),
		userDir:    cfg.UserOverrideDir,
		projectDir: cfg.ProjectRoot,
	}
}

// ReadFile resolves name through the chain and returns the file contents.
func (r *Resolver) ReadFile(name string) ([]byte, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// Open resolves name and returns an open file handle.
func (r *Resolver) Open(name string) (fs.File, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// Stat resolves name and returns its FileInfo.
func (r *Resolver) Stat(name string) (fs.FileInfo, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

// Lookup reports the absolute path that would be resolved, plus the
// layer name that hit. Returns ("", "", err) on miss. Used by `darken
// doctor` to surface which layer served each role.
func (r *Resolver) Lookup(name string) (path, layer string, err error) {
	candidates := r.candidates(name)
	for _, c := range candidates {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, c.layer, nil
		}
	}
	return "", "", fmt.Errorf("substrate: %s not found in any override layer", name)
}

func (r *Resolver) resolve(name string) (string, error) {
	for _, c := range r.candidates(name) {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, nil
		}
	}
	return "", &MissError{Name: name}
}

type candidate struct {
	path  string
	layer string
}

func (r *Resolver) candidates(name string) []candidate {
	var out []candidate
	if r.flagDir != "" {
		out = append(out, candidate{filepath.Join(r.flagDir, name), "flag"})
	}
	if r.envDir != "" {
		out = append(out, candidate{filepath.Join(r.envDir, name), "env"})
	}
	if r.userDir != "" {
		out = append(out, candidate{filepath.Join(r.userDir, name), "user"})
	}
	if r.projectDir != "" && strings.HasPrefix(name, ".scion/templates/") {
		out = append(out, candidate{filepath.Join(r.projectDir, name), "project"})
	}
	return out
}

// MissError indicates the resolver could not find a file in any layer.
// Phase 2 will catch this and fall through to the embedded layer.
type MissError struct {
	Name string
}

func (e *MissError) Error() string {
	return fmt.Sprintf("substrate: %s not found in any override layer", e.Name)
}

// IsMiss reports whether err is a MissError (helpful for Phase 2's
// embedded fallback).
func IsMiss(err error) bool {
	var m *MissError
	return errors.As(err, &m)
}
```

- [ ] **Step 4: Run tests, verify they pass**

```bash
cd internal/substrate && go test ./... -count=1
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Lint**

```bash
go vet ./internal/substrate/...
gofmt -l internal/
```

Expected: no output.

- [ ] **Step 6: Commit**

```
feat(substrate): add layered resolver for substrate file lookup

internal/substrate.Resolver picks the highest-priority layer where a
substrate-relative path exists: flag → env → user-overrides → project-
local (templates only) → miss. Phase 2 adds an embedded fallback.

5 unit tests cover precedence + the templates-only restriction on the
project layer.
```

---

### Task 4: Wire resolver into `darken doctor`

**Files:**
- Modify: `cmd/darken/main.go` (parse global `--substrate-overrides` flag)
- Modify: `cmd/darken/doctor.go` (use resolver; report layer per role)
- Test: `cmd/darken/doctor_test.go`

- [ ] **Step 1: Add a global `--substrate-overrides` flag in main.go**

Replace the early `--help` check + `flag.Parse` block with a flagset that captures `--substrate-overrides`, then dispatches positional args:

```go
var globalFlags struct {
	substrateOverrides string
}

func main() {
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" || a == "help" {
			printUsage()
			os.Exit(0)
		}
	}

	fs := flag.NewFlagSet("darken", flag.ContinueOnError)
	fs.StringVar(&globalFlags.substrateOverrides, "substrate-overrides", "", "path to substrate override directory (overrides $DARKEN_SUBSTRATE_OVERRIDES and ~/.config/darken/overrides/)")
	fs.Usage = printUsage
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	args := fs.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(2)
	}
	// ... existing dispatch loop unchanged
}

// substrate builds a *substrate.Resolver from globalFlags + the user's
// home dir + the project root (best-effort).
func substrateResolver() *substrate.Resolver {
	cfg := substrate.Config{
		FlagOverride: globalFlags.substrateOverrides,
	}
	if home, err := os.UserHomeDir(); err == nil {
		cfg.UserOverrideDir = filepath.Join(home, ".config", "darken", "overrides")
	}
	if root, err := repoRoot(); err == nil {
		cfg.ProjectRoot = root
	}
	return substrate.New(cfg)
}
```

(Add the import for `github.com/danmestas/darken/internal/substrate` and `path/filepath`.)

- [ ] **Step 2: Wire `darken doctor` to use the resolver for per-harness preflight**

Replace `doctorHarness`'s `os.ReadFile(manifestPath)` with `substrateResolver().ReadFile(".scion/templates/" + name + "/scion-agent.yaml")`. Update the error path: `substrate.IsMiss(err)` → "no template defined for harness X (override or embed missing)".

Add to the OK report: which layer served the manifest. Use `substrateResolver().Lookup(...)`.

- [ ] **Step 3: Update doctor_test.go for the new resolver-based read**

The existing test plants a manifest at `<tmp>/.scion/templates/sme/scion-agent.yaml` and sets `DARKISH_REPO_ROOT=<tmp>`. Update:
- Rename env var `DARKISH_REPO_ROOT` to `DARKEN_REPO_ROOT` (Task 5).
- Confirm the project-layer reads via the resolver still work for templates.

- [ ] **Step 4: Tests**

```bash
go test ./cmd/darken/... -count=1
```

Expected: all tests PASS (doctor + main + spawn + bootstrap + apply + create-harness + skills + creds + images + list + orchestrate).

- [ ] **Step 5: Commit**

```
feat(cli): wire substrate.Resolver into darken doctor

darken now accepts a global --substrate-overrides flag. doctor's
per-harness preflight reads manifests through the resolver (project
layer only for Phase 1) and reports which layer served the manifest.
```

---

### Task 5: Rename env vars `DARKISH_*` → `DARKEN_*`

**Files:**
- Modify: `cmd/darken/repoinfo.go`, `cmd/darken/doctor_test.go`, `cmd/darken/create_harness_test.go`, any other test files referencing `DARKISH_REPO_ROOT`

- [ ] **Step 1: Search**

```bash
git grep -nE 'DARKISH_(REPO_ROOT|SUBSTRATE)'
```

- [ ] **Step 2: Replace `DARKISH_REPO_ROOT` → `DARKEN_REPO_ROOT` in all hits**

Edit each file. The only env var actually used today is `DARKISH_REPO_ROOT` in `repoinfo.go`'s `repoRoot()`.

- [ ] **Step 3: Verify tests still pass**

```bash
go test ./cmd/darken/... -count=1
```

- [ ] **Step 4: Commit**

```
refactor(cli): rename env var DARKISH_REPO_ROOT → DARKEN_REPO_ROOT

Aligns with the binary rename. No functional change; tests updated to
use the new env var name.
```

---

### Task 6: Wire resolver into stage scripts and template-reading subcommands

**Files:**
- Modify: `cmd/darken/spawn.go` — uses `runShell(stage-creds.sh ...)` from project root; will switch to substrate resolver in Phase 2 when scripts are embedded
- Modify: `cmd/darken/bootstrap.go` — same; pre-Phase-2 it stays project-rooted
- Modify: `cmd/darken/create_harness.go` — reads template stub paths via resolver (writes still go to user-or-project location based on `--scope`)
- Modify: `cmd/darken/apply.go` — reads `.scion/templates/<harness>/scion-agent.yaml` via resolver for model_swap; reads `.scion/templates/<harness>/system-prompt.md` for prompt_edit

The Phase 1 scope here is conservative: any path that reads a TEMPLATE goes through the resolver (project layer only at this stage). Anything that reads a SCRIPT or DOCKERFILE stays project-rooted (Phase 2 embeds those + adds the embedded layer).

- [ ] **Step 1: apply.go template reads**

Replace in `swapModel`:
```go
manifest := filepath.Join(root, ".scion", "templates", harness, "scion-agent.yaml")
body, err := os.ReadFile(manifest)
```
with:
```go
body, err := substrateResolver().ReadFile(".scion/templates/" + harness + "/scion-agent.yaml")
```

(Note: the WRITE in `swapModel` still goes to `filepath.Join(root, ".scion", "templates", ...)` — model swaps mutate the project-local manifest, not the embedded substrate. Add a comment.)

Same pattern in `editPrompt` for system-prompt.md.

- [ ] **Step 2: create-harness `--scope` flag**

Currently writes to `<repoRoot>/.scion/templates/<role>/`. Add `--scope=user|project`, default `user`:

```go
scope := fs.String("scope", "user", "where to scaffold: user|project")
// ...
var dir string
switch *scope {
case "user":
    home, err := os.UserHomeDir()
    if err != nil {
        return err
    }
    dir = filepath.Join(home, ".config", "darken", "overrides", ".scion", "templates", role)
case "project":
    root, err := repoRoot()
    if err != nil {
        return err
    }
    dir = filepath.Join(root, ".scion", "templates", role)
default:
    return fmt.Errorf("--scope must be user|project, got %q", *scope)
}
```

(The roster row insertion at `<repoRoot>/.design/harness-roster.md` always happens at project scope — operators document their roster in their working repo regardless of where the manifest lives.)

- [ ] **Step 3: Tests**

Update `create_harness_test.go` to test both scopes. Add the `--scope=project` case explicitly; keep one test for `--scope=user`.

- [ ] **Step 4: Run full suite**

```bash
go test ./... -count=1
go vet ./...
gofmt -l cmd/ internal/
```

Expected: all clean.

- [ ] **Step 5: Commit**

```
feat(cli): wire resolver into apply + create-harness

- apply.go reads templates through substrate.Resolver (project layer
  only for now). Writes still target project-local manifests since
  model_swap and prompt_edit mutate the working repo's substrate.
- create-harness gets --scope=user|project. Default user → writes to
  ~/.config/darken/overrides/.scion/templates/<role>/. Project keeps
  the existing behavior.

Phase 2 will add the embedded substrate layer; the resolver's
fall-through behavior is the seam.
```

---

### Task 7: Final verification

- [ ] **Step 1: Full test suite**

```bash
go test ./... -count=1
```

Expected: all green. Test count should be the same as before plus 5 new resolver tests.

- [ ] **Step 2: Sanity check the binary**

```bash
make darken
bin/darken --help
bin/darken version  # not added in Phase 1; should just print usage
bin/darken doctor   # may FAIL on missing creds/images locally; that's fine, just verify it runs
```

- [ ] **Step 3: Update the symlink**

```bash
ln -sf "$PWD/bin/darken" ~/.local/bin/darken
which darken && darken --help | head -3
```

(Old `~/.local/bin/darkish` symlink can stay as a transition aid for one PR cycle; remove in Phase 4 dogfooding.)

- [ ] **Step 4: Commit any final cleanup**

If symlinks need updating in scripts or docs: a cleanup commit. Otherwise skip.

---

## Done definition

Phase 1 ships when:

1. All tests pass: `go test ./... -count=1`
2. `bin/darken --help` works and shows all subcommands
3. `bin/darken doctor sme` (or any harness) reads its template through the resolver and reports the layer in output
4. `~/.config/darken/overrides/.scion/templates/<role>/scion-agent.yaml` (if present) overrides the project-local copy
5. Project-local `.scion/templates/<role>/` still wins over absent user overrides (backward-compat)
6. `bin/darkish` symlink can be deleted with no functional impact (the operator may still want it as a transition)
7. PR open, CI green, ready for review

## What Phase 2 picks up

- `//go:embed` of templates, scripts, Dockerfiles, the two skills
- Resolver layer 5 (embedded) — the fall-through case for `MissError`
- `darken init` subcommand
- `darken version` subcommand reporting embedded substrate hash
