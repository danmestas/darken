package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rosterFixture mirrors the real .design/harness-roster.md table shape:
// 8 columns, header + separator + at least one existing data row. New
// rows must land *after* the separator and inside the table body.
const rosterFixture = `# Harness Roster

## Roster

| Role | Backend | Model | Max turns | Max duration | Detached | Escalation-axis affinity | One-line role |
|---|---|---|---|---|---|---|---|
` + "| `orchestrator` | claude | claude-opus-4-7 | 200 | 4h | false | All axes | the operator's only handle |\n"

func TestCreateHarnessProjectScope(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.MkdirAll(filepath.Join(tmp, ".scion", "templates"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"), []byte(rosterFixture), 0o644)

	err := runCreateHarness([]string{
		"newrole",
		"--scope", "project",
		"--backend", "claude",
		"--model", "claude-sonnet-4-6",
		"--skills", "danmestas/agent-skills/skills/hipp",
		"--description", "Test role",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{
		"scion-agent.yaml", "agents.md", "system-prompt.md",
	} {
		path := filepath.Join(tmp, ".scion", "templates", "newrole", f)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s not created: %v", path, err)
		}
	}
	rosterAssertRowAfterSeparator(t, filepath.Join(tmp, ".design", "harness-roster.md"), "newrole")
}

func TestCreateHarnessUserScope(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Fake $HOME so the user-scope write lands under tmp instead of the
	// real ~/.config/darken/overrides/.
	fakeHome := filepath.Join(tmp, "home")
	t.Setenv("HOME", fakeHome)

	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"), []byte(rosterFixture), 0o644)

	err := runCreateHarness([]string{
		"newrole",
		"--scope", "user",
		"--backend", "claude",
		"--model", "claude-sonnet-4-6",
		"--skills", "danmestas/agent-skills/skills/hipp",
		"--description", "Test role",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"scion-agent.yaml", "agents.md", "system-prompt.md"} {
		path := filepath.Join(fakeHome, ".config", "darken", "overrides", ".scion", "templates", "newrole", f)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s not created: %v", path, err)
		}
	}

	// Roster always lives in the working repo, regardless of scope.
	rosterAssertRowAfterSeparator(t, filepath.Join(tmp, ".design", "harness-roster.md"), "newrole")
}

func TestCreateHarnessRejectsBadScope(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	fakeHome := filepath.Join(tmp, "home")
	t.Setenv("HOME", fakeHome)

	err := runCreateHarness([]string{
		"newrole",
		"--scope", "wrong",
		"--backend", "claude",
		"--model", "claude-sonnet-4-6",
		"--description", "Test role",
	})
	if err == nil {
		t.Fatal("expected error on invalid scope, got nil")
	}
	if !strings.Contains(err.Error(), "--scope") {
		t.Fatalf("expected error to mention --scope, got: %v", err)
	}
}

// TestCreateHarnessRowHasEightCells regression-guards the row template
// shape. The roster table has 8 columns; an off-by-one row template
// generates malformed markdown that doesn't render as a table row.
func TestCreateHarnessRowHasEightCells(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.MkdirAll(filepath.Join(tmp, ".scion", "templates"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"), []byte(rosterFixture), 0o644)

	err := runCreateHarness([]string{
		"newrole",
		"--scope", "project",
		"--backend", "codex",
		"--model", "gpt-5.5",
		"--description", "Test role",
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := os.ReadFile(filepath.Join(tmp, ".design", "harness-roster.md"))
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.Contains(line, "newrole") {
			continue
		}
		// 8 columns => 9 pipe characters (leading + trailing + 7 between cells).
		if got := strings.Count(line, "|"); got != 9 {
			t.Fatalf("expected 9 pipe chars (8 cells), got %d in line: %q", got, line)
		}
		// Confirm the backend cell is present.
		if !strings.Contains(line, "| codex |") {
			t.Fatalf("expected backend cell '| codex |' in row, got: %q", line)
		}
		return
	}
	t.Fatal("newrole row not found in roster")
}

// rosterAssertRowAfterSeparator confirms the line containing roleName
// appears strictly after the markdown table separator (|---|...|).
// A row inserted *above* the header would be a regression.
func rosterAssertRowAfterSeparator(t *testing.T, path, roleName string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	sepIdx := -1
	roleIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isMarkdownTableSeparator(trimmed) && sepIdx == -1 {
			sepIdx = i
		}
		if strings.Contains(line, roleName) && roleIdx == -1 {
			roleIdx = i
		}
	}
	if sepIdx == -1 {
		t.Fatalf("roster missing table separator")
	}
	if roleIdx == -1 {
		t.Fatalf("roster missing role %q", roleName)
	}
	if roleIdx <= sepIdx {
		t.Fatalf("role %q at line %d landed at-or-above separator at line %d (rows must come AFTER the separator)", roleName, roleIdx+1, sepIdx+1)
	}
}
