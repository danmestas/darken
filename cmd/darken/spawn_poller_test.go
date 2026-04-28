package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubScionList writes a fake `scion` binary to a tmp dir and prepends
// it to PATH. The stub reads the JSON body from the env var
// SCION_STUB_OUTPUT and prints it on `scion list --format json` calls.
func stubScionList(t *testing.T, jsonBody string) {
	t.Helper()
	stubDir := t.TempDir()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"list\" ]; then\n" +
		"  cat <<'EOF'\n" + jsonBody + "\nEOF\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
}

func TestPollUntilReady_ReturnsWhenRunning(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"running"}]`)

	start := time.Now()
	phase, err := pollUntilReady("researcher-1", 5*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if phase != "running" {
		t.Fatalf("expected phase=running, got %q", phase)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("returned too slowly (%s); should poll fast and return on first running tick", elapsed)
	}
}

func TestPollUntilReady_ErrorsOnAgentError(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"error"}]`)

	_, err := pollUntilReady("researcher-1", 5*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when agent phase=error")
	}
	if !strings.Contains(err.Error(), "error") {
		t.Fatalf("error should mention agent error, got: %v", err)
	}
}

func TestPollUntilReady_TimesOut(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"starting"}]`)

	start := time.Now()
	_, err := pollUntilReady("researcher-1", 500*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error should mention timeout, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestPollUntilReady_AgentNotFound(t *testing.T) {
	// scion list returns empty array — our agent isn't in the list yet.
	// The poller should keep polling until the configured timeout.
	stubScionList(t, `[]`)

	_, err := pollUntilReady("researcher-1", 300*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout when agent never appears")
	}
}

func TestPollUntilReady_ScionListErrors(t *testing.T) {
	// scion is not on PATH at all → poller should error after first attempt.
	t.Setenv("PATH", "/nonexistent")
	_, err := pollUntilReady("researcher-1", 1*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when scion CLI is missing")
	}
}
