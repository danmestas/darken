package main

import (
	"strings"
	"testing"
)

// TestSpawnHelpPrintsSubcommandDocs confirms that `darken spawn --help` returns
// nil and prints spawn-specific synopsis instead of falling through to top-level.
func TestSpawnHelpPrintsSubcommandDocs(t *testing.T) {
	out, err := captureStderr(func() error {
		return runSpawn([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("spawn --help should return nil, got: %v", err)
	}
	for _, want := range []string{"darken spawn", "--type", "Example"} {
		if !strings.Contains(out, want) {
			t.Fatalf("spawn --help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Subcommands:") {
		t.Fatalf("spawn --help should not print top-level usage:\n%s", out)
	}
}

// TestDoctorHelpPrintsSubcommandDocs confirms that `darken doctor --help` returns
// nil and prints doctor-specific synopsis.
func TestDoctorHelpPrintsSubcommandDocs(t *testing.T) {
	out, err := captureStderr(func() error {
		return runDoctor([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("doctor --help should return nil, got: %v", err)
	}
	for _, want := range []string{"darken doctor", "--init", "Example"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor --help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Subcommands:") {
		t.Fatalf("doctor --help should not print top-level usage:\n%s", out)
	}
}

// TestUpHelpPrintsSubcommandDocs confirms that `darken up --help` returns nil
// and prints up-specific synopsis.
func TestUpHelpPrintsSubcommandDocs(t *testing.T) {
	out, err := captureStderr(func() error {
		return runUp([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("up --help should return nil, got: %v", err)
	}
	for _, want := range []string{"darken up", "--no-bones", "Example"} {
		if !strings.Contains(out, want) {
			t.Fatalf("up --help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Subcommands:") {
		t.Fatalf("up --help should not print top-level usage:\n%s", out)
	}
}

// TestDownHelpPrintsSubcommandDocs confirms that `darken down --help` returns nil
// and prints down-specific synopsis.
func TestDownHelpPrintsSubcommandDocs(t *testing.T) {
	out, err := captureStderr(func() error {
		return runDown([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("down --help should return nil, got: %v", err)
	}
	for _, want := range []string{"darken down", "--yes", "Example"} {
		if !strings.Contains(out, want) {
			t.Fatalf("down --help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Subcommands:") {
		t.Fatalf("down --help should not print top-level usage:\n%s", out)
	}
}
