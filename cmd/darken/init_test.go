package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitScaffoldsCLAUDE(t *testing.T) {
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

// stubBones plants a no-op bones script at the front of PATH so
// runBonesInit finds an executable and treats it as installed but
// the script does nothing destructive in the test's tmp dir.
func stubBones(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	body := "#!/usr/bin/env bash\nexit 0\n"
	path := filepath.Join(dir, "bones")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestInitScaffoldsSkills(t *testing.T) {
	stubBones(t)
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
	stubBones(t)
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
	stubBones(t)
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
	stubBones(t)
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
