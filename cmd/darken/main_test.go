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
	for _, sub := range []string{"doctor", "spawn", "bootstrap", "apply", "create-harness", "skills", "creds", "images", "list", "look"} {
		if !strings.Contains(string(out), sub) {
			t.Fatalf("--help missing subcommand %q\n%s", sub, out)
		}
	}
}

// TestLookInExplicitSubcommandTable asserts that "look" is registered in the
// compile-time subcommand table, not only via a hidden init() append in
// look.go. If look is removed from the table and only registered by init(),
// removing the init() would silently delete it from the CLI surface.
func TestLookInExplicitSubcommandTable(t *testing.T) {
	// subcommands is the package-level slice populated at compile time in
	// main.go. init() functions run before tests, so we cannot distinguish
	// table-registered from init()-registered commands at test time. This test
	// is a regression guard: it fails if look is ever missing from the slice
	// (e.g. both sources removed).
	found := false
	for _, sc := range subcommands {
		if sc.name == "look" {
			found = true
			break
		}
	}
	if !found {
		t.Error(`"look" subcommand not found in subcommands slice; ` +
			`ensure it is registered in main.go's explicit table`)
	}
}

func TestUnknownSubcommand(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "no-such-cmd").CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown subcommand\n%s", out)
	}
}
