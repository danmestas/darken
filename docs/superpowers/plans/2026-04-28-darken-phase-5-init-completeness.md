# Darken Phase 5 — Init Completeness + Spawn Resolver + Operator Feedback

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `darken init <repo>` actually finish setting up a working repo for orchestrator-mode end-to-end. Today the command writes a CLAUDE.md that *references* skills the working repo doesn't have, asks `darken spawn` to run scripts the working repo doesn't carry, and gives the operator no visible signal that the session is in orchestrator mode. After Phase 5: `darken init <fresh-repo> && cd <fresh-repo> && claude code` opens a session that auto-loads the orchestrator skill, has bones workspace bootstrapped, shows a clear status line, and can `darken spawn` workers without erroring on missing files.

**Architecture:** Extend the substrate resolver pattern from Phases 1+2 to script execution (`spawn.go`, `bootstrap.go`). Extend `darken init` to extract embedded skills + write a Claude Code statusLine config + run `bones init` when available + append .gitignore entries. Add a `darken status` subcommand that backs the statusLine. Fix one stale Makefile path (agent-infra → bones rename).

**Tech Stack:** Go 1.23+, stdlib only. Reuses `internal/substrate.Resolver` + `embed.FS` from Phase 2. No new dependencies.

---

## File structure

### Created
- `cmd/darken/status.go` — `runStatus` for the statusLine command
- `cmd/darken/status_test.go` — covers output format

### Modified
- `cmd/darken/spawn.go` — `runShell` calls swap to substrate-resolver-backed script extraction
- `cmd/darken/bootstrap.go` — same swap for the script execution paths
- `cmd/darken/init.go` — scaffolds skills, runs bones init, writes statusLine config, appends gitignore
- `cmd/darken/init_test.go` — covers the new scaffolds
- `cmd/darken/main.go` — registers `status` subcommand
- `internal/substrate/data/templates/CLAUDE.md.tmpl` — directive language so Claude announces orchestrator mode on first response
- `images/Makefile` — `AGENT_INFRA_PATH` default points at `$(HOME)/projects/bones`; rename references
- `internal/substrate/data/` — regenerated via `make sync-embed-data`

### NOT modified
- The harness templates under `.scion/templates/<role>/` — Phase 5 is operator-side init/spawn polish, not a worker behavior change.

---

## Tasks

### Task 1: Substrate-resolver-backed script execution in `spawn.go`

**Why:** `runShell(filepath.Join(root, "scripts", "stage-creds.sh"), ...)` assumes the working repo carries substrate scripts. New repos don't. Extract from embedded substrate at runtime.

**Files:**
- Modify: `cmd/darken/spawn.go`
- Create: `cmd/darken/script_runner.go` — shared helper for "extract embedded script to temp, exec, clean up"
- Create: `cmd/darken/script_runner_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/darken/script_runner_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSubstrateScript_ExtractsAndExecs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Provide a project-layer override so the resolver finds the script.
	dir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptBody := "#!/bin/sh\necho hello-from-stub\n"
	scriptPath := filepath.Join(dir, "demo-script.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// runSubstrateScript should find the script via the resolver, write it
	// to a temp file with exec permissions, and run it. The body should
	// flow back to the caller via stdout.
	out, err := runSubstrateScriptCaptured("scripts/demo-script.sh", []string{})
	if err != nil {
		t.Fatalf("runSubstrateScript failed: %v", err)
	}
	if !strings.Contains(out, "hello-from-stub") {
		t.Fatalf("expected stub output, got %q", out)
	}
}

func TestRunSubstrateScript_FailsCleanly_OnMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	err := runSubstrateScript("scripts/no-such-script-anywhere.sh", []string{})
	if err == nil {
		t.Fatal("expected error for non-existent script")
	}
}
```

(Note: `runSubstrateScriptCaptured` is a test-only helper; production `runSubstrateScript` writes to stdout/stderr directly. Test helper captures via tempfile redirection. Optional — can also do `runSubstrateScript` test by checking exit status of a known-true script body and a known-false body.)

- [ ] **Step 2: Run the test, verify it fails**

```
cd cmd/darken && go test -run TestRunSubstrateScript -count=1
```

Expected: undefined symbols.

- [ ] **Step 3: Write the implementation**

Create `cmd/darken/script_runner.go`:

```go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runSubstrateScript reads a substrate-relative script via the resolver,
// extracts it to a temp file with exec permissions, runs it with the
// given args, and cleans up the temp file. Stdout and stderr are
// inherited so the user sees script progress in-place.
//
// Path is substrate-relative (e.g. "scripts/stage-creds.sh"), not an
// OS path. The resolver layers project-local → user override → embedded.
func runSubstrateScript(substratePath string, args []string) error {
	body, err := substrateResolver().ReadFile(substratePath)
	if err != nil {
		return fmt.Errorf("substrate script %s: %w", substratePath, err)
	}

	tmp, err := os.CreateTemp("", "darken-script-*.sh")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}

	c := exec.Command("bash", append([]string{tmp.Name()}, args...)...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// runSubstrateScriptCaptured is the test-only variant that returns stdout
// as a string. Production callers use runSubstrateScript above.
func runSubstrateScriptCaptured(substratePath string, args []string) (string, error) {
	body, err := substrateResolver().ReadFile(substratePath)
	if err != nil {
		return "", fmt.Errorf("substrate script %s: %w", substratePath, err)
	}

	tmp, err := os.CreateTemp("", "darken-script-*.sh")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", err
	}

	out, err := exec.Command("bash", append([]string{tmp.Name()}, args...)...).CombinedOutput()
	return string(out), err
}

// _ silences "unused" if the build uses only one form.
var _ = filepath.Join
```

- [ ] **Step 4: Run the test, verify it passes**

```
go test -run TestRunSubstrateScript -count=1 ./cmd/darken
```

- [ ] **Step 5: Wire into spawn.go**

Replace the two `runShell(filepath.Join(root, "scripts", ...))` calls with `runSubstrateScript("scripts/...", ...)`:

```go
// before:
if err := runShell(filepath.Join(root, "scripts", "stage-creds.sh"), "all"); err != nil {
    fmt.Fprintln(os.Stderr, "spawn: stage-creds non-fatal:", err)
}
if err := runShell(filepath.Join(root, "scripts", "stage-skills.sh"), *harnessType); err != nil {
    return fmt.Errorf("stage-skills failed: %w", err)
}

// after:
if err := runSubstrateScript("scripts/stage-creds.sh", []string{"all"}); err != nil {
    fmt.Fprintln(os.Stderr, "spawn: stage-creds non-fatal:", err)
}
if err := runSubstrateScript("scripts/stage-skills.sh", []string{*harnessType}); err != nil {
    return fmt.Errorf("stage-skills failed: %w", err)
}
```

The `root` variable is no longer needed for these two calls; trim if it's not used elsewhere in `runSpawn`.

- [ ] **Step 6: Run all tests + spawn smoke**

```
go test ./... -count=1
go vet ./... && gofmt -l cmd/ internal/
```

- [ ] **Step 7: Commit**

```
fix(spawn): use substrate resolver to extract scripts at runtime

darken spawn called <cwd>/scripts/stage-creds.sh and stage-skills.sh
directly. New repos created via `darken init` don't have a scripts/
dir — those live only in darkish-factory. The Phase 1+2 substrate
resolver was supposed to handle this; spawn.go was never wired through.

New helper runSubstrateScript reads the script body via the resolver
(layered: flag → env → user → project → embedded), writes it to a
temp file with exec perms, runs it, cleans up. Same script body that
shipped with this binary; no host-side dependency on darkish-factory.

Tests cover the success path (script extracted + executed) and miss
path (resolver fails through; clean error).
```

---

### Task 2: Same fix for `bootstrap.go`

**Why:** `bootstrap.go`'s `ensureHubSecrets` and `ensureAllSkillsStaged` also call `runShell(filepath.Join(root, "scripts", ...))`. Same root-cause as Task 1.

**Files:**
- Modify: `cmd/darken/bootstrap.go`

- [ ] **Step 1: Audit the runShell calls in bootstrap.go**

```
grep -nE 'runShell|filepath\.Join.*scripts' cmd/darken/bootstrap.go
```

Expected: 2 sites — `ensureHubSecrets` and `ensureAllSkillsStaged`.

- [ ] **Step 2: Replace with runSubstrateScript**

`ensureHubSecrets`:

```go
func ensureHubSecrets() error {
    return runSubstrateScript("scripts/stage-creds.sh", []string{"all"})
}
```

`ensureAllSkillsStaged`:

```go
func ensureAllSkillsStaged() error {
    root, err := repoRoot()
    if err != nil {
        return err
    }
    dirs, err := os.ReadDir(filepath.Join(root, ".scion", "templates"))
    if err != nil {
        // No project-local templates is fine; embedded substrate provides them.
        // Iterate the embedded template list instead.
        return ensureAllSkillsStagedFromEmbedded()
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
}

// ensureAllSkillsStagedFromEmbedded iterates the embedded .scion/templates/
// list when the working repo has no project-local templates dir.
func ensureAllSkillsStagedFromEmbedded() error {
    entries, err := fs.ReadDir(substrate.EmbeddedFS(), "data/.scion/templates")
    if err != nil {
        return err
    }
    for _, e := range entries {
        if !e.IsDir() || e.Name() == "base" {
            continue
        }
        if err := runSubstrateScript("scripts/stage-skills.sh", []string{e.Name()}); err != nil {
            fmt.Fprintf(os.Stderr, "bootstrap: stage-skills %s failed: %v\n", e.Name(), err)
        }
    }
    return nil
}
```

(Add `io/fs` and substrate imports if not already present.)

- [ ] **Step 3: Tests pass**

```
go test ./... -count=1
```

The existing `TestBootstrapStepsAreOrdered` should still pass — its stubs catch the bash invocation regardless of whether the script comes from CWD or temp.

- [ ] **Step 4: Commit**

```
fix(bootstrap): use substrate resolver for stage-* scripts

ensureHubSecrets + ensureAllSkillsStaged now extract scripts from the
embedded substrate (same pattern as Task 1's spawn.go fix). When the
working repo has no project-local .scion/templates/, iterate the
embedded template list instead of erroring.
```

---

### Task 3: New `darken status` subcommand

**Why:** Backs the statusLine config that Task 5 will scaffold into init'd repos. Outputs a one-line status: orchestrator mode + substrate hash + active worker count. Must be fast (called every prompt by Claude Code).

**Files:**
- Create: `cmd/darken/status.go`
- Create: `cmd/darken/status_test.go`
- Modify: `cmd/darken/main.go` (register subcommand)

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"strings"
	"testing"
)

func TestStatusOutputFormat(t *testing.T) {
	out, err := captureStdout(func() error { return runStatus(nil) })
	if err != nil {
		t.Fatal(err)
	}
	// Format: [darken: orchestrator-mode | substrate <12-hex>]
	if !strings.HasPrefix(out, "[darken:") {
		t.Fatalf("status output missing [darken: prefix: %q", out)
	}
	if !strings.Contains(out, "substrate ") {
		t.Fatalf("status output missing substrate hash: %q", out)
	}
	if !strings.Contains(out, "orchestrator-mode") {
		t.Fatalf("status output missing mode label: %q", out)
	}
}

func TestStatusRejectsArgs(t *testing.T) {
	if err := runStatus([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}
```

- [ ] **Step 2: Implement**

```go
// Package main — `darken status` produces a single-line summary suitable
// for Claude Code's statusLine.command. Must be fast — called every
// prompt. Avoid external commands (no scion list, no docker calls).
package main

import (
	"errors"
	"fmt"

	"github.com/danmestas/darken/internal/substrate"
)

func runStatus(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken status")
	}
	hash := substrate.EmbeddedHash()
	if len(hash) > 12 {
		hash = hash[:12]
	}
	fmt.Printf("[darken: orchestrator-mode | substrate %s]\n", hash)
	return nil
}
```

(Future: append active worker count via `scion list --format json` parse, but only if it stays sub-100ms. Phase 5 ships static.)

- [ ] **Step 3: Register in main.go**

Add to subcommands:
```go
{"status", "print one-line status (statusLine-friendly)", runStatus},
```

- [ ] **Step 4: Tests + build**

```
go test ./cmd/darken/... -count=1
make darken
bin/darken status   # prints "[darken: orchestrator-mode | substrate ...]"
```

- [ ] **Step 5: Commit**

```
feat(cli): add darken status subcommand

Backs the statusLine.command config that `darken init` will scaffold
in subsequent task. Single-line output, fast — must run every prompt.
No external command calls; reads only the embedded substrate hash.
```

---

### Task 4: `darken init` scaffolds skills + bones init + statusLine + gitignore

**Why:** The big init upgrade. After this, `darken init <repo>` produces a fully wired working repo.

**Files:**
- Modify: `cmd/darken/init.go`
- Modify: `cmd/darken/init_test.go`
- Modify: `internal/substrate/data/templates/CLAUDE.md.tmpl` (more directive)

- [ ] **Step 1: Update CLAUDE.md template — be directive about announcing**

In `internal/substrate/data/templates/CLAUDE.md.tmpl`, add a new section near the top:

```
## On every session start

Your VERY FIRST output to the operator must be the line:

> 🔧 **darken orchestrator-mode primed.** substrate `{{.SubstrateHash12}}` · workers ready · awaiting intent.

Then load and apply the `orchestrator-mode` skill before responding to
the operator's actual task. The skill body lives at
`.claude/skills/orchestrator-mode/SKILL.md` (scaffolded by `darken init`).

This announcement is the operator's only signal that the session is
in orchestrator mode. Don't skip it.
```

(The `{{.SubstrateHash12}}` field gets passed in by `renderCLAUDE`. Update the template-rendering code to inject it.)

- [ ] **Step 2: Update renderCLAUDE in init.go**

```go
data := struct {
    RepoName        string
    SubstrateHash12 string
}{
    RepoName:        filepath.Base(targetDir),
    SubstrateHash12: firstN(substrate.EmbeddedHash(), 12),
}
```

(Add a `firstN(s string, n int) string` helper somewhere.)

- [ ] **Step 3: Add scaffolding helpers**

In `init.go`, add four new helpers called after writing CLAUDE.md:

```go
// scaffoldSkill copies a skill from the embedded substrate into
// .claude/skills/<name>/SKILL.md so Claude Code's project-local skill
// discovery picks it up.
func scaffoldSkill(targetDir, name string) error {
    body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/"+name+"/SKILL.md")
    if err != nil {
        return err
    }
    dst := filepath.Join(targetDir, ".claude", "skills", name, "SKILL.md")
    if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
        return err
    }
    return os.WriteFile(dst, body, 0o644)
}

// scaffoldStatusLine writes .claude/settings.local.json with a
// statusLine.command pointing at `darken status`. If the file already
// exists, leaves it alone (don't clobber other settings).
func scaffoldStatusLine(targetDir string) error {
    path := filepath.Join(targetDir, ".claude", "settings.local.json")
    if _, err := os.Stat(path); err == nil {
        return nil // existing settings; don't clobber
    }
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    body := `{
  "statusLine": {
    "command": "darken status",
    "type": "command"
  }
}
`
    return os.WriteFile(path, []byte(body), 0o644)
}

// scaffoldGitignore appends darken-related entries to <target>/.gitignore.
// Idempotent — only appends entries not already present.
func scaffoldGitignore(targetDir string) error {
    path := filepath.Join(targetDir, ".gitignore")
    var existing []byte
    if b, err := os.ReadFile(path); err == nil {
        existing = b
    }
    entries := []string{
        "# darken: scion runtime + per-spawn worktrees + claude-code worktrees",
        ".scion/agents/",
        ".scion/skills-staging/",
        ".scion/audit.jsonl",
        ".claude/worktrees/",
        ".claude/settings.local.json",
        ".superpowers/",
    }
    var add []string
    for _, e := range entries {
        if !strings.Contains(string(existing), e) {
            add = append(add, e)
        }
    }
    if len(add) == 0 {
        return nil
    }
    f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return err
    }
    defer f.Close()
    if len(existing) > 0 && existing[len(existing)-1] != '\n' {
        f.WriteString("\n")
    }
    for _, e := range add {
        f.WriteString(e + "\n")
    }
    return nil
}

// runBonesInit shells out to `bones init` in the target dir if bones is
// on PATH. Soft-fail: bones being missing is not fatal — operator
// without bones still gets a usable orchestrator session.
func runBonesInit(targetDir string) error {
    if _, err := exec.LookPath("bones"); err != nil {
        return nil // soft-fail; bones not on PATH
    }
    c := exec.Command("bones", "init")
    c.Dir = targetDir
    c.Stdout = os.Stdout
    c.Stderr = os.Stderr
    return c.Run()
}
```

- [ ] **Step 4: Wire all four into runInit**

In `runInit`, after `os.WriteFile(claudePath, ...)`:

```go
// Scaffold skills (project-local copies of the embedded host-mode skills)
for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
    if err := scaffoldSkill(target, skill); err != nil {
        fmt.Fprintf(os.Stderr, "init: skill scaffold %s failed: %v\n", skill, err)
    } else {
        fmt.Printf("scaffolded .claude/skills/%s/SKILL.md\n", skill)
    }
}

// Scaffold statusLine + gitignore
if err := scaffoldStatusLine(target); err != nil {
    fmt.Fprintf(os.Stderr, "init: statusLine scaffold failed: %v\n", err)
} else {
    fmt.Println("scaffolded .claude/settings.local.json")
}
if err := scaffoldGitignore(target); err != nil {
    fmt.Fprintf(os.Stderr, "init: .gitignore append failed: %v\n", err)
} else {
    fmt.Println("appended darken entries to .gitignore")
}

// bones init (soft-fail if bones not on PATH)
if err := runBonesInit(target); err != nil {
    fmt.Fprintf(os.Stderr, "init: bones init failed: %v\n", err)
} else if _, err := exec.LookPath("bones"); err == nil {
    fmt.Println("ran `bones init` for workspace bootstrap")
}
```

- [ ] **Step 5: Update tests**

Add to `init_test.go`:

```go
func TestInitScaffoldsSkills(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("DARKEN_REPO_ROOT", tmp)
    if err := runInit([]string{tmp}); err != nil {
        t.Fatal(err)
    }
    for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
        path := filepath.Join(tmp, ".claude", "skills", skill, "SKILL.md")
        if _, err := os.Stat(path); err != nil {
            t.Fatalf("skill %s not scaffolded: %v", skill, err)
        }
    }
}

func TestInitWritesStatusLineSettings(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("DARKEN_REPO_ROOT", tmp)
    if err := runInit([]string{tmp}); err != nil {
        t.Fatal(err)
    }
    body, err := os.ReadFile(filepath.Join(tmp, ".claude", "settings.local.json"))
    if err != nil {
        t.Fatalf("settings.local.json not created: %v", err)
    }
    if !strings.Contains(string(body), `"command": "darken status"`) {
        t.Fatalf("settings missing statusLine.command: %s", body)
    }
}

func TestInitAppendsGitignore(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("DARKEN_REPO_ROOT", tmp)
    // Plant a pre-existing .gitignore to confirm append (not overwrite).
    os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("# pre-existing\n*.log\n"), 0o644)

    if err := runInit([]string{tmp}); err != nil {
        t.Fatal(err)
    }
    body, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
    if !strings.Contains(string(body), "# pre-existing") {
        t.Fatalf("init clobbered existing .gitignore: %s", body)
    }
    if !strings.Contains(string(body), ".scion/agents/") {
        t.Fatalf("init didn't append darken entries: %s", body)
    }
}

func TestInitSecondRunIsIdempotent(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("DARKEN_REPO_ROOT", tmp)
    if err := runInit([]string{tmp}); err != nil {
        t.Fatal(err)
    }
    body1, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
    if err := runInit([]string{tmp}); err != nil {
        t.Fatal(err)
    }
    body2, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
    if string(body1) != string(body2) {
        t.Fatalf("second init mutated .gitignore (not idempotent):\nwas: %q\nnow: %q", body1, body2)
    }
}
```

- [ ] **Step 6: Smoke + commit**

```
go test ./cmd/darken/... -count=1
make darken
bin/darken init /tmp/init-smoke
ls -la /tmp/init-smoke/.claude/skills/  # both skills present
cat /tmp/init-smoke/.claude/settings.local.json
cat /tmp/init-smoke/.gitignore
rm -rf /tmp/init-smoke
```

Commit:
```
feat(init): scaffold skills + bones init + statusLine + gitignore

darken init now produces a fully-wired working repo:
- .claude/skills/orchestrator-mode/SKILL.md (extracted from embed)
- .claude/skills/subagent-to-subharness/SKILL.md (extracted from embed)
- .claude/settings.local.json (statusLine → darken status)
- .gitignore appends (.scion/agents/, .scion/skills-staging/,
  .scion/audit.jsonl, .claude/worktrees/, .superpowers/)
- bones init in the target dir (soft-fail if bones not on PATH)

CLAUDE.md template now directs Claude to announce orchestrator mode
on first response. The substrate hash is rendered into the announce
line so operators see at a glance which substrate version is loaded.

All scaffolding is idempotent — re-running darken init is safe.
```

---

### Task 5: Fix Makefile path (agent-infra → bones)

**Why:** `images/Makefile:17` defaults `AGENT_INFRA_PATH ?= $(HOME)/projects/agent-infra`. The repo was renamed to `bones` (per agent-infra git history). Fresh clones don't have agent-infra; `make prebuild-bones` errors with "no such file."

**Files:**
- Modify: `images/Makefile`

- [ ] **Step 1: Change default path**

```
- AGENT_INFRA_PATH ?= $(HOME)/projects/agent-infra
+ BONES_SOURCE_PATH ?= $(HOME)/projects/bones
```

- [ ] **Step 2: Rename references in `prebuild-bones` target**

```
cd $(BONES_SOURCE_PATH) && \
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/claude/bin/bones ./cmd/bones && \
  ...
```

(All four backends.)

- [ ] **Step 3: Sync embed data + drift guard**

```
make sync-embed-data
bash scripts/test-embed-drift.sh
```

- [ ] **Step 4: Smoke test prebuild from clean state**

```
rm -rf images/{claude,codex,pi,gemini}/bin
make -C images prebuild-bones
ls images/claude/bin/bones  # should exist
```

- [ ] **Step 5: Commit**

```
fix(images/Makefile): rename AGENT_INFRA_PATH → BONES_SOURCE_PATH

agent-infra was renamed to bones upstream. The default path
$(HOME)/projects/agent-infra no longer exists on fresh machines.
Updates the variable name + default to track the rename.

Operators with a non-default path keep their existing override:
  make -C images prebuild-bones BONES_SOURCE_PATH=/some/other/path
```

---

### Task 6: Final verification + push + PR

- [ ] **Full test suite**
- [ ] **Lint** (`go vet`, `gofmt`)
- [ ] **Build + manual smoke**:
  - `bin/darken status` prints status line
  - `bin/darken init /tmp/init-test` produces all expected files
  - `bin/darken spawn ...` against /tmp/init-test (with bones on PATH for `bones init`) goes further than before — at minimum gets past the stage-creds + stage-skills steps without "no such script" errors
- [ ] **Drift guard**: `bash scripts/test-embed-drift.sh` PASS
- [ ] **Push branch + open PR**

PR body should:
- Cite the live edgesync-fslite session that surfaced the bugs
- Operator action items: tag v0.1.4 after merge to ship the fix to brew users
- Backward-compat note: existing repos created via prior `darken init` will be missing `.claude/skills/`; operators should `darken init --force` to refresh OR run a new helper if Phase 5 ships one (defer to Phase 6 if needed).

---

## Done definition

Phase 5 ships when:

1. All Go tests pass (`go test ./... -count=1`)
2. `bin/darken init /tmp/fresh-repo` produces:
   - CLAUDE.md (with substrate hash injected)
   - `.claude/skills/orchestrator-mode/SKILL.md`
   - `.claude/skills/subagent-to-subharness/SKILL.md`
   - `.claude/settings.local.json` with statusLine → `darken status`
   - `.gitignore` with darken entries appended
   - `bones init`'d if bones is on PATH
3. `bin/darken spawn ...` from a fresh-init'd repo extracts scripts from embedded substrate; doesn't error on missing CWD scripts
4. `bin/darken status` prints `[darken: orchestrator-mode | substrate <12-hex>]`
5. `bash scripts/test-embed-drift.sh` PASS
6. `make -C images prebuild-bones` works with the corrected path
7. PR open, CI green, ready for review

## Operator validation post-merge

After tag `v0.1.4` (which Phase 5 should also propose):

```bash
brew upgrade darken
darken version  # v0.1.4

# Test the full flow on a fresh repo
mkdir -p /tmp/darken-test
cd /tmp/darken-test
git init
darken init .
ls -la .claude/skills/  # should show both skills
cat CLAUDE.md | grep "darken orchestrator-mode primed"  # should match

# Open Claude Code
claude code
# First response should start with "🔧 darken orchestrator-mode primed..."

# Status line at the bottom of the Claude Code UI should show:
# [darken: orchestrator-mode | substrate ee346ff63f0a]

# Try a spawn
# > "have a researcher produce a brief on /tmp/darken-test/README.md"
# Should NOT error with "no such script" — should reach scion broker
```

## What Phase 6 might pick up

- A `darken init --refresh` flag for existing repos to pull in new scaffolding (skills, settings, gitignore) without overwriting CLAUDE.md
- `darken doctor` detection of the "init was run on an old version" case
- Active worker count in `darken status` (if scion list stays fast)
- Bones-side skill bundling — currently bones doesn't ship discoverable skills; if it does, scaffold those too
