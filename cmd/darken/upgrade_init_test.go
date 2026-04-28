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

	// Stub bones/scion/docker so verifyInitPrereqs passes without
	// shelling out to anything real. Empty no-op scripts are enough —
	// runInit only calls bones via runBonesInit (which we want to
	// succeed silently).
	stubDir := t.TempDir()
	for _, name := range []string{"bones", "scion", "docker"} {
		p := filepath.Join(stubDir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("stub %s: %v", name, err)
		}
	}
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
	// Confirm doctor --init ran (init_verify writes lines like "OK ..." or "FAIL ...").
	if !strings.Contains(out, "CLAUDE.md") && !strings.Contains(out, ".claude") {
		t.Fatalf("expected init-doctor output, got:\n%s", out)
	}
}

func TestUpgradeInit_RejectsArgs(t *testing.T) {
	if err := runUpgradeInit([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}
