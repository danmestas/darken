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
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	out, err := captureCombined(func() error { return runUpgradeInit(nil) })
	if err != nil {
		t.Fatalf("upgrade-init failed: %v\noutput:\n%s", err, out)
	}
	// Confirm at least one scaffold line from init showed up.
	if !strings.Contains(out, "scaffolded") && !strings.Contains(out, "preserved") {
		t.Fatalf("expected init output, got:\n%s", out)
	}
	// Confirm doctor --init actually ran. runInitDoctor emits unique
	// check names like "orchestrator-mode skill scaffolded" and
	// "CLAUDE.md present" — runInit's own output never contains these
	// exact phrases, so a hit here proves the doctor phase executed.
	if !strings.Contains(out, "orchestrator-mode skill scaffolded") &&
		!strings.Contains(out, "CLAUDE.md present") {
		t.Fatalf("expected init-doctor check lines, got:\n%s", out)
	}
}

func TestUpgradeInit_RejectsArgs(t *testing.T) {
	if err := runUpgradeInit([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}

// TestUpgradeInit_PrereqFailureSurfaces proves that errors from the
// runInit half of the composition propagate. We intentionally do NOT
// stub bones/scion/docker on PATH, so verifyInitPrereqs (called at
// the top of runInit) returns a non-nil error and runUpgradeInit
// must surface it without proceeding to the doctor phase.
//
// Simulating an init-success-doctor-failure path is harder to set up
// inline (init's own --refresh recreates the very files doctor would
// flag), so we cover the runDoctor failure path indirectly: the
// composition is one straight-line `if err := runInit(...); err !=
// nil { return err }` followed by `return runDoctor(...)`. If
// runInit's error propagates, the runDoctor result must propagate
// too — there's no third branch to mask it.
func TestUpgradeInit_PrereqFailureSurfaces(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)

	// Empty PATH dir — bones/scion/docker absent → verifyInitPrereqs
	// must fail.
	stubDir := t.TempDir()
	t.Setenv("PATH", stubDir)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	_, err = captureCombined(func() error { return runUpgradeInit(nil) })
	if err == nil {
		t.Fatal("expected error when init prereqs are missing, got nil")
	}
	if !strings.Contains(err.Error(), "prereqs missing") {
		t.Fatalf("expected prereq-missing error, got: %v", err)
	}
}
