package main

import (
	"fmt"
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
# Path of the import-state file. Each role registered via 'templates import'
# adds a line; 'templates push' then requires the role be present.
state="` + filepath.Join(stubDir, "scion-import-state") + `"

# Strip global flags to find the subcommand.
while [ "$1" = "--global" ] || [ "$1" = "--no-hub" ]; do shift; done

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
  templates)
    sub="$2"
    case "$sub" in
      import)
        if [ "$3" = "--all" ]; then
          dir="$4"
          if [ ! -d "$dir" ]; then
            echo "Error: import --all: directory not found: $dir" >&2
            exit 1
          fi
          for d in "$dir"/*/; do
            [ -d "$d" ] || continue
            role=$(basename "$d")
            echo "$role" >> "$state"
          done
        else
          if [ -z "$3" ]; then
            echo "Error: import: path required" >&2
            exit 1
          fi
          role=$(basename "$3")
          echo "$role" >> "$state"
        fi
        ;;
      push)
        role="$3"
        if [ -z "$role" ]; then
          echo "Error: push: role required" >&2
          exit 1
        fi
        if [ ! -f "$state" ] || ! grep -qx "$role" "$state"; then
          echo "Error: template '$role' not found locally" >&2
          exit 1
        fi
        ;;
      list)
        if [ -f "$state" ]; then cat "$state"; fi
        ;;
    esac
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

// TestSetup_UploadsAllTemplatesToHub confirms runSetup calls PushTemplate for
// all 14 canonical roles via the ScionClient interface.
func TestSetup_UploadsAllTemplatesToHub(t *testing.T) {
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

	// The PATH scion stub logs every scion invocation. Verify that
	// --global templates push <role> appears for every canonical role.
	body, _ := os.ReadFile(logPath)
	log := string(body)
	for _, role := range canonicalRoles {
		want := "--global templates push " + role
		if !strings.Contains(log, want) {
			t.Errorf("expected scion %q to be called; log:\n%s", want, log)
		}
	}
}

// TestSetup_GroveInit_InvokedOnce confirms runSetup calls GroveInit exactly
// once when no .scion/grove-id file is present (fresh project).
func TestSetup_GroveInit_InvokedOnce(t *testing.T) {
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

	mc := &mockScionClient{
		serverStatusOut: "Status: ok\n",
		secretListOut:   "claude_auth\ncodex_auth\n",
	}
	setDefaultClient(t, mc)

	if err := runSetup(nil); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if mc.groveInitCalls != 1 {
		t.Errorf("GroveInit called %d times; want 1 (fresh project, no grove-id)", mc.groveInitCalls)
	}
}

// TestSetup_GroveInit_SkippedOnReSetup confirms runSetup does NOT call
// GroveInit when .scion/grove-id already exists (idempotent guard).
func TestSetup_GroveInit_SkippedOnReSetup(t *testing.T) {
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

	// Pre-plant a grove-id file to simulate an already-initialised project.
	scionDir := filepath.Join(root, ".scion")
	if err := os.MkdirAll(scionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scionDir, "grove-id"),
		[]byte("existing-grove-uuid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{
		serverStatusOut: "Status: ok\n",
		secretListOut:   "claude_auth\ncodex_auth\n",
	}
	setDefaultClient(t, mc)

	if err := runSetup(nil); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if mc.groveInitCalls != 0 {
		t.Errorf("GroveInit called %d times; want 0 (grove-id already present)", mc.groveInitCalls)
	}
}

// TestSetup_TemplateUploadFailureAbortsSetup confirms that a PushTemplate error
// propagates and aborts setup.
func TestSetup_TemplateUploadFailureAbortsSetup(t *testing.T) {
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

	// Inject a mock that passes all operations except PushTemplate.
	mc := &mockScionClient{
		serverStatusOut: "Status: ok\n",
		secretListOut:   "claude_auth\ncodex_auth\n",
		pushTemplateErr: fmt.Errorf("push failed: connection refused"),
	}
	setDefaultClient(t, mc)

	if err := runSetup(nil); err == nil {
		t.Fatal("expected setup to fail when template upload fails")
	}
}

// TestSetup_GroveInit_UsesTargetDir_NotCwd confirms that ensureGroveInit
// uses the explicit setup target directory, not the process working directory.
// This guards against the divergence where `darken setup /path/to/project`
// is run from a different directory: grove init must happen in /path/to/project.
func TestSetup_GroveInit_UsesTargetDir_NotCwd(t *testing.T) {
	targetDir := t.TempDir()
	otherDir := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", targetDir)
	stubAllBinariesForSetup(t)

	// cwd is otherDir — NOT the setup target.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{
		serverStatusOut: "Status: ok\n",
		secretListOut:   "claude_auth\ncodex_auth\n",
	}
	setDefaultClient(t, mc)

	if err := runSetup([]string{targetDir}); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if mc.groveInitDir != targetDir {
		t.Errorf("GroveInit used dir %q; want %q (the setup target, not cwd %q)",
			mc.groveInitDir, targetDir, otherDir)
	}
}
