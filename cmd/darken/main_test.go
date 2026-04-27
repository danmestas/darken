package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestMainHelp(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help should not error: %v\n%s", err, out)
	}
	for _, sub := range []string{"doctor", "spawn", "bootstrap", "apply", "create-harness", "skills", "creds", "images", "list"} {
		if !strings.Contains(string(out), sub) {
			t.Fatalf("--help missing subcommand %q\n%s", sub, out)
		}
	}
}

func TestUnknownSubcommand(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "no-such-cmd").CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown subcommand\n%s", out)
	}
}
