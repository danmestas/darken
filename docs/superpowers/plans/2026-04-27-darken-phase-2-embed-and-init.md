# Darken Phase 2 — Embed Substrate + `darken init` + `darken version`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `darken` binary fully self-contained — templates, scripts, Dockerfiles, and the two host-mode skills are embedded via `//go:embed` and become resolver layer 5 (the always-present fallback). Add `darken init` to scaffold a CLAUDE.md in any working repo. Add `darken version` to surface the binary version + embedded substrate hash.

**Architecture:** A new `internal/substrate/embed.go` file with `//go:embed` directives + an `EmbeddedFS()` helper. The `Resolver` gains a fifth layer that's checked when all four override layers miss. `MissError` is no longer the terminal state — Phase 1's seam pays off here.

**Tech Stack:** Go 1.23+, stdlib only. New `go:embed` usage. No new module dependencies.

---

## File structure

### Created
- `internal/substrate/embed.go` — `//go:embed` directives + `EmbeddedFS()` exposing the `embed.FS`. Computes a SHA-256 hash of the embedded tree at init time (or lazily) for `darken version`.
- `internal/substrate/embed_test.go` — verifies a known file (e.g. `.scion/templates/researcher/scion-agent.yaml`) is embedded.
- `cmd/darken/init.go` — `runInit` subcommand: scaffolds `<path>/CLAUDE.md`, `<path>/.darken/config.yaml`, appends gitignore entries.
- `cmd/darken/init_test.go` — covers idempotency, `--dry-run`, `--force`.
- `cmd/darken/version.go` — `runVersion` prints `darken vX.Y.Z (substrate sha256:<first-12-of-hash>)`.
- `cmd/darken/version_test.go` — basic output assertion.
- `internal/substrate/templates/CLAUDE.md.tmpl` — the CLAUDE.md template embedded into the binary.

### Modified
- `internal/substrate/resolver.go` — `Resolver` accepts an optional `embed.FS` (or new `WithEmbedded` option) and falls through to it on miss.
- `internal/substrate/resolver_test.go` — adds an embedded-fallback test.
- `cmd/darken/main.go` — register `init` and `version` subcommands.
- `cmd/darken/orchestrate.go` — falls through to embedded `.claude/skills/orchestrator-mode/SKILL.md` when both project copy and `~/projects/agent-skills/` are absent.

### Embedded sources

```
//go:embed .scion/templates
//go:embed scripts/stage-creds.sh scripts/stage-skills.sh scripts/spawn.sh scripts/bootstrap.sh
//go:embed images/Makefile images/claude/Dockerfile images/claude/darkish-prelude.sh
//go:embed images/codex/Dockerfile images/codex/darkish-prelude.sh
//go:embed images/pi/Dockerfile images/pi/darkish-prelude.sh
//go:embed images/gemini/Dockerfile images/gemini/darkish-prelude.sh
//go:embed .claude/skills/orchestrator-mode/SKILL.md
//go:embed .claude/skills/subagent-to-subharness/SKILL.md
//go:embed internal/substrate/templates/CLAUDE.md.tmpl
```

(Adjust if Go embed prohibits embedding from outside the package. If it does, mirror the files into `internal/substrate/data/...` and embed from there. The Phase 2 implementer must verify which path Go accepts.)

**NOT embedded:**
- Worker-side skills under `~/projects/agent-skills/skills/<name>/` (ousterhout, hipp, dx-audit, tigerstyle, caveman, etc.) — these stay APM-resolved per harness manifests
- Bones binaries (`images/<backend>/bin/`) — pre-built on the host, not in the binary
- Per-project state (`.scion/agents/`, `.scion/audit.jsonl`)

---

## Tasks

### Task 1: `internal/substrate/embed.go` + `EmbeddedFS()`

**Goal:** Add the `//go:embed` directives and expose the embedded filesystem.

**Files:**
- Create: `internal/substrate/embed.go`
- Create: `internal/substrate/embed_test.go`

**Caveat:** Go's `//go:embed` cannot reach files outside the package's directory tree. If the directives can't reach `../../.scion/templates/`, the implementer must:
1. Either move the substrate sources under `internal/substrate/data/` (heavier — duplicates the files)
2. Or use a `go:generate` build step that copies them at build time
3. Or restructure the project so substrate sources live at the binary's package level (highest churn, not preferred)

**Implementer's first job:** verify which approach Go accepts and document the choice in `embed.go`'s header comment.

- [ ] **Step 1: Verify the embed approach**

```bash
# Try a minimal directive first to see if cross-package embed works.
cat > internal/substrate/embed_probe.go <<'EOF'
package substrate
import _ "embed"
//go:embed ../../.scion/templates/researcher/scion-agent.yaml
var probe string
EOF
go build ./internal/substrate
rm internal/substrate/embed_probe.go
```

If that fails (Go forbids `..` in embed paths — it does), pivot to option 1: copy substrate sources into `internal/substrate/data/`.

- [ ] **Step 2: Mirror substrate into the package**

Create `internal/substrate/data/` and a `Makefile` target `make sync-embed-data` that rsyncs the substrate sources into it:

```makefile
.PHONY: sync-embed-data
sync-embed-data:
	rm -rf internal/substrate/data
	mkdir -p internal/substrate/data
	cp -R .scion internal/substrate/data/
	cp -R scripts internal/substrate/data/
	cp -R images internal/substrate/data/
	cp -R .claude/skills/orchestrator-mode internal/substrate/data/skills/
	cp -R .claude/skills/subagent-to-subharness internal/substrate/data/skills/
	# Strip per-harness staged skills (regenerable)
	rm -rf internal/substrate/data/.scion/skills-staging
	# Strip per-harness bin/ dirs (gitignored, regenerable)
	rm -rf internal/substrate/data/images/*/bin
```

Add `internal/substrate/data/` to `.gitignore` so the synced tree isn't versioned (it's regenerable from the substrate source-of-truth at the project root).

Wait — that's wrong. If the data isn't versioned, `go install` and CI won't have it. The synced tree MUST be committed for `go install` to work.

Revise: commit `internal/substrate/data/` to git. Add a CI check that `make sync-embed-data` produces no diff (so we catch drift between the canonical sources and the embedded copies).

- [ ] **Step 3: Write the failing test**

`internal/substrate/embed_test.go`:

```go
package substrate

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbedded_ContainsResearcherManifest(t *testing.T) {
	f, err := EmbeddedFS().Open("data/.scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatalf("embedded researcher manifest missing: %v", err)
	}
	defer f.Close()
	info, _ := f.Stat()
	if info.Size() == 0 {
		t.Fatal("embedded researcher manifest is empty")
	}
}

func TestEmbedded_ContainsAllRoles(t *testing.T) {
	want := []string{
		"orchestrator", "researcher", "designer",
		"planner-t1", "planner-t2", "planner-t3", "planner-t4",
		"tdd-implementer", "verifier", "reviewer",
		"sme", "admin", "darwin",
	}
	for _, role := range want {
		path := "data/.scion/templates/" + role + "/scion-agent.yaml"
		_, err := fs.Stat(EmbeddedFS(), path)
		if err != nil {
			t.Errorf("role %s manifest missing from embed: %v", role, err)
		}
	}
}

func TestEmbedded_ContainsHostSkills(t *testing.T) {
	for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
		path := "data/skills/" + skill + "/SKILL.md"
		body, err := fs.ReadFile(EmbeddedFS(), path)
		if err != nil {
			t.Errorf("skill %s missing: %v", skill, err)
			continue
		}
		if !strings.Contains(string(body), "name: "+skill) {
			t.Errorf("skill %s body missing frontmatter name", skill)
		}
	}
}

func TestEmbedded_HasStableHash(t *testing.T) {
	h := EmbeddedHash()
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars: %q", len(h), h)
	}
}
```

- [ ] **Step 4: Run tests, verify they fail**

```bash
go test ./internal/substrate/ -run TestEmbedded -count=1
```

Expected: `EmbeddedFS` / `EmbeddedHash` undefined.

- [ ] **Step 5: Implement embed.go**

```go
// Package substrate (continued).
//
// Embedded substrate is the always-present fallback layer of the
// resolver chain. The data/ tree is a mirror of the project's
// .scion/templates/, scripts/, images/, and host-mode skills,
// regenerated by `make sync-embed-data`. CI guards against drift.
package substrate

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sync"
)

//go:embed data
var embeddedFS embed.FS

// EmbeddedFS returns the binary's embedded substrate as an fs.FS.
// Paths are rooted at "data/" — e.g. "data/.scion/templates/researcher/scion-agent.yaml".
func EmbeddedFS() fs.FS {
	return embeddedFS
}

var (
	hashOnce sync.Once
	hashStr  string
)

// EmbeddedHash returns a SHA-256 hex digest of the embedded tree's
// concatenated contents (path + body, sorted by path). Stable across
// builds of the same source. Surfaced by `darken version`.
func EmbeddedHash() string {
	hashOnce.Do(func() {
		h := sha256.New()
		_ = fs.WalkDir(embeddedFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			body, _ := fs.ReadFile(embeddedFS, path)
			h.Write([]byte(path))
			h.Write([]byte{0})
			h.Write(body)
			return nil
		})
		hashStr = hex.EncodeToString(h.Sum(nil))
	})
	return hashStr
}
```

- [ ] **Step 6: Run sync + tests**

```bash
make sync-embed-data
git add internal/substrate/data
go test ./internal/substrate/... -count=1
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```
feat(substrate): embed substrate + EmbeddedHash via go:embed

internal/substrate/data/ mirrors the canonical sources (templates,
scripts, Dockerfiles, host-mode skills); regenerated via
`make sync-embed-data`. EmbeddedFS() exposes it as an fs.FS;
EmbeddedHash() returns a stable SHA-256 of the tree for use in
`darken version`.

Tests verify all 13 role manifests + both host-mode skills are present
in the embed.
```

---

### Task 2: Wire embedded layer into Resolver

**Files:**
- Modify: `internal/substrate/resolver.go`
- Modify: `internal/substrate/resolver_test.go`

- [ ] **Step 1: Write the failing test**

Append to resolver_test.go:

```go
func TestResolver_FallsThroughToEmbedded(t *testing.T) {
	// No overrides set; Phase 1 would have returned MissError. Phase 2
	// must fall through to the embedded substrate.
	r := New(Config{})
	body, err := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatalf("expected fallback to embedded; got error: %v", err)
	}
	if !strings.Contains(string(body), "default_harness_config:") {
		t.Fatalf("embedded researcher manifest looks wrong: %q", string(body)[:50])
	}
}

func TestResolver_LookupReportsEmbeddedLayer(t *testing.T) {
	r := New(Config{})
	_, layer, err := r.Lookup(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if layer != "embedded" {
		t.Fatalf("expected layer=embedded, got %q", layer)
	}
}
```

(Also import "strings" if not already.)

- [ ] **Step 2: Implement the embedded layer**

In resolver.go, modify `resolve` and `Lookup` to fall through to the embedded FS when no override layer matches. The key challenge: embedded paths are rooted at `data/`, so the resolver needs to translate `name` → `data/<name>` for the embedded lookup.

```go
// resolve walks override layers; on miss, falls through to embedded.
func (r *Resolver) resolve(name string) (string, error) {
	for _, c := range r.candidates(name) {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, nil
		}
	}
	// Embedded layer (always present). Returned "path" is a sentinel
	// since the file lives in the embed.FS, not the host filesystem.
	embedPath := "data/" + name
	if _, err := fs.Stat(EmbeddedFS(), embedPath); err == nil {
		return embedSentinel(name), nil
	}
	return "", &MissError{Name: name}
}

// embedSentinel returns a virtual path identifier for embedded files,
// distinguishable from real fs paths. ReadFile/Open/Stat detect this
// prefix and route to the embed.FS.
func embedSentinel(name string) string {
	return "embed://" + name
}
```

Then `ReadFile`, `Open`, `Stat`, `Lookup` need to detect the sentinel:

```go
func (r *Resolver) ReadFile(name string) ([]byte, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	if rest, ok := strings.CutPrefix(p, "embed://"); ok {
		return fs.ReadFile(EmbeddedFS(), "data/"+rest)
	}
	return os.ReadFile(p)
}
// ... similar for Open, Stat
```

For `Lookup`, the layer name on embedded fall-through is `"embedded"`:

```go
func (r *Resolver) Lookup(name string) (path, layer string, err error) {
	for _, c := range r.candidates(name) {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, c.layer, nil
		}
	}
	embedPath := "data/" + name
	if _, err := fs.Stat(EmbeddedFS(), embedPath); err == nil {
		return embedSentinel(name), "embedded", nil
	}
	return "", "", &MissError{Name: name}
}
```

- [ ] **Step 3: Run tests, verify all pass**

```bash
go test ./internal/substrate/... -count=1 -v
```

Expected: all 7+ tests pass (existing 6 + 2 new).

- [ ] **Step 4: Update existing TestResolver_MissesReturnMissError**

That test asserted `IsMiss(err) == true` for a non-existent path. Phase 2's embedded layer means most reasonable paths now succeed via fallback. Update to use a path that genuinely doesn't exist anywhere:

```go
func TestResolver_MissesReturnMissError(t *testing.T) {
	r := New(Config{})
	_, err := r.ReadFile(".scion/templates/this-role-does-not-exist/scion-agent.yaml")
	// ... rest unchanged
}
```

- [ ] **Step 5: Commit**

```
feat(substrate): add embedded layer as resolver fallback

Resolver now falls through to internal/substrate/data/ (embedded via
go:embed) when no override layer matches. Lookup reports "embedded"
as the layer name. Sentinel path "embed://<name>" routes ReadFile/
Open/Stat to the embed.FS instead of the host filesystem.

MissError is now reserved for paths that don't exist in any layer
INCLUDING embedded — i.e. genuine "this role/script doesn't exist
anywhere" cases.
```

---

### Task 3: `darken version` subcommand

**Files:**
- Create: `cmd/darken/version.go`
- Create: `cmd/darken/version_test.go`
- Modify: `cmd/darken/main.go` (register subcommand)

- [ ] **Step 1: Write the failing test**

`cmd/darken/version_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestVersionPrintsBinaryAndSubstrateHash(t *testing.T) {
	out, err := captureStdout(func() error { return runVersion(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "darken") {
		t.Fatalf("version output missing binary name: %q", out)
	}
	if !strings.Contains(out, "substrate sha256:") {
		t.Fatalf("version output missing substrate hash: %q", out)
	}
}
```

- [ ] **Step 2: Implement version.go**

```go
package main

import (
	"errors"
	"fmt"

	"github.com/danmestas/darken/internal/substrate"
)

// version is overridden at build time via -ldflags="-X main.version=v0.1.0".
// Defaults to "dev" for source-tree builds.
var version = "dev"

func runVersion(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken version")
	}
	hash := substrate.EmbeddedHash()
	if len(hash) > 12 {
		hash = hash[:12]
	}
	fmt.Printf("darken %s (substrate sha256:%s)\n", version, hash)
	return nil
}
```

- [ ] **Step 3: Register in main.go**

Add to the subcommands slice:
```go
{"version", "print binary version + embedded substrate hash", runVersion},
```

- [ ] **Step 4: Update Makefile to inject version**

Modify the `darken` target:
```makefile
DARKEN_VERSION ?= dev
.PHONY: darken
darken:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w -X main.version=$(DARKEN_VERSION)" -o bin/darken ./cmd/darken
```

- [ ] **Step 5: Tests pass**

```bash
go test ./cmd/darken/... -count=1
make darken
bin/darken version  # should print: darken dev (substrate sha256:<12 hex chars>)
```

- [ ] **Step 6: Commit**

```
feat(cli): add darken version subcommand

Prints binary version + first-12-chars of substrate.EmbeddedHash().
Version is overridable at build time via -ldflags="-X main.version=...".
Makefile defaults to "dev"; release builds will inject the git tag.
```

---

### Task 4: `darken init` subcommand

**Files:**
- Create: `cmd/darken/init.go`
- Create: `cmd/darken/init_test.go`
- Create: `internal/substrate/data/templates/CLAUDE.md.tmpl` (template body)
- Modify: `cmd/darken/main.go` (register subcommand)
- Modify: `internal/substrate/embed.go` (ensure templates/ embedded — already covered if data/ is fully embedded)

- [ ] **Step 1: Write the CLAUDE.md template**

`internal/substrate/data/templates/CLAUDE.md.tmpl`:

```markdown
# {{.RepoName}} — Darkish Factory orchestrator mode

This repo is initialized for the Darkish Factory orchestration substrate.
By default, Claude Code in this repo operates as **the orchestrator** for
the §7 pipeline running against this working repo.

**On session start, invoke the `orchestrator-mode` skill** to load the
full §7 loop, role roster, escalation classifier, and host-mode protocol.
Workers spawn as containerized subharnesses via `darken spawn`; their
worktrees live under `.scion/agents/<name>/workspace/` here.

## Quick reference

```bash
darken doctor              # preflight + per-harness checks
darken spawn r1 --type researcher "<task>"    # dispatch a worker
darken list                # see live agents
darken apply               # gate darwin recommendations (post-pipeline)
```

## Substrate

The substrate (templates, scripts, Dockerfiles, the two host-mode skills)
is embedded in `darken` itself. Override per-machine in
`~/.config/darken/overrides/`, per-project in `.scion/templates/<role>/`,
or per-invocation via `--substrate-overrides <path>`.

Run `darken doctor <harness>` to see which substrate layer served any
particular role.
```

- [ ] **Step 2: Write the failing test**

`cmd/darken/init_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitScaffoldsCLAUDE(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not created: %v", err)
	}
	if !strings.Contains(string(body), "orchestrator-mode") {
		t.Fatalf("CLAUDE.md missing orchestrator-mode reference: %q", body)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	// Second init on same dir should not error or duplicate.
	if err := runInit([]string{tmp}); err != nil {
		t.Fatalf("second init should be idempotent, got: %v", err)
	}
}

func TestInitForceOverwrites(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Plant a CLAUDE.md that's not from us.
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# pre-existing"), 0o644)

	// Without --force, second init should leave the existing file alone.
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if !strings.HasPrefix(string(body), "# pre-existing") {
		t.Fatalf("init without --force should not overwrite, got: %q", body)
	}

	// With --force, it should be replaced.
	if err := runInit([]string{"--force", tmp}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if strings.HasPrefix(string(body), "# pre-existing") {
		t.Fatalf("init --force should overwrite pre-existing CLAUDE.md")
	}
}

func TestInitDryRun(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	out, err := captureStdout(func() error { return runInit([]string{"--dry-run", tmp}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would create") {
		t.Fatalf("--dry-run output should mention 'would create': %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "CLAUDE.md")); err == nil {
		t.Fatal("--dry-run should not create files")
	}
}
```

- [ ] **Step 3: Implement init.go**

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/danmestas/darken/internal/substrate"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print actions without executing")
	force := fs.Bool("force", false, "overwrite existing CLAUDE.md")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pos := fs.Args()
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

	if *dryRun {
		if exists && !*force {
			fmt.Printf("would skip %s (already exists; use --force to overwrite)\n", claudePath)
		} else {
			fmt.Printf("would create %s\n", claudePath)
		}
		return nil
	}

	if exists && !*force {
		fmt.Printf("skipped %s (already exists; use --force to overwrite)\n", claudePath)
		return nil
	}

	body, err := renderClaudeMD(target)
	if err != nil {
		return err
	}
	if err := os.WriteFile(claudePath, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", claudePath)
	return nil
}

func renderClaudeMD(targetDir string) (string, error) {
	body, err := readEmbedded("data/templates/CLAUDE.md.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("claude").Parse(string(body))
	if err != nil {
		return "", err
	}
	data := struct{ RepoName string }{
		RepoName: filepath.Base(targetDir),
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func readEmbedded(path string) ([]byte, error) {
	body, err := fsReadEmbedded(path)
	if err == nil {
		return body, nil
	}
	return nil, fmt.Errorf("embedded template not found: %s", path)
}

func fsReadEmbedded(path string) ([]byte, error) {
	return fs.ReadFile(substrate.EmbeddedFS(), path)
}

// errors import is consumed transitively; Go vet flags it as unused if not.
var _ = errors.New
```

(Yes, this aliases stdlib `flag` with the local var name conflicting with the `errors` import — the implementer should clean up the imports / aliasing as needed. The signal here is the structural shape; rename locally as cleanup.)

- [ ] **Step 4: Register in main.go subcommands**

```go
{"init", "scaffold CLAUDE.md in a target dir", runInit},
{"version", "print binary version + embedded substrate hash", runVersion},
```

(version was registered in Task 3; just verify it's still there.)

- [ ] **Step 5: Tests pass**

```bash
go test ./cmd/darken/... -count=1
make darken
bin/darken init --dry-run /tmp/some-target  # should print plans, not write
```

- [ ] **Step 6: Commit**

```
feat(cli): add darken init subcommand

Scaffolds CLAUDE.md in a target directory from the embedded
data/templates/CLAUDE.md.tmpl. Idempotent (skips if exists);
--force overwrites; --dry-run reports actions without writing.

Operators run `darken init <repo>` once per working repo to enable
host-mode orchestrator. The CLAUDE.md tells Claude Code to invoke
the orchestrator-mode skill at session start.
```

---

### Task 5: `darken orchestrate` falls through to embedded skill

**Files:**
- Modify: `cmd/darken/orchestrate.go`
- Modify: `cmd/darken/orchestrate_test.go`

Phase 1's `orchestrate` looked up the skill from project copy then `~/projects/agent-skills/`. Phase 2 adds the embedded copy as the final fallback so a fresh-installed `darken` works without any additional clones.

- [ ] **Step 1: Update orchestrate.go**

Add the embedded layer as the third fallback:

```go
candidates := []string{}
if root, err := repoRoot(); err == nil {
    candidates = append(candidates, filepath.Join(root, ".claude", "skills", "orchestrator-mode", "SKILL.md"))
}
if home, err := os.UserHomeDir(); err == nil {
    candidates = append(candidates, filepath.Join(home, "projects", "agent-skills", "skills", "orchestrator-mode", "SKILL.md"))
}

for _, p := range candidates {
    body, err := os.ReadFile(p)
    if err == nil {
        _, err = os.Stdout.Write(body)
        return err
    }
}

// Phase 2 fallback: embedded
body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
if err == nil {
    _, err = os.Stdout.Write(body)
    return err
}

return fmt.Errorf("orchestrator skill not found in project, agent-skills, or embedded substrate")
```

- [ ] **Step 2: Add a test for embedded fallback**

```go
func TestOrchestrateFallsThroughToEmbedded(t *testing.T) {
	// Set DARKEN_REPO_ROOT to a tmp dir with no project skill, AND
	// HOME to a tmp dir with no agent-skills clone. Embedded fallback
	// must serve the skill.
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "fakehome"))

	out, err := captureStdout(func() error { return runOrchestrate(nil) })
	if err != nil {
		t.Fatalf("expected embedded fallback, got: %v", err)
	}
	if !strings.Contains(out, "Orchestrator mode") {
		t.Fatalf("expected embedded skill body, got: %q", out)
	}
}
```

- [ ] **Step 3: Tests pass**

```bash
go test ./cmd/darken/... -count=1
```

- [ ] **Step 4: Commit**

```
feat(cli): orchestrate falls through to embedded skill

Lookup order is now: project copy → ~/projects/agent-skills/ →
embedded substrate. Fresh `darken` installs work without any
external clone of agent-skills for the host-mode skills.
```

---

### Task 6: `make sync-embed-data` + CI drift guard

**Files:**
- Modify: `Makefile`
- Create: `scripts/test-embed-drift.sh` (or similar)
- Document the contract in CLAUDE.md or a CONTRIBUTING note

- [ ] **Step 1: Add Makefile target**

```makefile
SUBSTRATE_DATA := internal/substrate/data
.PHONY: sync-embed-data
sync-embed-data:
	rm -rf $(SUBSTRATE_DATA)
	mkdir -p $(SUBSTRATE_DATA) $(SUBSTRATE_DATA)/skills
	cp -R .scion $(SUBSTRATE_DATA)/
	cp -R scripts $(SUBSTRATE_DATA)/
	cp -R images $(SUBSTRATE_DATA)/
	cp -R .claude/skills/orchestrator-mode $(SUBSTRATE_DATA)/skills/
	cp -R .claude/skills/subagent-to-subharness $(SUBSTRATE_DATA)/skills/
	mkdir -p $(SUBSTRATE_DATA)/templates
	cp scripts/CLAUDE.md.tmpl $(SUBSTRATE_DATA)/templates/ 2>/dev/null || true
	rm -rf $(SUBSTRATE_DATA)/.scion/skills-staging
	rm -rf $(SUBSTRATE_DATA)/.scion/agents
	rm -rf $(SUBSTRATE_DATA)/images/*/bin
```

(Adjust the cp lines based on where the CLAUDE.md.tmpl actually lives — if it lives directly in `internal/substrate/data/templates/`, no copy needed.)

- [ ] **Step 2: Add a drift test**

`scripts/test-embed-drift.sh`:

```bash
#!/usr/bin/env bash
# Verifies that internal/substrate/data/ is in sync with the canonical
# substrate sources. Run as a CI check + a pre-commit guard.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# Snapshot current state, run sync, diff
BEFORE=$(find internal/substrate/data -type f -exec sha256sum {} \; | sort)
make sync-embed-data >/dev/null
AFTER=$(find internal/substrate/data -type f -exec sha256sum {} \; | sort)

if [[ "${BEFORE}" != "${AFTER}" ]]; then
  echo "FAIL: internal/substrate/data/ is out of sync with sources"
  echo "Run 'make sync-embed-data' and commit the result"
  exit 1
fi
echo "PASS"
```

`chmod +x` it.

- [ ] **Step 3: Run drift test, commit data tree**

```bash
make sync-embed-data
git add internal/substrate/data
bash scripts/test-embed-drift.sh   # confirms no diff
```

- [ ] **Step 4: Commit**

```
chore(build): add sync-embed-data + drift guard

`make sync-embed-data` regenerates internal/substrate/data/ from the
canonical sources. CI runs scripts/test-embed-drift.sh to detect
embedded-vs-canonical skew. Operators run sync-embed-data after
modifying .scion/templates/, scripts/, images/, or the host-mode
skills, and commit the result alongside the source change.
```

---

### Task 7: Final verification

- [ ] **Full test suite**: `go test ./... -count=1` — all packages green
- [ ] **Lint**: `go vet ./...`, `gofmt -l cmd/ internal/`
- [ ] **Build + smoke**: `make darken && bin/darken version` (substrate hash present), `bin/darken init --dry-run /tmp/test`, `bin/darken doctor researcher` (should report `embedded` layer if no project override)
- [ ] **Drift test**: `bash scripts/test-embed-drift.sh` — PASS
- [ ] **Symlink still works**: `darken version` from any cwd
- [ ] **Push + open PR**

---

## Done definition

Phase 2 ships when:

1. All Go tests pass
2. `darken version` reports binary + substrate hash
3. `darken init <path>` scaffolds a CLAUDE.md from the embedded template
4. `darken doctor <role>` against a clean working repo (no `.scion/templates/<role>/`) reports `served from embedded layer`
5. `darken orchestrate` works without `~/projects/agent-skills/` cloned (embedded fallback)
6. `make sync-embed-data` is committed and `scripts/test-embed-drift.sh` PASSes
7. PR open, CI green, ready for review

## What Phase 3 picks up

- `goreleaser` config + GitHub Actions release workflow
- `danmestas/homebrew-tap` repo setup
- Tag `v0.1.0`
- `bin/darken version` reports the real semver instead of `dev`

## What Phase 4 picks up

- darkish-factory itself consumes the released `darken` binary (delete the source-tree symlink)
- README updated with install instructions + new-repo workflow
- Migration note for existing operators
