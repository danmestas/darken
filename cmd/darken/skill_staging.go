package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// resolveSkillRef resolves a skill reference (APM-style or bare name) to an
// absolute path in the canonical skills directory. Matches the resolution
// rule in scripts/stage-skills.sh.
func resolveSkillRef(ref, canonical string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "danmestas/agent-skills/skills/"):
		return filepath.Join(canonical, strings.TrimPrefix(ref, "danmestas/agent-skills/skills/")), nil
	case strings.Count(ref, "/") >= 2 && strings.Contains(ref, "/skills/"):
		return "", fmt.Errorf("resolveSkillRef: external org refs not supported: %s", ref)
	default:
		return filepath.Join(canonical, ref), nil
	}
}

// skillBaseName returns the last path component of a skill ref.
func skillBaseName(ref string) string {
	i := strings.LastIndex(ref, "/")
	if i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// buildSkillsStaging is the Go implementation of the skill staging pipeline.
// It reads the manifest for harnessType from templatesDir, resolves each skill
// ref against canonical, copies skills to stageDir, and then applies
// role-visibility filtering. Typed errors are returned for every failure mode.
//
// This consolidates what was previously split between scripts/stage-skills.sh
// (manifest reading + copy) and skill_filter.go (role filter) into one Go
// entry point shared by spawn and future skill commands.
func buildSkillsStaging(harnessType, templatesDir, stageDir, canonical string) error {
	manifestPath := filepath.Join(templatesDir, harnessType, "scion-agent.yaml")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("buildSkillsStaging: read manifest for %s: %w", harnessType, err)
	}
	manifest, err := loadHarnessManifest(body)
	if err != nil {
		return fmt.Errorf("buildSkillsStaging: manifest for %s: %w", harnessType, err)
	}

	// Wipe and recreate the staging dir (idempotent rebuild).
	if err := os.RemoveAll(stageDir); err != nil {
		return fmt.Errorf("buildSkillsStaging: clean staging dir: %w", err)
	}
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return fmt.Errorf("buildSkillsStaging: create staging dir: %w", err)
	}

	for _, ref := range manifest.Skills {
		src, err := resolveSkillRef(ref, canonical)
		if err != nil {
			return fmt.Errorf("buildSkillsStaging: %w", err)
		}
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("buildSkillsStaging: source skill missing for ref %q at %s", ref, src)
		}
		dest := filepath.Join(stageDir, skillBaseName(ref))
		if err := copyDir(src, dest); err != nil {
			return fmt.Errorf("buildSkillsStaging: copy %q: %w", ref, err)
		}
	}

	// Apply role-visibility filter on the fully-staged directory.
	if err := applyRoleFilter(stageDir, harnessType); err != nil {
		return fmt.Errorf("buildSkillsStaging: role filter for %s: %w", harnessType, err)
	}
	return nil
}

// copyDir copies a directory tree from src to dst. The destination must not
// exist; it is created by copyDir.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// stageSkillsForRole is the single entry point for skill staging called by
// spawn. It resolves the templates directory (project-local or embedded),
// the canonical skills source, and the output staging directory, then
// delegates to buildSkillsStaging which handles the full pipeline:
// manifest read → ref resolution → copy → role-visibility filter.
//
// Falls back to the substrate shell script when the templates directory
// or canonical skills directory cannot be resolved, preserving the
// pre-REVIEW-7 behavior for operators without a local agent-config tree.
func stageSkillsForRole(harnessType string) error {
	root, rootErr := repoRoot()
	if rootErr != nil {
		// No repo root: fall back to shell script.
		return stageSkillsViaScript(harnessType)
	}

	templatesDir, cleanup, err := resolveTemplatesDir()
	if err != nil {
		return stageSkillsViaScript(harnessType)
	}
	defer cleanup()

	canonical := skillsCanonical()
	if _, err := os.Stat(canonical); err != nil {
		// Canonical skills dir not present locally: fall back to shell script
		// so operators without an agent-config checkout can still spawn.
		return stageSkillsViaScript(harnessType)
	}

	stageDir := filepath.Join(root, ".scion", "skills-staging", harnessType)
	return buildSkillsStaging(harnessType, templatesDir, stageDir, canonical)
}

// stageSkillsViaScript is the shell-script fallback for stageSkillsForRole.
// Used when the Go pipeline cannot resolve its inputs (no canonical skills
// dir, no repo root). Applies the role filter after the script runs.
func stageSkillsViaScript(harnessType string) error {
	if err := runSubstrateScript("scripts/stage-skills.sh", []string{harnessType}); err != nil {
		return fmt.Errorf("stage-skills script failed: %w", err)
	}
	root, err := repoRoot()
	if err != nil {
		return nil // no repo root — nothing to filter
	}
	stagingDir := filepath.Join(root, ".scion", "skills-staging", harnessType)
	return applyRoleFilter(stagingDir, harnessType)
}
