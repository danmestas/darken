# Darken Phase 7 — Async Spawn + Cold-Start Progress

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `darken spawn` return as soon as the spawned agent reaches `phase=running` (typically 5-15s) instead of blocking until the agent completes (potentially hours). Print one-line progress to stderr while waiting. Failed handshakes (auth, image missing) surface as exit-1 with a `darken doctor`-style remediation hint. A new `--watch` flag preserves the legacy "block + tail" behavior for operators who want it.

**Architecture:** `scion start` already returns immediately (the container runs detached per `scion start --help`). After `scion start` returns, poll `scion list --format json` until our agent's `phase` is `running` or `error`, with a configurable timeout (default 15s, override via `DARKEN_SPAWN_READY_TIMEOUT`). Progress lines go to stderr; final newline on success, error map + exit 1 on failure. `--watch` skips the polling and instead passes `--attach` through to `scion start`.

**Tech Stack:** Go 1.23+, stdlib only. Reuses `os/exec` for shell-out + `encoding/json` for parsing scion list. No new dependencies.

**Precondition:** Phases 1-6 are merged. `darken spawn` lives at `cmd/darken/spawn.go`. `runSubstrateScript` (Phase 5 Task 1) handles the stage-creds/stage-skills calls; this task only modifies the post-staging behavior.

---

## File structure

### Modified
- `cmd/darken/spawn.go` — adds `--watch` flag; replaces blocking `c.Run()` on `scion start` with `dispatch + poll + report`
- `cmd/darken/spawn_test.go` — adds tests for async return, timeout, error early-exit, --watch passthrough
- `cmd/darken/doctor.go` — `remediationFor` (existing in Phase 1) gains a few new patterns the spawn poller hits (image not found, auth resolution failed). Most are already there; verify and add anything missing.

### Created
- `cmd/darken/spawn_poller.go` — new file holding the poll loop, agent state shape, and progress printing. Keeps spawn.go small.
- `cmd/darken/spawn_poller_test.go` — unit tests for the poller against a stub `scion list` binary

### NOT modified
- `cmd/darken/repoinfo.go`, `internal/substrate/*` — Phase 7 doesn't touch substrate
- `.scion/templates/*` — worker behavior unchanged

---

## Scion data model used

`scion list --format json` returns a JSON array of `AgentInfo` objects. Phase 7 uses these fields:

| Field | Purpose |
|---|---|
| `name` | Human name to match our agent against |
| `phase` | Lifecycle: `created`, `provisioning`, `cloning`, `starting`, `running`, `stopping`, `stopped`, `error` |
| `activity` | Runtime activity inside `running` phase (`idle`, `thinking`, `executing`, ...) — Phase 7 doesn't read this; Phase 8 might |

Confirmed via `~/projects/scion/pkg/api/types.go:AgentInfo`. The "ready" predicate is `phase == "running"`.

Progress mapping (printed to stderr as the poller sees phase transitions):

| Phase | Operator-visible label |
|---|---|
| `created` | `queued` |
| `provisioning` | `provisioning` |
| `cloning` | `cloning workspace` |
| `starting` | `container starting` |
| `running` | `ready` (with elapsed seconds) — return success |
| `error` | error remediation map + return non-nil error |

---

## Tasks

### Task 1: Spawn poller — `pollUntilReady()`

**Why:** Isolate the polling logic from `runSpawn` so it's unit-testable against a fake scion CLI.

**Files:**
- Create: `cmd/darken/spawn_poller.go`
- Create: `cmd/darken/spawn_poller_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/spawn_poller_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubScionList writes a fake `scion` binary to a tmp dir and prepends
// it to PATH. The stub reads the JSON body from the env var
// SCION_STUB_OUTPUT and prints it on `scion list --format json` calls.
func stubScionList(t *testing.T, jsonBody string) {
	t.Helper()
	stubDir := t.TempDir()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"list\" ]; then\n" +
		"  cat <<'EOF'\n" + jsonBody + "\nEOF\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
}

func TestPollUntilReady_ReturnsWhenRunning(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"running"}]`)

	start := time.Now()
	phase, err := pollUntilReady("researcher-1", 5*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if phase != "running" {
		t.Fatalf("expected phase=running, got %q", phase)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("returned too slowly (%s); should poll fast and return on first running tick", elapsed)
	}
}

func TestPollUntilReady_ErrorsOnAgentError(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"error"}]`)

	_, err := pollUntilReady("researcher-1", 5*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when agent phase=error")
	}
	if !strings.Contains(err.Error(), "error") {
		t.Fatalf("error should mention agent error, got: %v", err)
	}
}

func TestPollUntilReady_TimesOut(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"starting"}]`)

	start := time.Now()
	_, err := pollUntilReady("researcher-1", 500*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error should mention timeout, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestPollUntilReady_AgentNotFound(t *testing.T) {
	// scion list returns empty array — our agent isn't in the list yet.
	// The poller should keep polling until the configured timeout.
	stubScionList(t, `[]`)

	_, err := pollUntilReady("researcher-1", 300*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout when agent never appears")
	}
}

func TestPollUntilReady_ScionListErrors(t *testing.T) {
	// scion is not on PATH at all → poller should error after first attempt.
	t.Setenv("PATH", "/nonexistent")
	_, err := pollUntilReady("researcher-1", 1*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when scion CLI is missing")
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail**

```bash
cd /Users/dmestas/projects/darkish-factory
go test -run TestPollUntilReady ./cmd/darken/... -count=1
```

Expected: undefined symbol `pollUntilReady`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/spawn_poller.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// agentInfo is a partial mirror of scion's pkg/api/types.go AgentInfo.
// Only the fields Phase 7's poller needs.
type agentInfo struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

// pollUntilReady runs `scion list --format json` in a tick loop, looking
// for an agent whose name matches the given one and whose phase has
// transitioned to "running" (success) or "error" (failure). Returns the
// terminal phase string and a nil error on running, or the phase + a
// non-nil error on error/timeout/scion-CLI-missing.
//
// timeout: max wall-clock to wait. interval: time between polls.
//
// Caller is expected to have already invoked `scion start <name> ...`
// before calling pollUntilReady — this function only watches for the
// state transition; it doesn't dispatch the agent itself.
func pollUntilReady(agentName string, timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastPhase string
	for {
		agents, err := scionListAgents()
		if err != nil {
			return "", fmt.Errorf("scion list failed: %w", err)
		}
		for _, a := range agents {
			if a.Name != agentName {
				continue
			}
			if a.Phase != lastPhase {
				lastPhase = a.Phase
			}
			switch a.Phase {
			case "running":
				return "running", nil
			case "error":
				return "error", fmt.Errorf("agent %q transitioned to error phase", agentName)
			}
		}
		if time.Now().After(deadline) {
			return lastPhase, fmt.Errorf("timeout waiting for agent %q to reach running phase (last seen: %q)", agentName, lastPhase)
		}
		time.Sleep(interval)
	}
}

// scionListAgents shells out to `scion list --format json` and parses
// the result into agentInfo slices.
func scionListAgents() ([]agentInfo, error) {
	out, err := exec.Command("scion", "list", "--format", "json").Output()
	if err != nil {
		return nil, err
	}
	var agents []agentInfo
	if err := json.Unmarshal(out, &agents); err != nil {
		return nil, fmt.Errorf("parse scion list output: %w", err)
	}
	return agents, nil
}
```

- [ ] **Step 4: Run the tests, verify they pass**

```bash
go test -run TestPollUntilReady ./cmd/darken/... -count=1 -v
```

Expected: 5/5 PASS.

- [ ] **Step 5: Lint**

```bash
go vet ./cmd/darken/... && gofmt -l cmd/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/spawn_poller.go cmd/darken/spawn_poller_test.go
git commit -m "feat(spawn): add pollUntilReady — wait for agent phase=running

New poll loop watches scion list --format json for the spawned agent's
lifecycle phase. Returns when phase transitions to running (success)
or error (failure), or after timeout. Used by Phase 7 Task 2 to make
darken spawn async.

Tests cover: success path, agent-error early exit, timeout while
stuck in starting, agent never appears, scion CLI missing."
```

---

### Task 2: Wire `pollUntilReady` into `runSpawn` (default async behavior)

**Files:**
- Modify: `cmd/darken/spawn.go`
- Modify: `cmd/darken/spawn_test.go`

**Why:** Replace the blocking `c.Run()` on `scion start` with `c.Run()` then `pollUntilReady()`. Result: spawn returns when the agent is ready for work, not when it completes.

- [ ] **Step 1: Read the current `runSpawn` shape**

```bash
grep -A 30 "^func runSpawn" cmd/darken/spawn.go
```

Expected: the function calls `runSubstrateScript` for stage-creds/stage-skills, then builds a scion start command, then `c.Run()`.

- [ ] **Step 2: Update the spawn test to expect async behavior**

The existing `TestSpawnInvokesStageThenScion` test asserts the bash log shows stage-creds + stage-skills + `start smoke-1`. After Phase 7, we ALSO need it to confirm the spawn returned promptly (didn't wait for an agent that never finishes — the bash stub's scion exit 0 means the agent completes immediately, so the test stays green either way).

Add a new test `TestSpawnReturnsAfterReady`:

```go
func TestSpawnReturnsAfterReady(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// scion stub: scion start logs the call AND scion list returns running
	scionStub := `#!/bin/sh
echo "$0 $@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"smoke-1","phase":"running"}]'; exit 0 ;;
  *)     exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	start := time.Now()
	if err := runSpawn([]string{"smoke-1", "--type", "researcher", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Should return promptly because phase=running on first poll.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("spawn returned too slowly (%s); should poll fast and exit on first running tick", elapsed)
	}

	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "start smoke-1") {
		t.Fatalf("scion start not invoked: %s", body)
	}
	if !strings.Contains(string(body), "list --format json") {
		t.Fatalf("scion list not invoked for ready-poll: %s", body)
	}
}
```

(Add `import "time"` if not present.)

- [ ] **Step 3: Run the test, verify it fails (or passes weakly)**

```bash
go test -run TestSpawnReturnsAfterReady ./cmd/darken/... -count=1
```

Expected: FAIL — runSpawn doesn't poll yet, so the assertion `scion list --format json` not invoked` fires.

- [ ] **Step 4: Modify `runSpawn` to call `pollUntilReady` after scion start**

Inside `runSpawn`, after the existing `c := exec.Command("scion", cmd...)` and `c.Run()`, add:

```go
// Read timeout override from env; default 15s.
timeout := 15 * time.Second
if v := os.Getenv("DARKEN_SPAWN_READY_TIMEOUT"); v != "" {
	if d, err := time.ParseDuration(v); err == nil {
		timeout = d
	}
}

// Poll for ready (or error / timeout). Print one-line progress to stderr.
fmt.Fprintf(os.Stderr, "[spawning %s] container starting\n", name)
phase, err := pollUntilReady(name, timeout, 500*time.Millisecond)
if err != nil {
	fmt.Fprintf(os.Stderr, "[spawning %s] FAILED at phase=%s — %v\n", name, phase, err)
	return fmt.Errorf("agent %s did not reach ready: %w", name, err)
}
fmt.Fprintf(os.Stderr, "[spawning %s] ready\n", name)
return nil
```

Note: previously `runSpawn` returned `c.Run()` directly. Now it returns success (nil) after polling, so `c.Run()`'s exit status is consumed but not propagated. If `scion start` fails (returns non-zero), the agent never appears in scion list and pollUntilReady will timeout. Operators see a clearer error: "agent smoke-1 did not reach ready: timeout..." than "exit status 1" alone.

If you want to preserve scion start's exit code as a faster-fail path, do `if err := c.Run(); err != nil { return err }` before the poll. That gives a hybrid: scion's own immediate failures (e.g., template not found) bubble up, and post-dispatch failures still get the timeout path.

- [ ] **Step 5: Add `time` import if not present**

```go
import (
    // ... existing imports ...
    "time"
)
```

- [ ] **Step 6: Run all spawn tests**

```bash
go test -run TestSpawn ./cmd/darken/... -count=1 -v
```

Expected: both `TestSpawnInvokesStageThenScion` (existing) and `TestSpawnReturnsAfterReady` (new) PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/spawn.go cmd/darken/spawn_test.go
git commit -m "feat(spawn): async by default — return when agent reaches phase=running

After scion start dispatches the container (already detached), poll
scion list for the agent's lifecycle phase. Return when phase=running
(typically 5-15s) instead of blocking until the agent completes.

Failed handshake (image missing, auth, timeout) surfaces as exit-1
with a phase-aware error message.

Timeout overridable via DARKEN_SPAWN_READY_TIMEOUT (Go duration:
'30s', '2m'). Default 15s.

The orchestrator's §7 loop already assumes async semantics (it
dispatches + reads via scion look). Phase 7 makes the contract
explicit."
```

---

### Task 3: Cold-start progress on stderr (per-phase)

**Files:**
- Modify: `cmd/darken/spawn_poller.go` — add a progress callback so `pollUntilReady` can emit phase-transition events
- Modify: `cmd/darken/spawn.go` — pass a callback that prints progress lines

**Why:** Task 2 prints just the start + final lines. This task fills in the middle: every phase transition (provisioning → cloning → starting → running) gets its own line on stderr.

- [ ] **Step 1: Add callback parameter to pollUntilReady**

Modify `cmd/darken/spawn_poller.go`'s `pollUntilReady` signature:

```go
// pollUntilReady (...) returns the terminal phase + nil on running,
// or phase + non-nil error on error/timeout. The onPhaseChange
// callback is invoked once per distinct phase observed (skipped if
// nil). Call site uses this to print progress.
func pollUntilReady(agentName string, timeout, interval time.Duration, onPhaseChange func(phase string)) (string, error) {
    // ... existing logic ...
    for {
        agents, err := scionListAgents()
        // ...
        for _, a := range agents {
            if a.Name != agentName {
                continue
            }
            if a.Phase != lastPhase {
                lastPhase = a.Phase
                if onPhaseChange != nil {
                    onPhaseChange(a.Phase)
                }
            }
            // ... existing switch on phase ...
        }
        // ...
    }
}
```

Update the existing TestPollUntilReady_ tests to pass `nil` for the callback (no change in test outcome).

- [ ] **Step 2: Add a test for the callback**

```go
func TestPollUntilReady_CallbackFiresOnPhaseChange(t *testing.T) {
	// First call: phase=starting. Second call: phase=running.
	// Use a script that flips state via a sentinel file.
	stubDir := t.TempDir()
	flagFile := filepath.Join(stubDir, "called")
	body := `#!/bin/sh
if [ ! -f ` + flagFile + ` ]; then
  touch ` + flagFile + `
  echo '[{"name":"researcher-1","phase":"starting"}]'
else
  echo '[{"name":"researcher-1","phase":"running"}]'
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	var phases []string
	_, err := pollUntilReady("researcher-1", 5*time.Second, 50*time.Millisecond,
		func(phase string) { phases = append(phases, phase) })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"starting", "running"}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("expected phases %v, got %v", want, phases)
	}
}
```

(Add `import "reflect"` if not already present.)

- [ ] **Step 3: Wire the callback in `runSpawn` to print progress**

In `cmd/darken/spawn.go`, define the progress label map and use the callback:

```go
phaseLabels := map[string]string{
    "created":      "queued",
    "provisioning": "provisioning",
    "cloning":      "cloning workspace",
    "starting":     "container starting",
    "running":      "ready",
}

start := time.Now()
phase, err := pollUntilReady(name, timeout, 500*time.Millisecond, func(p string) {
    label, ok := phaseLabels[p]
    if !ok {
        label = p
    }
    fmt.Fprintf(os.Stderr, "[spawning %s] %s\n", name, label)
})
if err != nil {
    return fmt.Errorf("agent %s did not reach ready: %w", name, err)
}
fmt.Fprintf(os.Stderr, "[spawning %s] ready (%.1fs)\n", name, time.Since(start).Seconds())
_ = phase
```

(Remove the explicit `[spawning X] container starting` line from Task 2 since the callback now prints transitions including starting.)

- [ ] **Step 4: Run all tests**

```bash
go test ./cmd/darken/... -count=1
```

All PASS.

- [ ] **Step 5: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/spawn_poller.go cmd/darken/spawn_poller_test.go cmd/darken/spawn.go
git commit -m "feat(spawn): per-phase progress on stderr via pollUntilReady callback

pollUntilReady now invokes a per-phase-transition callback so callers
can print progress without polluting the poller. spawn.go uses it to
emit one-line progress per scion lifecycle transition:

  [spawning researcher-1] queued
  [spawning researcher-1] provisioning
  [spawning researcher-1] cloning workspace
  [spawning researcher-1] container starting
  [spawning researcher-1] ready (12.3s)

Quiet on success, loud on failure (handshake error, timeout)."
```

---

### Task 4: `--watch` flag for legacy block-and-tail behavior

**Files:**
- Modify: `cmd/darken/spawn.go`
- Modify: `cmd/darken/spawn_test.go`

**Why:** Some operators want to watch the agent run in their terminal (e.g., for debugging a specific spawn). The `--watch` flag preserves that path. Implementation: pass `--attach` through to `scion start`.

- [ ] **Step 1: Write the failing test**

```go
func TestSpawn_WatchFlagPassesAttach(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	scionStub := `#!/bin/sh
echo "$0 $@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"smoke-watch","phase":"running"}]'; exit 0 ;;
esac
`
	os.WriteFile(filepath.Join(dir, "scion"), []byte(scionStub), 0o755)
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"smoke-watch", "--type", "researcher", "--watch", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "--attach") {
		t.Fatalf("--watch should pass --attach to scion start: %s", body)
	}
}
```

- [ ] **Step 2: Add the flag and pass-through**

In `runSpawn`, after the existing `noStage := fs.Bool(...)` line:

```go
watch := fs.Bool("watch", false, "block + attach to the agent's session (legacy behavior)")
```

After building the `cmd` slice for scion args:

```go
if *watch {
    cmd = append(cmd, "--attach")
}
```

When `--watch` is set, the operator wants to attach immediately; in that mode, `scion start --attach` blocks (per scion docs). Skip the readiness poll in that case:

```go
if *watch {
    // Legacy mode: scion start --attach blocks until agent exits.
    return c.Run()
}

// Default: dispatch + poll for ready.
if err := c.Run(); err != nil {
    return err
}
// ... pollUntilReady call ...
```

- [ ] **Step 3: Run all spawn tests**

```bash
go test -run TestSpawn ./cmd/darken/... -count=1 -v
```

All PASS, including the new `TestSpawn_WatchFlagPassesAttach`.

- [ ] **Step 4: Commit**

```bash
git add cmd/darken/spawn.go cmd/darken/spawn_test.go
git commit -m "feat(spawn): --watch flag for legacy block + attach behavior

darken spawn --watch passes --attach through to scion start, which
runs blocking (operator's terminal becomes the agent's PTY). For
when you want to watch a specific spawn end-to-end.

Default behavior remains async: dispatch + poll until ready + return."
```

---

### Task 5: Final verification + push + PR

- [ ] **Step 1: Full test suite**

```bash
go test ./... -count=1
```

All 3 packages green. Test count should be the existing total + ~5-7 new (depending on whether the callback test was added).

- [ ] **Step 2: Lint**

```bash
go vet ./... && gofmt -l cmd/ internal/
```

- [ ] **Step 3: Build + manual smoke**

```bash
make darken
bin/darken --help                    # confirm spawn shows --watch flag in subcommand help if applicable

# Live smoke (requires scion server + at least one image):
# bin/darken spawn smoke-r1 --type researcher "say hi"
# Expected stderr stream:
#   [spawning smoke-r1] queued
#   [spawning smoke-r1] provisioning
#   [spawning smoke-r1] container starting
#   [spawning smoke-r1] ready (12.3s)
# Then control returns to operator; agent keeps running async.
# scion list shows it; scion attach smoke-r1 to follow.
```

- [ ] **Step 4: Drift guard**

```bash
bash scripts/test-embed-drift.sh
```

PASS expected (Phase 7 doesn't touch internal/substrate/data/).

- [ ] **Step 5: Push branch**

```bash
git push -u origin feat/darken-phase-7
```

(Branch should have been created off main at the start of Phase 7 work.)

- [ ] **Step 6: Open PR**

```bash
gh pr create --repo danmestas/darken \
  --title "Phase 7: async spawn + cold-start progress" \
  --body "$(cat <<'EOF'
## Summary

Phase 7 of the darken DX roadmap. `darken spawn` now returns when the agent reaches \`phase=running\` (typically 5-15s) instead of blocking until completion. Per-phase progress streams to stderr. New \`--watch\` flag preserves legacy block + attach behavior.

## What lands

**Task 1 — pollUntilReady poller**
- New \`spawn_poller.go\` with poll loop against \`scion list --format json\`
- 5 unit tests cover success/error/timeout/missing-agent/missing-CLI

**Task 2 — Async spawn (default)**
- \`runSpawn\` calls \`pollUntilReady\` after \`scion start\`
- Returns when phase=running, errors on phase=error or timeout
- \`DARKEN_SPAWN_READY_TIMEOUT\` env var overrides default 15s

**Task 3 — Per-phase progress**
- Callback fires on each phase transition; runSpawn prints labeled lines:
  \`[spawning <name>] queued → provisioning → cloning workspace → container starting → ready (12.3s)\`

**Task 4 — \`--watch\` flag**
- Passes \`--attach\` through to \`scion start\`
- Operator's terminal becomes the agent's PTY (blocking)
- Default async; \`--watch\` opts into the legacy path

## Operator action items post-merge

\`\`\`bash
git tag -a v0.1.6 -m \"darken v0.1.6 — async spawn + cold-start progress\"
git push origin v0.1.6 && gh run watch
brew upgrade darken
\`\`\`

## What Phase 8 picks up

\`darken dashboard\` (open scion's web UI) + \`darken history\` (audit log viewer).
EOF
)"
```

---

## Done definition

Phase 7 ships when:

1. `go test ./... -count=1` — all green; ~5-7 new tests in `spawn_poller_test.go` and `spawn_test.go`
2. `bin/darken spawn <name> --type <role> "task"` returns within ~15s on a healthy system, stderr shows phase progression
3. Failed handshake (e.g. missing image) surfaces as `agent X did not reach ready: ...` with non-zero exit
4. `bin/darken spawn <name> --type <role> --watch "task"` blocks + attaches per legacy behavior
5. `bash scripts/test-embed-drift.sh` PASS
6. PR open, CI green

## Open questions / risks

1. **Timeout calibration** — 15s default may be too short for first-time image pulls (could be 60s+). Operators with slow connections set `DARKEN_SPAWN_READY_TIMEOUT=60s`. Document this in Phase 8's docs/roadmap update.

2. **Race between scion start and first poll** — if `scion start` returns before the agent registers in `scion list`, the first poll may see an empty list. The poller correctly keeps polling until timeout; no agent → eventually times out. The progress callback won't fire during the gap, so operators see "container starting" only once the agent appears in list. Acceptable.

3. **Bash stub flakiness** — the cold-start progress test (Task 3 Step 2) relies on a stateful bash stub (sentinel file). On parallel test runs the stub state could leak. Mitigation: each test gets `t.TempDir()`, so the sentinel file is per-test. Verify by running `go test -count=10` to confirm no flakes.

4. **`--watch` with broken agent** — if the agent fails immediately, `scion start --attach` returns non-zero, which we propagate. Good.

5. **Backward-compat for `darken spawn` callers** — anyone scripting against `darken spawn` and expecting it to block on completion now gets early returns. Document the breaking change in the PR. Operators want async-by-default though, so this is correct.

## What Phase 8 picks up

Plan to be written when this merges. Adds:
- `darken dashboard` opens scion's web UI in browser
- `darken history` reads `.scion/audit.jsonl` and prints tabular summary
- Status line enrichment: optional active-worker count
