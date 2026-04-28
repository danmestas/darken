# Darken `uninstall-init` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `darken uninstall-init` — symmetric counterpart to `darken init` that removes the project scaffolds init wrote, preserving operator customizations and `.scion/` runtime state. Refactor init's scaffold list into a shared `initArtifacts` helper so init and uninstall share one source of truth.

**Architecture:** Three new files (`artifacts.go`, `manifest.go`, `uninstall_init.go`) and an in-place refactor of `init.go`. The `artifact` type carries a `Body()` closure returning the bytes init would write. `runInit` iterates the list, writes each artifact, and persists `.scion/init-manifest.json` with each artifact's path + SHA-256. `runUninstallInit` reads the manifest (or falls back to comparing against `Body()` if absent), prints a manifest-then-prompt, then removes pristine entries. Six new tests + two updated init tests.

**Tech Stack:** Go 1.23+, stdlib only (`crypto/sha256`, `encoding/json`, `os`, `path/filepath`, `strings`, `bufio`). No new dependencies.

**Precondition:** Phase 9 is shipped (v0.1.10 in the tap). Branch `feat/darken-uninstall-init` already exists with the spec at `docs/superpowers/specs/2026-04-28-darken-uninstall-init-design.md` and this plan committed.

---

## File structure

### Created
- `cmd/darken/artifacts.go` — `artifact` type + `initArtifacts(target string) []artifact` (single source of truth for what init writes)
- `cmd/darken/artifacts_test.go` — covers each artifact's `Body()` returns the same bytes the existing scaffold helpers produce
- `cmd/darken/manifest.go` — `initManifest` struct + `writeInitManifest`, `readInitManifest` helpers (`.scion/init-manifest.json` persistence)
- `cmd/darken/manifest_test.go` — write-then-read round-trip + missing-file returns nil-manifest-no-error
- `cmd/darken/uninstall_init.go` — `runUninstallInit` (the new subcommand)
- `cmd/darken/uninstall_init_test.go` — six tests from spec §Testing

### Modified
- `cmd/darken/init.go` — `runInit` rewritten to iterate `initArtifacts(target)`; calls `writeInitManifest` after artifacts are written. Existing helpers (`scaffoldSkill`, `scaffoldStatusLine`, `scaffoldGitignore`, `renderCLAUDE`) deleted (their bodies move into `Body()` closures in `artifacts.go`).
- `cmd/darken/init_test.go` — adds `TestInit_WritesInitManifest` and `TestUninstallInit_FallbackWhenManifestMissing` (the latter test fits init_test.go because it exercises both); existing init tests should keep passing.
- `cmd/darken/main.go` — register `uninstall-init` subcommand after `upgrade-init`

### NOT modified
- `internal/substrate/*` — no substrate changes
- `cmd/darken/upgrade_init.go` — composition with init/doctor still valid; init's behavior is unchanged externally
- `cmd/darken/doctor.go` — drift check still compares against embedded `Body()`-equivalent; no change needed

---

## Disposition logic (binding for Task 4)

For each artifact in `initArtifacts(root)`:

| Kind | Manifest present | Manifest absent |
|---|---|---|
| `file` | `sha256(projectBytes) == manifest.sha256` → PRISTINE; else CUSTOMIZED | `bytes.Equal(projectBytes, Body())` → PRISTINE; else CUSTOMIZED |
| `gitignore-lines` | All 7 lines present (line-by-line `strings.TrimSpace` match) → PRISTINE; any missing → CUSTOMIZED | Same as left (manifest doesn't help here) |

If the project file is missing entirely → MISSING (no-op for that artifact).

For `gitignore-lines`, the action on `--force` or PRISTINE is: read the file, drop the 7 matching lines, atomic-write via temp + rename.

---

## Tasks

### Task 1: Add `artifact` type and `initArtifacts(target)` helper

**Files:**
- Create: `cmd/darken/artifacts.go`
- Create: `cmd/darken/artifacts_test.go`

**Why:** Single source of truth for what `darken init` scaffolds. Both `runInit` (writer) and `runUninstallInit` (reader) consume this list. No drift possible by design.

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/artifacts_test.go`:

```go
package main

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danmestas/darken/internal/substrate"
)

func TestInitArtifacts_ListIsCompleteAndStable(t *testing.T) {
	target := t.TempDir()
	arts := initArtifacts(target)

	// We expect exactly 5 artifacts (4 files + 1 gitignore-lines).
	if len(arts) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(arts))
	}

	// Confirm the relative paths cover what init writes.
	wantPaths := map[string]string{
		"CLAUDE.md":                                       "file",
		".claude/skills/orchestrator-mode/SKILL.md":       "file",
		".claude/skills/subagent-to-subharness/SKILL.md":  "file",
		".claude/settings.local.json":                     "file",
		".gitignore":                                      "gitignore-lines",
	}
	for _, art := range arts {
		wantKind, ok := wantPaths[art.RelPath]
		if !ok {
			t.Errorf("unexpected artifact: %q", art.RelPath)
			continue
		}
		if art.Kind != wantKind {
			t.Errorf("artifact %q: expected kind %q, got %q", art.RelPath, wantKind, art.Kind)
		}
		delete(wantPaths, art.RelPath)
	}
	for missing := range wantPaths {
		t.Errorf("missing artifact: %q", missing)
	}
}

func TestInitArtifacts_BodyMatchesEmbeddedSkill(t *testing.T) {
	target := t.TempDir()
	arts := initArtifacts(target)

	// orchestrator-mode SKILL.md body must match the embedded copy byte-for-byte.
	var found bool
	for _, art := range arts {
		if art.RelPath != ".claude/skills/orchestrator-mode/SKILL.md" {
			continue
		}
		got, err := art.Body()
		if err != nil {
			t.Fatalf("Body() error: %v", err)
		}
		want, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
		if err != nil {
			t.Fatalf("read embedded: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("orchestrator-mode SKILL.md Body() differs from embedded:\nwant len=%d\ngot len=%d", len(want), len(got))
		}
		found = true
	}
	if !found {
		t.Fatal("orchestrator-mode artifact not in list")
	}
}

func TestInitArtifacts_CLAUDEBodyTemplatedWithRepoName(t *testing.T) {
	target := filepath.Join(t.TempDir(), "myproject")
	if err := mkdirIfMissing(t, target); err != nil {
		t.Fatal(err)
	}
	arts := initArtifacts(target)

	for _, art := range arts {
		if art.RelPath != "CLAUDE.md" {
			continue
		}
		body, err := art.Body()
		if err != nil {
			t.Fatalf("Body() error: %v", err)
		}
		s := string(body)
		// Repo basename must appear (template substitutes RepoName).
		if !strings.Contains(s, "myproject") {
			t.Fatalf("CLAUDE.md should reference target basename 'myproject', got:\n%s", s[:min(200, len(s))])
		}
		// Substrate hash prefix must appear.
		if !strings.Contains(s, substrate.EmbeddedHash()[:12]) {
			t.Fatalf("CLAUDE.md should contain first 12 chars of embedded hash, got:\n%s", s[:min(200, len(s))])
		}
		return
	}
	t.Fatal("CLAUDE.md artifact not in list")
}

func TestInitArtifacts_GitignoreLinesContainsExpectedSet(t *testing.T) {
	arts := initArtifacts(t.TempDir())
	for _, art := range arts {
		if art.RelPath != ".gitignore" {
			continue
		}
		body, err := art.Body()
		if err != nil {
			t.Fatalf("Body() error: %v", err)
		}
		s := string(body)
		// Confirm key lines are present in the canonical gitignore body.
		for _, want := range []string{
			".scion/agents/",
			".scion/skills-staging/",
			".scion/audit.jsonl",
			".claude/worktrees/",
			".claude/settings.local.json",
			".superpowers/",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("gitignore Body() missing %q:\n%s", want, s)
			}
		}
		return
	}
	t.Fatal(".gitignore artifact not in list")
}

// mkdirIfMissing is a tiny helper (defined in this test file) that
// makes the target dir if absent. Used by the CLAUDE.md test which
// passes a non-existent subpath of t.TempDir().
func mkdirIfMissing(t *testing.T, dir string) error {
	t.Helper()
	return osMkdirAll(dir, 0o755)
}
```

(Note: `osMkdirAll` and `min` are stdlib calls; if the file doesn't already import `os` and Go ≥1.21 isn't assumed, replace `osMkdirAll` with `os.MkdirAll` and add `os` to imports. Keep the test file imports minimal — only `io/fs`, `os`, `path/filepath`, `strings`, `testing`, plus the substrate package.)

- [ ] **Step 2: Run tests, verify they fail**

```bash
cd /Users/dmestas/projects/darkish-factory
go test -run TestInitArtifacts ./cmd/darken/... -count=1
```

Expected: undefined `initArtifacts` (or `artifact` type).

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/artifacts.go`:

```go
// Package main — artifact list for `darken init`.
//
// initArtifacts is the single source of truth for what init scaffolds.
// Both runInit (writer) and runUninstallInit (reader) consume this
// list. Adding a new scaffold means appending one entry here; both
// commands pick it up automatically.
package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/danmestas/darken/internal/substrate"
)

// artifact describes one file or file-region that `darken init` writes
// into a target directory.
type artifact struct {
	// RelPath is the artifact's path relative to the init target dir.
	RelPath string
	// Kind is "file" (whole-file owned) or "gitignore-lines" (line-set
	// appended into a possibly-shared file).
	Kind string
	// Body returns the bytes init would write at the current binary's
	// substrate version. For "gitignore-lines" the bytes are the
	// newline-joined set of lines including the leading comment.
	Body func() ([]byte, error)
}

// gitignoreLines is the canonical line set that init appends to the
// project's .gitignore. Stored as a slice so uninstall can also iterate
// for line-presence checks. The leading comment is the first entry.
var gitignoreLines = []string{
	"# darken: scion runtime + per-spawn worktrees + claude-code worktrees",
	".scion/agents/",
	".scion/skills-staging/",
	".scion/audit.jsonl",
	".claude/worktrees/",
	".claude/settings.local.json",
	".superpowers/",
}

// settingsLocalJSON is the body init writes to .claude/settings.local.json.
const settingsLocalJSON = `{
  "statusLine": {
    "command": "darken status",
    "type": "command"
  }
}
`

// initArtifacts returns the artifact list keyed to the target directory.
// Order is stable across calls and across runs.
func initArtifacts(targetDir string) []artifact {
	return []artifact{
		{
			RelPath: "CLAUDE.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return claudeMdBody(targetDir) },
		},
		{
			RelPath: ".claude/skills/orchestrator-mode/SKILL.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return embeddedSkillBody("orchestrator-mode") },
		},
		{
			RelPath: ".claude/skills/subagent-to-subharness/SKILL.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return embeddedSkillBody("subagent-to-subharness") },
		},
		{
			RelPath: ".claude/settings.local.json",
			Kind:    "file",
			Body:    func() ([]byte, error) { return []byte(settingsLocalJSON), nil },
		},
		{
			RelPath: ".gitignore",
			Kind:    "gitignore-lines",
			Body:    func() ([]byte, error) { return []byte(strings.Join(gitignoreLines, "\n") + "\n"), nil },
		},
	}
}

// claudeMdBody renders the embedded CLAUDE.md.tmpl with the target
// dir's basename as RepoName and the first 12 chars of the embedded
// substrate hash as SubstrateHash12. Replaces the deleted renderCLAUDE.
func claudeMdBody(targetDir string) ([]byte, error) {
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/templates/CLAUDE.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("embedded CLAUDE.md.tmpl: %w", err)
	}
	tmpl, err := template.New("claude").Parse(string(body))
	if err != nil {
		return nil, err
	}
	data := struct {
		RepoName        string
		SubstrateHash12 string
	}{
		RepoName:        filepath.Base(targetDir),
		SubstrateHash12: firstN(substrate.EmbeddedHash(), 12),
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

// embeddedSkillBody returns the embedded SKILL.md body for the given
// skill name. Replaces the per-skill read in scaffoldSkill.
func embeddedSkillBody(name string) ([]byte, error) {
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/"+name+"/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("embedded skill %s: %w", name, err)
	}
	return body, nil
}
```

(`firstN` already exists in `init.go`. After Task 2's refactor, `firstN` may move to `artifacts.go`; for now, leave it where it is and reference it.)

- [ ] **Step 4: Run tests, verify they pass**

```bash
go test -run TestInitArtifacts ./cmd/darken/... -count=1
```

Expected: 4/4 PASS.

- [ ] **Step 5: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/artifacts.go cmd/darken/artifacts_test.go
git commit -m "feat(init): factor artifact list into initArtifacts helper

New cmd/darken/artifacts.go defines the artifact type and an
initArtifacts(target) helper that returns the list of scaffolds
darken init writes. Both runInit (writer, refactored next) and
runUninstallInit (reader, new) consume this list, eliminating the
change-amplification risk of duplicating init's knowledge.

Adds 4 tests covering: artifact list completeness, embedded skill
body equality, CLAUDE.md templating, gitignore canonical lines."
```

---

### Task 2: Refactor `runInit` to consume `initArtifacts`

**Files:**
- Modify: `cmd/darken/init.go` — replace inline scaffold logic with a single iteration over `initArtifacts(target)`. Delete `scaffoldSkill`, `scaffoldStatusLine`, `scaffoldGitignore`, `renderCLAUDE`, `readEmbeddedTemplate` helpers (their behavior moves into `Body()` closures from Task 1).

**Why:** Strategic programming. Without this refactor, init.go and uninstall_init.go would each carry their own copy of the scaffold list. The "shared abstraction" only works if init actually consumes it.

- [ ] **Step 1: Confirm existing init tests** (regression baseline)

```bash
go test -run TestInit ./cmd/darken/... -count=1
```

Expected: green. We'll re-run after the refactor and expect the same green.

- [ ] **Step 2: Rewrite `runInit`**

Replace the body of `runInit` in `cmd/darken/init.go` (the function currently spans roughly lines 16–125):

```go
func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "print actions without executing")
	force := flags.Bool("force", false, "overwrite existing CLAUDE.md")
	refresh := flags.Bool("refresh", false, "re-scaffold skills/statusLine/.gitignore without clobbering CLAUDE.md (use --force with --refresh to also regenerate CLAUDE.md)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if err := verifyInitPrereqs(); err != nil {
		return err
	}

	pos := flags.Args()
	target := "."
	if len(pos) > 0 {
		target = pos[0]
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("target dir does not exist: %s", target)
	}

	arts := initArtifacts(target)

	// Decision: should we (re)write CLAUDE.md?
	claudePath := filepath.Join(target, "CLAUDE.md")
	_, claudeExists := statResult(claudePath)
	var writeCLAUDE bool
	switch {
	case *refresh && *force:
		writeCLAUDE = true
	case *refresh:
		writeCLAUDE = false
	case *force:
		writeCLAUDE = true
	case !claudeExists:
		writeCLAUDE = true
	default:
		writeCLAUDE = false
	}

	if *dryRun {
		for _, art := range arts {
			if art.RelPath == "CLAUDE.md" && !writeCLAUDE {
				fmt.Printf("would skip %s (already exists; use --force to overwrite)\n", filepath.Join(target, art.RelPath))
				continue
			}
			fmt.Printf("would write %s\n", filepath.Join(target, art.RelPath))
		}
		return nil
	}

	// Write each artifact.
	for _, art := range arts {
		if err := writeArtifact(target, art, writeCLAUDE, *refresh); err != nil {
			fmt.Fprintf(os.Stderr, "init: %s: %v\n", art.RelPath, err)
		}
	}

	// Persist the manifest after all artifacts are written.
	// (Task 3 implements writeInitManifest; this call is the seam.)
	if err := writeInitManifest(target, arts); err != nil {
		fmt.Fprintf(os.Stderr, "init: manifest write failed: %v\n", err)
	}

	// bones init (unchanged)
	if err := runBonesInit(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: bones init failed: %v\n", err)
	} else if _, err := exec.LookPath("bones"); err == nil {
		fmt.Println("ran `bones init` for workspace bootstrap")
	}

	return nil
}

// writeArtifact dispatches on art.Kind to write a file or append the
// gitignore-lines block. Idempotent for gitignore-lines (skips lines
// already present).
func writeArtifact(target string, art artifact, writeCLAUDE, refresh bool) error {
	dst := filepath.Join(target, art.RelPath)
	switch art.Kind {
	case "file":
		if art.RelPath == "CLAUDE.md" {
			if !writeCLAUDE {
				if _, exists := statResult(dst); exists {
					if refresh {
						fmt.Printf("preserved %s (use --refresh --force to regenerate)\n", dst)
					} else {
						fmt.Printf("skipped %s (already exists; use --force to overwrite)\n", dst)
					}
				}
				return nil
			}
			body, err := art.Body()
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, body, 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", dst)
			return nil
		}
		if art.RelPath == ".claude/settings.local.json" {
			// Don't clobber existing settings (operator may have added other keys).
			if _, exists := statResult(dst); exists {
				return nil
			}
		}
		body, err := art.Body()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return err
		}
		fmt.Printf("scaffolded %s\n", art.RelPath)
		return nil

	case "gitignore-lines":
		// Append only lines not already present (idempotent).
		var existing []byte
		if b, err := os.ReadFile(dst); err == nil {
			existing = b
		}
		var add []string
		for _, line := range gitignoreLines {
			if !strings.Contains(string(existing), line) {
				add = append(add, line)
			}
		}
		if len(add) == 0 {
			return nil
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			f.WriteString("\n")
		}
		for _, line := range add {
			f.WriteString(line + "\n")
		}
		fmt.Println("appended darken entries to .gitignore")
		return nil

	default:
		return fmt.Errorf("unknown artifact kind: %s", art.Kind)
	}
}

// statResult is a tiny helper: reports (info, exists) without an error
// for the caller to handle. Existence is the only signal we need at
// these call sites.
func statResult(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}
```

Then DELETE the now-unused helpers from init.go:
- `renderCLAUDE` (lines ~127–151) — replaced by `claudeMdBody` in artifacts.go
- `readEmbeddedTemplate` (lines ~153–159) — replaced by inline `fs.ReadFile` in `claudeMdBody`
- `scaffoldSkill` (lines ~169–182) — replaced by `embeddedSkillBody` + `writeArtifact`
- `scaffoldStatusLine` (lines ~184–203) — replaced by `writeArtifact` for `.claude/settings.local.json`
- `scaffoldGitignore` (lines ~205–243) — replaced by `writeArtifact` for `.gitignore`
- `firstN` (lines ~161–167) — MOVE to artifacts.go (used by `claudeMdBody`)

Keep:
- `runBonesInit` (still used)

After the changes, `init.go` should consist of only: `runInit`, `writeArtifact`, `statResult`, `runBonesInit`, and the imports.

- [ ] **Step 3: Add `firstN` to `artifacts.go`** (move from init.go)

In `cmd/darken/artifacts.go`, append below `embeddedSkillBody`:

```go
// firstN returns the first n characters of s, or all of s if shorter.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
```

And remove it from `init.go`.

- [ ] **Step 4: Run init tests, verify they still pass**

```bash
go test -run TestInit ./cmd/darken/... -count=1
```

Expected: green. The refactor is behavior-preserving — same files written with same bytes.

- [ ] **Step 5: Run the artifact tests too** (sanity)

```bash
go test -run TestInitArtifacts ./cmd/darken/... -count=1
```

Expected: 4/4 PASS.

- [ ] **Step 6: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/init.go cmd/darken/artifacts.go
git commit -m "refactor(init): consume initArtifacts instead of inline scaffolds

runInit now iterates initArtifacts(target) and dispatches on
art.Kind. Behavior is unchanged: same files written with same bytes
(verified by existing init tests). The scaffoldSkill,
scaffoldStatusLine, scaffoldGitignore, renderCLAUDE, and
readEmbeddedTemplate helpers are deleted — their bodies live in
artifacts.go's Body() closures now.

Calls writeInitManifest as a seam for Task 3 (the manifest writer
itself doesn't exist yet — Task 3 will land it; in the meantime the
call falls through with a TODO error message that the next task
makes go away)."
```

(Note: At this commit, `writeInitManifest` is undefined — code won't compile yet. Task 3 immediately follows and adds it. The implementer should NOT push between Task 2 and Task 3; both land before any push. If you need to compile-test in between, comment out the `writeInitManifest` call temporarily and uncomment in Task 3 Step 4.)

**Implementer note:** prefer a clean two-step approach — at the end of Task 2's `runInit`, replace the `writeInitManifest` call with a compile-passing TODO comment (`// TODO(task-3): writeInitManifest(target, arts)`), and put the real call in place during Task 3. This keeps each commit individually compilable.

---

### Task 3: Add `.scion/init-manifest.json` write/read

**Files:**
- Create: `cmd/darken/manifest.go` — `initManifest` type + `writeInitManifest`, `readInitManifest`
- Create: `cmd/darken/manifest_test.go` — round-trip + missing-file tests
- Modify: `cmd/darken/init.go` — uncomment `writeInitManifest(target, arts)` call

**Why:** Persisting the rendered hashes at init time eliminates the templated-CLAUDE.md version-drift false-positive (spec Risk #3). Uninstall reads the manifest first; falls back to `Body()` only if the manifest is missing.

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/manifest_test.go`:

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestInitManifest_WriteThenRead(t *testing.T) {
	target := t.TempDir()

	// Synthesize an artifact list with deterministic bodies.
	arts := []artifact{
		{
			RelPath: "CLAUDE.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return []byte("hello"), nil },
		},
		{
			RelPath: ".gitignore",
			Kind:    "gitignore-lines",
			Body:    func() ([]byte, error) { return []byte("line1\nline2\n"), nil },
		},
	}

	if err := writeInitManifest(target, arts); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Confirm the file landed in the expected place.
	mp := filepath.Join(target, ".scion", "init-manifest.json")
	if _, err := os.Stat(mp); err != nil {
		t.Fatalf("manifest not written at %s: %v", mp, err)
	}

	// Read it back.
	got, err := readInitManifest(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got == nil {
		t.Fatal("expected manifest, got nil")
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if len(got.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(got.Artifacts))
	}

	// Verify the recorded sha matches sha256("hello") for CLAUDE.md.
	wantSha := hex.EncodeToString(sha256.New().Sum([]byte("hello"))[:0]) // placeholder
	h := sha256.Sum256([]byte("hello"))
	wantSha = hex.EncodeToString(h[:])
	for _, a := range got.Artifacts {
		if a.Path == "CLAUDE.md" && a.SHA256 != wantSha {
			t.Errorf("CLAUDE.md sha256 = %s, want %s", a.SHA256, wantSha)
		}
	}
}

func TestInitManifest_ReadMissingReturnsNilNoError(t *testing.T) {
	target := t.TempDir()
	got, err := readInitManifest(target)
	if err != nil {
		t.Fatalf("expected nil error for missing manifest, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil manifest for missing file, got %+v", got)
	}
}

func TestInitManifest_ReadMalformedReturnsError(t *testing.T) {
	target := t.TempDir()
	mp := filepath.Join(target, ".scion", "init-manifest.json")
	if err := os.MkdirAll(filepath.Dir(mp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mp, []byte("not-json{"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readInitManifest(target)
	if err == nil {
		t.Fatal("expected error for malformed manifest, got nil")
	}
	if got != nil {
		t.Errorf("expected nil manifest on parse error, got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
go test -run TestInitManifest ./cmd/darken/... -count=1
```

Expected: undefined `writeInitManifest` / `readInitManifest`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/manifest.go`:

```go
// Package main — `.scion/init-manifest.json` schema + I/O.
//
// runInit writes the manifest after scaffolds are written, recording
// each artifact's path + SHA-256 of the bytes written. runUninstallInit
// reads it to classify each artifact as PRISTINE / CUSTOMIZED without
// re-rendering templated bodies (which can drift across binary versions).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/danmestas/darken/internal/substrate"
)

// initManifest is the on-disk representation at <target>/.scion/init-manifest.json.
type initManifest struct {
	SchemaVersion int                  `json:"schema_version"`
	DarkenVersion string               `json:"darken_version"`
	SubstrateHash string               `json:"substrate_hash"`
	Artifacts     []manifestArtifact   `json:"artifacts"`
}

// manifestArtifact records one artifact's identity at init time.
type manifestArtifact struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
}

// writeInitManifest computes each artifact's SHA-256 (using its Body())
// and writes the manifest atomically (temp + rename). The .scion dir is
// created if missing.
func writeInitManifest(target string, arts []artifact) error {
	man := initManifest{
		SchemaVersion: 1,
		DarkenVersion: darkenVersion(), // existing helper from version.go
		SubstrateHash: substrate.EmbeddedHash(),
	}
	for _, art := range arts {
		body, err := art.Body()
		if err != nil {
			return fmt.Errorf("manifest: %s body: %w", art.RelPath, err)
		}
		h := sha256.Sum256(body)
		man.Artifacts = append(man.Artifacts, manifestArtifact{
			Path:   art.RelPath,
			Kind:   art.Kind,
			SHA256: hex.EncodeToString(h[:]),
		})
	}

	scionDir := filepath.Join(target, ".scion")
	if err := os.MkdirAll(scionDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(scionDir, "init-manifest.json")
	tmp := dst + ".tmp"

	body, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// readInitManifest reads <target>/.scion/init-manifest.json. Returns
// (nil, nil) if the file is missing — older inits or operator deletion.
// Returns (nil, err) on parse failure.
func readInitManifest(target string) (*initManifest, error) {
	mp := filepath.Join(target, ".scion", "init-manifest.json")
	body, err := os.ReadFile(mp)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var man initManifest
	if err := json.Unmarshal(body, &man); err != nil {
		return nil, fmt.Errorf("parse init-manifest.json: %w", err)
	}
	return &man, nil
}
```

(`darkenVersion()` exists in `cmd/darken/version.go`. If its name differs, adjust accordingly — search with `grep -rn "func.*[Vv]ersion" cmd/darken/version.go`.)

- [ ] **Step 4: Re-enable the `writeInitManifest` call in `runInit`**

If Task 2 left a `// TODO(task-3): writeInitManifest(target, arts)` comment, replace it with:

```go
if err := writeInitManifest(target, arts); err != nil {
    fmt.Fprintf(os.Stderr, "init: manifest write failed: %v\n", err)
}
```

- [ ] **Step 5: Run all tests**

```bash
go test ./cmd/darken/... -count=1
```

Expected: full suite green; new manifest tests + existing init tests + Task 1's artifact tests all pass.

- [ ] **Step 6: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/manifest.go cmd/darken/manifest_test.go cmd/darken/init.go
git commit -m "feat(init): persist scaffold hashes to .scion/init-manifest.json

runInit now writes a manifest at the end of scaffolding recording
each artifact's path + SHA-256 of the bytes written. The manifest
schema is documented inline; readInitManifest tolerates a missing
file (returns nil) so legacy init'd repos remain compatible —
uninstall falls back to comparing against the binary's current
Body() output for those.

Eliminates the templated-CLAUDE.md version-drift false-positive
that the spec called out as Risk #3."
```

---

### Task 4: Implement `runUninstallInit`

**Files:**
- Create: `cmd/darken/uninstall_init.go`
- Create: `cmd/darken/uninstall_init_test.go`
- Modify: `cmd/darken/main.go` — register subcommand

**Why:** Phase deliverable. After Task 3's manifest exists at init time, this task lights up the symmetric teardown.

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/uninstall_init_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runUninstallInitTestSetup runs `darken init` against a fresh tempdir
// and returns the target path. Plants stub `bones`/`scion`/`docker` so
// init's prereq check passes.
func runUninstallInitTestSetup(t *testing.T) string {
	t.Helper()
	target := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", target)
	stubDir := t.TempDir()
	for _, bin := range []string{"bones", "scion", "docker"} {
		if err := os.WriteFile(filepath.Join(stubDir, bin), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(target); err != nil {
		t.Fatal(err)
	}

	if err := runInit(nil); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return target
}

func TestUninstallInit_PristineRemovesAll(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	if err := runUninstallInit([]string{"--yes"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	for _, p := range []string{
		"CLAUDE.md",
		".claude/skills/orchestrator-mode/SKILL.md",
		".claude/skills/subagent-to-subharness/SKILL.md",
		".claude/settings.local.json",
		".scion/init-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(target, p)); err == nil {
			t.Errorf("expected %s to be removed", p)
		}
	}

	// .claude/skills/ should be empty-rmdir'd; .claude/ may also be gone.
	if _, err := os.Stat(filepath.Join(target, ".claude", "skills")); err == nil {
		t.Errorf("expected .claude/skills/ to be rmdir'd")
	}

	// .gitignore should still exist but no darken-managed lines.
	body, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to still exist: %v", err)
	}
	if strings.Contains(string(body), ".scion/agents/") {
		t.Errorf(".gitignore should have darken lines stripped, got:\n%s", body)
	}
}

func TestUninstallInit_CustomizedKept(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	skillPath := filepath.Join(target, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("customized\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureCombined(func() error { return runUninstallInit([]string{"--yes"}) })
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}

	// Customized file remains.
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("customized SKILL.md should remain, got: %v", err)
	}
	// Manifest output mentions KEEP and customized.
	if !strings.Contains(out, "KEEP") || !strings.Contains(out, "customized") {
		t.Errorf("expected KEEP / customized in output:\n%s", out)
	}
}

func TestUninstallInit_ForceRemovesCustomized(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	skillPath := filepath.Join(target, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("customized\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runUninstallInit([]string{"--yes", "--force"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	if _, err := os.Stat(skillPath); err == nil {
		t.Errorf("--force should remove customized files, %s still present", skillPath)
	}
}

func TestUninstallInit_GitignoreSurgical(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	gitignorePath := filepath.Join(target, ".gitignore")
	body, _ := os.ReadFile(gitignorePath)
	body = append(body, []byte("\n*.log\nnode_modules/\n")...)
	if err := os.WriteFile(gitignorePath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runUninstallInit([]string{"--yes"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	got, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{"*.log", "node_modules/"} {
		if !strings.Contains(s, want) {
			t.Errorf("operator line %q should be preserved:\n%s", want, s)
		}
	}
	for _, gone := range []string{".scion/agents/", ".scion/audit.jsonl", ".superpowers/"} {
		if strings.Contains(s, gone) {
			t.Errorf("darken line %q should be stripped:\n%s", gone, s)
		}
	}
}

func TestUninstallInit_DryRunMakesNoChanges(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	out, err := captureCombined(func() error { return runUninstallInit([]string{"--dry-run"}) })
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	// All artifacts still present.
	for _, p := range []string{
		"CLAUDE.md",
		".claude/skills/orchestrator-mode/SKILL.md",
		".scion/init-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Errorf("dry-run should not remove %s: %v", p, err)
		}
	}
	if !strings.Contains(out, "REMOVE") {
		t.Errorf("expected manifest output to mention REMOVE:\n%s", out)
	}
}

func TestUninstallInit_NotInitdRepoErrors(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)

	err := runUninstallInit(nil)
	if err == nil {
		t.Fatal("expected error in empty dir")
	}
	if !strings.Contains(err.Error(), "not in an init'd repo") {
		t.Errorf("error should hint at init: %v", err)
	}
}

func TestUninstallInit_FallbackWhenManifestMissing(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	// Delete the manifest so uninstall must fall back to Body() comparison.
	if err := os.Remove(filepath.Join(target, ".scion", "init-manifest.json")); err != nil {
		t.Fatal(err)
	}

	if err := runUninstallInit([]string{"--yes"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	// Pristine artifacts should still be removed via Body() comparison.
	for _, p := range []string{
		".claude/skills/orchestrator-mode/SKILL.md",
		".claude/settings.local.json",
	} {
		if _, err := os.Stat(filepath.Join(target, p)); err == nil {
			t.Errorf("expected %s to be removed via Body() fallback", p)
		}
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
go test -run TestUninstallInit ./cmd/darken/... -count=1
```

Expected: undefined `runUninstallInit`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/uninstall_init.go`:

```go
// Package main — `darken uninstall-init` removes the project scaffolds
// `darken init` wrote, preserving operator-customized files and the
// .scion/ runtime tree. Reads the per-project manifest at
// .scion/init-manifest.json to compare bytes, falling back to the
// binary's current Body() output if the manifest is missing.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	statePristine   = "PRISTINE"
	stateCustomized = "CUSTOMIZED"
	stateMissing    = "MISSING"
)

// classified pairs an artifact with its disposition for the current run.
type classified struct {
	Art    artifact
	State  string
	Reason string // human-readable explanation for the manifest line
}

func runUninstallInit(args []string) error {
	fs := flag.NewFlagSet("uninstall-init", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print manifest, exit without prompting")
	yes := fs.Bool("yes", false, "skip interactive prompt")
	force := fs.Bool("force", false, "also remove CUSTOMIZED artifacts")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not in an init'd repo (run from a directory where 'darken init' was run): %w", err)
	}

	// Sanity: at least one of CLAUDE.md or .claude/ should exist for us
	// to consider this an init'd repo. Otherwise the operator is in the
	// wrong dir.
	if !looksInitd(root) {
		return errors.New("not in an init'd repo (no CLAUDE.md or .claude/ found)")
	}

	manifest, err := readInitManifest(root)
	if err != nil {
		// Malformed manifest — warn and fall back.
		fmt.Fprintf(os.Stderr, "uninstall-init: manifest read failed (%v); falling back to Body() comparison\n", err)
		manifest = nil
	}

	arts := initArtifacts(root)
	classes := make([]classified, 0, len(arts))
	for _, art := range arts {
		c := classifyArtifact(root, art, manifest)
		classes = append(classes, c)
	}

	printUninstallManifest(root, manifest, classes)

	if *dryRun {
		return nil
	}

	if !*yes {
		ok, err := confirmTTY()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	failed := applyUninstall(root, classes, *force)

	// Empty-rmdir candidates (best-effort).
	for _, d := range []string{
		filepath.Join(root, ".claude", "skills", "orchestrator-mode"),
		filepath.Join(root, ".claude", "skills", "subagent-to-subharness"),
		filepath.Join(root, ".claude", "skills"),
		filepath.Join(root, ".claude"),
	} {
		_ = os.Remove(d) // os.Remove on a non-empty dir errors; we ignore
	}

	// Remove the init manifest last — it's our own state.
	mp := filepath.Join(root, ".scion", "init-manifest.json")
	if err := os.Remove(mp); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "uninstall-init: failed to remove manifest: %v\n", err)
		failed++
	}

	// Summary.
	removed, kept := 0, 0
	for _, c := range classes {
		switch c.State {
		case statePristine:
			removed++
		case stateCustomized:
			if !*force {
				kept++
			} else {
				removed++
			}
		}
	}
	fmt.Printf("removed %d files, patched .gitignore, kept %d customized\n", removed, kept)
	if failed > 0 {
		return fmt.Errorf("%d artifacts failed to remove (see stderr)", failed)
	}
	return nil
}

// looksInitd is a cheap heuristic: at least one expected scaffold
// exists at the project root. Avoids the surprise of running
// uninstall-init in a non-darken dir and getting a noisy manifest.
func looksInitd(root string) bool {
	for _, p := range []string{"CLAUDE.md", ".claude"} {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			return true
		}
	}
	return false
}

// classifyArtifact determines PRISTINE / CUSTOMIZED / MISSING for one artifact.
func classifyArtifact(root string, art artifact, manifest *initManifest) classified {
	dst := filepath.Join(root, art.RelPath)
	body, err := os.ReadFile(dst)
	if errors.Is(err, os.ErrNotExist) {
		return classified{Art: art, State: stateMissing, Reason: "not present"}
	}
	if err != nil {
		return classified{Art: art, State: stateCustomized, Reason: fmt.Sprintf("read error: %v", err)}
	}

	switch art.Kind {
	case "file":
		// Manifest-first comparison.
		if manifest != nil {
			for _, ma := range manifest.Artifacts {
				if ma.Path == art.RelPath {
					h := sha256.Sum256(body)
					if hex.EncodeToString(h[:]) == ma.SHA256 {
						return classified{Art: art, State: statePristine, Reason: "matches recorded hash"}
					}
					return classified{Art: art, State: stateCustomized, Reason: "differs from recorded hash"}
				}
			}
		}
		// Fallback: compare against current Body().
		want, err := art.Body()
		if err != nil {
			return classified{Art: art, State: stateCustomized, Reason: fmt.Sprintf("Body() error: %v", err)}
		}
		if bytes.Equal(body, want) {
			return classified{Art: art, State: statePristine, Reason: "matches embedded body"}
		}
		return classified{Art: art, State: stateCustomized, Reason: "differs from embedded body"}

	case "gitignore-lines":
		// Check that all 7 canonical lines are present (TrimSpace match).
		all := true
		for _, line := range gitignoreLines {
			if !lineInBody(body, line) {
				all = false
				break
			}
		}
		if all {
			return classified{Art: art, State: statePristine, Reason: "will strip 7 darken-managed lines"}
		}
		return classified{Art: art, State: stateCustomized, Reason: "darken lines edited or partially removed"}
	}

	return classified{Art: art, State: stateCustomized, Reason: "unknown kind"}
}

// lineInBody returns true if the given line (after TrimSpace) appears
// as a TrimSpace'd line in body.
func lineInBody(body []byte, target string) bool {
	target = strings.TrimSpace(target)
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == target {
			return true
		}
	}
	return false
}

// printUninstallManifest formats the disposition table to stdout.
func printUninstallManifest(root string, manifest *initManifest, classes []classified) {
	fmt.Printf("darken uninstall-init — manifest for %s\n", root)
	if manifest != nil {
		fmt.Printf("init-manifest: .scion/init-manifest.json (darken %s)\n\n", manifest.DarkenVersion)
	} else {
		fmt.Printf("init-manifest: (none — falling back to embedded Body() comparison)\n\n")
	}

	var nRemove, nPatch, nKeep int
	for _, c := range classes {
		var verb string
		switch {
		case c.Art.Kind == "gitignore-lines" && c.State == statePristine:
			verb = "PATCH"
			nPatch++
		case c.State == statePristine:
			verb = "REMOVE"
			nRemove++
		case c.State == stateCustomized:
			verb = "KEEP"
			nKeep++
		case c.State == stateMissing:
			verb = "MISS"
		}
		fmt.Printf("%-7s  %-50s  (%s)\n", verb, c.Art.RelPath, c.Reason)
	}
	fmt.Printf("\n%d files to remove, %d file to patch, %d customized file kept.\n", nRemove, nPatch, nKeep)
}

// confirmTTY prints the prompt and reads a y/N answer from stdin.
// Errors if stdin is not a terminal — callers running non-interactively
// must pass --yes.
func confirmTTY() (bool, error) {
	fi, err := os.Stdin.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false, errors.New("non-interactive context: pass --yes to confirm")
	}
	fmt.Print("Proceed? [y/N]: ")
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}

// applyUninstall performs the deletions. Returns the count of failed removals.
func applyUninstall(root string, classes []classified, force bool) int {
	failed := 0
	for _, c := range classes {
		shouldRemove := c.State == statePristine || (c.State == stateCustomized && force)
		if !shouldRemove {
			continue
		}
		dst := filepath.Join(root, c.Art.RelPath)
		switch c.Art.Kind {
		case "file":
			if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "uninstall: failed to remove %s: %v\n", dst, err)
				failed++
			}
		case "gitignore-lines":
			if err := stripGitignoreLines(dst, gitignoreLines); err != nil {
				fmt.Fprintf(os.Stderr, "uninstall: failed to patch %s: %v\n", dst, err)
				failed++
			}
		}
	}
	return failed
}

// stripGitignoreLines reads the file, drops any line whose TrimSpace
// equals one of the targets, and atomic-writes the result back.
// Preserves a trailing newline if the file had one.
func stripGitignoreLines(path string, targets []string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // nothing to strip
		}
		return err
	}
	skip := make(map[string]bool, len(targets))
	for _, t := range targets {
		skip[strings.TrimSpace(t)] = true
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if skip[strings.TrimSpace(line)] {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Register subcommand in `cmd/darken/main.go`**

In the `subcommands` slice, add after the `{"upgrade-init", ...}` entry:

```go
{"uninstall-init", "remove the project scaffolds darken init wrote (preserves customizations)", runUninstallInit},
```

- [ ] **Step 5: Run tests**

```bash
go test -run TestUninstallInit ./cmd/darken/... -count=1
go test ./cmd/darken/... -count=1
```

Expected: 7/7 new uninstall tests pass; full package green.

- [ ] **Step 6: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/uninstall_init.go cmd/darken/uninstall_init_test.go cmd/darken/main.go
git commit -m "feat(cli): darken uninstall-init

Symmetric counterpart to darken init / upgrade-init. Removes the
project scaffolds init wrote (CLAUDE.md, .claude/skills/, settings,
.gitignore lines), preserves operator customizations and .scion/
runtime state.

Reads .scion/init-manifest.json (Task 3) for byte-equality
comparison; falls back to comparing against the binary's current
Body() output when the manifest is missing.

Three flags: --dry-run, --yes, --force. Non-interactive context
without --yes errors with a hint instead of silently aborting.

Seven tests covering: pristine removal, customized retention,
--force override, surgical .gitignore patch, --dry-run safety,
not-init'd error, manifest-missing fallback."
```

---

### Task 5: Verification + push + PR

- [ ] **Step 1: Full test suite**

```bash
go test ./... -count=1
```

Expected: all 3 packages green; ~12 new tests across artifacts/manifest/uninstall_init.

- [ ] **Step 2: Lint**

```bash
go vet ./... && gofmt -l cmd/ internal/
```

- [ ] **Step 3: Build + smoke**

```bash
make darken
bin/darken --help                              # confirm uninstall-init listed
```

In a separate test directory (don't touch the dev repo):

```bash
mkdir /tmp/uninstall-smoke && cd /tmp/uninstall-smoke
bin/darken init                                # writes scaffolds + .scion/init-manifest.json
ls -la .claude/skills/                         # confirm orchestrator-mode + subagent-to-subharness present
cat .scion/init-manifest.json                  # inspect schema
bin/darken uninstall-init --dry-run            # manifest output, no changes
bin/darken uninstall-init --yes                # full teardown
ls -la                                         # confirm CLAUDE.md gone, .claude/ gone, .gitignore patched
cd / && rm -rf /tmp/uninstall-smoke
```

- [ ] **Step 4: Drift guard**

```bash
cd /Users/dmestas/projects/darkish-factory
bash scripts/test-embed-drift.sh
```

Expected: PASS. Phase 9's drift guard should be unaffected.

- [ ] **Step 5: Push + PR**

```bash
git push -u origin feat/darken-uninstall-init
gh pr create --repo danmestas/darken \
  --title "Add darken uninstall-init: symmetric teardown for project scaffolds" \
  --body "$(cat <<'EOF'
## Summary

Adds `darken uninstall-init` — the symmetric counterpart to `darken init` and `upgrade-init`. Removes the project scaffolds init wrote, preserves operator-customized files and `.scion/` runtime state.

Refactors `runInit` to consume a shared `initArtifacts(target)` helper so init and uninstall draw from one source of truth. Persists `.scion/init-manifest.json` at init time so uninstall can compare against recorded hashes (eliminates the templated-CLAUDE.md version-drift false-positive).

Spec: \`docs/superpowers/specs/2026-04-28-darken-uninstall-init-design.md\`
Plan: \`docs/superpowers/plans/2026-04-28-darken-uninstall-init.md\`

## What's new

- \`darken uninstall-init\` — manifest-then-prompt UX, three flags (\`--dry-run\`, \`--yes\`, \`--force\`)
- \`cmd/darken/artifacts.go\` — shared \`initArtifacts(target)\` helper
- \`cmd/darken/manifest.go\` — \`.scion/init-manifest.json\` schema + I/O
- Refactored \`runInit\` to consume \`initArtifacts\` (behavior preserved)

## Test plan

- [x] \`go test ./... -count=1\` — all green; ~12 new tests
- [x] \`bin/darken init\` writes \`.scion/init-manifest.json\` (manual)
- [x] \`bin/darken uninstall-init --dry-run\` shows manifest, no changes (manual)
- [x] \`bin/darken uninstall-init --yes\` removes pristine, preserves customized (manual)
- [x] \`bin/darken uninstall-init --yes --force\` removes customized too (manual)
- [x] \`bash scripts/test-embed-drift.sh\` PASS

## Operator action post-merge

Tag \`v0.1.11\`. (Or roll into \`v0.2.0\` if cutting that to mark "operator-grade complete for the solo path".)

## Design notes

- \`bin/darken doctor\` drift check from Phase 9 still works — it compares against the embedded body, not the manifest. Both signals coexist.
- Existing init'd repos (no \`.scion/init-manifest.json\` yet) continue to work — uninstall falls back to comparing against the binary's current \`Body()\` for those.
- Operator-customized files default to KEEP. \`--force\` is the escape hatch.
EOF
)"
```

---

## Done definition

This work ships when:

1. `go test ./... -count=1` — all packages green; ~12 new tests
2. `bin/darken init` writes `.scion/init-manifest.json` with the expected schema
3. `bin/darken uninstall-init --dry-run` prints a manifest, makes no changes
4. `bin/darken uninstall-init --yes` removes pristine artifacts + patches `.gitignore` + rmdirs empty `.claude/` subtrees
5. `bin/darken uninstall-init --yes --force` also removes customized artifacts
6. Existing init tests + Phase 8/9 tests still green
7. `bash scripts/test-embed-drift.sh` PASS
8. PR open, CI green
9. Post-merge: tag `v0.1.11`

## What this leaves for later

- `darken upgrade-init --clean-first` flag chaining `uninstall-init --yes --force` first
- `darken verify-init` (subset of doctor's drift check, more granular)
- Manifest schema migrations as init's surface evolves

## Risks / open questions

1. **Init refactor may surface regressions in obscure flag combos.** The `--refresh + --force`, `--dry-run`, and existing-CLAUDE.md interaction logic moves from a flat function to a kind-dispatched loop. Existing init tests verify the behaviorally important paths; if a regression slips through, it'll show up in `bin/darken init --refresh --force` smoke. Mitigation: Step 1 of Task 2 captures the regression baseline before any code change.

2. **`darkenVersion()` may be named differently in `cmd/darken/version.go`.** Task 3 Step 3 calls it; if the function name differs, the implementer adjusts. Cheap fix, no design impact.

3. **Manifest sha256 for `gitignore-lines` is recorded but not used by classify.** It's there for forward-compatibility (future tooling may detect "operator running newer init's gitignore body in an older repo"); cheap to record, harmless if ignored. Mentioned here so reviewers don't flag as dead code.

4. **The smoke test in Task 5 Step 3 runs `bones init` in /tmp.** If the operator has bones globally configured to write outside the target dir, this could leak state. Mitigation: the smoke is optional ("don't touch the dev repo" framing); for paranoid CI, run inside a docker container.
