# Darken Phase 8 — Routine Observability

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wrap scion's existing surfaces so the operator has a one-keystroke path to "what's running" (web dashboard) and "what happened" (audit log). After Phase 8: operators don't need to remember scion's CLI flags or hub URLs — `darken dashboard` opens the right thing in a browser; `darken history` summarizes the audit log without forcing them to `jq` it.

**Architecture:** No new state, no new services. `darken dashboard` parses `scion server status`, computes the web URL (default `http://localhost:8080`), opens it in the operator's default browser. `darken history` reads `.scion/audit.jsonl` from CWD, prints a tabular summary with optional filters. Both subcommands are thin and composable.

**Tech Stack:** Go 1.23+, stdlib only. Reuses `os/exec` for shell-out + `encoding/json` for audit-log parsing. No new dependencies.

**Precondition:** Phases 1-7 are merged. Phase 7's hotfix (v0.1.7) is shipped. Operator has scion server running with `--workstation` (or `--enable-web`) so the dashboard URL is reachable.

---

## File structure

### Created
- `cmd/darken/dashboard.go` — `runDashboard` parses scion server status + opens URL via `open` (macOS) or `xdg-open` (Linux)
- `cmd/darken/dashboard_test.go` — unit tests against stub `scion server status` output
- `cmd/darken/history.go` — `runHistory` reads `.scion/audit.jsonl`, parses, formats
- `cmd/darken/history_test.go` — covers happy path, filters, format json
- `docs/AUDIT_LOG_SCHEMA.md` — documents the schema `darken history` parses

### Modified
- `cmd/darken/main.go` — registers `dashboard` + `history` subcommands

### NOT modified
- `internal/substrate/*` — no substrate changes needed
- `cmd/darken/spawn.go` — no spawn changes
- The audit log producer side (orchestrator-mode skill writes the file). This phase READS only.

---

## Audit log schema

`.scion/audit.jsonl` is one JSON object per line. Fields:

| Field | Type | Description |
|---|---|---|
| `timestamp` | string (RFC3339) | When the decision was made |
| `decision_id` | string (UUID) | Globally unique per decision |
| `harness` | string | Which harness made the call (`orchestrator`, `darwin`, etc.) |
| `type` | string | Decision category: `route`, `dispatch`, `escalate`, `ratify`, `apply` |
| `outcome` | string | Terminal state: `ratified`, `escalated`, `applied`, `aborted` |
| `payload` | object | Free-form; type-specific (e.g. dispatch has `target_role`, `agent_name`) |

The orchestrator-mode skill writes these. `darken history` reads + formats.

---

## Tasks

### Task 1: `darken dashboard` — open scion's web UI

**Files:**
- Create: `cmd/darken/dashboard.go`
- Create: `cmd/darken/dashboard_test.go`
- Modify: `cmd/darken/main.go` — register subcommand

**Why:** Operator wants one keystroke to open the scion dashboard. Today: remember the URL (`http://localhost:8080`), confirm `--enable-web` is on, type into browser. After: `darken dashboard`.

- [ ] **Step 1: Write the failing test**

Create `cmd/darken/dashboard_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubScionServerStatus plants a fake `scion` binary that returns a
// known status output for `scion server status` calls.
func stubScionServerStatus(t *testing.T, statusBody string) {
	t.Helper()
	stubDir := t.TempDir()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"server\" ] && [ \"$2\" = \"status\" ]; then\n" +
		"  cat <<'EOF'\n" + statusBody + "\nEOF\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
}

// stubOpener plants a fake `open` (macOS) or `xdg-open` (Linux) that
// records the URL it was called with to a log file.
func stubOpener(t *testing.T, openerName string) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "opened.log")
	body := "#!/bin/sh\necho \"$1\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, openerName), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend stub dir to PATH (existing scion stub still wins for scion calls).
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

func TestDashboard_DefaultURLOpened(t *testing.T) {
	statusOut := `Scion Server Status
  Daemon:        running (PID: 12345)
  Log file:      /Users/dmestas/.scion/server.log
  PID file:      /Users/dmestas/.scion/server.pid

Components:
  Hub API:         running
`
	stubScionServerStatus(t, statusOut)
	logPath := stubOpener(t, "open")

	if err := runDashboard(nil); err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}

	body, _ := os.ReadFile(logPath)
	if !strings.Contains(string(body), "http://localhost:8080") {
		t.Fatalf("expected dashboard to open http://localhost:8080, got: %q", body)
	}
}

func TestDashboard_RejectsArgs(t *testing.T) {
	stubScionServerStatus(t, "")
	if err := runDashboard([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}

func TestDashboard_FailsWhenServerDown(t *testing.T) {
	// scion server status exits non-zero when daemon isn't running.
	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := runDashboard(nil)
	if err == nil {
		t.Fatal("expected error when scion server is down")
	}
	if !strings.Contains(err.Error(), "scion server") {
		t.Fatalf("error should mention scion server: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail**

```bash
cd /Users/dmestas/projects/darkish-factory
go test -run TestDashboard ./cmd/darken/... -count=1
```

Expected: undefined `runDashboard`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/dashboard.go`:

```go
// Package main — `darken dashboard` opens scion's web UI in the
// operator's default browser. Thin wrapper: parse scion server
// status, compute URL, exec `open` (macOS) or `xdg-open` (Linux).
package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// dashboardURL is scion's default web port when --workstation or
// --enable-web is set. Today this is hardcoded; future Phase 9 may
// parse it out of scion server status output if scion exposes a
// configurable port via a known field.
const dashboardURL = "http://localhost:8080"

func runDashboard(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken dashboard")
	}

	// Confirm scion server is running before opening the URL — saves
	// the operator from staring at a connection-refused page.
	if err := exec.Command("scion", "server", "status").Run(); err != nil {
		return fmt.Errorf("scion server not running (run `scion server start --workstation`): %w", err)
	}

	// Open the URL via the platform's default opener.
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	c := exec.Command(opener, dashboardURL)
	return c.Run()
}
```

- [ ] **Step 4: Register subcommand in `cmd/darken/main.go`**

Add to `subcommands` slice:
```go
{"dashboard", "open scion's web UI in the default browser", runDashboard},
```

- [ ] **Step 5: Run tests, verify they pass**

```bash
go test -run TestDashboard ./cmd/darken/... -count=1
```

Expected: 3/3 PASS.

- [ ] **Step 6: Lint clean**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/dashboard.go cmd/darken/dashboard_test.go cmd/darken/main.go
git commit -m "feat(cli): darken dashboard opens scion web UI

Thin wrapper: confirms scion server is running, then execs the
platform opener (open on macOS, xdg-open on Linux) against
http://localhost:8080.

Errors with a one-line remediation if scion server is down.
URL is hardcoded for now; Phase 9+ may parse it out of scion
server status if scion exposes a configurable port."
```

---

### Task 2: Document the audit log schema

**Files:**
- Create: `docs/AUDIT_LOG_SCHEMA.md`

**Why:** Phase 8's `darken history` reads `.scion/audit.jsonl`. The schema has been implicit (orchestrator-mode skill writes it). Document it before Phase 8's reader assumes a shape.

- [ ] **Step 1: Write the doc**

```markdown
# .scion/audit.jsonl — schema

The orchestrator's audit log is a JSON-Lines file at `.scion/audit.jsonl`
in any darken-init'd repo. One JSON object per line. Append-only.

The orchestrator-mode skill writes these entries; `darken history`
reads + formats them.

## Required fields

| Field | Type | Description |
|---|---|---|
| `timestamp` | string (RFC3339) | When the decision was made |
| `decision_id` | string (UUID) | Globally unique per decision |
| `harness` | string | Which harness made the call (e.g. `orchestrator`, `darwin`) |
| `type` | string | Decision category — see Type values below |
| `outcome` | string | Terminal state — see Outcome values below |
| `payload` | object | Type-specific freeform data |

## Type values

| Value | When it fires | Payload fields |
|---|---|---|
| `route` | Routing classifier picks light/heavy/tier | `tier`, `confidence`, `reasons` (array) |
| `dispatch` | Orchestrator spawns a subharness | `target_role`, `agent_name`, `task` (truncated) |
| `escalate` | Stage-1 or Stage-2 classifier escalates | `axis` (taste/architecture/ethics/reversibility), `summary` |
| `ratify` | Decision auto-ratified (no operator involvement) | `axis`, `confidence` |
| `apply` | `darken apply` ratifies a darwin recommendation | `recommendation_id`, `target_harness` |

## Outcome values

| Value | Meaning |
|---|---|
| `ratified` | Auto-approved by classifier |
| `escalated` | Sent to operator for decision |
| `applied` | Operator approved + applied |
| `aborted` | Operator declined or system rolled back |

## Example

\`\`\`jsonl
{"timestamp":"2026-04-28T07:14:32Z","decision_id":"uuid-1","harness":"orchestrator","type":"route","outcome":"ratified","payload":{"tier":"heavy","confidence":0.92,"reasons":["multi-module","schema-change"]}}
{"timestamp":"2026-04-28T07:14:35Z","decision_id":"uuid-2","harness":"orchestrator","type":"dispatch","outcome":"ratified","payload":{"target_role":"researcher","agent_name":"r1","task":"audit auth flow"}}
{"timestamp":"2026-04-28T07:18:01Z","decision_id":"uuid-3","harness":"orchestrator","type":"escalate","outcome":"escalated","payload":{"axis":"reversibility","summary":"propose drop populated table"}}
\`\`\`

## Stability

The schema is **frozen at v1**. New types may be added; existing fields
won't be removed or repurposed without a schema_version bump on
individual entries. `darken history` skips entries with unknown types
gracefully (one-line warning to stderr).
```

- [ ] **Step 2: Commit**

```bash
git add docs/AUDIT_LOG_SCHEMA.md
git commit -m "docs: document .scion/audit.jsonl schema

darken history (Phase 8) reads this file. Document the contract so
the orchestrator-mode skill (writer) and darken history (reader)
share an explicit spec."
```

---

### Task 3: `darken history` — read + format `.scion/audit.jsonl`

**Files:**
- Create: `cmd/darken/history.go`
- Create: `cmd/darken/history_test.go`
- Modify: `cmd/darken/main.go` — register subcommand

**Why:** Operator wants `darken history` for "what happened today". Today they'd `cat .scion/audit.jsonl | jq -r '...'` with their own format string. After: one command, sane defaults.

- [ ] **Step 1: Write the failing tests**

Create `cmd/darken/history_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleAuditLog = `{"timestamp":"2026-04-28T07:14:32Z","decision_id":"uuid-1","harness":"orchestrator","type":"route","outcome":"ratified","payload":{"tier":"heavy"}}
{"timestamp":"2026-04-28T07:14:35Z","decision_id":"uuid-2","harness":"orchestrator","type":"dispatch","outcome":"ratified","payload":{"target_role":"researcher","agent_name":"r1"}}
{"timestamp":"2026-04-28T07:18:01Z","decision_id":"uuid-3","harness":"orchestrator","type":"escalate","outcome":"escalated","payload":{"axis":"reversibility"}}
`

func plantAuditLog(t *testing.T, body string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	scionDir := filepath.Join(tmp, ".scion")
	os.MkdirAll(scionDir, 0o755)
	logPath := filepath.Join(scionDir, "audit.jsonl")
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return logPath
}

func TestHistory_PrintsTabularSummary(t *testing.T) {
	plantAuditLog(t, sampleAuditLog)

	out, err := captureStdout(func() error { return runHistory(nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"2026-04-28T07:14:32Z",
		"orchestrator",
		"route",
		"ratified",
		"dispatch",
		"escalate",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestHistory_LastNLimit(t *testing.T) {
	plantAuditLog(t, sampleAuditLog)

	out, err := captureStdout(func() error { return runHistory([]string{"--last", "1"}) })
	if err != nil {
		t.Fatal(err)
	}
	// --last 1 should show only the most-recent entry (the escalate).
	if !strings.Contains(out, "escalate") {
		t.Fatalf("expected most-recent entry, got:\n%s", out)
	}
	// Should NOT contain the older entries (route or dispatch).
	if strings.Contains(out, "route") {
		t.Fatalf("--last 1 should exclude older entries, got:\n%s", out)
	}
}

func TestHistory_FormatJSON(t *testing.T) {
	plantAuditLog(t, sampleAuditLog)

	out, err := captureStdout(func() error { return runHistory([]string{"--format", "json"}) })
	if err != nil {
		t.Fatal(err)
	}
	// JSON format should be raw JSONL (each line a parseable object).
	if !strings.Contains(out, `"decision_id":"uuid-1"`) {
		t.Fatalf("expected raw JSONL output, got:\n%s", out)
	}
}

func TestHistory_EmptyLogReturnsZeroEntries(t *testing.T) {
	plantAuditLog(t, "")

	out, err := captureStdout(func() error { return runHistory(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no audit entries") {
		t.Fatalf("expected friendly empty message, got:\n%s", out)
	}
}

func TestHistory_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// Don't plant the file.

	_, err := captureStdout(func() error { return runHistory(nil) })
	if err == nil {
		t.Fatal("expected error when audit log missing")
	}
	if !strings.Contains(err.Error(), "audit") {
		t.Fatalf("error should mention audit log: %v", err)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
go test -run TestHistory ./cmd/darken/... -count=1
```

Expected: undefined `runHistory`.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/history.go`:

```go
// Package main — `darken history` reads .scion/audit.jsonl and prints
// a tabular summary or raw JSON. Filters: --last N (most-recent N),
// --since DUR (Go duration), --format text|json.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// auditEntry mirrors docs/AUDIT_LOG_SCHEMA.md. Fields are loose
// because we tolerate unknown payload shapes.
type auditEntry struct {
	Timestamp  string                 `json:"timestamp"`
	DecisionID string                 `json:"decision_id"`
	Harness    string                 `json:"harness"`
	Type       string                 `json:"type"`
	Outcome    string                 `json:"outcome"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
}

func runHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	last := fs.Int("last", 0, "show only the most-recent N entries (0 = all)")
	since := fs.String("since", "", "show entries since the given Go duration ago (e.g. '1h', '24h')")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not in an init'd repo: %w", err)
	}
	logPath := filepath.Join(root, ".scion", "audit.jsonl")

	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("audit log read failed at %s: %w", logPath, err)
	}
	defer f.Close()

	var entries []auditEntry
	var rawLines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e auditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			fmt.Fprintf(os.Stderr, "history: skipping malformed entry: %v\n", err)
			continue
		}
		entries = append(entries, e)
		rawLines = append(rawLines, line)
	}

	// Apply --since filter.
	if *since != "" {
		dur, err := time.ParseDuration(*since)
		if err != nil {
			return fmt.Errorf("--since: invalid duration %q: %w", *since, err)
		}
		cutoff := time.Now().Add(-dur)
		var filtered []auditEntry
		var filteredRaw []string
		for i, e := range entries {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err != nil {
				continue // skip entries with unparseable timestamps
			}
			if t.After(cutoff) {
				filtered = append(filtered, e)
				filteredRaw = append(filteredRaw, rawLines[i])
			}
		}
		entries = filtered
		rawLines = filteredRaw
	}

	// Apply --last filter (after --since).
	if *last > 0 && len(entries) > *last {
		entries = entries[len(entries)-*last:]
		rawLines = rawLines[len(rawLines)-*last:]
	}

	if len(entries) == 0 {
		fmt.Println("no audit entries")
		return nil
	}

	switch *format {
	case "json":
		for _, line := range rawLines {
			fmt.Println(line)
		}
	case "text":
		fmt.Printf("%-21s  %-14s  %-9s  %-10s  %s\n", "TIMESTAMP", "HARNESS", "TYPE", "OUTCOME", "DETAIL")
		for _, e := range entries {
			detail := summarizePayload(e.Type, e.Payload)
			fmt.Printf("%-21s  %-14s  %-9s  %-10s  %s\n", e.Timestamp, e.Harness, e.Type, e.Outcome, detail)
		}
	default:
		return fmt.Errorf("--format: must be text or json, got %q", *format)
	}

	_ = errors.New // silence unused if compiler complains
	return nil
}

// summarizePayload returns a short string describing the most-relevant
// payload field for the given decision type. Best-effort; payload
// structure isn't strictly enforced (per AUDIT_LOG_SCHEMA stability
// note). Unknown types fall through to JSON-marshalled payload.
func summarizePayload(decisionType string, payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	switch decisionType {
	case "route":
		if tier, ok := payload["tier"].(string); ok {
			return "tier=" + tier
		}
	case "dispatch":
		role, _ := payload["target_role"].(string)
		name, _ := payload["agent_name"].(string)
		if role != "" && name != "" {
			return name + " <- " + role
		}
	case "escalate":
		if axis, ok := payload["axis"].(string); ok {
			return "axis=" + axis
		}
	case "ratify":
		if axis, ok := payload["axis"].(string); ok {
			return "axis=" + axis
		}
	case "apply":
		if id, ok := payload["recommendation_id"].(string); ok {
			return id
		}
	}
	// Fallback: compact JSON.
	if b, err := json.Marshal(payload); err == nil {
		return string(b)
	}
	return ""
}
```

- [ ] **Step 4: Register subcommand in `cmd/darken/main.go`**

Add to `subcommands` slice:
```go
{"history", "tabular view of .scion/audit.jsonl", runHistory},
```

- [ ] **Step 5: Run tests, verify they pass**

```bash
go test -run TestHistory ./cmd/darken/... -count=1
```

Expected: 5/5 PASS.

- [ ] **Step 6: Lint**

```bash
go vet ./... && gofmt -l cmd/
```

- [ ] **Step 7: Commit**

```bash
git add cmd/darken/history.go cmd/darken/history_test.go cmd/darken/main.go
git commit -m "feat(cli): darken history reads .scion/audit.jsonl

Reads the audit log from CWD's .scion/audit.jsonl, prints a tabular
summary or raw JSONL. Filters: --last N (most-recent N), --since DUR
(Go duration), --format text|json.

Schema documented in docs/AUDIT_LOG_SCHEMA.md. Malformed entries skip
with a stderr warning; the reader is permissive."
```

---

### Task 4: Final verification + push + PR

- [ ] **Step 1: Full suite**

```bash
go test ./... -count=1
```

Expected: all 3 packages green; ~8 new tests across dashboard + history.

- [ ] **Step 2: Lint**

```bash
go vet ./... && gofmt -l cmd/ internal/
```

- [ ] **Step 3: Build + smoke**

```bash
make darken
bin/darken --help                              # confirm dashboard + history visible
bin/darken history                             # if .scion/audit.jsonl exists, prints table; else friendly error
```

For a manual dashboard smoke (only if scion server is running):

```bash
bin/darken dashboard    # opens browser to http://localhost:8080
```

- [ ] **Step 4: Drift guard**

```bash
bash scripts/test-embed-drift.sh
```

PASS expected (Phase 8 doesn't touch internal/substrate/data/).

- [ ] **Step 5: Push + PR**

```bash
git push -u origin feat/darken-phase-8
gh pr create --repo danmestas/darken \
  --title "Phase 8: routine observability — darken dashboard + darken history" \
  --body "..."  # operator runs from the plan's PR body in step 6
```

PR body should:
- Cite the spec section (Phase 8 in `2026-04-28-darken-DX-roadmap-design.md`)
- List both new subcommands with one-line descriptions
- Link to `docs/AUDIT_LOG_SCHEMA.md`
- Operator action: tag v0.1.8

---

## Done definition

Phase 8 ships when:

1. `go test ./... -count=1` — all green; ~8 new tests
2. `bin/darken dashboard` opens scion's web UI on macOS + Linux (verified manually)
3. `bin/darken history` reads `.scion/audit.jsonl` and prints a tabular summary
4. `bin/darken history --last 5` shows the most-recent 5 entries
5. `bin/darken history --since 1h` shows entries from the last hour
6. `bin/darken history --format json` prints raw JSONL
7. `docs/AUDIT_LOG_SCHEMA.md` documents the file format
8. `bash scripts/test-embed-drift.sh` PASS
9. PR open, CI green

## What Phase 9 picks up

- `darken redispatch <agent>` — kill + re-spawn with same task (recovery primitive)
- Substrate-hash drift detection in `darken doctor`
- `darken upgrade-init` convenience wrapper for post-`brew upgrade` cleanup
- Worker auto-redispatch policy in the orchestrator-mode skill (no Go change)

## Risks / open questions

1. **Hardcoded dashboard URL** — `http://localhost:8080` is the default but operators with custom port configs would need an override. Phase 9+ could parse `~/.scion/settings.yaml` server.web.port if we hit a real case. Out of scope for Phase 8.

2. **Audit log doesn't exist yet** — no orchestrator runs have happened in earnest, so `darken history` may often print "no audit entries" until the orchestrator-mode skill is exercised against a real pipeline. Acceptable: the empty-state handling is the operator's first signal that the loop hasn't been run.

3. **`scion server status` fails when daemon down** — `darken dashboard` correctly errors with a remediation hint. Operators see "scion server not running (run `scion server start --workstation`)".

4. **`darken history --since` with timezone** — `time.ParseDuration` is timezone-agnostic; we compute `time.Now().Add(-dur)` in the local TZ and compare against parsed RFC3339 (UTC by convention). If audit timestamps are local-TZ instead of UTC, the comparison still works (both point to the same instant). Should be fine; flag if operators report unexpected exclusions.
