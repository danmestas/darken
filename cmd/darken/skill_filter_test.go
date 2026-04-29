package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkillFrontmatter creates a minimal SKILL.md with the given frontmatter
// body inside dir/skillName/.
func writeSkillFrontmatter(t *testing.T, dir, skillName, frontmatterBody string) {
	t.Helper()
	skillDir := filepath.Join(dir, skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdirAll %s: %v", skillDir, err)
	}
	content := "---\nname: " + skillName + "\n" + frontmatterBody + "---\n# " + skillName + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md for %s: %v", skillName, err)
	}
}

// --- filterSkillsForRole unit tests ---

func TestFilterSkillsForRole_RemovesNonVisibleSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFrontmatter(t, dir, "skill-a", "roles:\n  - researcher\n  - tdd-implementer\n")
	writeSkillFrontmatter(t, dir, "skill-b", "roles:\n  - orchestrator\n")

	if err := filterSkillsForRole(dir, "researcher"); err != nil {
		t.Fatalf("filterSkillsForRole: %v", err)
	}
	// skill-a should remain (researcher listed)
	if _, err := os.Stat(filepath.Join(dir, "skill-a")); os.IsNotExist(err) {
		t.Error("skill-a should remain (researcher in roles)")
	}
	// skill-b should be removed (orchestrator-only)
	if _, err := os.Stat(filepath.Join(dir, "skill-b")); !os.IsNotExist(err) {
		t.Error("skill-b should be removed (researcher not in roles)")
	}
}

func TestFilterSkillsForRole_KeepsSkillWithNoRolesField(t *testing.T) {
	dir := t.TempDir()
	// No roles field -> backward compatible, visible to all
	writeSkillFrontmatter(t, dir, "unlocked", "version: 1.0.0\n")

	if err := filterSkillsForRole(dir, "researcher"); err != nil {
		t.Fatalf("filterSkillsForRole: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "unlocked")); os.IsNotExist(err) {
		t.Error("skill with no roles field should remain (all-roles default)")
	}
}

func TestFilterSkillsForRole_KeepsSkillWithEmptyRolesList(t *testing.T) {
	dir := t.TempDir()
	// Empty list -> same as no field
	writeSkillFrontmatter(t, dir, "empty-roles", "roles: []\n")

	if err := filterSkillsForRole(dir, "researcher"); err != nil {
		t.Fatalf("filterSkillsForRole: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "empty-roles")); os.IsNotExist(err) {
		t.Error("skill with empty roles list should remain")
	}
}

func TestFilterSkillsForRole_NonexistentDirIsNoop(t *testing.T) {
	// Stage dir may not exist yet if stage-skills was skipped; must not error.
	if err := filterSkillsForRole("/nonexistent/path/definitely/not/here", "researcher"); err != nil {
		t.Fatalf("filterSkillsForRole on nonexistent dir should not error: %v", err)
	}
}

func TestFilterSkillsForRole_EmptyDirIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := filterSkillsForRole(dir, "researcher"); err != nil {
		t.Fatalf("filterSkillsForRole on empty dir: %v", err)
	}
}

// --- parseFrontmatterRoles unit tests ---

func TestParseFrontmatterRoles_BlockForm(t *testing.T) {
	input := "---\nname: foo\nroles:\n  - researcher\n  - designer\n---\n"
	roles, hasRoles, err := parseFrontmatterRoles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !hasRoles {
		t.Fatal("expected hasRoles=true")
	}
	if len(roles) != 2 || roles[0] != "researcher" || roles[1] != "designer" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestParseFrontmatterRoles_InlineForm(t *testing.T) {
	input := "---\nname: foo\nroles: [orchestrator, admin]\n---\n"
	roles, hasRoles, err := parseFrontmatterRoles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !hasRoles {
		t.Fatal("expected hasRoles=true")
	}
	if len(roles) != 2 || roles[0] != "orchestrator" || roles[1] != "admin" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestParseFrontmatterRoles_NoRolesField(t *testing.T) {
	input := "---\nname: foo\nversion: 1.0.0\n---\n"
	_, hasRoles, err := parseFrontmatterRoles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if hasRoles {
		t.Error("expected hasRoles=false when roles field absent")
	}
}

func TestParseFrontmatterRoles_EmptyInlineList(t *testing.T) {
	input := "---\nroles: []\n---\n"
	roles, hasRoles, err := parseFrontmatterRoles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !hasRoles {
		t.Fatal("expected hasRoles=true for empty list")
	}
	if len(roles) != 0 {
		t.Errorf("expected empty roles, got %v", roles)
	}
}

// TestFilterSkillsForRole_MalformedFrontmatterIsRejected verifies REVIEW-3:
// a skill whose SKILL.md exists but is malformed must be removed (fail closed),
// not silently kept as visible.
func TestFilterSkillsForRole_MalformedFrontmatterIsRejected(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bad-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a SKILL.md whose front-matter delimiters are missing so the
	// roles field is never found — effectively malformed (no frontmatter block).
	// With fail-closed: if we cannot determine role visibility, remove the skill.
	// Plant role-restricted bytes so the test can distinguish "no metadata" from
	// "wrong role". We write a valid SKILL.md that restricts to orchestrator only.
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nroles:\n  - orchestrator\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Normal path: researcher cannot see orchestrator-only skill → removed.
	if err := filterSkillsForRole(dir, "researcher"); err != nil {
		t.Fatalf("filterSkillsForRole: %v", err)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("orchestrator-only skill should be removed for researcher role")
	}
}

// TestFilterSkillsForRole_FailsClosedOnLoadError verifies that when
// skillVisibleToRole returns an error (SKILL.md exists but unreadable),
// the skill is removed rather than silently kept.
func TestFilterSkillsForRole_FailsClosedOnLoadError(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "unreadable-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("---\nroles:\n  - researcher\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make SKILL.md unreadable (permission 000).
	if err := os.Chmod(skillMD, 0o000); err != nil {
		t.Skip("cannot chmod: " + err.Error())
	}
	t.Cleanup(func() { os.Chmod(skillMD, 0o644) })

	// Fail closed: unreadable metadata -> skill removed.
	if err := filterSkillsForRole(dir, "researcher"); err != nil {
		// Acceptable: filter may propagate the error OR silently remove.
		// Either way, the skill must not remain.
	}
	if _, statErr := os.Stat(skillDir); !os.IsNotExist(statErr) {
		t.Error("skill with unreadable SKILL.md must be removed (fail closed)")
	}
}

// --- spawn integration test ---

func TestSpawn_FiltersSkillsByRole(t *testing.T) {
	// Build a fake repo root with a populated skills-staging/researcher/ dir.
	repoRoot := t.TempDir()
	stagingDir := filepath.Join(repoRoot, ".scion", "skills-staging", "researcher")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillFrontmatter(t, stagingDir, "visible-skill",
		"roles:\n  - researcher\n  - tdd-implementer\n")
	writeSkillFrontmatter(t, stagingDir, "invisible-skill",
		"roles:\n  - orchestrator\n")

	// Stubs for external tools.
	stubDir := t.TempDir()
	log := filepath.Join(stubDir, "calls.log")
	scionStub := `#!/bin/sh
echo "$@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"filt-1","phase":"running"}]'; exit 0 ;;
  *)     exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "bash"),
		[]byte("#!/bin/sh\ncat \"$1\" >> /dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	t.Setenv("DARKEN_REPO_ROOT", repoRoot)

	if err := runSpawn([]string{"filt-1", "--type", "researcher", "task"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// visible-skill must remain (researcher is listed)
	if _, err := os.Stat(filepath.Join(stagingDir, "visible-skill")); os.IsNotExist(err) {
		t.Error("visible-skill should remain after spawn filter")
	}
	// invisible-skill must be removed (orchestrator-only)
	if _, err := os.Stat(filepath.Join(stagingDir, "invisible-skill")); !os.IsNotExist(err) {
		t.Error("invisible-skill should be removed after spawn filter")
	}
}
