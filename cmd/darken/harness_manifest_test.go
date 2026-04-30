package main

import (
	"os"
	"strings"
	"testing"
)

func TestLoadHarnessManifest_ParsesBackendAndSkills(t *testing.T) {
	body := []byte("default_harness_config: claude\nskills:\n  - danmestas/agent-skills/skills/ousterhout\n  - caveman\n")
	m, err := loadHarnessManifest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Backend != "claude" {
		t.Errorf("Backend: want claude, got %q", m.Backend)
	}
	if len(m.Skills) != 2 {
		t.Fatalf("Skills: want 2, got %d: %v", len(m.Skills), m.Skills)
	}
	if m.Skills[0] != "danmestas/agent-skills/skills/ousterhout" {
		t.Errorf("Skills[0]: got %q", m.Skills[0])
	}
	if m.Skills[1] != "caveman" {
		t.Errorf("Skills[1]: got %q", m.Skills[1])
	}
}

func TestLoadHarnessManifest_AllKnownBackends(t *testing.T) {
	for _, backend := range []string{"claude", "codex", "pi", "gemini"} {
		body := []byte("default_harness_config: " + backend + "\n")
		m, err := loadHarnessManifest(body)
		if err != nil {
			t.Errorf("backend %q: unexpected error: %v", backend, err)
			continue
		}
		if m.Backend != backend {
			t.Errorf("backend %q: got %q", backend, m.Backend)
		}
	}
}

func TestLoadHarnessManifest_RejectsUnknownBackend(t *testing.T) {
	body := []byte("default_harness_config: unknown-backend\n")
	_, err := loadHarnessManifest(body)
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
	if !strings.Contains(err.Error(), "unknown-backend") {
		t.Errorf("error should name the bad backend, got: %v", err)
	}
}

func TestLoadHarnessManifest_EmptySkillsList(t *testing.T) {
	body := []byte("default_harness_config: codex\nskills: []\n")
	m, err := loadHarnessManifest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Skills) != 0 {
		t.Errorf("expected empty skills list, got %v", m.Skills)
	}
}

func TestLoadHarnessManifest_MissingBackendErrors(t *testing.T) {
	body := []byte("skills:\n  - some-skill\n")
	_, err := loadHarnessManifest(body)
	if err == nil {
		t.Fatal("expected error when default_harness_config is absent")
	}
}

func TestLoadHarnessManifest_SkillsAsInlineList(t *testing.T) {
	body := []byte("default_harness_config: pi\nskills:\n  - alpha\n  - beta\n  - gamma\n")
	m, err := loadHarnessManifest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Skills) != 3 {
		t.Fatalf("want 3 skills, got %d: %v", len(m.Skills), m.Skills)
	}
}

// TestLoadHarnessManifest_ParsesCommandArgs verifies that loadHarnessManifest
// reads the command_args block sequence from a manifest body.
func TestLoadHarnessManifest_ParsesCommandArgs(t *testing.T) {
	body := []byte("default_harness_config: claude\ncommand_args:\n  - --betas\n  - context-1m-2025-08-07\n")
	m, err := loadHarnessManifest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.CommandArgs) != 2 {
		t.Fatalf("CommandArgs: want 2 entries, got %d: %v", len(m.CommandArgs), m.CommandArgs)
	}
	if m.CommandArgs[0] != "--betas" {
		t.Errorf("CommandArgs[0]: want --betas, got %q", m.CommandArgs[0])
	}
	if m.CommandArgs[1] != "context-1m-2025-08-07" {
		t.Errorf("CommandArgs[1]: want context-1m-2025-08-07, got %q", m.CommandArgs[1])
	}
}

// TestLoadHarnessManifest_EmptyCommandArgs verifies that a manifest without
// command_args returns a nil or empty slice with no error.
func TestLoadHarnessManifest_EmptyCommandArgs(t *testing.T) {
	body := []byte("default_harness_config: codex\n")
	m, err := loadHarnessManifest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.CommandArgs) != 0 {
		t.Errorf("CommandArgs: want empty, got %v", m.CommandArgs)
	}
}

// TestDoctorHarness_UsesTypedManifest verifies that doctorHarness delegates
// backend/skills extraction to the typed loader (REVIEW-6 integration).
func TestDoctorHarness_UsesTypedManifest_UnknownBackend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", dir)

	stubDir := t.TempDir()
	os.WriteFile(stubDir+"/docker", []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(stubDir+"/scion", []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir)

	hd := dir + "/.scion/templates/badharness"
	os.MkdirAll(hd, 0o755)
	os.WriteFile(hd+"/scion-agent.yaml",
		[]byte("default_harness_config: bogus-backend\nskills: []\n"), 0o644)

	report, err := doctorHarness("badharness")
	if err == nil {
		t.Fatalf("expected error for unknown backend, got nil\nreport:\n%s", report)
	}
	if !strings.Contains(report, "bogus-backend") && !strings.Contains(err.Error(), "bogus-backend") {
		t.Errorf("output should mention bogus-backend; report: %q err: %v", report, err)
	}
}
