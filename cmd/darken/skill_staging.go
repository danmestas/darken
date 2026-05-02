package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danmestas/darken/internal/substrate"
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

// loadManifestForRole resolves and parses the scion-agent.yaml for the given
// harnessType. Uses resolveTemplatesDir (project-local or embedded) so it works
// in any environment where staging works. Returns an error when the manifest
// cannot be read or parsed.
func loadManifestForRole(harnessType string) (HarnessManifest, error) {
	// Allow test overrides via DARKEN_TEMPLATES_DIR.
	if dir := os.Getenv("DARKEN_TEMPLATES_DIR"); dir != "" {
		body, err := os.ReadFile(filepath.Join(dir, harnessType, "scion-agent.yaml"))
		if err != nil {
			return HarnessManifest{}, fmt.Errorf("loadManifestForRole: %w", err)
		}
		return loadHarnessManifest(body)
	}
	templatesDir, cleanup, err := resolveTemplatesDir()
	if err != nil {
		return HarnessManifest{}, fmt.Errorf("loadManifestForRole: resolve templates: %w", err)
	}
	defer cleanup()
	body, err := os.ReadFile(filepath.Join(templatesDir, harnessType, "scion-agent.yaml"))
	if err != nil {
		return HarnessManifest{}, fmt.Errorf("loadManifestForRole: read manifest: %w", err)
	}
	return loadHarnessManifest(body)
}

// stageSkillsForRole is the single entry point for skill staging called
// by spawn. Mode-driven resolution: the manifest's default_mode (or the
// DARKEN_MODE_OVERRIDE env var) names a mode YAML in DARKEN_MODES_DIR;
// internal/substrate.ResolveSkillsFromFS expands the extends chain and
// returns a deduped, ordered skill list. Each skill is copied from
// canonical (~/projects/agent-skills/skills/<name>) into the per-role
// staging dir, then role-visibility filtering runs.
//
// Pre-Phase-G this routed through scripts/stage-skills.sh. The bash
// remains for container-side use (spawn.sh) and the operator-facing
// `darken skills` add/remove/diff commands; host-side lifecycle calls
// now live entirely in Go.
func stageSkillsForRole(harnessType string) error {
	return stageSkillsNative(harnessType)
}

// stageSkillsNative is the Go implementation of stage-skills.sh rebuild
// mode. Atomic publish via tmpdir + rename; the prior bash needed an
// explicit lock dir for parallel-invocation safety, but Go's os.Rename
// is atomic on POSIX so the lock is unnecessary.
//
// Important: applyRoleFilter is called at the end on every code path,
// even when the resolution returns no skills or the manifest declares
// no default_mode. This matches the bash + Go-shim contract: the
// fail-closed role filter must run on the live staging dir so unreadable
// or out-of-role skills get removed regardless of whether a rebuild
// happened.
func stageSkillsNative(harnessType string) error {
	root, err := repoRoot()
	if err != nil {
		return fmt.Errorf("stage-skills: repo root: %w", err)
	}
	stageDir := filepath.Join(root, ".scion", "skills-staging", harnessType)

	templatesDir, modesDir, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		return fmt.Errorf("stage-skills: resolve substrate: %w", err)
	}
	defer cleanup()

	manifestPath := filepath.Join(templatesDir, harnessType, "scion-agent.yaml")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		// No manifest — soft skip; still apply role filter on existing
		// stage dir so fail-closed semantics hold.
		fmt.Fprintf(os.Stderr, "stage-skills: read manifest for %s: %v\n", harnessType, err)
		return applyRoleFilter(stageDir, harnessType)
	}

	mode := os.Getenv("DARKEN_MODE_OVERRIDE")
	if mode == "" {
		mode = scanField(string(body), "default_mode:")
	}
	if mode == "" {
		// No mode declared and no override — soft no-op for the rebuild
		// step; still run the role filter on the existing staging dir.
		return applyRoleFilter(stageDir, harnessType)
	}

	skills, err := substrate.ResolveSkillsFromFS(os.DirFS(modesDir), mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stage-skills: resolve mode %q for %s: %v\n", mode, harnessType, err)
		return applyRoleFilter(stageDir, harnessType)
	}
	if len(skills) == 0 {
		return applyRoleFilter(stageDir, harnessType)
	}

	canonical := skillsCanonical()
	tmpDir := stageDir + ".tmp"
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("stage-skills: prepare tmp dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("stage-skills: create tmp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) // best-effort cleanup if rename failed

	for _, ref := range skills {
		src, err := resolveSkillRef(ref, canonical)
		if err != nil {
			return fmt.Errorf("stage-skills: %w", err)
		}
		if _, err := os.Stat(src); err != nil {
			// Source missing — log and skip this skill rather than abort
			// the whole rebuild. Matches bash's WARNING behavior in
			// "all" mode and avoids one missing canonical skill from
			// blocking the entire role.
			fmt.Fprintf(os.Stderr, "stage-skills: source skill missing for ref %q at %s\n", ref, src)
			continue
		}
		name := skillBaseName(ref)
		dest := filepath.Join(tmpDir, name)
		if err := copyDir(src, dest); err != nil {
			return fmt.Errorf("stage-skills: copy %q: %w", ref, err)
		}
		fmt.Printf("stage-skills: copied %s → %s\n", name, filepath.Join(stageDir, name))
	}

	if err := os.RemoveAll(stageDir); err != nil {
		return fmt.Errorf("stage-skills: clean stage dir: %w", err)
	}
	if err := os.Rename(tmpDir, stageDir); err != nil {
		return fmt.Errorf("stage-skills: publish: %w", err)
	}

	return applyRoleFilter(stageDir, harnessType)
}
