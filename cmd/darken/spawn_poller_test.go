package main

import (
	"os"
	"path/filepath"
	"reflect"
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

// TestScionListAgents_StripsWarningPrefix asserts Bug 19: scion sometimes
// prepends a dev-auth WARNING line before the JSON array. scionListAgents
// must strip any non-JSON prefix lines so json.Unmarshal succeeds.
func TestScionListAgents_StripsWarningPrefix(t *testing.T) {
	stubDir := t.TempDir()
	// Stub output: WARNING line then valid JSON array.
	stubOutput := "WARNING: dev-auth mode; hub token unset\n" +
		`[{"name":"worker-1","phase":"running","template":"researcher"}]`
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '" + stubOutput + "'\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	agents, err := scionListAgents()
	if err != nil {
		t.Fatalf("expected success stripping WARNING prefix, got: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "worker-1" {
		t.Fatalf("expected [{worker-1 running researcher}], got %+v", agents)
	}
}

func TestPollUntilReady_ReturnsWhenRunning(t *testing.T) {
	stubScionList(t, `[{"name":"researcher-1","phase":"running"}]`)

	start := time.Now()
	phase, err := pollUntilReady("researcher-1", 5*time.Second, 100*time.Millisecond, nil)
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

	_, err := pollUntilReady("researcher-1", 5*time.Second, 100*time.Millisecond, nil)
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
	_, err := pollUntilReady("researcher-1", 500*time.Millisecond, 50*time.Millisecond, nil)
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

	_, err := pollUntilReady("researcher-1", 300*time.Millisecond, 50*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout when agent never appears")
	}
}

func TestPollUntilReady_ScionListErrors(t *testing.T) {
	// scion is not on PATH at all → poller should error after first attempt.
	t.Setenv("PATH", "/nonexistent")
	_, err := pollUntilReady("researcher-1", 1*time.Second, 100*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected error when scion CLI is missing")
	}
}

// TestJsonStart_WhitespacePrefixedLine asserts F-11: jsonStart must recognise
// a JSON array or object that is preceded by leading spaces or tabs on the
// first meaningful line, returning from the '[' / '{' byte rather than from
// the start of the line.
func TestJsonStart_WhitespacePrefixedLine(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
		// wantByte is the first byte of the slice jsonStart should return.
		wantByte byte
	}{
		{
			name:     "leading spaces before array",
			input:    []byte("  \t []\n"),
			wantByte: '[',
		},
		{
			name:     "leading tabs before object",
			input:    []byte("\t\t{}\n"),
			wantByte: '{',
		},
		{
			name:     "warning line then indented array",
			input:    []byte("WARNING: dev-auth mode\n  [{\"name\":\"a\"}]\n"),
			wantByte: '[',
		},
		{
			name:     "no whitespace prefix still works",
			input:    []byte("[{\"name\":\"a\"}]\n"),
			wantByte: '[',
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonStart(tc.input)
			if len(got) == 0 {
				t.Fatalf("jsonStart returned empty slice for input %q", tc.input)
			}
			if got[0] != tc.wantByte {
				t.Fatalf("jsonStart(%q)[0] = %q, want %q", tc.input, got[0], tc.wantByte)
			}
		})
	}
}

func TestPollUntilReady_CallbackFiresOnPhaseChange(t *testing.T) {
	// First call: phase=starting. Second call: phase=running.
	// Use a script that flips state via a sentinel file.
	stubDir := t.TempDir()
	flagFile := filepath.Join(stubDir, "called")
	body := `#!/bin/sh
if [ ! -f ` + flagFile + ` ]; then
  touch ` + flagFile + `
  echo '[{"name":"researcher-1","phase":"starting"}]'
else
  echo '[{"name":"researcher-1","phase":"running"}]'
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	var phases []string
	_, err := pollUntilReady("researcher-1", 5*time.Second, 50*time.Millisecond,
		func(phase string) { phases = append(phases, phase) })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"starting", "running"}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("expected phases %v, got %v", want, phases)
	}
}
