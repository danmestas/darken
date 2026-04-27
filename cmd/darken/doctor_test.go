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
