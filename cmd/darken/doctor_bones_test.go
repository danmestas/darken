package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckBones_PresentSurfacesVersion plants a fake `bones` that prints
// a version line, then asserts doctorBroad's OK line for the bones-cli
// check includes the version in parentheses.
func TestCheckBones_PresentSurfacesVersion(t *testing.T) {
	dir := t.TempDir()
	bones := filepath.Join(dir, "bones")
	body := `#!/bin/sh
case "$1" in
  --version) echo "bones 1.4.0 (commit deadbeef)" ;;
esac
exit 0
`
	if err := os.WriteFile(bones, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	if err := checkBones(); err != nil {
		t.Fatalf("checkBones should succeed when bones is on PATH: %v", err)
	}

	got := bonesVersion()
	if !strings.Contains(got, "1.4.0") {
		t.Errorf("bonesVersion should surface version string, got %q", got)
	}
}

// TestCheckBones_MissingReturnsError verifies the warn-severity check
// fires when bones is absent.
func TestCheckBones_MissingReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty PATH — no bones

	err := checkBones()
	if err == nil {
		t.Fatal("checkBones should error when bones is not on PATH")
	}
	if !strings.Contains(err.Error(), "bones") {
		t.Errorf("error should mention bones, got: %v", err)
	}
}

// TestBonesVersion_FailureReturnsEmpty asserts that when bones is missing
// (or fails), bonesVersion returns "" so the renderer falls back to plain
// "OK label" instead of "OK label ()".
func TestBonesVersion_FailureReturnsEmpty(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if got := bonesVersion(); got != "" {
		t.Errorf("bonesVersion with no bones on PATH should be empty, got %q", got)
	}
}
