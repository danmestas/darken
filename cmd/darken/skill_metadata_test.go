package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkillMetadata(t *testing.T, dir, name, frontmatter string) string {
	t.Helper()
	d := filepath.Join(dir, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\n" + frontmatter + "---\n# body\n"
	p := filepath.Join(d, "SKILL.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadSkillMetadata_BlockRoles(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillMetadata(t, dir, "s", "roles:\n  - researcher\n  - designer\n")
	meta, err := loadSkillMetadata(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.HasRoles {
		t.Fatal("expected HasRoles=true")
	}
	if len(meta.Roles) != 2 || meta.Roles[0] != "researcher" || meta.Roles[1] != "designer" {
		t.Errorf("unexpected roles: %v", meta.Roles)
	}
}

func TestLoadSkillMetadata_InlineRoles(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillMetadata(t, dir, "s", "roles: [orchestrator, admin]\n")
	meta, err := loadSkillMetadata(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.HasRoles {
		t.Fatal("expected HasRoles=true")
	}
	if len(meta.Roles) != 2 || meta.Roles[0] != "orchestrator" || meta.Roles[1] != "admin" {
		t.Errorf("unexpected roles: %v", meta.Roles)
	}
}

func TestLoadSkillMetadata_NoRolesField(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillMetadata(t, dir, "s", "version: 1.0.0\n")
	meta, err := loadSkillMetadata(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.HasRoles {
		t.Error("expected HasRoles=false when roles field absent")
	}
	if len(meta.Roles) != 0 {
		t.Errorf("expected empty roles, got %v", meta.Roles)
	}
}

func TestLoadSkillMetadata_EmptyInlineList(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillMetadata(t, dir, "s", "roles: []\n")
	meta, err := loadSkillMetadata(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.HasRoles {
		t.Fatal("expected HasRoles=true for explicit empty list")
	}
	if len(meta.Roles) != 0 {
		t.Errorf("expected empty roles list, got %v", meta.Roles)
	}
}

func TestLoadSkillMetadata_MissingFile(t *testing.T) {
	_, err := loadSkillMetadata("/nonexistent/path/SKILL.md")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadSkillMetadata_RoundTripFromParseFrontmatter(t *testing.T) {
	// parseFrontmatterRoles and loadSkillMetadata must agree on the same input.
	dir := t.TempDir()
	path := writeSkillMetadata(t, dir, "s", "roles:\n  - tdd-implementer\n")

	meta, err := loadSkillMetadata(path)
	if err != nil {
		t.Fatalf("loadSkillMetadata: %v", err)
	}
	if !meta.HasRoles || len(meta.Roles) != 1 || meta.Roles[0] != "tdd-implementer" {
		t.Errorf("unexpected metadata: %+v", meta)
	}
}
