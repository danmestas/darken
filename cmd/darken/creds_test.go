package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreds_StagesClaudeViaScionClient verifies the native Go path:
// runCreds("claude") reads from the macOS Keychain and pushes via
// ScionClient.PushFileSecret with the canonical name and target path.
func TestCreds_StagesClaudeViaScionClient(t *testing.T) {
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "security"),
		[]byte("#!/bin/sh\necho 'fake-creds-blob'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := runCreds([]string{"claude"}); err != nil {
		t.Fatalf("runCreds: %v", err)
	}
	if len(mc.pushFileSecretCalls) != 1 {
		t.Fatalf("expected 1 PushFileSecret call; got %d", len(mc.pushFileSecretCalls))
	}
	call := mc.pushFileSecretCalls[0]
	if call[0] != "claude_auth" {
		t.Errorf("secret name: want claude_auth, got %s", call[0])
	}
	if call[1] != "/home/scion/.claude/.credentials.json" {
		t.Errorf("target: want /home/scion/.claude/.credentials.json, got %s", call[1])
	}
}

// TestCreds_PiUsesEnvSecret verifies the env-secret path: when
// OPENROUTER_API_KEY is set, runCreds("pi") pushes it via PushEnvSecret
// rather than PushFileSecret.
func TestCreds_PiUsesEnvSecret(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no security/scion stubs needed
	t.Setenv("OPENROUTER_API_KEY", "test-key-xyz")
	mc := &mockScionClient{}
	setDefaultClient(t, mc)
	if err := runCreds([]string{"pi"}); err != nil {
		t.Fatalf("runCreds: %v", err)
	}
	if len(mc.pushEnvSecretCalls) != 1 {
		t.Fatalf("expected 1 PushEnvSecret call; got %d", len(mc.pushEnvSecretCalls))
	}
	if mc.pushEnvSecretCalls[0][0] != "OPENROUTER_API_KEY" {
		t.Errorf("env name: want OPENROUTER_API_KEY, got %s", mc.pushEnvSecretCalls[0][0])
	}
	if mc.pushEnvSecretCalls[0][1] != "test-key-xyz" {
		t.Errorf("env value: want test-key-xyz, got %s", mc.pushEnvSecretCalls[0][1])
	}
}

// TestCreds_AllSoftFailsPerBackend verifies the "all" mode soft-fails
// per backend: a missing claude keychain shouldn't prevent codex/pi
// from being staged. With no keychain, no codex file, no env vars, no
// gemini file: zero successful pushes, zero errors returned. HOME is
// redirected to a temp dir so the test is independent of the host's
// real ~/.codex and ~/.gemini state.
func TestCreds_AllSoftFailsPerBackend(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	mc := &mockScionClient{}
	setDefaultClient(t, mc)
	if err := runCreds([]string{"all"}); err != nil {
		t.Fatalf("all mode should not return error on per-backend failures: %v", err)
	}
	if len(mc.pushFileSecretCalls)+len(mc.pushEnvSecretCalls) != 0 {
		t.Fatalf("expected zero pushes when all backends absent; got %d file + %d env",
			len(mc.pushFileSecretCalls), len(mc.pushEnvSecretCalls))
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
