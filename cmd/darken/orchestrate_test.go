package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestratePrintsLocalSkill(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKISH_REPO_ROOT", tmp)

	dir := filepath.Join(tmp, ".claude", "skills", "orchestrator-mode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := "# Orchestrator mode (host)\n\ntest body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(func() error { return runOrchestrate(nil) })
	if err != nil {
		t.Fatalf("runOrchestrate: %v", err)
	}
	if !strings.Contains(out, "test body") {
		t.Fatalf("expected skill body in output, got: %q", out)
	}
}

func TestOrchestrateRejectsArgs(t *testing.T) {
	if err := runOrchestrate([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}
