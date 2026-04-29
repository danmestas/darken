package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredsForwardsToScript(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runCreds([]string{"claude"})
	body, _ := os.ReadFile(log)
	// runSubstrateScript extracts the embedded script to a randomly-named
	// tmpfile, so we no longer assert on "stage-creds.sh" in the path.
	// The contract is that the forwarded backend arg reaches bash.
	if !strings.Contains(string(body), "claude") {
		t.Fatalf("backend arg `claude` not forwarded: %s", body)
	}
}

// TestStageCredsNoSuccessOnFailure verifies that stage-creds.sh does NOT
// print the "pushed" success line when the scion hub PUT fails (scion
// exits non-zero). Uses the "all" dispatch path (stage_pi || true) which
// is the context where set -e is suppressed inside push_env_secret.
func TestStageCredsNoSuccessOnFailure(t *testing.T) {
	stubDir := t.TempDir()
	// scion stub exits 1 (simulates hub PUT failure).
	// Also stub security to suppress the claude keychain warning path.
	for _, b := range []string{"scion", "security"} {
		script := "#!/bin/sh\nexit 1\n"
		os.WriteFile(filepath.Join(stubDir, b), []byte(script), 0o755)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	// "all" triggers stage_pi || true which is the set-e-suppressed path.
	out, _ := runSubstrateScriptCaptured("scripts/stage-creds.sh", []string{"all"})
	if strings.Contains(out, "pushed") {
		t.Fatalf("success line must not appear when scion exits non-zero, got: %q", out)
	}
}

// TestStageCredsSuccessOnOK verifies that stage-creds.sh prints the
// "pushed" success line when scion exits 0 (hub PUT succeeded).
func TestStageCredsSuccessOnOK(t *testing.T) {
	stubDir := t.TempDir()
	// scion stub exits 0 (simulates successful PUT).
	for _, b := range []string{"scion", "security"} {
		script := "#!/bin/sh\nexit 0\n"
		os.WriteFile(filepath.Join(stubDir, b), []byte(script), 0o755)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	out, _ := runSubstrateScriptCaptured("scripts/stage-creds.sh", []string{"all"})
	if !strings.Contains(out, "pushed") {
		t.Fatalf("success line must appear when scion exits 0, got: %q", out)
	}
}
