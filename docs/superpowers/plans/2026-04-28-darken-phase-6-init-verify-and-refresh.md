# Darken Phase 6 — Init Verify + Refresh + Prereq Checks

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close audit items 6-8 — give the operator a way to verify an init'd repo is correct, refresh it after `brew upgrade darken`, and fail-fast at init time when prereqs are missing.

**Architecture:** Three additions to existing `cmd/darken/` files. New `runInitDoctor()` that asserts every scaffold from Phase 5 is present and well-formed. New `--refresh` and broadened `--force` flags on `darken init` that re-extract embedded skills/templates without clobbering customizations (unless `--force`). New `verifyInitPrereqs()` called at the top of `runInit` that errors with install hints when bones / scion / Docker are missing. All read paths use the substrate resolver from Phase 1+2 — no new architecture.

**Tech Stack:** Go 1.23+, stdlib only. No new dependencies. Reuses `internal/substrate.Resolver`, `internal/substrate.EmbeddedFS`, and the existing `doctor.go` reporting pattern.

**Precondition:** Phase 5 (`docs/superpowers/plans/2026-04-28-darken-phase-5-init-completeness.md`) must be merged before Phase 6 begins. Phase 6 calls Phase 5's helpers (`scaffoldSkill`, `scaffoldStatusLine`, `scaffoldGitignore`, `runBonesInit`, `renderCLAUDE`). The implementer should branch from `main` after the Phase 5 merge commit, not stack on Phase 5's open PR.

---

## File structure

### Modified
- `cmd/darken/init.go` — adds `--refresh` flag, broadens `--force`, calls `verifyInitPrereqs()` at start, refactors scaffolding into reusable helpers
- `cmd/darken/init_test.go` — adds tests for `--refresh`, `--force` behavior with refresh, prereq checks
- `cmd/darken/doctor.go` — adds `--init` flag dispatch in `runDoctor` to a new `runInitDoctor()`
- `cmd/darken/doctor_test.go` — adds tests for `darken doctor --init`

### Created
- `cmd/darken/init_verify.go` — new file holding `runInitDoctor()` and prereq-check helpers; keeps init.go from getting too big

### NOT modified
- `internal/substrate/` — Phase 1+2 resolver + Phase 2 embed are already what we need
- `internal/substrate/data/templates/CLAUDE.md.tmpl` — template stays as-is from Phase 5

---

## Tasks

### Task 1: `runInitDoctor()` — scoped per-init verification

**Why:** Operator wants a single command to confirm "is this repo's darken init complete and current?" Today they have to inspect files manually.

**Files:**
- Create: `cmd/darken/init_verify.go`
- Create test additions in: `cmd/darken/doctor_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/darken/doctor_test.go`:

```go
func TestInitDoctor_PassesOnCompleteInit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Plant a complete init scaffold (matches what Phase 5's runInit produces).
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness"), 0o755)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken orchestrator-mode\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md"),
		[]byte("---\nname: orchestrator-mode\n---\n# body\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness", "SKILL.md"),
		[]byte("---\nname: subagent-to-subharness\n---\n# body\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "settings.local.json"),
		[]byte(`{"statusLine":{"command":"darken status","type":"command"}}`), 0o644)
	os.WriteFile(filepath.Join(tmp, ".gitignore"),
		[]byte(".scion/agents/\n.scion/skills-staging/\n.scion/audit.jsonl\n.claude/worktrees/\n"), 0o644)

	report, err := runInitDoctor(tmp)
	if err != nil {
		t.Fatalf("expected init doctor to pass on complete scaffold; got: %v\nreport:\n%s", err, report)
	}
	for _, want := range []string{"OK", "CLAUDE.md", "orchestrator-mode", "subagent-to-subharness", "statusLine", ".gitignore"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestInitDoctor_FailsOnMissingCLAUDE(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// no CLAUDE.md planted
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when CLAUDE.md missing")
	}
	if !strings.Contains(report, "CLAUDE.md") {
		t.Fatalf("report should call out CLAUDE.md: %s", report)
	}
	if !strings.Contains(report, "darken init") {
		t.Fatalf("report should suggest `darken init`: %s", report)
	}
}

func TestInitDoctor_FailsOnMissingSkills(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)
	// skills NOT scaffolded
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when skills missing")
	}
	if !strings.Contains(report, "orchestrator-mode") {
		t.Fatalf("report should call out orchestrator-mode skill: %s", report)
	}
}

func TestInitDoctor_FailsOnMissingStatusLine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md"), []byte("name: orchestrator-mode"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness", "SKILL.md"), []byte("name: subagent-to-subharness"), 0o644)
	// no settings.local.json planted
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when settings.local.json missing")
	}
	if !strings.Contains(report, "statusLine") && !strings.Contains(report, "settings.local.json") {
		t.Fatalf("report should call out missing statusLine config: %s", report)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

```bash
cd /Users/dmestas/projects/darkish-factory
go test -run TestInitDoctor ./cmd/darken/... -count=1
```

Expected: undefined: `runInitDoctor`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/init_verify.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// initCheck describes a single required-file check inside an init'd repo.
// failHint is shown when the check fails — should be a one-liner with the
// remediation command the operator should run.
type initCheck struct {
	name     string
	path     string
	check    func(absPath string) error
	failHint string
}

// runInitDoctor runs the per-init verification pass against the given
// target directory. Returns a formatted report and an error if any
// check failed. Mirrors the doctorBroad/doctorHarness pattern.
func runInitDoctor(target string) (string, error) {
	checks := []initCheck{
		{
			name:     "CLAUDE.md present",
			path:     "CLAUDE.md",
			check:    fileNonEmpty,
			failHint: "run `darken init " + target + "` to scaffold",
		},
		{
			name:     "orchestrator-mode skill scaffolded",
			path:     ".claude/skills/orchestrator-mode/SKILL.md",
			check:    fileNonEmpty,
			failHint: "run `darken init --refresh` to extract from binary",
		},
		{
			name:     "subagent-to-subharness skill scaffolded",
			path:     ".claude/skills/subagent-to-subharness/SKILL.md",
			check:    fileNonEmpty,
			failHint: "run `darken init --refresh` to extract from binary",
		},
		{
			name:     "settings.local.json with statusLine command",
			path:     ".claude/settings.local.json",
			check:    statusLineConfigValid,
			failHint: "run `darken init --refresh` to recreate",
		},
		{
			name:     ".gitignore has darken entries",
			path:     ".gitignore",
			check:    gitignoreHasDarkenEntries,
			failHint: "run `darken init --refresh` to append entries",
		},
	}

	var sb strings.Builder
	var failed []string
	for _, c := range checks {
		abs := filepath.Join(target, c.path)
		if err := c.check(abs); err != nil {
			fmt.Fprintf(&sb, "FAIL  %s — %v\n", c.name, err)
			fmt.Fprintf(&sb, "      remediation: %s\n", c.failHint)
			failed = append(failed, c.name)
		} else {
			fmt.Fprintf(&sb, "OK    %s\n", c.name)
		}
	}

	if len(failed) > 0 {
		return sb.String(), fmt.Errorf("%d init checks failed: %s",
			len(failed), strings.Join(failed, ", "))
	}
	return sb.String(), nil
}

// fileNonEmpty asserts the path exists and is non-zero size.
func fileNonEmpty(absPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("missing or inaccessible: %s", absPath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("empty file: %s", absPath)
	}
	return nil
}

// statusLineConfigValid asserts settings.local.json parses as JSON and
// has a statusLine.command field set.
func statusLineConfigValid(absPath string) error {
	body, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("missing settings.local.json")
	}
	var cfg struct {
		StatusLine struct {
			Command string `json:"command"`
			Type    string `json:"type"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if cfg.StatusLine.Command == "" {
		return fmt.Errorf("statusLine.command not set")
	}
	return nil
}

// gitignoreHasDarkenEntries asserts the four canonical darken-related
// gitignore entries are present.
func gitignoreHasDarkenEntries(absPath string) error {
	body, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("missing .gitignore")
	}
	want := []string{
		".scion/agents/",
		".scion/skills-staging/",
		".claude/worktrees/",
	}
	var missing []string
	for _, w := range want {
		if !strings.Contains(string(body), w) {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing entries: %s", strings.Join(missing, ", "))
	}
	return nil
}
```

- [ ] **Step 4: Run the test, verify it passes**

```bash
go test -run TestInitDoctor ./cmd/darken/... -count=1
```

Expected: 4/4 PASS.

- [ ] **Step 5: Lint**

```bash
go vet ./cmd/darken/... && gofmt -l cmd/
```

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/init_verify.go cmd/darken/doctor_test.go
git commit -m "feat(cli): runInitDoctor verifies scaffolds in an init'd repo

Five checks cover Phase 5's init scaffolds: CLAUDE.md present,
both host-mode skills scaffolded, settings.local.json has
statusLine.command, .gitignore has darken entries.

Each FAIL surfaces a remediation hint. Tests cover the happy path
and three failure modes (missing CLAUDE.md, missing skills,
missing statusLine config)."
```

---

### Task 2: Wire `--init` flag into `runDoctor`

**Why:** Expose `runInitDoctor` as `darken doctor --init` so operators have a single command surface.

**Files:**
- Modify: `cmd/darken/doctor.go`
- Modify: `cmd/darken/doctor_test.go`

- [ ] **Step 1: Read the current `runDoctor` signature**

```bash
grep -A 12 "^func runDoctor" cmd/darken/doctor.go
```

Expected: `runDoctor` dispatches on `args[0]` for per-harness mode.

- [ ] **Step 2: Write the failing test**

Append to `cmd/darken/doctor_test.go`:

```go
func TestDoctor_InitFlagDispatchesToInitDoctor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// minimal init: just CLAUDE.md, missing skills → doctor --init should FAIL
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)

	out, err := captureStdout(func() error { return runDoctor([]string{"--init"}) })
	if err == nil {
		t.Fatalf("expected runDoctor --init to fail with missing skills; got nil err\noutput:\n%s", out)
	}
	if !strings.Contains(out, "orchestrator-mode") {
		t.Fatalf("expected output to call out missing orchestrator-mode skill, got: %s", out)
	}
}
```

- [ ] **Step 3: Run the test, verify it fails**

```bash
go test -run TestDoctor_InitFlag ./cmd/darken/... -count=1
```

Expected: dispatch doesn't recognize `--init`; either falls through to `doctorBroad` (which won't fail correctly) or errors differently.

- [ ] **Step 4: Implement the dispatch**

Modify `cmd/darken/doctor.go`'s `runDoctor` to handle `--init`:

```go
func runDoctor(args []string) error {
	// New: --init triggers per-init scaffold verification (Phase 6).
	for _, a := range args {
		if a == "--init" {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			report, err := runInitDoctor(root)
			fmt.Print(report)
			return err
		}
	}

	// Existing dispatch follows below — no change.
	if len(args) >= 1 {
		report, err := doctorHarness(args[0])
		fmt.Println(report)
		return err
	}
	report, err := doctorBroad()
	fmt.Println(report)
	return err
}
```

- [ ] **Step 5: Run the test, verify it passes**

```bash
go test -run TestDoctor ./cmd/darken/... -count=1
```

Expected: all `TestDoctor*` PASS, including the new one and the existing 4 (`TestDoctorReportsMissingScion`, `TestDoctorHarnessChecksImageSecretAndStaging`, `TestDoctorHarnessReadsUserOverridesLayer`, `TestDoctorHarnessPostMortemMapsAuthError`).

- [ ] **Step 6: Lint**

```bash
go vet ./cmd/darken/... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/doctor.go cmd/darken/doctor_test.go
git commit -m "feat(cli): darken doctor --init dispatches to runInitDoctor

Operator runs \`darken doctor --init\` from inside an init'd repo
to verify the scaffold is complete. Falls through to per-harness
or broad checks when --init isn't present."
```

---

### Task 3: `darken init --refresh` — re-scaffold without overwriting CLAUDE.md

**Why:** After `brew upgrade darken`, operators need a way to pull newer skill bodies / settings into existing init'd repos without clobbering customizations to CLAUDE.md.

**Files:**
- Modify: `cmd/darken/init.go`
- Modify: `cmd/darken/init_test.go`

- [ ] **Step 1: Read the current `runInit` shape**

```bash
grep -A 30 "^func runInit" cmd/darken/init.go
```

Expected: parses `--dry-run` and `--force`; writes CLAUDE.md if not exists or `--force`; calls scaffold helpers (`scaffoldSkill`, `scaffoldStatusLine`, `scaffoldGitignore`, `runBonesInit`).

- [ ] **Step 2: Write the failing tests**

Append to `cmd/darken/init_test.go`:

```go
func TestInitRefresh_PreservesCLAUDE(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Initial init creates the scaffold.
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	// Operator customizes CLAUDE.md.
	customCLAUDE := "# Custom CLAUDE.md\n\nMy own content here.\n"
	if err := os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte(customCLAUDE), 0o644); err != nil {
		t.Fatal(err)
	}

	// --refresh should NOT overwrite CLAUDE.md.
	if err := runInit([]string{"--refresh", tmp}); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if string(got) != customCLAUDE {
		t.Fatalf("CLAUDE.md should be preserved, got:\n%s", got)
	}
}

func TestInitRefresh_UpdatesSkills(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	// Operator stomps on a skill with stale content.
	skillPath := filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("STALE CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --refresh should re-extract from embedded substrate.
	if err := runInit([]string{"--refresh", tmp}); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(skillPath)
	if string(got) == "STALE CONTENT" {
		t.Fatal("skill body should have been refreshed from embedded substrate")
	}
	if !strings.Contains(string(got), "name: orchestrator-mode") {
		t.Fatalf("refreshed skill missing frontmatter: %s", string(got)[:100])
	}
}

func TestInitRefreshForce_OverwritesCLAUDE(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	customCLAUDE := "# Custom\n"
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte(customCLAUDE), 0o644)

	// --refresh --force regenerates CLAUDE.md.
	if err := runInit([]string{"--refresh", "--force", tmp}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if string(got) == customCLAUDE {
		t.Fatal("--refresh --force should regenerate CLAUDE.md")
	}
	if !strings.Contains(string(got), "darken orchestrator-mode") {
		t.Fatalf("regenerated CLAUDE.md missing template content: %s", got)
	}
}
```

- [ ] **Step 3: Run the tests, verify they fail**

```bash
go test -run TestInitRefresh ./cmd/darken/... -count=1
```

Expected: `--refresh` flag not recognized.

- [ ] **Step 4: Implement the flag in `runInit`**

Modify `cmd/darken/init.go`'s `runInit` to add the `refresh` flag and gate CLAUDE.md write semantics:

```go
func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "print actions without executing")
	force := flags.Bool("force", false, "overwrite existing CLAUDE.md / settings (use with --refresh)")
	refresh := flags.Bool("refresh", false, "re-scaffold an existing init; preserves CLAUDE.md unless --force")
	if err := flags.Parse(args); err != nil {
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

	claudePath := filepath.Join(target, "CLAUDE.md")
	exists := false
	if _, err := os.Stat(claudePath); err == nil {
		exists = true
	}

	// Decide whether to write CLAUDE.md.
	// Refresh mode: skip CLAUDE.md unless --force.
	// Non-refresh mode (initial init): write if not exists or --force.
	writeCLAUDE := false
	switch {
	case *refresh && *force:
		writeCLAUDE = true // explicit regeneration
	case *refresh:
		writeCLAUDE = false // preserve customizations
	case exists && !*force:
		writeCLAUDE = false // initial init, file already there, no force → skip
	default:
		writeCLAUDE = true // initial init, missing file → write
	}

	if *dryRun {
		if writeCLAUDE {
			fmt.Printf("would create %s\n", claudePath)
		} else {
			fmt.Printf("would skip %s (use --force to overwrite)\n", claudePath)
		}
		return nil
	}

	if writeCLAUDE {
		body, err := renderCLAUDE(target)
		if err != nil {
			return err
		}
		if err := os.WriteFile(claudePath, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", claudePath)
	} else if exists && !*refresh {
		fmt.Printf("skipped %s (already exists; use --force to overwrite)\n", claudePath)
	}

	// Skills, settings, gitignore, bones init: always run; idempotent and
	// safe to re-execute. --refresh re-extracts skills from embedded
	// substrate (overwrites the SKILL.md files in place).
	for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
		if err := scaffoldSkill(target, skill); err != nil {
			fmt.Fprintf(os.Stderr, "init: skill scaffold %s failed: %v\n", skill, err)
		} else {
			fmt.Printf("scaffolded .claude/skills/%s/SKILL.md\n", skill)
		}
	}
	if err := scaffoldStatusLine(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: statusLine scaffold failed: %v\n", err)
	}
	if err := scaffoldGitignore(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: .gitignore append failed: %v\n", err)
	}
	if err := runBonesInit(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: bones init failed: %v\n", err)
	}

	return nil
}
```

(Note: the existing `scaffoldSkill` from Phase 5 already overwrites — verify by checking its implementation. If it currently checks for existence first and skips, modify it to always overwrite when called. The `os.WriteFile` call should not check existence.)

- [ ] **Step 5: Verify `scaffoldSkill` overwrites unconditionally**

```bash
grep -A 15 "^func scaffoldSkill" cmd/darken/init.go
```

Expected: uses `os.WriteFile` directly, which truncates+writes. If it has an early-return on exists, remove that.

- [ ] **Step 6: Run the tests, verify they pass**

```bash
go test -run TestInit ./cmd/darken/... -count=1
```

Expected: all `TestInit*` PASS, including the 3 new + the 4 from Phase 5 (idempotent, force, dry-run, scaffolds).

- [ ] **Step 7: Lint**

```bash
go vet ./cmd/darken/... && gofmt -l cmd/
```

- [ ] **Step 8: Commit**

```bash
git add cmd/darken/init.go cmd/darken/init_test.go
git commit -m "feat(cli): darken init --refresh re-scaffolds without clobbering CLAUDE.md

After \`brew upgrade darken\`, operators run \`darken init --refresh\`
to pull newer skill bodies / statusLine config into existing
init'd repos. CLAUDE.md is preserved by default (operator may have
customized it); --force regenerates from the embedded template.

Skills, statusLine config, .gitignore entries, and bones init all
run on --refresh — they're idempotent and safe to re-execute."
```

---

### Task 4: Init-time prereq checks

**Why:** Today missing bones / scion / Docker surfaces mid-spawn with cryptic errors. Phase 6 surfaces them at init time with one-liner remediations.

**Files:**
- Modify: `cmd/darken/init.go` — call `verifyInitPrereqs(target)` at start
- Modify: `cmd/darken/init_verify.go` — add `verifyInitPrereqs()` helper

- [ ] **Step 1: Write the failing test**

Append to `cmd/darken/init_test.go`:

```go
func TestInit_FailsFastWhenBonesMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Replace PATH with a tmp dir that has scion + docker stubs but NO bones.
	stubDir := t.TempDir()
	for _, b := range []string{"scion", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	err := runInit([]string{tmp})
	if err == nil {
		t.Fatal("expected init to fail when bones missing from PATH")
	}
	if !strings.Contains(err.Error(), "bones") {
		t.Fatalf("error should mention bones: %v", err)
	}
	if !strings.Contains(err.Error(), "brew install") {
		t.Fatalf("error should suggest install hint: %v", err)
	}
}

func TestInit_FailsFastWhenScionMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	for _, b := range []string{"bones", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	err := runInit([]string{tmp})
	if err == nil {
		t.Fatal("expected init to fail when scion missing")
	}
	if !strings.Contains(err.Error(), "scion") {
		t.Fatalf("error should mention scion: %v", err)
	}
}

func TestInit_FailsFastWhenDockerMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	for _, b := range []string{"bones", "scion"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	err := runInit([]string{tmp})
	if err == nil {
		t.Fatal("expected init to fail when docker missing")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Fatalf("error should mention docker: %v", err)
	}
}

func TestInit_PassesWhenAllPrereqsPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	for _, b := range []string{"bones", "scion", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatalf("init should pass with all prereqs on PATH: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail**

```bash
go test -run TestInit_Fails ./cmd/darken/... -count=1
```

Expected: tests fail because no prereq check exists yet — runInit succeeds without checking PATH.

- [ ] **Step 3: Implement `verifyInitPrereqs` in `init_verify.go`**

Append to `cmd/darken/init_verify.go`:

```go
import (
	"os/exec"
)

// prereqTool describes a binary the operator needs on PATH for darken
// init to produce a working orchestrator-mode setup.
type prereqTool struct {
	name        string
	installHint string
}

// verifyInitPrereqs returns a non-nil error listing all missing
// prerequisite tools, with a one-liner install hint per tool. Called
// at the top of runInit so failures surface upfront, not mid-spawn.
func verifyInitPrereqs() error {
	tools := []prereqTool{
		{name: "bones", installHint: "brew install danmestas/tap/bones"},
		{name: "scion", installHint: "see https://github.com/GoogleCloudPlatform/scion (make install)"},
		{name: "docker", installHint: "install Docker Desktop or colima or podman"},
	}
	var missing []string
	for _, t := range tools {
		if _, err := exec.LookPath(t.name); err != nil {
			missing = append(missing, fmt.Sprintf("  - %s not on PATH; install via: %s", t.name, t.installHint))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("darken init prereqs missing:\n%s", strings.Join(missing, "\n"))
	}
	return nil
}
```

(Update the imports at the top of `init_verify.go` to add `os/exec`.)

- [ ] **Step 4: Wire `verifyInitPrereqs()` into `runInit`**

Modify `cmd/darken/init.go`'s `runInit` to call the prereq check at the start, after flag parsing:

```go
func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	// ... existing flag definitions ...
	if err := flags.Parse(args); err != nil {
		return err
	}

	// Phase 6: prereq check — fail fast if bones/scion/docker missing.
	if err := verifyInitPrereqs(); err != nil {
		return err
	}

	// ... rest of runInit unchanged ...
}
```

- [ ] **Step 5: Run the tests, verify they pass**

```bash
go test -run TestInit ./cmd/darken/... -count=1
```

Expected: all 4 new prereq tests PASS, plus existing tests still pass (they now plant the 3 stubs as part of t.Setenv("PATH", ...) where needed).

**IMPORTANT:** the existing Phase 5 init tests (`TestInitScaffoldsCLAUDE`, `TestInitIsIdempotent`, `TestInitForceOverwrites`, `TestInitDryRun`, `TestInitScaffoldsSkills`, `TestInitWritesStatusLineSettings`, `TestInitAppendsGitignore`, `TestInitSecondRunIsIdempotent`) DON'T plant prereq stubs and will now fail. Update each to add the same stub-setup-prereqs preamble. Add a test helper:

```go
// stubPrereqs plants no-op bones/scion/docker stubs on PATH for tests
// that don't care about prereq checks (just want runInit to proceed).
func stubPrereqs(t *testing.T) {
	t.Helper()
	stubDir := t.TempDir()
	for _, b := range []string{"bones", "scion", "docker"} {
		if err := os.WriteFile(filepath.Join(stubDir, b),
			[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubDir)
}
```

…and call `stubPrereqs(t)` at the top of each existing init test. (Add this helper to `cmd/darken/init_test.go`.)

- [ ] **Step 6: Re-run all tests**

```bash
go test ./cmd/darken/... -count=1
```

Expected: all PASS.

- [ ] **Step 7: Lint**

```bash
go vet ./cmd/darken/... && gofmt -l cmd/
```

- [ ] **Step 8: Commit**

```bash
git add cmd/darken/init.go cmd/darken/init_verify.go cmd/darken/init_test.go
git commit -m "feat(cli): darken init fails fast on missing prereqs

verifyInitPrereqs runs at the top of runInit and errors if bones,
scion, or docker is missing from PATH. Each missing tool gets a
one-liner install hint.

Today these surface mid-spawn with cryptic broker errors; Phase 6
surfaces them at init time. Test stubs plant no-op binaries to
satisfy the check; new stubPrereqs helper in init_test.go is
called by all existing init tests."
```

---

### Task 5: Final verification + push + PR

- [ ] **Step 1: Run the full suite**

```bash
go test ./... -count=1
```

Expected: all 3 packages green (cmd/darken, internal/staging, internal/substrate).

- [ ] **Step 2: Lint + format**

```bash
go vet ./...
gofmt -l cmd/ internal/
```

Expected: clean.

- [ ] **Step 3: Build + smoke**

```bash
make darken
bin/darken --help          # confirm subcommand list unchanged
bin/darken doctor --help   # verify --init flag exists if doctor surfaces flag help

# Manual smoke: init a tmp dir, doctor it, refresh, doctor again
TMP=$(mktemp -d)
bin/darken init "$TMP"
bin/darken doctor --init   # expects PASS but DARKEN_REPO_ROOT may need to point at $TMP
DARKEN_REPO_ROOT="$TMP" bin/darken doctor --init   # cleaner
DARKEN_REPO_ROOT="$TMP" bin/darken init --refresh "$TMP"
DARKEN_REPO_ROOT="$TMP" bin/darken doctor --init   # still PASSes after refresh
rm -rf "$TMP"
```

- [ ] **Step 4: Drift guard (sanity)**

```bash
bash scripts/test-embed-drift.sh
```

Expected: PASS (Phase 6 doesn't touch internal/substrate/data/).

- [ ] **Step 5: Sync embed data + commit (only if drift)**

If the drift guard FAILed (unexpected — Phase 6 doesn't change embed):

```bash
make sync-embed-data
git add internal/substrate/data
git commit -m "chore: sync embed data after Phase 6 changes"
```

- [ ] **Step 6: Push branch**

```bash
git push -u origin feat/darken-phase-6-init-verify-and-refresh
```

(The branch is created by the agent at the start of Phase 6 work. If you're continuing from `feat/darken-phase-5-init-completeness`, branch off main first — Phase 6 should not stack on Phase 5's open PR.)

- [ ] **Step 7: Open PR**

```bash
gh pr create --repo danmestas/darken \
  --title "Phase 6: init verify + refresh + prereq checks" \
  --body "$(cat <<'EOF'
## Summary

Phase 6 of the darken DX roadmap (spec at \`docs/superpowers/specs/2026-04-28-darken-DX-roadmap-design.md\`). Closes audit items 6-8.

## What lands

- **\`darken doctor --init\`** — scoped doctor pass that asserts every Phase 5 scaffold is present and well-formed (CLAUDE.md, both skills, settings.local.json statusLine config, .gitignore entries). Each FAIL surfaces a remediation hint pointing at the next command to run.

- **\`darken init --refresh\`** — re-runs scaffolding without overwriting CLAUDE.md. Pulls newer skill bodies + statusLine config from the binary's embedded substrate. Operator's path post-\`brew upgrade darken\` to inherit fixes.

- **\`darken init --force\`** broadened — when used with \`--refresh\`, regenerates CLAUDE.md from the embedded template. Without --refresh, --force keeps Phase 5's behavior (overwrite an existing CLAUDE.md on initial init).

- **Init-time prereq checks** — \`darken init\` errors fast if bones, scion, or docker are missing from PATH. Each missing tool gets a one-liner install hint. Today these surface mid-spawn; Phase 6 surfaces them upfront.

## Test plan

- [x] go test ./... -count=1 — all 3 packages green (12+ new tests across init + doctor)
- [x] go vet, gofmt — clean
- [x] make darken — builds
- [x] bin/darken doctor --init — passes on a fresh init'd repo
- [x] bin/darken init --refresh - leaves customized CLAUDE.md alone, refreshes skills
- [x] bin/darken init --refresh --force - regenerates everything
- [x] bin/darken init in env without bones/scion/docker - errors with install hints
- [x] scripts/test-embed-drift.sh — PASS
- [ ] **Operator validation**: tag v0.1.5 after merge, brew upgrade darken, run darken doctor --init in an existing init'd repo

## Operator action items post-merge

\`\`\`bash
git tag -a v0.1.5 -m "darken v0.1.5 — init verify + refresh + prereq checks"
git push origin v0.1.5
gh run watch
\`\`\`
EOF
)"
```

- [ ] **Step 8: Verify CI green**

```bash
gh run list --repo danmestas/darken --limit 1
```

(If CI is set up to run on PR; otherwise this is a no-op until merge.)

---

## Done definition

Phase 6 ships when:

1. `go test ./... -count=1` — all green (Phase 5 tests + ~10 new tests in init_test.go and doctor_test.go)
2. `bin/darken doctor --init` from a fresh `darken init`'d repo prints `OK` for all 5 checks and exits 0
3. `bin/darken doctor --init` from a partially-init'd repo (missing skill, say) prints FAIL + remediation hint and exits 1
4. `bin/darken init --refresh` against a repo with customized CLAUDE.md leaves CLAUDE.md alone, updates skills + settings + gitignore
5. `bin/darken init --refresh --force` against the same regenerates CLAUDE.md from the embedded template
6. `bin/darken init` in an env without bones/scion/docker errors with one-liner install hints, exits 1, doesn't write any files
7. PR open, CI green, ready for review

## What Phase 7 picks up

- Async spawn (returns at "ready" state)
- Cold-start progress on stderr
- `darken spawn --watch` for legacy blocking behavior
- Error mapping for broker-handshake failures via the existing `darken doctor` remediation table
