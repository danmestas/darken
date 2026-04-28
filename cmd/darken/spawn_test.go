package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpawnInvokesStageThenScion(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// scion stub: log invocation args (used to assert `start smoke-1`).
	scionStub := filepath.Join(dir, "scion")
	if err := os.WriteFile(scionStub, []byte(
		"#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// bash stub: log args + dump the script body. spawn now extracts
	// embedded substrate scripts to a temp file, so the file name is
	// random — but the body's own header comment names the script
	// (e.g. "# stage-creds.sh — ..."), which we can grep for.
	bashStub := filepath.Join(dir, "bash")
	if err := os.WriteFile(bashStub, []byte(
		"#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"smoke-1", "--type", "researcher", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "stage-creds.sh") {
		t.Fatalf("stage-creds.sh not invoked: %s", body)
	}
	if !strings.Contains(string(body), "stage-skills.sh") {
		t.Fatalf("stage-skills.sh not invoked: %s", body)
	}
	if !strings.Contains(string(body), "start smoke-1") {
		t.Fatalf("scion start not invoked: %s", body)
	}
}
