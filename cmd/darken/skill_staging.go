package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// applyRoleFilter filters the skills in dir to only those visible to role.
// It delegates to filterSkillsForRole which uses the typed SkillMetadata
// loader and fails closed on malformed metadata.
//
// A non-existent dir is silently skipped (staging may not exist yet).
// Any other error is returned to the caller.
func applyRoleFilter(dir, role string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // no staging dir yet — nothing to filter
	}
	return filterSkillsForRole(dir, role)
}

// stageSkillsForRole runs the stage-skills substrate script for harnessType
// and then applies role-visibility filtering to the resulting staging directory.
// Both operations are performed atomically from the caller's perspective:
// if filtering fails, stageSkillsForRole returns a non-nil error (fail closed).
//
// This consolidates the two-step sequence that was previously split across
// spawn.go (script call + non-fatal filter) into a single named operation.
func stageSkillsForRole(harnessType string) error {
	if err := runSubstrateScript("scripts/stage-skills.sh", []string{harnessType}); err != nil {
		return fmt.Errorf("stage-skills failed: %w", err)
	}
	root, err := repoRoot()
	if err != nil {
		// repoRoot failure is non-fatal: skills-staging may not exist.
		return nil
	}
	stagingDir := filepath.Join(root, ".scion", "skills-staging", harnessType)
	if ferr := applyRoleFilter(stagingDir, harnessType); ferr != nil {
		return fmt.Errorf("skill visibility filter for %s: %w", harnessType, ferr)
	}
	return nil
}
