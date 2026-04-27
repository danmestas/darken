package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateHarnessProducesFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKISH_REPO_ROOT", tmp)
	os.MkdirAll(filepath.Join(tmp, ".scion", "templates"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"),
		[]byte("# Harness Roster\n\n## Roster\n\n| Role | Model |\n|---|---|\n"), 0o644)

	err := runCreateHarness([]string{
		"newrole",
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
