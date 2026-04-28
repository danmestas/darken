package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runUninstallInitTestSetup runs `darken init` against a fresh tempdir
// and returns the target path. Plants stub `bones`/`scion`/`docker` so
// init's prereq check passes.
func runUninstallInitTestSetup(t *testing.T) string {
	t.Helper()
	target := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", target)
	stubDir := t.TempDir()
	for _, bin := range []string{"bones", "scion", "docker"} {
		if err := os.WriteFile(filepath.Join(stubDir, bin), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(target); err != nil {
		t.Fatal(err)
	}

	if err := runInit(nil); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return target
}

func TestUninstallInit_PristineRemovesAll(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	if err := runUninstallInit([]string{"--yes"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	for _, p := range []string{
		"CLAUDE.md",
		".claude/skills/orchestrator-mode/SKILL.md",
		".claude/skills/subagent-to-subharness/SKILL.md",
		".claude/settings.local.json",
		".scion/init-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(target, p)); err == nil {
			t.Errorf("expected %s to be removed", p)
		}
	}

	if _, err := os.Stat(filepath.Join(target, ".claude", "skills")); err == nil {
		t.Errorf("expected .claude/skills/ to be rmdir'd")
	}

	body, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to still exist: %v", err)
	}
	if strings.Contains(string(body), ".scion/agents/") {
		t.Errorf(".gitignore should have darken lines stripped, got:\n%s", body)
	}
}

func TestUninstallInit_CustomizedKept(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	skillPath := filepath.Join(target, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("customized\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureCombined(func() error { return runUninstallInit([]string{"--yes"}) })
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("customized SKILL.md should remain, got: %v", err)
	}
	if !strings.Contains(out, "KEEP") || !strings.Contains(out, "customized") {
		t.Errorf("expected KEEP / customized in output:\n%s", out)
	}
}

func TestUninstallInit_ForceRemovesCustomized(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	skillPath := filepath.Join(target, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("customized\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runUninstallInit([]string{"--yes", "--force"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	if _, err := os.Stat(skillPath); err == nil {
		t.Errorf("--force should remove customized files, %s still present", skillPath)
	}
}

func TestUninstallInit_GitignoreSurgical(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	gitignorePath := filepath.Join(target, ".gitignore")
	body, _ := os.ReadFile(gitignorePath)
	body = append(body, []byte("\n*.log\nnode_modules/\n")...)
	if err := os.WriteFile(gitignorePath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runUninstallInit([]string{"--yes"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	got, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{"*.log", "node_modules/"} {
		if !strings.Contains(s, want) {
			t.Errorf("operator line %q should be preserved:\n%s", want, s)
		}
	}
	for _, gone := range []string{".scion/agents/", ".scion/audit.jsonl", ".superpowers/"} {
		if strings.Contains(s, gone) {
			t.Errorf("darken line %q should be stripped:\n%s", gone, s)
		}
	}
}

func TestUninstallInit_DryRunMakesNoChanges(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	out, err := captureCombined(func() error { return runUninstallInit([]string{"--dry-run"}) })
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	for _, p := range []string{
		"CLAUDE.md",
		".claude/skills/orchestrator-mode/SKILL.md",
		".scion/init-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Errorf("dry-run should not remove %s: %v", p, err)
		}
	}
	if !strings.Contains(out, "REMOVE") {
		t.Errorf("expected manifest output to mention REMOVE:\n%s", out)
	}
}

func TestUninstallInit_NotInitdRepoErrors(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)

	err := runUninstallInit(nil)
	if err == nil {
		t.Fatal("expected error in empty dir")
	}
	if !strings.Contains(err.Error(), "not in an init'd repo") {
		t.Errorf("error should hint at init: %v", err)
	}
}

func TestUninstallInit_FallbackWhenManifestMissing(t *testing.T) {
	target := runUninstallInitTestSetup(t)

	if err := os.Remove(filepath.Join(target, ".scion", "init-manifest.json")); err != nil {
		t.Fatal(err)
	}

	if err := runUninstallInit([]string{"--yes"}); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	for _, p := range []string{
		".claude/skills/orchestrator-mode/SKILL.md",
		".claude/settings.local.json",
	} {
		if _, err := os.Stat(filepath.Join(target, p)); err == nil {
			t.Errorf("expected %s to be removed via Body() fallback", p)
		}
	}
}
