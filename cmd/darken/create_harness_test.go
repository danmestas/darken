package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateHarnessProjectScope(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.MkdirAll(filepath.Join(tmp, ".scion", "templates"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"),
		[]byte("# Harness Roster\n\n## Roster\n\n| Role | Model |\n|---|---|\n"), 0o644)

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
	roster, _ := os.ReadFile(filepath.Join(tmp, ".design", "harness-roster.md"))
	if !strings.Contains(string(roster), "newrole") {
		t.Fatalf("roster missing newrole entry")
	}
}

func TestCreateHarnessUserScope(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Fake $HOME so the user-scope write lands under tmp instead of the
	// real ~/.config/darken/overrides/.
	fakeHome := filepath.Join(tmp, "home")
	t.Setenv("HOME", fakeHome)

	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"),
		[]byte("# Harness Roster\n\n## Roster\n\n| Role | Model |\n|---|---|\n"), 0o644)

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
	roster, _ := os.ReadFile(filepath.Join(tmp, ".design", "harness-roster.md"))
	if !strings.Contains(string(roster), "newrole") {
		t.Fatalf("roster missing newrole entry")
	}
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
