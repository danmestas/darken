package main

import (
	"strings"
	"testing"
)

func TestStatusOutputFormat(t *testing.T) {
	out, err := captureStdout(func() error { return runStatus(nil) })
	if err != nil {
		t.Fatal(err)
	}
	// Format: [darken: orchestrator-mode | substrate <12-hex>]
	if !strings.HasPrefix(out, "[darken:") {
		t.Fatalf("status output missing [darken: prefix: %q", out)
	}
	if !strings.Contains(out, "substrate ") {
		t.Fatalf("status output missing substrate hash: %q", out)
	}
	if !strings.Contains(out, "orchestrator-mode") {
		t.Fatalf("status output missing mode label: %q", out)
	}
}

func TestStatusRejectsArgs(t *testing.T) {
	if err := runStatus([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}
