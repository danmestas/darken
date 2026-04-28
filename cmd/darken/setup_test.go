package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubAllBinariesForSetup plants no-op bash + scion + docker + make
// stubs in a single tmpdir prepended to PATH. The bash stub logs each
// invocation so tests can assert the call sequence (init's bash for
// stage-skills, bootstrap's bash for stage-creds, etc.) and the
// scion stub no-ops for `server status`/`hub secret list` etc.
//
// Returns the log path. Tests grep its contents to verify what ran.
func stubAllBinariesForSetup(t *testing.T) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "binaries.log")

	bashStub := `#!/bin/sh
echo "bash $@" >> ` + logPath + `
exit 0
`
	scionStub := `#!/bin/sh
echo "scion $@" >> ` + logPath + `
case "$1" in
  list)
    case "$2" in
      "--format") echo "[]" ;;
      *) echo "" ;;
    esac
    ;;
  hub)
    if [ "$3" = "list" ]; then
      echo "claude_auth"
      echo "codex_auth"
      echo "gemini_auth"
    fi
    ;;
esac
exit 0
`
	dockerStub := `#!/bin/sh
echo "docker $@" >> ` + logPath + `
case "$1" in
  info) exit 0 ;;
  images)
    echo "local/darkish-claude:latest"
    echo "local/darkish-codex:latest"
    echo "local/darkish-pi:latest"
    echo "local/darkish-gemini:latest"
    ;;
esac
exit 0
`
	makeStub := `#!/bin/sh
echo "make $@" >> ` + logPath + `
exit 0
`
	bonesStub := `#!/bin/sh
echo "bones $@" >> ` + logPath + `
exit 0
`

	for name, body := range map[string]string{
		"bash":   bashStub,
		"scion":  scionStub,
		"docker": dockerStub,
		"make":   makeStub,
		"bones":  bonesStub,
	} {
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

func TestSetup_RunsInitThenBootstrap(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	logPath := stubAllBinariesForSetup(t)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runSetup(nil); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Init side: CLAUDE.md should exist.
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Errorf("init phase did not create CLAUDE.md: %v", err)
	}

	// Bootstrap side: the log should show scion server-status checks
	// (step 3 of bootstrap) AND the docker images listing (step 4).
	body, _ := os.ReadFile(logPath)
	got := string(body)
	if !strings.Contains(got, "scion server status") {
		t.Errorf("bootstrap phase did not run; missing `scion server status`:\n%s", got)
	}
	if !strings.Contains(got, "docker images") {
		t.Errorf("bootstrap phase did not run; missing `docker images`:\n%s", got)
	}
}

func TestSetup_ForceFlagPassedToInit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	stubAllBinariesForSetup(t)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	// Pre-plant a stale CLAUDE.md with a unique sentinel.
	stale := []byte("STALE-SENTINEL-DO-NOT-KEEP\n")
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), stale, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSetup([]string{"--force"}); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "STALE-SENTINEL") {
		t.Fatalf("--force did not regenerate CLAUDE.md; sentinel still present:\n%s", body)
	}
}

func TestSetup_AbortsOnInitFailure(t *testing.T) {
	logPath := stubAllBinariesForSetup(t)

	// Pass a non-existent positional target. runInit's existence check
	// fires before any artifacts are written; bootstrap should never run.
	bogus := filepath.Join(t.TempDir(), "does-not-exist-darken-setup-test")

	err := runSetup([]string{bogus})
	if err == nil {
		t.Fatal("expected error when init target missing")
	}
	if !strings.Contains(err.Error(), "target dir does not exist") {
		t.Fatalf("error should mention missing target: %v", err)
	}

	// Bootstrap was NOT called: no `scion server status` log entry from
	// bootstrap's step 3.
	body, _ := os.ReadFile(logPath)
	if strings.Contains(string(body), "scion server status") {
		t.Fatalf("bootstrap should not have been called after init failure:\n%s", body)
	}
}
