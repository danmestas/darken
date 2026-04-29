package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestCanonicalRoles_Has14_AndMatchesSpec(t *testing.T) {
	expected := []string{
		"admin", "base", "darwin", "designer", "orchestrator",
		"planner-t1", "planner-t2", "planner-t3", "planner-t4",
		"researcher", "reviewer", "sme", "tdd-implementer", "verifier",
	}
	if got := len(canonicalRoles); got != 14 {
		t.Fatalf("canonicalRoles: want 14 entries, got %d", got)
	}
	got := make([]string, len(canonicalRoles))
	copy(got, canonicalRoles)
	sort.Strings(got)
	for i, name := range expected {
		if got[i] != name {
			t.Errorf("canonicalRoles[%d]: want %q, got %q", i, name, got[i])
		}
	}
}

func TestCanonicalRoles_TemplatesExistOnDisk(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not in a git repo: %v", err)
	}
	templatesDir := filepath.Join(root, ".scion", "templates")
	for _, role := range canonicalRoles {
		manifestPath := filepath.Join(templatesDir, role, "scion-agent.yaml")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Errorf("role %q: scion-agent.yaml missing at %s: %v", role, manifestPath, err)
		}
	}
}
