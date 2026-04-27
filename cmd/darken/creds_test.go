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
	if !strings.Contains(string(body), "stage-creds.sh") {
		t.Fatalf("creds did not invoke stage-creds.sh: %s", body)
	}
	if !strings.Contains(string(body), "claude") {
		t.Fatalf("backend arg `claude` not forwarded: %s", body)
	}
}
