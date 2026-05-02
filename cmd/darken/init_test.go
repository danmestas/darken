package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitScaffoldsCLAUDE(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not created: %v", err)
	}
	if !strings.Contains(string(body), "orchestrator-mode") {
		t.Fatalf("CLAUDE.md missing orchestrator-mode reference: %q", body)
	}
	// {{.RepoName}} should be substituted with the dir basename.
	if !strings.Contains(string(body), filepath.Base(tmp)) {
		t.Fatalf("expected RepoName substitution, got: %q", body)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	// Second init on same dir should not error or duplicate.
	if err := runInit([]string{tmp}); err != nil {
		t.Fatalf("second init should be idempotent, got: %v", err)
	}
}

func TestInitForceOverwrites(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Plant a CLAUDE.md that's not from us.
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# pre-existing"), 0o644)

	// Without --force, second init should leave the existing file alone.
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if !strings.HasPrefix(string(body), "# pre-existing") {
		t.Fatalf("init without --force should not overwrite, got: %q", body)
	}

	// With --force, it should be replaced.
	if err := runInit([]string{"--force", tmp}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if strings.HasPrefix(string(body), "# pre-existing") {
		t.Fatalf("init --force should overwrite pre-existing CLAUDE.md")
	}
}

func TestInitDryRun(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	out, err := captureStdout(func() error { return runInit([]string{"--dry-run", tmp}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would create") {
		t.Fatalf("--dry-run output should mention 'would create': %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "CLAUDE.md")); err == nil {
		t.Fatal("--dry-run should not create files")
	}
}

// stubPrereqs plants no-op bones/scion/docker stubs on PATH for tests
// that don't care about prereq checks (just want runInit to proceed).
func stubPrereqs(t *testing.T) {
	t.Helper()
	stubDir := t.TempDir()
	for _, b := range []string{"bones", "scion", "docker"} {
		if err := os.WriteFile(filepath.Join(stubDir, b),
			[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubDir)
}

func TestInitScaffoldsSkills(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
		path := filepath.Join(tmp, ".claude", "skills", skill, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("skill %s not scaffolded: %v", skill, err)
		}
	}
}

func TestInitWritesStatusLineSettings(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(tmp, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("settings.local.json not created: %v", err)
	}
	if !strings.Contains(string(body), `"command": "darken status"`) {
		t.Fatalf("settings missing statusLine.command: %s", body)
	}
}

func TestInitAppendsGitignore(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// Plant a pre-existing .gitignore to confirm append (not overwrite).
	os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("# pre-existing\n*.log\n"), 0o644)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if !strings.Contains(string(body), "# pre-existing") {
		t.Fatalf("init clobbered existing .gitignore: %s", body)
	}
	if !strings.Contains(string(body), ".scion/agents/") {
		t.Fatalf("init didn't append darken entries: %s", body)
	}
}

func TestInitSecondRunIsIdempotent(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	body1, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	body2, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if string(body1) != string(body2) {
		t.Fatalf("second init mutated .gitignore (not idempotent):\nwas: %q\nnow: %q", body1, body2)
	}
}

func TestInitRefresh_PreservesCLAUDE(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Initial init creates the scaffold.
	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	// Operator customizes CLAUDE.md.
	customCLAUDE := "# Custom CLAUDE.md\n\nMy own content here.\n"
	if err := os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte(customCLAUDE), 0o644); err != nil {
		t.Fatal(err)
	}

	// --refresh should NOT overwrite CLAUDE.md.
	if err := runInit([]string{"--refresh", tmp}); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if string(got) != customCLAUDE {
		t.Fatalf("CLAUDE.md should be preserved, got:\n%s", got)
	}
}

func TestInitRefresh_UpdatesSkills(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	// Operator stomps on a skill with stale content.
	skillPath := filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("STALE CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --refresh should re-extract from embedded substrate.
	if err := runInit([]string{"--refresh", tmp}); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(skillPath)
	if string(got) == "STALE CONTENT" {
		t.Fatal("skill body should have been refreshed from embedded substrate")
	}
	if !strings.Contains(string(got), "name: orchestrator-mode") {
		t.Fatalf("refreshed skill missing frontmatter: %s", string(got)[:100])
	}
}

func TestInitRefreshForce_OverwritesCLAUDE(t *testing.T) {
	stubPrereqs(t)
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatal(err)
	}
	customCLAUDE := "# Custom\n"
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte(customCLAUDE), 0o644)

	// --refresh --force regenerates CLAUDE.md.
	if err := runInit([]string{"--refresh", "--force", tmp}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	if string(got) == customCLAUDE {
		t.Fatal("--refresh --force should regenerate CLAUDE.md")
	}
	if !strings.Contains(string(got), "darken orchestrator-mode") {
		t.Fatalf("regenerated CLAUDE.md missing template content: %s", got)
	}
}

func TestInit_FailsFastWhenBonesMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Replace PATH with a tmp dir that has scion + docker stubs but NO bones.
	stubDir := t.TempDir()
	for _, b := range []string{"scion", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	err := runInit([]string{tmp})
	if err == nil {
		t.Fatal("expected init to fail when bones missing from PATH")
	}
	if !strings.Contains(err.Error(), "bones") {
		t.Fatalf("error should mention bones: %v", err)
	}
	if !strings.Contains(err.Error(), "brew install") {
		t.Fatalf("error should suggest install hint: %v", err)
	}
}

func TestInit_FailsFastWhenScionMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	for _, b := range []string{"bones", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	err := runInit([]string{tmp})
	if err == nil {
		t.Fatal("expected init to fail when scion missing")
	}
	if !strings.Contains(err.Error(), "scion") {
		t.Fatalf("error should mention scion: %v", err)
	}
}

func TestInit_FailsFastWhenDockerMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	for _, b := range []string{"bones", "scion"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	err := runInit([]string{tmp})
	if err == nil {
		t.Fatal("expected init to fail when docker missing")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Fatalf("error should mention docker: %v", err)
	}
}

// TestInit_BonesAlreadyInitializedIsNoOp verifies that when bones init
// exits non-zero with an "already initialized" message, runInit treats
// the workspace as already bootstrapped: returns nil AND emits no
// "bones init failed" warning to stderr.
func TestInit_BonesAlreadyInitializedIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	// scion + docker exit 0; bones exits 1 with the already-initialized message.
	for _, b := range []string{"scion", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	os.WriteFile(filepath.Join(stubDir, "bones"),
		[]byte("#!/bin/sh\necho 'workspace already initialized' >&2\nexit 1\n"), 0o755)
	t.Setenv("PATH", stubDir)

	stderr, err := captureStderr(func() error { return runInit([]string{tmp}) })
	if err != nil {
		t.Fatalf("runInit should no-op when bones reports already-initialized, got: %v", err)
	}
	if strings.Contains(stderr, "bones init failed") {
		t.Fatalf("already-initialized should not log 'bones init failed', got stderr: %q", stderr)
	}
}

// TestParseBonesVersion covers the formats parseBonesVersion is expected to
// handle, including the exact `bones --version` output and a few corruptions.
func TestParseBonesVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"canonical", "bones 0.6.2 (commit a6c35ab, built 2026-05-01T20:32:45Z)\n", "0.6.2"},
		{"trimmed", "bones 0.6.1\n", "0.6.1"},
		{"leading space", "  bones 1.0.0\n", "1.0.0"},
		{"empty", "", ""},
		{"wrong tool", "barnacle 9.9.9\n", ""},
		{"no version", "bones\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBonesVersion(tc.in); got != tc.want {
				t.Fatalf("parseBonesVersion(%q): want %q, got %q", tc.in, tc.want, got)
			}
		})
	}
}

// TestSemverLess covers the dotted version comparator.
func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.6.1", "0.6.2", true},
		{"0.6.2", "0.6.2", false},
		{"0.6.3", "0.6.2", false},
		{"0.5.99", "0.6.0", true},
		{"1.0.0", "0.99.99", false},
		{"0.6", "0.6.2", true},   // missing patch component compares as 0
		{"0.6.2", "0.6", false},  // symmetric
		{"abc", "0.6.2", true},   // non-numeric -> 0
		{"0.6.2", "abc", false},  // non-numeric -> 0
	}
	for _, tc := range cases {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			if got := semverLess(tc.a, tc.b); got != tc.want {
				t.Fatalf("semverLess(%q,%q): want %v, got %v", tc.a, tc.b, tc.want, got)
			}
		})
	}
}

// TestWarnIfBonesOutdated_OldVersion stubs `bones --version` to print a
// pre-minimum release and verifies the warning is emitted to stderr with the
// brew upgrade hint.
func TestWarnIfBonesOutdated_OldVersion(t *testing.T) {
	stubDir := t.TempDir()
	bonesStub := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'bones 0.6.1 (commit deadbeef, built 2026-04-01T00:00:00Z)'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "bones"), []byte(bonesStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	stderr, err := captureStderr(func() error {
		warnIfBonesOutdated()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "warning: bones 0.6.1") {
		t.Fatalf("expected old-version warning, got: %q", stderr)
	}
	if !strings.Contains(stderr, "brew upgrade") {
		t.Fatalf("warning should suggest brew upgrade, got: %q", stderr)
	}
}

// TestWarnIfBonesOutdated_CurrentVersion confirms the warning is silent when
// bones is at the recommended minimum.
func TestWarnIfBonesOutdated_CurrentVersion(t *testing.T) {
	stubDir := t.TempDir()
	bonesStub := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'bones " + minBonesVersion + " (commit x, built y)'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "bones"), []byte(bonesStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	stderr, _ := captureStderr(func() error { warnIfBonesOutdated(); return nil })
	if strings.Contains(stderr, "warning: bones") {
		t.Fatalf("current version should not warn, got: %q", stderr)
	}
}

// TestWarnIfBonesOutdated_FutureVersion confirms forward-compat: a newer
// bones (e.g. 1.0.0) does not produce a warning.
func TestWarnIfBonesOutdated_FutureVersion(t *testing.T) {
	stubDir := t.TempDir()
	bonesStub := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'bones 1.0.0'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "bones"), []byte(bonesStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	stderr, _ := captureStderr(func() error { warnIfBonesOutdated(); return nil })
	if strings.Contains(stderr, "warning: bones") {
		t.Fatalf("future version should not warn, got: %q", stderr)
	}
}

// TestWarnIfBonesOutdated_UnparseableOutput confirms the warning silently
// no-ops when bones --version output can't be parsed (defensive: don't panic
// or print noise on bonesStub failures).
func TestWarnIfBonesOutdated_UnparseableOutput(t *testing.T) {
	stubDir := t.TempDir()
	bonesStub := "#!/bin/sh\necho 'gibberish'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "bones"), []byte(bonesStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	stderr, _ := captureStderr(func() error { warnIfBonesOutdated(); return nil })
	if stderr != "" {
		t.Fatalf("unparseable version should be silent, got: %q", stderr)
	}
}

func TestInit_PassesWhenAllPrereqsPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	stubDir := t.TempDir()
	for _, b := range []string{"bones", "scion", "docker"} {
		os.WriteFile(filepath.Join(stubDir, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", stubDir)

	if err := runInit([]string{tmp}); err != nil {
		t.Fatalf("init should pass with all prereqs on PATH: %v", err)
	}
}
