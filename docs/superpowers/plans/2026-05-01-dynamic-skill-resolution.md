# Dynamic skill resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-role static skill bundling (`skills:` field in role manifests) with operator-selectable named modes loaded from `.scion/modes/<name>.yaml`. Modes can compose via `extends:`. Operator picks at spawn with `--mode <name>`; absence defaults to the role's `default_mode`.

**Architecture:** From `docs/superpowers/specs/2026-05-01-dynamic-skill-resolution.md` (Ousterhout-revised). Mode YAML schema: `description`, `skills[]`, optional `extends`. No `name` field (filename IS the name). No `--skills` ad-hoc flag (one way to specify skills). Single-inheritance composition via `extends:`. Big-bang migration: 14 role-default modes + 1 base mode (`philosophy-base = [hipp, ousterhout]`) used by planner-t2/t3/t4. All other roles get flat mode files.

**Tech Stack:** Go (existing substrate package + `gopkg.in/yaml.v3` already used in the repo). New CLI subcommand surface under `cmd/darken/modes_*.go`.

---

## Current skill bundles (extracted from `.scion/templates/<role>/scion-agent.yaml`)

| Role | Current skills (in order) |
|---|---|
| admin | (empty) |
| base | (empty) |
| darwin | dx-audit |
| designer | norman, ousterhout |
| orchestrator | dx-audit |
| planner-t1 | hipp |
| planner-t2 | hipp, ousterhout |
| planner-t3 | hipp, ousterhout, superpowers |
| planner-t4 | hipp, ousterhout, spec-kit |
| researcher | (empty) |
| reviewer | ousterhout, hipp, dx-audit |
| sme | ousterhout, hipp |
| tdd-implementer | idiomatic-go, tigerstyle |
| verifier | tigerstyle |

Composition opportunity: planner-t2/t3/t4 all start with `[hipp, ousterhout]`. Extract as `philosophy-base`. Others stay flat (their orderings or sets don't match).

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/substrate/modes.go` (new) | Mode YAML schema, parser, recursive resolver | Define `Mode` struct, `LoadMode(name) (*Mode, error)`, `ResolveSkills(name) ([]string, error)` with cycle detection |
| `internal/substrate/modes_test.go` (new) | Unit tests for mode resolver | Flat, extends, multi-level chain, cycle, missing parent, missing skill |
| `.scion/modes/` (new directory) | Mode YAML files | `philosophy-base.yaml` + 14 `<role>.yaml` files (mirrors `.scion/templates/`) |
| `internal/substrate/data/.scion/modes/` (new directory) | Embedded copy for non-darkish-factory projects | Same 15 files; bootstrap extracts these alongside templates when project has no `.scion/modes/` |
| `.scion/templates/<role>/scion-agent.yaml` (×14) | Role manifest | Add `default_mode: <role>`, remove `skills:` field |
| `internal/substrate/data/.scion/templates/<role>/scion-agent.yaml` (×14) | Embedded copy of role manifests | Same edit |
| `cmd/darken/spawn.go` | Spawn CLI parsing + skill staging | Add `--mode <name>` flag; wire mode-resolved skills into staging |
| `cmd/darken/spawn_test.go` | Spawn integration tests | Golden-equivalence test per role (pre/post staging match) |
| `cmd/darken/modes_list.go` (new) | `darken modes list` command | List all `.scion/modes/*.yaml` with descriptions |
| `cmd/darken/modes_show.go` (new) | `darken modes show <name>` command | Print mode YAML + resolved skills |
| `cmd/darken/main.go` | Command dispatcher | Register `modes` subcommand group |

Files NOT changed: any test outside cmd/darken/ and internal/substrate/. Existing skill-staging logic stays — we only change what skill set is FED to it.

---

## Task 1: Mode parser + resolver (TDD)

**Files:**
- Create: `internal/substrate/modes.go`
- Create: `internal/substrate/modes_test.go`

- [ ] **Step 1: Write the unit test (RED)**

Create `internal/substrate/modes_test.go`:

```go
package substrate

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestResolveSkills_FlatMode(t *testing.T) {
	fs := fstest.MapFS{
		"go-systems.yaml": &fstest.MapFile{Data: []byte(`description: Go systems work
skills:
  - tdd
  - idiomatic-go
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "go-systems")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"tdd", "idiomatic-go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestResolveSkills_ExtendsParent(t *testing.T) {
	fs := fstest.MapFS{
		"philosophy.yaml": &fstest.MapFile{Data: []byte(`description: shared philosophy
skills:
  - hipp
  - ousterhout
`)},
		"planner.yaml": &fstest.MapFile{Data: []byte(`description: planner
extends: philosophy
skills:
  - superpowers
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "planner")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"hipp", "ousterhout", "superpowers"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestResolveSkills_DedupesFirstWins(t *testing.T) {
	fs := fstest.MapFS{
		"base.yaml": &fstest.MapFile{Data: []byte(`description: base
skills:
  - hipp
  - ousterhout
`)},
		"override.yaml": &fstest.MapFile{Data: []byte(`description: override
extends: base
skills:
  - ousterhout
  - extra
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "override")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"hipp", "ousterhout", "extra"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestResolveSkills_DetectsCycle(t *testing.T) {
	fs := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`description: a
extends: b
skills: []
`)},
		"b.yaml": &fstest.MapFile{Data: []byte(`description: b
extends: a
skills: []
`)},
	}
	_, err := ResolveSkillsFromFS(fs, "a")
	if err == nil {
		t.Fatal("expected cycle error; got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle: %v", err)
	}
}

func TestResolveSkills_MissingMode(t *testing.T) {
	fs := fstest.MapFS{}
	_, err := ResolveSkillsFromFS(fs, "nope")
	if err == nil {
		t.Fatal("expected not-found error; got nil")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should mention mode name: %v", err)
	}
}

func TestResolveSkills_MissingParent(t *testing.T) {
	fs := fstest.MapFS{
		"child.yaml": &fstest.MapFile{Data: []byte(`description: child
extends: ghost
skills: []
`)},
	}
	_, err := ResolveSkillsFromFS(fs, "child")
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should mention missing parent: %v", err)
	}
}

func TestResolveSkills_MultiLevelChain(t *testing.T) {
	fs := fstest.MapFS{
		"grandparent.yaml": &fstest.MapFile{Data: []byte(`description: gp
skills:
  - a
`)},
		"parent.yaml": &fstest.MapFile{Data: []byte(`description: p
extends: grandparent
skills:
  - b
`)},
		"child.yaml": &fstest.MapFile{Data: []byte(`description: c
extends: parent
skills:
  - c
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "child")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
	_ = filepath.Join // unused-import guard
}
```

- [ ] **Step 2: Run tests, expect compile-fail or test-fail**

```bash
go test ./internal/substrate/ -run TestResolveSkills -v
```

Expected: compile error (function undefined). That's the RED state.

- [ ] **Step 3: Implement the resolver (GREEN)**

Create `internal/substrate/modes.go`:

```go
package substrate

import (
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// Mode is a parsed .scion/modes/<name>.yaml file. The mode's name is the
// filename stem; there is no name field in the YAML itself.
type Mode struct {
	Description string   `yaml:"description"`
	Extends     string   `yaml:"extends,omitempty"`
	Skills      []string `yaml:"skills"`
}

// loadModeFromFS reads <name>.yaml from fsys and parses it.
func loadModeFromFS(fsys fs.FS, name string) (*Mode, error) {
	data, err := fs.ReadFile(fsys, name+".yaml")
	if err != nil {
		return nil, fmt.Errorf("mode %q: %w", name, err)
	}
	var m Mode
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("mode %q: parse: %w", name, err)
	}
	return &m, nil
}

// ResolveSkillsFromFS resolves the given mode name into an ordered, deduped
// skill name list. extends chains are walked recursively; first occurrence
// of a duplicate name wins. Cycles are detected and rejected.
func ResolveSkillsFromFS(fsys fs.FS, name string) ([]string, error) {
	visited := map[string]bool{}
	var stack []string
	return resolveRecursive(fsys, name, visited, stack)
}

func resolveRecursive(fsys fs.FS, name string, visited map[string]bool, stack []string) ([]string, error) {
	if visited[name] {
		return nil, fmt.Errorf("mode %q: cycle detected in extends chain: %s -> %s",
			stack[0], joinChain(stack), name)
	}
	visited[name] = true
	stack = append(stack, name)

	m, err := loadModeFromFS(fsys, name)
	if err != nil {
		return nil, err
	}

	var skills []string
	if m.Extends != "" {
		parentSkills, err := resolveRecursive(fsys, m.Extends, visited, stack)
		if err != nil {
			return nil, err
		}
		skills = append(skills, parentSkills...)
	}
	skills = append(skills, m.Skills...)
	return dedupePreserveFirst(skills), nil
}

func dedupePreserveFirst(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func joinChain(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	out := stack[0]
	for _, s := range stack[1:] {
		out += " -> " + s
	}
	return out
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/substrate/ -run TestResolveSkills -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Run the full substrate test suite**

```bash
go test ./internal/substrate/ -count=1
```

Expected: all PASS. No regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/substrate/modes.go internal/substrate/modes_test.go
git commit -m "feat(substrate): add mode YAML parser and recursive resolver

Mode files at .scion/modes/<name>.yaml carry description, optional
extends parent, and skills list. ResolveSkillsFromFS walks the
extends chain recursively, dedupes (first occurrence wins), and
rejects cycles. The mode's name is the filename stem; there is no
name field in the YAML.

Tests cover: flat mode, extends-one-level, multi-level chain,
duplicate-dedup, cycle detection, missing mode, missing parent."
```

---

## Task 2: Author the 1 base mode + 14 role-default modes

**Files:**
- Create: `.scion/modes/philosophy-base.yaml`
- Create: `.scion/modes/<role>.yaml` (×14)

The directory `.scion/modes/` must be created.

- [ ] **Step 1: Create the modes directory**

```bash
mkdir -p .scion/modes
```

- [ ] **Step 2: Author `philosophy-base.yaml`**

Create `.scion/modes/philosophy-base.yaml`:

```yaml
description: "Hipp + Ousterhout — design philosophy baseline shared by planners."
skills:
  - hipp
  - ousterhout
```

- [ ] **Step 3: Author 14 role-default modes**

Each of these is the canonical mode for its role. The `description` is uniform; `skills` and `extends` follow the current bundle for that role.

`.scion/modes/admin.yaml`:

```yaml
description: "Default skills for the admin harness."
skills: []
```

`.scion/modes/base.yaml`:

```yaml
description: "Default skills for the base harness."
skills: []
```

`.scion/modes/darwin.yaml`:

```yaml
description: "Default skills for the darwin harness."
skills:
  - dx-audit
```

`.scion/modes/designer.yaml`:

```yaml
description: "Default skills for the designer harness."
skills:
  - norman
  - ousterhout
```

`.scion/modes/orchestrator.yaml`:

```yaml
description: "Default skills for the orchestrator harness."
skills:
  - dx-audit
```

`.scion/modes/planner-t1.yaml`:

```yaml
description: "Default skills for the planner-t1 harness."
skills:
  - hipp
```

`.scion/modes/planner-t2.yaml`:

```yaml
description: "Default skills for the planner-t2 harness."
extends: philosophy-base
skills: []
```

`.scion/modes/planner-t3.yaml`:

```yaml
description: "Default skills for the planner-t3 harness."
extends: philosophy-base
skills:
  - superpowers
```

`.scion/modes/planner-t4.yaml`:

```yaml
description: "Default skills for the planner-t4 harness."
extends: philosophy-base
skills:
  - spec-kit
```

`.scion/modes/researcher.yaml`:

```yaml
description: "Default skills for the researcher harness."
skills: []
```

`.scion/modes/reviewer.yaml`:

```yaml
description: "Default skills for the reviewer harness."
skills:
  - ousterhout
  - hipp
  - dx-audit
```

`.scion/modes/sme.yaml`:

```yaml
description: "Default skills for the sme harness."
skills:
  - ousterhout
  - hipp
```

`.scion/modes/tdd-implementer.yaml`:

```yaml
description: "Default skills for the tdd-implementer harness."
skills:
  - idiomatic-go
  - tigerstyle
```

`.scion/modes/verifier.yaml`:

```yaml
description: "Default skills for the verifier harness."
skills:
  - tigerstyle
```

- [ ] **Step 4: Sanity-check resolution**

Write a tiny throwaway probe to confirm modes parse and resolve correctly. Save as `/tmp/probe_modes.go` (NOT in the repo):

```go
package main

import (
	"fmt"
	"os"

	"github.com/danmestas/darken/internal/substrate"
)

func main() {
	for _, role := range []string{"admin", "planner-t3", "reviewer", "tdd-implementer", "verifier"} {
		got, err := substrate.ResolveSkillsFromFS(os.DirFS(".scion/modes"), role)
		fmt.Printf("%-18s skills=%v err=%v\n", role, got, err)
	}
}
```

Run from repo root:

```bash
go run /tmp/probe_modes.go
```

Expected output (modulo formatting):
```
admin              skills=[] err=<nil>
planner-t3         skills=[hipp ousterhout superpowers] err=<nil>
reviewer           skills=[ousterhout hipp dx-audit] err=<nil>
tdd-implementer    skills=[idiomatic-go tigerstyle] err=<nil>
verifier           skills=[tigerstyle] err=<nil>
```

If anything diverges, fix the YAML before continuing.

```bash
rm /tmp/probe_modes.go
```

- [ ] **Step 5: Commit**

```bash
git add .scion/modes/
git commit -m "feat(modes): author philosophy-base + 14 role-default modes

Mirrors current skills: bundles in role manifests (extracted from
.scion/templates/<role>/scion-agent.yaml). Three planner roles share
[hipp, ousterhout] via extends: philosophy-base. Other roles stay
flat because their skill sets do not share enough order to factor.

Subsequent task drops the now-redundant skills: field from each
manifest and adds default_mode: <role>."
```

---

## Task 3: Update role manifests — add `default_mode`, remove `skills`

**Files:**
- Modify: `.scion/templates/<role>/scion-agent.yaml` (×14)

The `skills:` field in each manifest currently does double duty: it lists the skill names AND a `source:` mount entry. The `source:` entry is unrelated to skill names — it's a Docker volume mount for the staging-out dir. Verify by reading one manifest first, then make the edits surgical.

- [ ] **Step 1: Read one manifest to confirm structure**

```bash
cat .scion/templates/planner-t3/scion-agent.yaml
```

Note the structure: `skills:` field is a sequence, contains skill name entries (`- danmestas/agent-skills/skills/X`) plus a mount entry (`- source: ./.scion/skills-staging/planner-t3/`). Wait — that doesn't parse as a homogeneous sequence. Re-check.

If the existing manifest mixes skill names and mount entries under `skills:`, this needs a careful look. Read the actual structure and report DONE_WITH_CONCERNS describing the layout if unclear; the controller will adapt.

If `skills:` is a flat sequence of skill paths and there's a separate `volumes:` field with the mount, then the change is just removing the skill paths.

- [ ] **Step 2: Edit each manifest**

For each role in `[admin, base, darwin, designer, orchestrator, planner-t1, planner-t2, planner-t3, planner-t4, researcher, reviewer, sme, tdd-implementer, verifier]`:

In `.scion/templates/<role>/scion-agent.yaml`:
- Remove the `skills:` block entirely (the field and all its entries).
- Add `default_mode: <role>` at the top level (alongside other fields like `description`, `image`, `model`).
- Leave `volumes:` and other fields untouched.

The `volumes` mount path (`./.scion/skills-staging/<role>/`) should stay — it's where the stager writes to, regardless of which mode populated it.

- [ ] **Step 3: Verify the manifests still parse**

```bash
for f in .scion/templates/*/scion-agent.yaml; do
  if ! python3 -c "import yaml; yaml.safe_load(open('$f'))" 2>&1; then
    echo "PARSE FAIL: $f"
    exit 1
  fi
done && echo "all 14 manifests parse"
```

Expected: `all 14 manifests parse`.

- [ ] **Step 4: Verify each has `default_mode`**

```bash
for f in .scion/templates/*/scion-agent.yaml; do
  role=$(basename $(dirname "$f"))
  if ! grep -q "^default_mode: $role" "$f"; then
    echo "MISSING default_mode: $f"
    exit 1
  fi
done && echo "all 14 manifests have default_mode"
```

- [ ] **Step 5: Verify `skills:` field is gone**

```bash
for f in .scion/templates/*/scion-agent.yaml; do
  if grep -q "^skills:" "$f"; then
    echo "STILL HAS skills: $f"
    exit 1
  fi
done && echo "no manifest has skills: field"
```

- [ ] **Step 6: Commit**

```bash
git add .scion/templates/
git commit -m "refactor(templates): replace skills: with default_mode in role manifests

Each role manifest now points at a mode in .scion/modes/<role>.yaml
via default_mode: <role>. The skills: field is removed because the
mode owns that information. volumes: mount paths unchanged."
```

---

## Task 4: Wire mode resolution into stage-skills script + spawn flow

This is the integration task. Existing skill staging is driven by `scripts/stage-skills.sh` reading the manifest's `skills:` field. With that field removed, the script (or its caller) needs to read the role's mode instead.

**Files:**
- Identify and modify the staging entry point. Likely `scripts/stage-skills.sh` (which reads scion-agent.yaml). May also need `cmd/darken/spawn.go` if spawn-time resolution differs from bootstrap-time.

- [ ] **Step 1: Read the current stage-skills.sh**

```bash
cat scripts/stage-skills.sh
```

Identify where it parses `skills:` from the manifest. Determine the simplest hook point.

- [ ] **Step 2: Decide the hook**

Two reasonable options:

(a) Have stage-skills.sh read `default_mode` from the manifest, then read the mode YAML to get the skill list. Pure shell, no Go changes here.

(b) Move skill-name resolution into a Go helper invoked by darken (not the shell script). Wire it into the bootstrap and spawn paths.

Option (a) is simpler if the shell script is already doing YAML parsing. Option (b) is cleaner if it forces all skill resolution through one Go path.

Pick one. Document the choice in the commit message.

- [ ] **Step 3: Implement the chosen option**

Edit the relevant file(s). Skill-resolution logic should call `substrate.ResolveSkillsFromFS` (or a similar exported helper) on `.scion/modes/`.

- [ ] **Step 4: Verify the staging step still produces the same output**

Pre-migration baseline: run staging on each of the 14 roles BEFORE the migration commits and capture the output (e.g. `find .scion/skills-staging/<role>/ -type f -print | sort`).

Post-migration: re-run staging and capture the same listing. The two listings must match for every role.

```bash
# Pre (run on main):
git stash; git checkout main
for role in admin base darwin designer orchestrator planner-t1 planner-t2 planner-t3 planner-t4 researcher reviewer sme tdd-implementer verifier; do
  rm -rf /tmp/baseline-$role
  bash scripts/stage-skills.sh "$role" > /dev/null 2>&1 || true
  find .scion/skills-staging/$role -type f 2>/dev/null | sort > /tmp/baseline-$role.txt || true
done

# Restore branch and stash:
git checkout docs/dynamic-skill-resolution-spec
git stash pop

# Post:
for role in admin base darwin designer orchestrator planner-t1 planner-t2 planner-t3 planner-t4 researcher reviewer sme tdd-implementer verifier; do
  rm -rf .scion/skills-staging/$role
  bash scripts/stage-skills.sh "$role" > /dev/null 2>&1 || true
  find .scion/skills-staging/$role -type f 2>/dev/null | sort > /tmp/after-$role.txt || true
  diff /tmp/baseline-$role.txt /tmp/after-$role.txt || echo "DIFF: $role"
done
```

If any role shows a diff, the migration broke equivalence. Fix.

- [ ] **Step 5: Commit**

```bash
git add scripts/stage-skills.sh   # or whichever files changed
# Possibly cmd/darken/spawn.go etc.
git commit -m "feat(staging): resolve skills via .scion/modes/ default_mode

Stage-skills now reads default_mode from the role manifest and
resolves the skill list from .scion/modes/<mode>.yaml using the
recursive ResolveSkillsFromFS helper. skills: field on the
manifest is no longer read."
```

---

## Task 5: Add `--mode` flag to `darken spawn`

**Files:**
- Modify: `cmd/darken/spawn.go`
- Modify: `cmd/darken/spawn_test.go`

- [ ] **Step 1: Find the spawn flag-parsing block**

```bash
grep -n "flag.Parse\|--type\|FlagSet" cmd/darken/spawn.go | head
```

- [ ] **Step 2: Add `--mode <name>` flag**

Define the flag next to the existing `--type` flag. Default value: empty string ("use role's default_mode").

- [ ] **Step 3: Pipe the mode value through to staging**

Where the spawn flow currently calls into staging, pass the explicit mode name (or empty for default-from-manifest).

- [ ] **Step 4: Add a unit test**

`TestSpawn_ExplicitMode_OverridesDefault`: spawn with `--type researcher --mode tdd-implementer` (using a mock or test stub for the actual container start). Assert the staging step received `tdd-implementer` as the mode name, not `researcher`.

- [ ] **Step 5: Run cmd/darken tests**

```bash
go test ./cmd/darken/ -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/darken/spawn.go cmd/darken/spawn_test.go
git commit -m "feat(spawn): add --mode flag for per-spawn skill-set selection

darken spawn now accepts --mode <name> to override the role's
default_mode. Unset falls back to role's default. The resolved mode
flows through to stage-skills."
```

---

## Task 6: Add `darken modes list` and `darken modes show <name>` commands

**Files:**
- Create: `cmd/darken/modes.go`
- Create: `cmd/darken/modes_test.go`
- Modify: `cmd/darken/main.go` (or wherever subcommands register)

- [ ] **Step 1: Create modes.go with `runModes(args []string) error`**

Switch on subcommand: `list` and `show`. `list` walks `.scion/modes/*.yaml` and prints filename-stem + description. `show <name>` cats the YAML file and prints the resolved skill list below.

- [ ] **Step 2: Register the subcommand in main.go**

Add `"modes":` to whatever switch table dispatches commands.

- [ ] **Step 3: Write tests**

- `TestModesList_PrintsAllModes`: with a tmpdir of fixture mode files, runModes(["list"]) produces output containing each mode name and description.
- `TestModesShow_PrintsResolvedSkills`: runModes(["show", "planner-t3"]) prints the mode YAML and the resolved skills [hipp, ousterhout, superpowers].

- [ ] **Step 4: Verify**

```bash
go test ./cmd/darken/ -count=1
go build ./cmd/darken/
./bin/darken modes list
./bin/darken modes show planner-t3
```

Expected: both commands work and show sane output.

- [ ] **Step 5: Commit**

```bash
git add cmd/darken/modes.go cmd/darken/modes_test.go cmd/darken/main.go
git commit -m "feat(cli): add darken modes list and darken modes show

Surface the new mode files via two readonly subcommands. list
prints all .scion/modes/*.yaml names + descriptions. show <name>
prints the YAML body and the resolved skill set (after extends
expansion)."
```

---

## Task 7: Embed modes in the binary for non-darkish-factory projects

**Files:**
- Modify: `internal/substrate/data/.scion/modes/` (new directory copied from `.scion/modes/`)
- Modify: `internal/substrate/data/.scion/templates/<role>/scion-agent.yaml` (×14, mirror of repo-root edits)
- Possibly: `internal/substrate/embed.go` if explicit registration is needed

- [ ] **Step 1: Copy modes into the embedded data tree**

```bash
mkdir -p internal/substrate/data/.scion/modes
cp .scion/modes/*.yaml internal/substrate/data/.scion/modes/
```

- [ ] **Step 2: Mirror the manifest edits into embedded copies**

For each role, copy the edited `.scion/templates/<role>/scion-agent.yaml` to `internal/substrate/data/.scion/templates/<role>/scion-agent.yaml`. The two trees must stay in sync.

```bash
for r in .scion/templates/*/scion-agent.yaml; do
  role=$(basename $(dirname "$r"))
  cp "$r" "internal/substrate/data/.scion/templates/$role/scion-agent.yaml"
done
```

- [ ] **Step 3: Verify embed picks them up**

```bash
go build ./internal/substrate/
go test ./internal/substrate/ -count=1
```

Expected: PASS.

- [ ] **Step 4: Run cmd/darken tests with the embedded copies in play**

```bash
go test ./cmd/darken/ -count=1
```

Expected: PASS (including any test that exercises the embedded fallback path).

- [ ] **Step 5: Commit**

```bash
git add internal/substrate/data/
git commit -m "feat(substrate): embed mode files for non-darkish-factory projects

Mirrors .scion/modes/ and the edited role manifests into
internal/substrate/data/. Bootstrap's resolveTemplatesDir already
extracts the embedded tree to a tmpdir when the consuming project
has no .scion/templates/; modes ride the same path."
```

---

## Task 8: Add the failure-path tests

**Files:**
- Modify: `cmd/darken/spawn_test.go` (or wherever spawn integration tests live)

- [ ] **Step 1: Add three failure-path tests**

- `TestSpawn_UnknownMode_Fails`: spawn with `--mode definitely-not-a-real-mode`. Expect non-zero exit with "mode not found" error.
- `TestSpawn_ModeReferencesMissingSkill_Fails`: temporarily author a mode file that references a skill that doesn't exist in `agent-config/skills/`. Expect "skill not found" error.
- `TestSpawn_CycleInExtends_Fails`: temporarily author two modes (`a` extends `b`, `b` extends `a`). Expect "cycle detected" error.

- [ ] **Step 2: Run**

```bash
go test ./cmd/darken/ -run TestSpawn_ -v -count=1
```

Expected: all three new tests PASS (each verifying that the appropriate error fires).

- [ ] **Step 3: Commit**

```bash
git add cmd/darken/spawn_test.go
git commit -m "test(spawn): cover mode-resolution failure paths

Three tests pin the error contracts: unknown --mode, mode-references-
missing-skill, and cycle in extends chain. Each runs the actual
spawn entry path and asserts the expected error message."
```

---

## Self-review (post-plan)

Spec coverage:

- Mode YAML schema (description, skills, extends; no name; no targets/categories) → Task 1.
- Resolution algorithm (recursive extends, dedupe-first-wins, cycle detection) → Task 1.
- 14 role-default modes + 1 base mode → Task 2.
- Manifest changes (default_mode added, skills field removed) → Task 3.
- Stager wires through default_mode → Task 4.
- `--mode` flag at spawn → Task 5.
- `darken modes list` / `show` → Task 6.
- Embedded copies for non-darkish-factory projects → Task 7.
- Failure-path tests → Task 8.
- Backward-compat (golden equivalence pre/post staging) → Task 4 step 4.

No placeholders. Method/file names consistent.

Notes for the executor: if Task 4 step 1 reveals the staging script structure is more complex than assumed, surface it as a question before plowing into Step 2 — the option (a) vs (b) decision may shift.
