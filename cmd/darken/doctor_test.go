package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsMissingScion(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "scion")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	report, err := doctorBroad()
	if err == nil {
		t.Fatalf("doctor should report failure when scion is broken")
	}
	if !strings.Contains(report, "scion") {
		t.Fatalf("report must mention scion, got %q", report)
	}
}

func TestDoctorHarnessChecksImageSecretAndStaging(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", dir)

	// Plant stubs so the test doesn't depend on the dev box's real docker / scion state.
	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir)

	hd := filepath.Join(dir, ".scion", "templates", "sme")
	os.MkdirAll(hd, 0o755)
	os.WriteFile(filepath.Join(hd, "scion-agent.yaml"),
		[]byte("default_harness_config: codex\nskills:\n  - danmestas/agent-skills/skills/ousterhout\n"), 0o644)

	report, err := doctorHarness("sme")
	if err == nil {
		t.Fatalf("expected per-harness preflight to fail without staging dir")
	}
	if !strings.Contains(report, "skills-staging") {
		t.Fatalf("report should call out missing staging dir, got %q", report)
	}
}

func TestDoctorHarnessReadsUserOverridesLayer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// User-scope override directory in the same tmp tree (so we can mutate
	// it without touching the real ~/.config/darken/overrides/).
	overrideHome := filepath.Join(tmp, "fakehome")
	t.Setenv("HOME", overrideHome)

	overrideDir := filepath.Join(overrideHome, ".config", "darken", "overrides", ".scion", "templates", "smeoverride")
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overrideDir, "scion-agent.yaml"),
		[]byte("default_harness_config: claude\nskills: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plant docker/scion stubs so doctorHarness's image+secret checks don't
	// hang on real binaries.
	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir)

	report, _ := doctorHarness("smeoverride")
	if !strings.Contains(report, "user layer") {
		t.Fatalf("expected report to identify user layer; got: %q", report)
	}
}

func TestDoctorHarnessPostMortemMapsAuthError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", dir)
	logDir := filepath.Join(dir, ".scion", "agents", "smoke-1")
	os.MkdirAll(logDir, 0o755)
	os.WriteFile(filepath.Join(logDir, "agent.log"),
		[]byte("broker: auth resolution failed: codex\n"), 0o644)

	report := postMortemFor(filepath.Join(logDir, "agent.log"))
	if !strings.Contains(report, "stage-creds.sh") {
		t.Fatalf("post-mortem should map auth error to stage-creds remediation, got %q", report)
	}
}

func TestInitDoctor_PassesOnCompleteInit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Plant a complete init scaffold (matches what Phase 5's runInit produces).
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness"), 0o755)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken orchestrator-mode\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md"),
		[]byte("---\nname: orchestrator-mode\n---\n# body\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness", "SKILL.md"),
		[]byte("---\nname: subagent-to-subharness\n---\n# body\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "settings.local.json"),
		[]byte(`{"statusLine":{"command":"darken status","type":"command"}}`), 0o644)
	os.WriteFile(filepath.Join(tmp, ".gitignore"),
		[]byte(".scion/agents/\n.scion/skills-staging/\n.scion/audit.jsonl\n.claude/worktrees/\n"), 0o644)

	report, err := runInitDoctor(tmp)
	if err != nil {
		t.Fatalf("expected init doctor to pass on complete scaffold; got: %v\nreport:\n%s", err, report)
	}
	for _, want := range []string{"OK", "CLAUDE.md", "orchestrator-mode", "subagent-to-subharness", "statusLine", ".gitignore"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestInitDoctor_FailsOnMissingCLAUDE(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// no CLAUDE.md planted
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when CLAUDE.md missing")
	}
	if !strings.Contains(report, "CLAUDE.md") {
		t.Fatalf("report should call out CLAUDE.md: %s", report)
	}
	if !strings.Contains(report, "darken init") {
		t.Fatalf("report should suggest `darken init`: %s", report)
	}
}

func TestInitDoctor_FailsOnMissingSkills(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)
	// skills NOT scaffolded
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when skills missing")
	}
	if !strings.Contains(report, "orchestrator-mode") {
		t.Fatalf("report should call out orchestrator-mode skill: %s", report)
	}
}

func TestInitDoctor_FailsOnMissingStatusLine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md"), []byte("name: orchestrator-mode"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness", "SKILL.md"), []byte("name: subagent-to-subharness"), 0o644)
	// no settings.local.json planted
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when settings.local.json missing")
	}
	if !strings.Contains(report, "statusLine") && !strings.Contains(report, "settings.local.json") {
		t.Fatalf("report should call out missing statusLine config: %s", report)
	}
}
