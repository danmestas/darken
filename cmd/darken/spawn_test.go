package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpawnInvokesStageThenScion(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// scion stub: log invocation args (used to assert `start smoke-1`).
	// Also handle `list --format json` so the post-start poller sees
	// phase=running and exits promptly. (Phase 7 Task 2 wires the poll
	// after scion start; without the list branch the poller would
	// fail to parse empty stdout and runSpawn would error.)
	scionStub := filepath.Join(dir, "scion")
	if err := os.WriteFile(scionStub, []byte(
		"#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"+
			"case \"$1\" in\n"+
			"  start) exit 0 ;;\n"+
			"  list)  echo '[{\"name\":\"smoke-1\",\"phase\":\"running\"}]'; exit 0 ;;\n"+
			"  *)     exit 0 ;;\n"+
			"esac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// bash stub: log args + dump the script body. spawn now extracts
	// embedded substrate scripts to a temp file, so the file name is
	// random — but the body's own header comment names the script
	// (e.g. "# stage-creds.sh — ..."), which we can grep for.
	bashStub := filepath.Join(dir, "bash")
	if err := os.WriteFile(bashStub, []byte(
		"#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"smoke-1", "--type", "researcher", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// stage-creds is now native Go (Phase F), not a bash invocation;
	// the test asserts spawn proceeded to scion start, which is the
	// real ordering contract.
	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "start") {
		t.Fatalf("scion start not invoked: %s", body)
	}
	// REVIEW-7 consolidated skill staging into the Go-side
	// buildSkillsStaging; spawn no longer invokes a stage-skills.sh
	// shell script, so the prior bash-stub assertion has been removed.
	if !strings.Contains(string(body), "start smoke-1") {
		t.Fatalf("scion start not invoked: %s", body)
	}
}

func TestSpawnReturnsAfterReady(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// scion stub: scion start logs the call AND scion list returns running
	scionStub := `#!/bin/sh
echo "$0 $@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"smoke-1","phase":"running"}]'; exit 0 ;;
  *)     exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	start := time.Now()
	if err := runSpawn([]string{"smoke-1", "--type", "researcher", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Should return promptly because phase=running on first poll.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("spawn returned too slowly (%s); should poll fast and exit on first running tick", elapsed)
	}

	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "start smoke-1") {
		t.Fatalf("scion start not invoked: %s", body)
	}
	if !strings.Contains(string(body), "list --format json") {
		t.Fatalf("scion list not invoked for ready-poll: %s", body)
	}
}

func TestSpawn_WatchFlagPassesAttach(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	scionStub := `#!/bin/sh
echo "$0 $@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"smoke-watch","phase":"running"}]'; exit 0 ;;
esac
`
	os.WriteFile(filepath.Join(dir, "scion"), []byte(scionStub), 0o755)
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"smoke-watch", "--type", "researcher", "--watch", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "--attach") {
		t.Fatalf("--watch should pass --attach to scion start: %s", body)
	}
}

// TestRunSpawn_TaskAsPositional pins the v0.1.15 fix: positional words
// after flags are forwarded verbatim to scion and no --notify flag is
// injected by darken itself.
func TestRunSpawn_TaskAsPositional(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	scionStub := `#!/bin/sh
echo "$@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"agentname","phase":"running"}]'; exit 0 ;;
  *)     exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\ncat \"$1\" >> /dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"agentname", "--type", "researcher", "do the thing", "with multiple words"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	body, _ := os.ReadFile(log)
	got := string(body)

	// Positional words must appear in the scion invocation.
	if !strings.Contains(got, "do the thing") {
		t.Errorf("posArg 'do the thing' not forwarded to scion: %s", got)
	}
	if !strings.Contains(got, "with multiple words") {
		t.Errorf("posArg 'with multiple words' not forwarded to scion: %s", got)
	}
	// darken must NOT inject --notify into the scion call.
	if strings.Contains(got, "--notify") {
		t.Errorf("darken injected --notify flag into scion args: %s", got)
	}
}

// TestRunSpawn_FailsWithoutType guards that --type is required.
func TestRunSpawn_FailsWithoutType(t *testing.T) {
	err := runSpawn([]string{"myagent"})
	if err == nil {
		t.Fatal("expected error when --type is missing, got nil")
	}
}

// TestSpawn_DoesNotForwardCommandArgsToScion asserts that runSpawn does NOT
// append manifest command_args to the scion start argv. scion start has no
// --betas flag; forwarding raw harness flags through the orchestration CLI
// crosses the abstraction boundary. command_args stays readable in the struct
// but is a no-op until upstream scion exposes harness-level flag routing.
func TestSpawn_DoesNotForwardCommandArgsToScion(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	// Create a temporary templates dir with a manifest that declares command_args.
	// No canonical skills dir needed because we use --no-stage.
	tmpDir := t.TempDir()
	harnessDir := filepath.Join(tmpDir, "custom-role")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "default_harness_config: claude\ncommand_args:\n  - --betas\n  - context-1m-2025-08-07\n"
	if err := os.WriteFile(filepath.Join(harnessDir, "scion-agent.yaml"),
		[]byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKEN_TEMPLATES_DIR", tmpDir)

	// Scion stub for the readiness poller only (start is handled by mock).
	stubDir := t.TempDir()
	scionStub := `#!/bin/sh
case "$1" in
  list) echo '[{"name":"cmd-agent","phase":"running"}]'; exit 0 ;;
  *)    exit 0 ;;
esac
`
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte(scionStub), 0o755)
	os.WriteFile(filepath.Join(stubDir, "bash"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"cmd-agent", "--type", "custom-role", "--no-stage", "task"}); err != nil {
		t.Fatalf("runSpawn: %v", err)
	}

	if len(mc.startAgentCalls) == 0 {
		t.Fatal("StartAgent was not called")
	}
	args := strings.Join(mc.startAgentCalls[0], " ")
	// command_args must NOT be forwarded to scion start.
	if strings.Contains(args, "--betas") {
		t.Errorf("--betas must not be forwarded to scion start; args: %s", args)
	}
	if strings.Contains(args, "context-1m-2025-08-07") {
		t.Errorf("context-1m-2025-08-07 must not be forwarded to scion start; args: %s", args)
	}
}

// TestWithModeOverride_SetsEnvVar pins that DARKEN_MODE_OVERRIDE is set
// inside the closure when a non-empty mode name is passed, and is restored
// (unset, in this test) after the closure returns. This is the mechanism
// `darken spawn --mode <name>` uses to route an override into the staging
// pipeline without changing function signatures down the call chain.
func TestWithModeOverride_SetsEnvVar(t *testing.T) {
	t.Setenv("DARKEN_MODE_OVERRIDE", "") // start unset
	os.Unsetenv("DARKEN_MODE_OVERRIDE")
	called := false
	err := withModeOverride("custom-mode", func() error {
		called = true
		got := os.Getenv("DARKEN_MODE_OVERRIDE")
		if got != "custom-mode" {
			t.Errorf("DARKEN_MODE_OVERRIDE = %q; want %q", got, "custom-mode")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withModeOverride: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
	if got := os.Getenv("DARKEN_MODE_OVERRIDE"); got != "" {
		t.Errorf("DARKEN_MODE_OVERRIDE leaked after withModeOverride returned: %q", got)
	}
}

// TestWithModeOverride_EmptyDoesNotSet confirms the no-override path is a
// pure pass-through: an empty mode argument leaves the env var untouched
// so the script falls back to the manifest's default_mode.
func TestWithModeOverride_EmptyDoesNotSet(t *testing.T) {
	t.Setenv("DARKEN_MODE_OVERRIDE", "")
	os.Unsetenv("DARKEN_MODE_OVERRIDE")
	err := withModeOverride("", func() error {
		if got := os.Getenv("DARKEN_MODE_OVERRIDE"); got != "" {
			t.Errorf("DARKEN_MODE_OVERRIDE should not be set for empty override; got %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withModeOverride: %v", err)
	}
}

// TestSpawn_ManifestParseError_IsFatal asserts that runSpawn returns a non-nil
// error when the manifest for the requested role exists on disk but cannot be
// parsed (e.g. unknown backend). Silently degrading on parse errors hides
// configuration mistakes that should be fixed before the agent starts.
func TestSpawn_ManifestParseError_IsFatal(t *testing.T) {
	// Write a manifest with an unknown backend value so loadHarnessManifest
	// returns a parse error (not a file-not-found error).
	tmpDir := t.TempDir()
	harnessDir := filepath.Join(tmpDir, "bad-role")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	badManifest := "default_harness_config: totally-unknown-backend\n"
	if err := os.WriteFile(filepath.Join(harnessDir, "scion-agent.yaml"),
		[]byte(badManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKEN_TEMPLATES_DIR", tmpDir)

	// --no-stage skips credential/skills staging so the test only exercises
	// the manifest-load path.
	err := runSpawn([]string{"bad-agent", "--type", "bad-role", "--no-stage", "task"})
	if err == nil {
		t.Fatal("expected error for malformed manifest, got nil")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error should mention 'manifest'; got: %v", err)
	}
}
