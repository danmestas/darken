package main

import (
	"strings"
	"testing"
)

func TestVersionPrintsBinaryAndSubstrateHash(t *testing.T) {
	out, err := captureStdout(func() error { return runVersion(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "darken") {
		t.Fatalf("version output missing binary name: %q", out)
	}
	if !strings.Contains(out, "substrate sha256:") {
		t.Fatalf("version output missing substrate hash: %q", out)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	if err := runVersion([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}
