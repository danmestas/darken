package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsForwardsToScript(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runSkills([]string{"sme", "--diff"})
	body, _ := os.ReadFile(log)
	// runSubstrateScript extracts the embedded script to a randomly-named
	// tmpfile, so we no longer assert on "stage-skills.sh" in the path.
	// The contract is that harness + flags reach bash unchanged.
	if !strings.Contains(string(body), "sme") {
		t.Fatalf("harness `sme` not forwarded: %s", body)
	}
	if !strings.Contains(string(body), "--diff") {
		t.Fatalf("--diff not forwarded: %s", body)
	}
}
