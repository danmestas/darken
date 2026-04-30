package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStripANSI asserts that stripANSI removes ANSI escape sequences
// from input while leaving plain text untouched.
func TestStripANSI(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no escapes",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "color reset",
			input: "\x1b[0mhello\x1b[0m",
			want:  "hello",
		},
		{
			name:  "bold green text",
			input: "\x1b[1;32mrunning\x1b[0m",
			want:  "running",
		},
		{
			name:  "cursor movement",
			input: "\x1b[2J\x1b[Hsome output",
			want:  "some output",
		},
		{
			name:  "mixed content",
			input: "phase: \x1b[32mrunning\x1b[0m (pid 42)",
			want:  "phase: running (pid 42)",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripANSI([]byte(tc.input))
			if string(got) != tc.want {
				t.Errorf("stripANSI(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestRunLook_StripsANSIFromScionOutput asserts that runLook pipes scion
// look output through stripANSI so the caller sees clean text.
func TestRunLook_StripsANSIFromScionOutput(t *testing.T) {
	// Stub scion to emit ANSI-decorated output on `look` subcommand.
	stubDir := t.TempDir()
	rawOutput := "\x1b[1;32mworker-1\x1b[0m phase: \x1b[33mrunning\x1b[0m\nsome log line\n"
	script := "#!/bin/sh\nprintf '" + rawOutput + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	var buf bytes.Buffer
	if err := runLookInto([]string{"worker-1"}, &buf); err != nil {
		t.Fatalf("runLookInto returned error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output still contains ANSI escapes: %q", out)
	}
	if !strings.Contains(out, "worker-1") {
		t.Errorf("expected agent name in output, got: %q", out)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("expected phase in output, got: %q", out)
	}
}

// TestRunLook_RequiresAgentArg asserts that runLook errors when no
// agent name is supplied.
func TestRunLook_RequiresAgentArg(t *testing.T) {
	var buf bytes.Buffer
	err := runLookInto([]string{}, &buf)
	if err == nil {
		t.Fatal("expected error when no agent name provided")
	}
	if !strings.Contains(err.Error(), "agent") {
		t.Errorf("error should mention agent argument, got: %v", err)
	}
}
