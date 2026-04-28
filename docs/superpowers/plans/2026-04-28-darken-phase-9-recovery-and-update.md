# Darken Phase 9 — Recovery + Update Primitives

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the loop on operator recovery (worker hangs → `darken redispatch`) and post-`brew upgrade` substrate coherence (drift detection → `darken upgrade-init`). After Phase 9: a stuck worker is a one-keystroke recovery; a stale project after a binary upgrade is a one-keystroke fix; the orchestrator-mode skill explicitly documents the auto-redispatch policy that the §7 loop has informally implied since v0.1.0.

**Architecture:** Three thin Go subcommands + one skill doc edit. `darken redispatch` wraps `scion stop` + a fresh `runSpawn` invocation, looking up the agent's role from `scion list --format json`. Drift detection in `darken doctor` reads `<repo>/.claude/skills/orchestrator-mode/SKILL.md`, hashes it, and compares against the embedded substrate's copy. `darken upgrade-init` is a 10-line wrapper over `darken init --refresh` + `darken doctor --init`. The orchestrator-mode SKILL.md gains a "Recovery policy" section codifying the heartbeat→redispatch→escalate-after-3 contract.

**Tech Stack:** Go 1.23+, stdlib only (`crypto/sha256`, `encoding/json`, `os/exec`). No new dependencies.

**Precondition:** Phases 1–8 are merged. v0.1.8 is shipped (dashboard + history). Operator has scion server running with `--workstation`.

---

## File structure

### Created
- `cmd/darken/redispatch.go` — `runRedispatch` looks up role via `scion list --format json`, calls `scion stop`, then re-invokes `runSpawn` with the same name + role
- `cmd/darken/redispatch_test.go` — covers role lookup, stop+respawn happy path, missing-agent error
- `cmd/darken/upgrade_init.go` — `runUpgradeInit` wraps `runInit` (with `--refresh`) then `runDoctor` (with `--init`)
- `cmd/darken/upgrade_init_test.go` — covers happy path + propagates errors from either step

### Modified
- `cmd/darken/doctor.go` — `doctorBroad` adds a "project skills hash matches binary substrate" check via a new `checkSubstrateDrift` helper
- `cmd/darken/doctor_test.go` — covers drift PASS, drift FAIL, skill missing
- `cmd/darken/spawn_poller.go` — extends `agentInfo` struct with `Template string \`json:"template"\`` so redispatch can read the role
- `cmd/darken/main.go` — registers `redispatch` + `upgrade-init` subcommands
- `internal/substrate/data/skills/orchestrator-mode/SKILL.md` — new "## Recovery policy" subsection inserted under "## Failure modes to know"

### NOT modified
- `cmd/darken/spawn.go` — `runSpawn` is reused as-is by redispatch; no behavior change
- `cmd/darken/init.go` — `runInit` is reused as-is by upgrade-init
- `cmd/darken/init_verify.go` — `runInitDoctor` is reused as-is
- `internal/substrate/embed.go` — global `EmbeddedHash()` stays; per-file comparison uses `fs.ReadFile` directly

---

## Drift detection — comparison strategy

The check compares **byte-equality** of `<repo>/.claude/skills/orchestrator-mode/SKILL.md` (project copy, written by `darken init`) against the embedded copy at `data/skills/orchestrator-mode/SKILL.md` (read via `substrate.EmbeddedFS()`).

Why byte-equal not hash:
- Files are tiny (~7KB); byte-comparison is simple and removes a sha256 dance.
- The check is binary: equal or not. No need for a hash digest in the output.
- If we later want to drift-check the **whole** `orchestrator-mode/` directory tree, `fs.WalkDir` + path-by-path comparison is the natural extension; nothing about Phase 9 forecloses that.

The check produces three outcomes:
- **OK** — file exists on disk and matches embedded byte-for-byte.
- **WARN** (not a failure that exits non-zero) — file exists but differs. Suggest `darken upgrade-init`.
- **SKIP** — file missing (project not init'd or operator nuked `.claude/`). Suggest `darken init`.

The **WARN** vs FAIL distinction matters: drift is normal when an operator customizes their orchestrator skill. We don't want `darken doctor` to exit 1 just because they're carrying their own loop tweaks. The drift check writes its line to stdout but does NOT add to `failed` slice. PR description should call this out.

---

## Tasks

### Task 1: Substrate-hash drift detection in `darken doctor`

**Files:**
- Modify: `cmd/darken/doctor.go` — add `checkSubstrateDrift` helper + call from `doctorBroad`
- Modify: `cmd/darken/doctor_test.go` — three new test cases

**Why:** Operator runs `brew upgrade darken` and forgets to refresh project skills. They then dispatch a researcher with the **old** orchestrator skill — confusing failures result. Drift check makes the staleness visible before they spawn anything.

- [ ] **Step 1: Write the failing tests**

Add to `cmd/darken/doctor_test.go`:

```go
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danmestas/darken/internal/substrate"
)

// plantOrchestratorSkill writes the embedded orchestrator-mode SKILL.md
// (or a custom body) to <root>/.claude/skills/orchestrator-mode/SKILL.md
// and sets DARKEN_REPO_ROOT to root. Returns the file path.
func plantOrchestratorSkill(t *testing.T, body string) (root, skillPath string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	skillDir := filepath.Join(root, ".claude", "skills", "orchestrator-mode")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath = filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, skillPath
}

// readEmbeddedOrchestratorSkill returns the embedded SKILL.md body —
// useful for the "in-sync" test case.
func readEmbeddedOrchestratorSkill(t *testing.T) string {
	t.Helper()
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestSubstrateDrift_InSync(t *testing.T) {
	embedded := readEmbeddedOrchestratorSkill(t)
	plantOrchestratorSkill(t, embedded)

	out, err := checkSubstrateDrift()
	if err != nil {
		t.Fatalf("expected nil error on in-sync, got %v", err)
	}
	if !strings.Contains(out, "in sync") {
		t.Fatalf("expected 'in sync', got: %s", out)
	}
}

func TestSubstrateDrift_Diverged(t *testing.T) {
	plantOrchestratorSkill(t, "# orchestrator-mode\n\nwildly different body\n")

	out, err := checkSubstrateDrift()
	if err != nil {
		t.Fatalf("drift should not error (warning only), got %v", err)
	}
	if !strings.Contains(out, "drift") {
		t.Fatalf("expected drift warning, got: %s", out)
	}
	if !strings.Contains(out, "darken upgrade-init") {
		t.Fatalf("expected remediation hint, got: %s", out)
	}
}

func TestSubstrateDrift_SkillMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	// Don't plant the skill.

	out, err := checkSubstrateDrift()
	if err != nil {
		t.Fatalf("missing skill should not error, got %v", err)
	}
	if !strings.Contains(out, "not initialized") && !strings.Contains(out, "darken init") {
		t.Fatalf("expected init hint, got: %s", out)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
cd /Users/dmestas/projects/darkish-factory
go test -run TestSubstrateDrift ./cmd/darken/... -count=1
```

Expected: undefined `checkSubstrateDrift`.

- [ ] **Step 3: Write the implementation**

Add to `cmd/darken/doctor.go` (above `doctorBroad`):

```go
// checkSubstrateDrift compares the project's orchestrator-mode SKILL.md
// against the embedded copy. Returns a single human-readable line
// describing one of three states (in sync / drift / not initialized).
//
// This is a WARN-level check — it never returns a non-nil error — so
// drift doesn't make `darken doctor` exit 1. Operators routinely
// customize their orchestrator loop; we only nudge them to refresh
// when they explicitly ran `brew upgrade darken` and forgot.
func checkSubstrateDrift() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "SKIP  substrate drift — not in an init'd repo (run `darken init`)\n", nil
	}
	projectPath := filepath.Join(root, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	projectBody, err := os.ReadFile(projectPath)
	if err != nil {
		return "SKIP  substrate drift — project skill not initialized at " + projectPath + " (run `darken init`)\n", nil
	}
	embeddedBody, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("embedded skill read failed: %w", err)
	}
	if bytes.Equal(projectBody, embeddedBody) {
		return "OK    substrate skills in sync with binary\n", nil
	}
	return "WARN  substrate drift — project's orchestrator-mode/SKILL.md differs from binary (run `darken upgrade-init` to refresh)\n", nil
}
```

Imports to add at the top of `doctor.go`:

```go
import (
	"bytes"
	"io/fs"
	// ... existing imports
	"github.com/danmestas/darken/internal/substrate"
)
```

(Note: `substrate` is already imported via `doctor.go` for `substrate.IsMiss`. Confirm with `grep -n substrate cmd/darken/doctor.go`.)

- [ ] **Step 4: Wire into `doctorBroad`**

Modify `doctorBroad` in `cmd/darken/doctor.go` to call `checkSubstrateDrift` after the existing checks. Per the WARN-level design, drift output goes straight to the report buffer without joining `failed`:

```go
func doctorBroad() (string, error) {
	checks := []check{
		{"docker daemon reachable", checkDocker},
		{"scion CLI present", checkScion},
		{"scion server status", checkScionServer},
		{"hub secrets present", checkHubSecrets},
		{"darken images built", checkImages},
	}

	var sb strings.Builder
	var failed []string
	for _, c := range checks {
		if err := c.run(); err != nil {
			fmt.Fprintf(&sb, "FAIL  %s — %v\n", c.name, err)
			fmt.Fprintf(&sb, "      remediation: %s\n", remediationFor(c.name, err))
			failed = append(failed, c.name)
		} else {
			fmt.Fprintf(&sb, "OK    %s\n", c.name)
		}
	}

	// Substrate-skill drift check (WARN-only — does not contribute to failed).
	driftLine, err := checkSubstrateDrift()
	if err != nil {
		fmt.Fprintf(&sb, "FAIL  substrate drift — %v\n", err)
		failed = append(failed, "substrate drift")
	} else {
		sb.WriteString(driftLine)
	}

	if len(failed) > 0 {
		return sb.String(), fmt.Errorf("%d checks failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return sb.String(), nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test -run TestSubstrateDrift ./cmd/darken/... -count=1
go test ./cmd/darken/... -count=1
```

Expected: 3/3 new PASS; full doctor suite still green.

- [ ] **Step 6: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/doctor.go cmd/darken/doctor_test.go
git commit -m "feat(doctor): substrate-skill drift detection

darken doctor now byte-compares <repo>/.claude/skills/orchestrator-mode/
SKILL.md against the embedded copy. Mismatch surfaces as a WARN line
with a 'darken upgrade-init' remediation hint. Drift does not make
doctor exit 1 — operators routinely customize the orchestrator loop;
this is a nudge, not a gate."
```

---

### Task 2: `darken redispatch <agent>` — kill + re-spawn

**Files:**
- Create: `cmd/darken/redispatch.go`
- Create: `cmd/darken/redispatch_test.go`
- Modify: `cmd/darken/spawn_poller.go` — add `Template` field to `agentInfo`
- Modify: `cmd/darken/main.go` — register subcommand

**Why:** Operator's researcher hangs at minute 11. Today: remember the role from session history, run `scion stop r1`, run `darken spawn r1 --type researcher` again. After: `darken redispatch r1`. The §7 loop in the orchestrator-mode skill has implied this primitive since v0.1.0; Phase 9 makes it concrete.

**Design:** Redispatch looks up the agent's role from `scion list --format json` (so the operator doesn't have to remember). If the agent isn't in the list (already stopped, never existed), error with a hint to use `darken spawn` directly. Worker worktree at `.scion/agents/<name>/` is preserved by scion across stop/start — redispatch treats it as a fresh start; commits are the durable state, in-flight uncommitted work is acceptable to lose.

- [ ] **Step 1: Extend `agentInfo` struct in `spawn_poller.go`**

```go
type agentInfo struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Template string `json:"template"` // role/template name used at spawn time (e.g. "researcher")
}
```

This is additive; existing `pollUntilReady` callers don't care about the new field.

- [ ] **Step 2: Write the failing tests**

Create `cmd/darken/redispatch_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubScionForRedispatch plants a fake `scion` that:
//   - returns a JSON list with the named agent + its template, when
//     called as `scion list --format json`
//   - records every invocation to a log file (one line per invocation,
//     args space-separated)
//
// The log lets tests assert the call sequence (e.g. list → stop → start).
func stubScionForRedispatch(t *testing.T, agentName, template string) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "scion.log")

	body := `#!/bin/sh
echo "$@" >> ` + logPath + `
case "$1" in
  list)
    cat <<EOF
[{"name":"` + agentName + `","phase":"running","template":"` + template + `"}]
EOF
    ;;
  stop) exit 0 ;;
  start) exit 0 ;;
  hub) exit 0 ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

func TestRedispatch_StopThenRespawn(t *testing.T) {
	logPath := stubScionForRedispatch(t, "r1", "researcher")
	// Avoid the post-spawn poll waiting forever — short ready timeout.
	t.Setenv("DARKEN_SPAWN_READY_TIMEOUT", "100ms")

	// Run redispatch on r1.
	err := runRedispatch([]string{"r1"})
	// Spawn's poller may time out (stub doesn't transition phase); we
	// don't care about poller success here, only that stop + start were
	// both invoked in order.
	_ = err

	body, _ := os.ReadFile(logPath)
	got := string(body)
	stopIdx := strings.Index(got, "stop r1")
	startIdx := strings.Index(got, "start r1")
	if stopIdx < 0 {
		t.Fatalf("expected `scion stop r1` invocation, got log:\n%s", got)
	}
	if startIdx < 0 {
		t.Fatalf("expected `scion start r1` invocation, got log:\n%s", got)
	}
	if stopIdx >= startIdx {
		t.Fatalf("stop must precede start, got log:\n%s", got)
	}
	if !strings.Contains(got, "--type researcher") {
		t.Fatalf("expected `--type researcher` from list lookup, got log:\n%s", got)
	}
}

func TestRedispatch_AgentNotInList(t *testing.T) {
	stubDir := t.TempDir()
	body := `#!/bin/sh
case "$1" in
  list) echo "[]" ;;
  *) exit 0 ;;
esac
`
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := runRedispatch([]string{"ghost"})
	if err == nil {
		t.Fatal("expected error when agent not in scion list")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should mention not found: %v", err)
	}
	if !strings.Contains(err.Error(), "darken spawn") {
		t.Fatalf("error should hint at darken spawn: %v", err)
	}
}

func TestRedispatch_RequiresAgentArg(t *testing.T) {
	if err := runRedispatch(nil); err == nil {
		t.Fatal("expected error when no agent name given")
	}
}
```

- [ ] **Step 3: Run tests, verify they fail**

```bash
go test -run TestRedispatch ./cmd/darken/... -count=1
```

Expected: undefined `runRedispatch`.

- [ ] **Step 4: Write the implementation**

Create `cmd/darken/redispatch.go`:

```go
// Package main — `darken redispatch <agent>` kills the named agent
// (via scion stop) and re-spawns it with the same role. The role is
// looked up from `scion list --format json`. Worker worktree is
// preserved by scion across stop/start; redispatch treats it as a
// fresh start (commits are the durable state).
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func runRedispatch(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: darken redispatch <agent>")
	}
	name := args[0]

	// Look up the role from scion list --format json.
	agents, err := scionListAgents()
	if err != nil {
		return fmt.Errorf("scion list failed: %w", err)
	}
	var role string
	for _, a := range agents {
		if a.Name == name {
			role = a.Template
			break
		}
	}
	if role == "" {
		return fmt.Errorf("agent %q not found in scion list (use `darken spawn %s --type <role>` to start fresh)", name, name)
	}

	// Stop the agent. Tolerate "already stopped" — scion stop returns 0
	// on a missing agent in current versions; if that changes, treat
	// non-zero as a soft failure (we still want to attempt re-spawn).
	stop := exec.Command("scion", "stop", name)
	stop.Stdout = os.Stdout
	stop.Stderr = os.Stderr
	if err := stop.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "redispatch: scion stop %s returned %v (continuing)\n", name, err)
	}

	// Re-spawn via the existing spawn path. This invokes the readiness
	// poll, prints per-phase progress, etc.
	return runSpawn([]string{name, "--type", role})
}
```

- [ ] **Step 5: Register subcommand in `cmd/darken/main.go`**

Add to `subcommands` slice (alphabetical-ish placement near `orchestrate`):

```go
{"redispatch", "kill + re-spawn an agent with the same role", runRedispatch},
```

- [ ] **Step 6: Run tests**

```bash
go test -run TestRedispatch ./cmd/darken/... -count=1
```

Expected: 3/3 PASS.

- [ ] **Step 7: Run full poller tests too** (we modified `agentInfo`)

```bash
go test ./cmd/darken/... -count=1
```

Expected: existing `TestPollUntilReady_*` still green; new struct field is additive.

- [ ] **Step 8: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 9: Commit**

```bash
git add cmd/darken/redispatch.go cmd/darken/redispatch_test.go cmd/darken/spawn_poller.go cmd/darken/main.go
git commit -m "feat(cli): darken redispatch <agent>

Looks up the agent's role from scion list --format json, runs scion
stop, then re-invokes the spawn path with the same name + role.
Worker worktree at .scion/agents/<name>/ is preserved by scion;
redispatch treats it as a fresh start (commits are the durable state).

If the agent isn't in scion's list (never existed, already cleaned
up), error with a hint to use darken spawn directly."
```

---

### Task 3: `darken upgrade-init` — refresh + verify in one command

**Files:**
- Create: `cmd/darken/upgrade_init.go`
- Create: `cmd/darken/upgrade_init_test.go`
- Modify: `cmd/darken/main.go` — register subcommand

**Why:** Post-`brew upgrade darken`, the operator currently runs two commands: `darken init --refresh` (rewrites skills + statusLine + .gitignore) then `darken doctor --init` (verifies). One command, one mental model.

**Design:** Pure composition. `runUpgradeInit` invokes `runInit` with `--refresh`, then `runDoctor` with `--init`. Errors from either step propagate. No new flags; no new state.

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/upgrade_init_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeInit_RefreshesAndVerifies(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	// Plant a stub `bones` so init doesn't try to shell out.
	stubDir := t.TempDir()
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	// Pre-init the target so --refresh has something to refresh.
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Working dir governs target inference for runUpgradeInit.
	prev, _ := os.Getwd()
	defer os.Chdir(prev)
	os.Chdir(root)

	out, err := captureCombined(func() error { return runUpgradeInit(nil) })
	if err != nil {
		t.Fatalf("upgrade-init failed: %v\noutput:\n%s", err, out)
	}
	// Confirm at least one scaffold line from init showed up.
	if !strings.Contains(out, "scaffolded") && !strings.Contains(out, "preserved") {
		t.Fatalf("expected init output, got:\n%s", out)
	}
	// Confirm doctor --init ran (init_verify writes lines like "OK ..." or "MISS ...").
	// Either of these substrings indicates the init-doctor path executed.
	if !strings.Contains(out, "CLAUDE.md") && !strings.Contains(out, ".claude") {
		t.Fatalf("expected init-doctor output, got:\n%s", out)
	}
}

func TestUpgradeInit_RejectsArgs(t *testing.T) {
	if err := runUpgradeInit([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}
```

(`captureCombined` is a test helper that captures both stdout and stderr — add it to `cmd/darken/testhelpers_test.go` if absent. See Step 2.)

- [ ] **Step 2: Add `captureCombined` to `cmd/darken/testhelpers_test.go`**

Verify whether `captureCombined` already exists:

```bash
grep -n captureCombined cmd/darken/testhelpers_test.go
```

If missing, append this helper after the existing `captureStdout` (mirrors `captureStdout`'s synchronous pattern; redirects both streams to pipes, concatenates output):

```go
// captureCombined runs fn with both stdout and stderr pointed at
// in-memory pipes and returns the concatenated output. Used by tests
// that exercise multi-step subcommands writing to a mix of streams
// (e.g. upgrade-init invokes init + doctor).
func captureCombined(fn func() error) (string, error) {
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	err := fn()
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outBuf, _ := io.ReadAll(rOut)
	errBuf, _ := io.ReadAll(rErr)
	return string(outBuf) + string(errBuf), err
}
```

- [ ] **Step 3: Run tests, verify they fail**

```bash
go test -run TestUpgradeInit ./cmd/darken/... -count=1
```

Expected: undefined `runUpgradeInit`.

- [ ] **Step 4: Write the implementation**

Create `cmd/darken/upgrade_init.go`:

```go
// Package main — `darken upgrade-init` is the post-`brew upgrade
// darken` convenience: refresh the project's scaffolds against the
// new binary's embedded substrate, then verify with doctor --init.
//
// Equivalent to: `darken init --refresh && darken doctor --init`.
package main

import (
	"errors"
)

func runUpgradeInit(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken upgrade-init")
	}
	if err := runInit([]string{"--refresh"}); err != nil {
		return err
	}
	return runDoctor([]string{"--init"})
}
```

- [ ] **Step 5: Register subcommand in `cmd/darken/main.go`**

Add to `subcommands` slice (place after `init`):

```go
{"upgrade-init", "refresh project scaffolds against the binary's substrate, then verify", runUpgradeInit},
```

- [ ] **Step 6: Run tests**

```bash
go test -run TestUpgradeInit ./cmd/darken/... -count=1
```

Expected: 2/2 PASS.

- [ ] **Step 7: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 8: Commit**

```bash
git add cmd/darken/upgrade_init.go cmd/darken/upgrade_init_test.go cmd/darken/main.go cmd/darken/testhelpers_test.go
git commit -m "feat(cli): darken upgrade-init wrapper

Composes darken init --refresh + darken doctor --init into a single
command. Intended as the canonical post-\`brew upgrade darken\` step:
refreshes scaffolds against the new binary's embedded substrate,
then verifies the project is healthy."
```

---

### Task 4: Document worker auto-redispatch policy in orchestrator-mode SKILL.md

**Files:**
- Modify: `.claude/skills/orchestrator-mode/SKILL.md` — **canonical** source; insert "## Recovery policy" subsection after "## Failure modes to know"
- Modify (auto-synced): `internal/substrate/data/skills/orchestrator-mode/SKILL.md` — embedded mirror, regenerated by `make sync-embed-data`

**Why:** Phase 9's spec calls for documenting the auto-redispatch contract: orchestrator detects hang via heartbeat, calls `darken redispatch` automatically, escalates to operator after 3 retries. The canonical SKILL.md already mentions "10-minute heartbeat timeout" in the Failure modes section but stops short of codifying the loop. Phase 9 makes the loop explicit so future orchestrator runs follow it.

**Design constraint:** Edit the **canonical** at `<repo>/.claude/skills/orchestrator-mode/SKILL.md`. The Makefile's `sync-embed-data` target copies it to `internal/substrate/data/skills/...` for embedding. Editing only the embedded mirror would fail `scripts/test-embed-drift.sh`. No Go code change.

- [ ] **Step 1: Read the current "Failure modes to know" section to find the insertion point**

```bash
grep -n "^## " .claude/skills/orchestrator-mode/SKILL.md
```

Confirm: section "## Failure modes to know" exists; the next `## ` heading is "## What this skill is NOT". Insertion goes between them.

- [ ] **Step 2: Insert the new subsection in the canonical**

Use Edit to insert this block in `.claude/skills/orchestrator-mode/SKILL.md` immediately before `## What this skill is NOT`:

```markdown
## Recovery policy

If a sub-harness hangs (no progress for 10 minutes; detect via `scion look <name>` heartbeat or session log), redispatch automatically:

1. **First hang:** call `darken redispatch <name>` and continue. The agent's worktree at `.scion/agents/<name>/` is preserved across the redispatch — committed work survives, in-flight uncommitted edits are acceptable to lose.
2. **Second hang on the same agent:** call `darken redispatch <name>` again, but flag the recurrence in the audit log (`type: escalate`, `axis: reversibility`, payload includes the redispatch count).
3. **Third hang:** stop redispatching. Escalate to the operator with the failure trace from `scion look <name> --logs`. The operator decides whether to redispatch a fourth time, change tactics, or abort the task.

The 3-strikes ceiling is deliberate: a worker that hangs three times in a row is signal that the task is mis-specified or the harness is misbehaving. Continued auto-redispatch wastes operator attention by burying the underlying problem.

After every redispatch (whether terminal or not), append an audit entry with `type: dispatch`, `outcome: ratified`, payload including `target_role`, `agent_name`, and a note that this was a redispatch (e.g. `payload.redispatch_of: "<previous decision_id>"`). This makes `darken history` show the recovery loop.

```

- [ ] **Step 3: Sync embedded mirror from canonical**

```bash
make sync-embed-data
```

Expected: `synced internal/substrate/data from canonical sources`.

- [ ] **Step 4: Verify the new section is in both copies**

```bash
grep -c "^## Recovery policy" .claude/skills/orchestrator-mode/SKILL.md \
  internal/substrate/data/skills/orchestrator-mode/SKILL.md
```

Expected: both files report `1`.

- [ ] **Step 5: Rebuild the binary and confirm hash bumped**

```bash
make darken
bin/darken version
```

The `embedded substrate` hash reported should differ from v0.1.8's. Note the new hash for the PR description.

- [ ] **Step 6: Drift guard**

```bash
bash scripts/test-embed-drift.sh
```

Expected: `PASS`. (`make sync-embed-data` was run in Step 3, so canonical ↔ embedded are in sync.)

- [ ] **Step 7: Run tests** (no Go change but the binary just rebuilt)

```bash
go test ./... -count=1
```

Expected: full suite green.

- [ ] **Step 8: Commit**

```bash
git add .claude/skills/orchestrator-mode/SKILL.md internal/substrate/data/skills/orchestrator-mode/SKILL.md
git commit -m "docs(skill): document auto-redispatch policy in orchestrator-mode

Adds a Recovery policy section under Failure modes. Codifies the
3-strikes loop:
  1. First hang: darken redispatch, continue.
  2. Second hang: darken redispatch + audit-log escalate flag.
  3. Third hang: stop redispatching, escalate to operator with
     scion look --logs trace.

Worker worktree is preserved across redispatch; committed work
survives, in-flight uncommitted edits are acceptable to lose.

Embedded mirror regenerated via make sync-embed-data."
```

---

### Task 5: Final verification + push + PR

- [ ] **Step 1: Full suite**

```bash
go test ./... -count=1
```

Expected: all packages green; ~10 new tests across drift detection, redispatch, upgrade-init.

- [ ] **Step 2: Lint**

```bash
go vet ./... && gofmt -l cmd/ internal/
```

- [ ] **Step 3: Build + smoke**

```bash
make darken
bin/darken --help                            # confirms redispatch + upgrade-init listed
bin/darken doctor                            # in-sync OK or drift WARN line, no crash
bin/darken redispatch nonexistent            # error: not found, hint to use spawn
bin/darken upgrade-init                      # runs init --refresh + doctor --init in CWD
```

- [ ] **Step 4: Drift guard (full)**

```bash
bash scripts/test-embed-drift.sh
```

Pass expected — Task 4 already synced canonical ↔ embedded.

- [ ] **Step 5: Verify substrate hash bumped**

```bash
bin/darken version
```

The `embedded substrate` hash should differ from v0.1.8's, reflecting the SKILL.md edit. Note the new hash for the PR description.

- [ ] **Step 6: Push + PR**

```bash
git push -u origin feat/darken-phase-9
gh pr create --repo danmestas/darken \
  --title "Phase 9: recovery + update primitives — redispatch, drift, upgrade-init" \
  --body "$(cat <<'EOF'
## Summary

Closes the operator-grade DX roadmap. Phase 9 ships:

- `darken redispatch <agent>` — kill + re-spawn with same role (looks up role from `scion list --format json`)
- `darken upgrade-init` — `darken init --refresh` + `darken doctor --init` in one command
- `darken doctor` — new substrate-skill drift WARN line (project's `orchestrator-mode/SKILL.md` vs embedded copy; nudges `darken upgrade-init` on mismatch)
- `internal/substrate/data/skills/orchestrator-mode/SKILL.md` — new "Recovery policy" section codifying the 3-strikes auto-redispatch loop

Spec: `docs/superpowers/specs/2026-04-28-darken-DX-roadmap-design.md` (Phase 9 section).

## Test plan

- [x] `go test ./... -count=1` — all green
- [x] `bin/darken doctor` shows new WARN drift line on a customized project (manual)
- [x] `bin/darken redispatch <agent>` against a running researcher (manual)
- [x] `bin/darken upgrade-init` from a slightly stale project init (manual)
- [x] `bash scripts/test-embed-drift.sh` PASS

## Operator action post-merge

Tag `v0.1.9`. Consider `v0.2.0` next to mark "operator-grade complete for the solo path" per spec §Versioning.

## Drift WARN — design note

`darken doctor`'s new drift check is intentionally WARN-level: it does NOT exit non-zero on mismatch. Operators who customize their orchestrator loop shouldn't have a noisy doctor; the line is a nudge, not a gate.
EOF
)"
```

---

## Done definition

Phase 9 ships when:

1. `go test ./... -count=1` — all green; ~10 new tests
2. `bin/darken doctor` reports drift line as OK / WARN / SKIP (manual)
3. `bin/darken redispatch <agent>` looks up role + runs stop+start (manual against a real spawn)
4. `bin/darken upgrade-init` refreshes + verifies (manual)
5. `internal/substrate/data/skills/orchestrator-mode/SKILL.md` has a "## Recovery policy" subsection
6. `bash scripts/test-embed-drift.sh` PASS
7. `bin/darken version` reports a new embedded substrate hash (vs v0.1.8)
8. PR open, CI green
9. Post-merge: tag v0.1.9

## What v0.2.0 picks up (not Phase 9)

- Per-file substrate-hash drift check expanded to walk the full `data/skills/` tree (currently only orchestrator-mode/SKILL.md)
- Side-by-side `SKILL.md.new` write strategy for `darken upgrade-init` (preserves operator customizations; current behavior is overwrite via `--refresh`)
- `darken redispatch --type <role>` flag override for cases where `scion list` no longer has the agent
- Audit-log rotation when `.scion/audit.jsonl` exceeds ~1MB

## Risks / open questions

1. **`scion list --format json` may not expose `template` field.** If the JSON output uses a different key (`role`, `type`, `harness_template`, …), Task 2's lookup returns empty. Mitigation: a manual smoke against `scion list --format json` BEFORE writing the implementation lets us match the actual key. Implementer should run `scion list --format json` against a real running agent and adjust the struct tag.

2. **Drift check false-positive on operator customizations.** Any operator-edited `.claude/skills/orchestrator-mode/SKILL.md` triggers the WARN. Acceptable: WARN doesn't fail doctor; operators who customize learn to ignore the line. v0.2.0 may add a `.darken-ignore-drift` marker file to suppress.

3. **`darken upgrade-init` clobbers operator skill customizations.** `darken init --refresh` rewrites scaffolded skills from the embedded substrate. Operators who edited their `.claude/skills/orchestrator-mode/SKILL.md` will lose those edits. Mitigation: spec §Risks 2 noted side-by-side `.new` files as a future improvement; out of scope for Phase 9. PR description should call this out.

4. **3-strikes ceiling is hardcoded in the skill.** No CLI flag. If an operator wants more or fewer retries, they edit their project copy of `orchestrator-mode/SKILL.md`. v0.2.0 may surface this as a config knob.

5. **Embed-drift guard requires `make sync-embed-data` after canonical edits.** The Makefile copies `<repo>/.claude/skills/orchestrator-mode/SKILL.md` (canonical) → `internal/substrate/data/skills/...` (embedded). Editing only one side will fail `scripts/test-embed-drift.sh`. Task 4 explicitly runs the sync.
