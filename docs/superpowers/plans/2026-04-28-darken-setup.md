# Darken `setup` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `darken setup` as a single-command convenience that composes `runInit` + `runBootstrap` for fresh-repo onboarding, plus a one-line `darken doctor` failure footer that points operators at setup, plus README/CLAUDE.md updates.

**Architecture:** Thin wrapper. `runSetup(args)` calls `runInit(args)` then `runBootstrap(nil)`. No new state, no smart-skip logic — underlying commands handle their own idempotency. Doctor's `doctorBroad` gains 3 lines that append a failure footer. README + CLAUDE.md replace the 3-command quick-start sequence with `darken setup`.

**Tech Stack:** Go 1.23+, stdlib only. No new dependencies.

**Precondition:** v0.1.12 is shipped (creds/skills/bootstrap fix). Branch `feat/darken-setup` already exists with the spec at `docs/superpowers/specs/2026-04-28-darken-setup-design.md` committed.

---

## File structure

### Created
- `cmd/darken/setup.go` — `runSetup(args)` (3 lines of body)
- `cmd/darken/setup_test.go` — three tests covering composition

### Modified
- `cmd/darken/main.go` — register `setup` subcommand after `init`
- `cmd/darken/doctor.go` — `doctorBroad` appends one line on failure
- `cmd/darken/doctor_test.go` — one new test for the footer
- `README.md` — replace 3-command quick-start with `darken setup`; add `upgrade-init` line; add grouped CLI reference section (Lifecycle / Operations / Inspection / Targeted setup / Authoring)
- `CLAUDE.md` — replace 3-command sequence with `darken setup`

### NOT modified
- `cmd/darken/init.go`, `cmd/darken/bootstrap.go` — composed via existing public functions; no changes needed
- `cmd/darken/init_verify.go` — `runInitDoctor` left as-is; its failure modes belong to `upgrade-init`'s mental model
- `cmd/darken/upgrade_init.go` — coexists; setup is for fresh, upgrade-init is for existing

---

## Tasks

### Task 1: `darken setup` subcommand

**Files:**
- Create: `cmd/darken/setup.go`
- Create: `cmd/darken/setup_test.go`
- Modify: `cmd/darken/main.go` — register subcommand

**Why:** Collapses the fresh-repo onboarding sequence (`darken init` + `darken bootstrap`) into one command. Single flag (`--force`) passes through to init.

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/setup_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubAllBinariesForSetup plants no-op bash + scion + docker + make
// stubs in a single tmpdir prepended to PATH. The bash stub logs each
// invocation so tests can assert the call sequence (init's bash for
// stage-skills, bootstrap's bash for stage-creds, etc.) and the
// scion stub no-ops for `server status`/`hub secret list` etc.
//
// Returns the log path. Tests grep its contents to verify what ran.
func stubAllBinariesForSetup(t *testing.T) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "binaries.log")

	bashStub := `#!/bin/sh
echo "bash $@" >> ` + logPath + `
exit 0
`
	scionStub := `#!/bin/sh
echo "scion $@" >> ` + logPath + `
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
esac
exit 0
`
	dockerStub := `#!/bin/sh
echo "docker $@" >> ` + logPath + `
case "$1" in
  info) exit 0 ;;
  images)
    echo "local/darkish-claude:latest"
    echo "local/darkish-codex:latest"
    echo "local/darkish-pi:latest"
    echo "local/darkish-gemini:latest"
    ;;
esac
exit 0
`
	makeStub := `#!/bin/sh
echo "make $@" >> ` + logPath + `
exit 0
`
	bonesStub := `#!/bin/sh
echo "bones $@" >> ` + logPath + `
exit 0
`

	for name, body := range map[string]string{
		"bash":   bashStub,
		"scion":  scionStub,
		"docker": dockerStub,
		"make":   makeStub,
		"bones":  bonesStub,
	} {
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

func TestSetup_RunsInitThenBootstrap(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	logPath := stubAllBinariesForSetup(t)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runSetup(nil); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Init side: CLAUDE.md should exist.
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Errorf("init phase did not create CLAUDE.md: %v", err)
	}

	// Bootstrap side: the log should show scion server-status checks
	// (step 3 of bootstrap) AND the docker images listing (step 4).
	body, _ := os.ReadFile(logPath)
	got := string(body)
	if !strings.Contains(got, "scion server status") {
		t.Errorf("bootstrap phase did not run; missing `scion server status`:\n%s", got)
	}
	if !strings.Contains(got, "docker images") {
		t.Errorf("bootstrap phase did not run; missing `docker images`:\n%s", got)
	}
}

func TestSetup_ForceFlagPassedToInit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	stubAllBinariesForSetup(t)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	// Pre-plant a stale CLAUDE.md with a unique sentinel.
	stale := []byte("STALE-SENTINEL-DO-NOT-KEEP\n")
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), stale, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSetup([]string{"--force"}); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "STALE-SENTINEL") {
		t.Fatalf("--force did not regenerate CLAUDE.md; sentinel still present:\n%s", body)
	}
}

func TestSetup_AbortsOnInitFailure(t *testing.T) {
	logPath := stubAllBinariesForSetup(t)

	// Pass a non-existent positional target. runInit's existence check
	// fires before any artifacts are written; bootstrap should never run.
	bogus := filepath.Join(t.TempDir(), "does-not-exist-darken-setup-test")

	err := runSetup([]string{bogus})
	if err == nil {
		t.Fatal("expected error when init target missing")
	}
	if !strings.Contains(err.Error(), "target dir does not exist") {
		t.Fatalf("error should mention missing target: %v", err)
	}

	// Bootstrap was NOT called: no `scion server status` log entry from
	// bootstrap's step 3.
	body, _ := os.ReadFile(logPath)
	if strings.Contains(string(body), "scion server status") {
		t.Fatalf("bootstrap should not have been called after init failure:\n%s", body)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
cd /Users/dmestas/projects/darkish-factory
go test -run TestSetup ./cmd/darken/... -count=1
```

Expected: undefined `runSetup`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/setup.go`:

```go
// Package main — `darken setup` is the fresh-repo onboarding shortcut.
// Composes runInit + runBootstrap. Single flag (--force) passes
// through to runInit for CLAUDE.md overwrite.
//
// For the existing-repo / post-`brew upgrade darken` path, see
// runUpgradeInit instead.
package main

func runSetup(args []string) error {
	if err := runInit(args); err != nil {
		return err
	}
	return runBootstrap(nil)
}
```

- [ ] **Step 4: Register subcommand in `cmd/darken/main.go`**

In the `subcommands` slice, add immediately after the existing `{"init", ...}` entry:

```go
{"setup", "scaffold project + bring machine prereqs online (one-shot fresh-repo onboarding)", runSetup},
```

- [ ] **Step 5: Run tests**

```bash
go test -run TestSetup ./cmd/darken/... -count=1
go test ./cmd/darken/... -count=1
```

Expected: 3/3 setup tests pass; full package green.

- [ ] **Step 6: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/setup.go cmd/darken/setup_test.go cmd/darken/main.go
git commit -m "feat(cli): darken setup — fresh-repo one-shot onboarding

Thin wrapper composing runInit + runBootstrap. Single flag (--force)
passes through to init for CLAUDE.md overwrite. Both underlying
commands already handle their own idempotency, so setup is safe to
re-run on partial state.

After this command ships, the README's quick-start collapses from
3 lines to 1 for the fresh-repo path. The existing-repo path
remains darken upgrade-init.

Three tests covering: full init+bootstrap call sequence, --force
propagation to init via CLAUDE.md regeneration, and abort-on-init-
failure (bootstrap not called when init's existence check fails)."
```

---

### Task 2: `darken doctor` failure footer

**Files:**
- Modify: `cmd/darken/doctor.go` — `doctorBroad` appends a setup nudge on failure
- Modify: `cmd/darken/doctor_test.go` — one new test

**Why:** Discoverability. Operator runs `darken doctor`, sees a failed check, and the footer points them at `darken setup` as the canonical multi-fix entry point. Avoids per-check remediation rewrites that would print "darken setup" five times in one fresh-machine run.

- [ ] **Step 1: Write the failing test**

Append to `cmd/darken/doctor_test.go`:

```go
func TestDoctorBroad_FooterMentionsSetupOnFailure(t *testing.T) {
	// Stub scion to exit non-zero so checkScion fails.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Stub docker too so checkDocker doesn't fail with a different
	// error pattern that distracts from the test.
	if err := os.WriteFile(filepath.Join(stubDir, "docker"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	report, err := doctorBroad()
	if err == nil {
		t.Fatal("expected doctor to fail when scion is broken")
	}
	if !strings.Contains(report, "darken setup") {
		t.Fatalf("failure report should mention `darken setup`:\n%s", report)
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

```bash
go test -run TestDoctorBroad_FooterMentionsSetupOnFailure ./cmd/darken/... -count=1
```

Expected: FAIL — report doesn't contain "darken setup".

- [ ] **Step 3: Add the footer to `doctorBroad`**

In `cmd/darken/doctor.go`, find the `doctorBroad` function. Modify the failure return to include a footer:

```go
if len(failed) > 0 {
    sb.WriteString("\n→ for a fresh project, run `darken setup` to bring everything online\n")
    return sb.String(), fmt.Errorf("%d checks failed: %s", len(failed), strings.Join(failed, ", "))
}
return sb.String(), nil
```

(The change is the single `sb.WriteString` line inserted before the `return sb.String(), fmt.Errorf(...)` call. Leave the success path unchanged.)

- [ ] **Step 4: Run test, verify it passes**

```bash
go test -run TestDoctorBroad ./cmd/darken/... -count=1
go test ./cmd/darken/... -count=1
```

Expected: new test passes; existing doctor tests still green.

- [ ] **Step 5: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/doctor.go cmd/darken/doctor_test.go
git commit -m "feat(doctor): footer nudges operators at darken setup

When doctorBroad finds any failed check, append a one-line footer
pointing at \`darken setup\` as the canonical fix-everything entry
point. No per-check remediation rewrites — those would hammer the
same hint five times on a fresh-machine run. One nudge, one entry
point, zero noise on healthy doctor runs."
```

---

### Task 3: README + CLAUDE.md updates + CLI reference

**Files:**
- Modify: `README.md` — collapse 3-command quick-start to `darken setup`; add `upgrade-init` line; add a grouped CLI reference table
- Modify: `CLAUDE.md` — replace 3-command sequence

**Why:** The whole point of `darken setup` is the README headline drops from 3 lines to 1. Operators learn from the README first; CLAUDE.md is the in-repo operator quick-reference. Plus: there's no organized CLI reference today — operators discover commands via `darken --help` (registration order, no grouping). A grouped table makes it obvious which command to reach for.

- [ ] **Step 1: Find the existing quick-start blocks**

```bash
cd /Users/dmestas/projects/darkish-factory
grep -n "darken init\|## Quick Start\|## Commands\|## CLI" README.md
grep -n "darken init" CLAUDE.md
```

Note the line ranges — the implementer will edit those exact spans in the next steps.

- [ ] **Step 2: Update `README.md` quick-start**

Replace the multi-line quick-start (which has `bin/darken creds` + `bin/darken bootstrap` etc.) with:

````markdown
## Quick Start

**Fresh project:**

```bash
darken setup
```

That's it — scaffolds CLAUDE.md, stages skills, ensures Docker/scion/images/secrets, and runs `bones init`.

**Existing project, post-`brew upgrade darken`:**

```bash
darken upgrade-init
```

Refreshes scaffolds against the new binary's substrate; verifies via `darken doctor --init`.
````

(If the existing README has different surrounding sections, preserve them. The minimal edit: collapse any block of `darken init` + `darken creds` + `darken bootstrap` into the `darken setup` line, and add the `upgrade-init` line if not already mentioned.)

- [ ] **Step 3: Add a grouped CLI reference section to `README.md`**

Insert a new `## CLI Reference` section in `README.md` after the Quick Start (or at whatever location reads well). The grouping reflects how operators actually reach for commands:

````markdown
## CLI Reference

`darken --help` lists everything in registration order. The grouping below is by purpose.

### Lifecycle

Use these for the standard project lifecycle.

| Command | Purpose |
|---|---|
| `darken setup` | One-shot fresh-repo onboarding (init + bootstrap) |
| `darken upgrade-init` | Refresh project scaffolds after `brew upgrade darken` |
| `darken uninstall-init` | Remove project scaffolds (preserves customizations + .scion/ runtime state) |
| `darken init` | Project-only scaffolds (CLAUDE.md, .claude/skills/, .gitignore). Prefer `setup` for first-time use. |

### Operations (the §7 loop)

Run, watch, and recover sub-harness workers.

| Command | Purpose |
|---|---|
| `darken spawn <name> --type <role> [task]` | Start an agent (async; default: returns at "ready") |
| `darken redispatch <name>` | Kill + re-spawn an agent with the same role |
| `darken list` | Pass-through to `scion list` |
| `darken apply` | Review + apply darwin recommendations |

### Inspection

Check state, recent decisions, and version coherence.

| Command | Purpose |
|---|---|
| `darken doctor [--init \| <harness>]` | Preflight + post-mortem health checks |
| `darken status` | One-line statusLine output (mode + substrate hash) |
| `darken dashboard` | Open scion's web UI in the default browser |
| `darken history` | Tabular view of `.scion/audit.jsonl` |
| `darken version` | Binary version + embedded substrate hash |

### Targeted setup

Use these for surgical operations when full `setup` is overkill.

| Command | Purpose |
|---|---|
| `darken bootstrap` | Machine prereqs + per-harness skill staging |
| `darken creds [<backend>]` | Refresh hub secrets |
| `darken images` | Wrap `make -C images` |
| `darken skills <harness> [--diff \| --add SKILL \| --remove SKILL]` | Manage staged skills per harness |

### Authoring

| Command | Purpose |
|---|---|
| `darken create-harness <name>` | Scaffold a new harness directory |
| `darken orchestrate` | Print host-mode orchestrator skill body (for piping into a fresh Claude Code session) |
````

(If a similar reference section already exists, replace it rather than adding a duplicate.)

- [ ] **Step 4: Update project-root `CLAUDE.md`**

Read `CLAUDE.md` at the project root. Find any block that lists `darken init` / `darken creds` / `darken bootstrap` as the operator quick-reference. Replace with:

```bash
# One-time setup (new project)
darken setup

# After `brew upgrade darken`
darken upgrade-init
```

If `CLAUDE.md` doesn't carry that sequence (e.g. it only references commands inline), this step is a no-op — note in the commit message if so.

- [ ] **Step 5: Visually scan the result**

```bash
head -120 README.md
head -80 CLAUDE.md
```

Confirm:
- The headline onboarding story is single-command (`darken setup`)
- `upgrade-init` is mentioned for the refresh path
- The CLI reference table is grouped (Lifecycle / Operations / Inspection / Targeted setup / Authoring)
- All ~18 subcommands appear in one of the groups (cross-check against `darken --help` output)

- [ ] **Step 6: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: collapse quick-start to darken setup + add grouped CLI reference

Fresh-project onboarding drops from 3 commands to 1:

  darken setup

The existing-project / post-\`brew upgrade darken\` path remains
darken upgrade-init.

Adds a grouped CLI reference (Lifecycle / Operations / Inspection /
Targeted setup / Authoring) so operators can discover commands by
purpose rather than by --help registration order. The targeted
commands (darken init, darken creds, darken bootstrap, darken skills)
remain for surgical operations; they're just no longer the headline."
```

---

### Task 4: Final verification + push + PR

- [ ] **Step 1: Full suite**

```bash
go test ./... -count=1
```

Expected: all 3 packages green; ~4 new tests across setup_test.go + doctor_test.go.

- [ ] **Step 2: Lint**

```bash
go vet ./... && gofmt -l cmd/ internal/
```

- [ ] **Step 3: Build + smoke**

```bash
make darken
bin/darken --help                                # confirm `setup` listed after `init`
```

In a separate /tmp dir:

```bash
mkdir /tmp/setup-smoke && cd /tmp/setup-smoke && git init -q
/Users/dmestas/projects/darkish-factory/bin/darken setup 2>&1 | head -30  # expect init + bootstrap output
ls -la                                            # CLAUDE.md, .claude/, .scion/init-manifest.json
cd / && rm -rf /tmp/setup-smoke
```

- [ ] **Step 4: Drift guard**

```bash
cd /Users/dmestas/projects/darkish-factory
bash scripts/test-embed-drift.sh
```

Expected: PASS. (No substrate edits in this PR.)

- [ ] **Step 5: Push + PR**

```bash
git push -u origin feat/darken-setup
gh pr create --repo danmestas/darken \
  --title "Add darken setup: fresh-repo one-shot onboarding" \
  --body "$(cat <<'EOF'
## Summary

- New \`darken setup\` subcommand — composes \`runInit\` + \`runBootstrap\` so a new operator on a new project runs **one command** instead of three.
- \`darken doctor\` failure footer now nudges operators at \`darken setup\` as the canonical fix-everything entry point.
- README + CLAUDE.md quick-start collapses from 3 lines to 1 for the fresh-repo path. The existing-repo / post-\`brew upgrade darken\` path remains \`darken upgrade-init\`.
- New grouped CLI reference section in the README (Lifecycle / Operations / Inspection / Targeted setup / Authoring) so operators can discover commands by purpose rather than \`--help\` registration order.

Spec: \`docs/superpowers/specs/2026-04-28-darken-setup-design.md\`
Plan: \`docs/superpowers/plans/2026-04-28-darken-setup.md\`

## What's new

- \`cmd/darken/setup.go\` — \`runSetup(args)\` (~3-line body composing init + bootstrap)
- \`cmd/darken/setup_test.go\` — three tests (init+bootstrap call sequence, --force propagation, abort-on-init-failure)
- \`cmd/darken/doctor.go\` — one-line footer in \`doctorBroad\`'s failure path
- \`cmd/darken/doctor_test.go\` — one new test for the footer
- \`README.md\` + \`CLAUDE.md\` — 3→1 collapse on the quick-start

## Test plan

- [x] \`go test ./... -count=1\` — all green; ~4 new tests
- [x] \`bin/darken setup\` from a fresh /tmp git repo runs init + bootstrap end-to-end (manual)
- [x] \`bin/darken doctor\` with a broken scion shows the setup nudge in the footer (manual)
- [x] \`bash scripts/test-embed-drift.sh\` PASS

## Operator action post-merge

Tag \`v0.1.13\`. (Or roll into \`v0.2.0\` to mark "operator-grade complete for the solo path".)

## Design notes

- \`darken init\` / \`darken creds\` / \`darken bootstrap\` / \`darken skills\` all stay — they're for targeted ops (credential rotation, image rebuilds, etc.). Setup is the headline; they're the surgical tools.
- Setup is safe to re-run on partial state. Both underlying commands handle their own idempotency.
- \`--force\` is the only flag setup accepts (passed through to init for CLAUDE.md overwrite). \`--refresh\` belongs to \`darken upgrade-init\`'s mental model; \`--dry-run\` is awkward across the init→bootstrap composition.
EOF
)"
```

---

## Done definition

This work ships when:

1. `go test ./... -count=1` — all packages green; ~4 new tests
2. `bin/darken setup` from a fresh /tmp dir runs init + bootstrap end-to-end (manual smoke)
3. `bin/darken doctor` with at least one broken check shows `darken setup` in the failure footer
4. README + CLAUDE.md quick-start is one command for the fresh path
5. README has a grouped CLI reference section covering all ~18 subcommands
6. `bash scripts/test-embed-drift.sh` PASS
6. PR open, CI green
7. Post-merge: tag `v0.1.13`

## What this leaves for later

- `darken setup --machine-only` flag for operators who want bootstrap without init in the current dir
- v0.2.0 cut consideration: setup + uninstall-init + upgrade-init form a coherent lifecycle triad ("operator-grade complete for the solo path")
- README polish: post-setup workflow examples (spawning a researcher, running orchestrator-mode)

## Risks / open questions

1. **Doctor footer false-positive when only scion CLI is missing.** Setup runs bootstrap which calls `scion server start` — but if scion CLI itself isn't installed, that fails too. The footer says "run `darken setup`" which won't help. Acceptable: the per-check remediation message for "scion CLI present" already says "make install in ~/projects/scion", which is louder than the footer and arrives first.

2. **CLAUDE.md may not actually carry a 3-command sequence.** If `grep -n "darken init" CLAUDE.md` returns no command-listing block, Task 3 Step 3 is a no-op. The implementer notes this in the commit message and proceeds; not a blocker.

3. **README structure varies** depending on what's there now. Task 3 Step 2 gives the canonical headline; the implementer adapts to whatever surrounding markdown exists. The minimal-edit rule: collapse any 3-command sequence into `darken setup`, add `upgrade-init` for the refresh path.
