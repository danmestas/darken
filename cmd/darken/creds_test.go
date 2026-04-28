package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredsForwardsToScript(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runCreds([]string{"claude"})
	body, _ := os.ReadFile(log)
	// runSubstrateScript extracts the embedded script to a randomly-named
	// tmpfile, so we no longer assert on "stage-creds.sh" in the path.
	// The contract is that the forwarded backend arg reaches bash.
	if !strings.Contains(string(body), "claude") {
		t.Fatalf("backend arg `claude` not forwarded: %s", body)
	}
}
