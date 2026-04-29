package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuildSkillsStaging_CopiesAndFilters verifies REVIEW-7: the Go staging
// pipeline resolves manifest skill refs, copies skills from the canonical
// source, and filters out skills not visible to the target role.
func TestBuildSkillsStaging_CopiesAndFilters(t *testing.T) {
	// Set up a canonical skills directory with two skills.
	canonical := t.TempDir()
	// skill-shared: no role restriction (visible to all)
	sharedDir := filepath.Join(canonical, "skill-shared")
	os.MkdirAll(sharedDir, 0o755)
	os.WriteFile(filepath.Join(sharedDir, "SKILL.md"),
		[]byte("---\nname: skill-shared\n---\n# body\n"), 0o644)
	// skill-orch-only: orchestrator only
	orchDir := filepath.Join(canonical, "skill-orch-only")
	os.MkdirAll(orchDir, 0o755)
	os.WriteFile(filepath.Join(orchDir, "SKILL.md"),
		[]byte("---\nname: skill-orch-only\nroles:\n  - orchestrator\n---\n# body\n"), 0o644)

	// Set up a harness manifest that references both skills.
	templateDir := t.TempDir()
	harnessDir := filepath.Join(templateDir, "researcher")
	os.MkdirAll(harnessDir, 0o755)
	os.WriteFile(filepath.Join(harnessDir, "scion-agent.yaml"), []byte(
		"default_harness_config: claude\n"+
			"skills:\n"+
			"  - skill-shared\n"+
			"  - skill-orch-only\n",
	), 0o644)

	// Set up output staging directory.
	stageDir := t.TempDir()

	err := buildSkillsStaging("researcher", templateDir, stageDir, canonical)
	if err != nil {
		t.Fatalf("buildSkillsStaging: %v", err)
	}

	// skill-shared must be present (no role restriction).
	if _, err := os.Stat(filepath.Join(stageDir, "skill-shared")); os.IsNotExist(err) {
		t.Error("skill-shared should be present in staging dir")
	}
	// skill-orch-only must be absent (orchestrator-only, target is researcher).
	if _, err := os.Stat(filepath.Join(stageDir, "skill-orch-only")); !os.IsNotExist(err) {
		t.Error("skill-orch-only should be removed for researcher role")
	}
}

// TestBuildSkillsStaging_MissingSourceSkillErrors verifies that a typed
// error is returned when a skill declared in the manifest is not found
// in the canonical source.
func TestBuildSkillsStaging_MissingSourceSkillErrors(t *testing.T) {
	canonical := t.TempDir()
	// No skills in canonical — manifest declares a missing skill.

	templateDir := t.TempDir()
	harnessDir := filepath.Join(templateDir, "researcher")
	os.MkdirAll(harnessDir, 0o755)
	os.WriteFile(filepath.Join(harnessDir, "scion-agent.yaml"), []byte(
		"default_harness_config: claude\n"+
			"skills:\n"+
			"  - nonexistent-skill\n",
	), 0o644)

	stageDir := t.TempDir()
	err := buildSkillsStaging("researcher", templateDir, stageDir, canonical)
	if err == nil {
		t.Fatal("expected error when source skill is missing, got nil")
	}
	if !containsString(err.Error(), "nonexistent-skill") {
		t.Errorf("error should name the missing skill, got: %v", err)
	}
}

// TestBuildSkillsStaging_EmptyManifestSkillsIsNoop verifies that a harness
// with no skills declared produces an empty staging dir without error.
func TestBuildSkillsStaging_EmptyManifestSkillsIsNoop(t *testing.T) {
	templateDir := t.TempDir()
	harnessDir := filepath.Join(templateDir, "researcher")
	os.MkdirAll(harnessDir, 0o755)
	os.WriteFile(filepath.Join(harnessDir, "scion-agent.yaml"), []byte(
		"default_harness_config: claude\n",
	), 0o644)

	stageDir := t.TempDir()
	if err := buildSkillsStaging("researcher", templateDir, stageDir, ""); err != nil {
		t.Fatalf("buildSkillsStaging with no skills: %v", err)
	}
	entries, _ := os.ReadDir(stageDir)
	if len(entries) != 0 {
		t.Errorf("staging dir should be empty, got %d entries", len(entries))
	}
}

// TestBuildSkillsStaging_UnknownBackendErrors verifies that a manifest with
// an invalid backend is rejected during staging.
func TestBuildSkillsStaging_UnknownBackendErrors(t *testing.T) {
	templateDir := t.TempDir()
	harnessDir := filepath.Join(templateDir, "researcher")
	os.MkdirAll(harnessDir, 0o755)
	os.WriteFile(filepath.Join(harnessDir, "scion-agent.yaml"), []byte(
		"default_harness_config: invalid-backend\n",
	), 0o644)

	stageDir := t.TempDir()
	err := buildSkillsStaging("researcher", templateDir, stageDir, "")
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
	if !containsString(err.Error(), "invalid-backend") {
		t.Errorf("error should mention the bad backend, got: %v", err)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || stringContains(s, substr))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
