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
	if !strings.Contains(string(body), "stage-skills.sh") {
		t.Fatalf("skills did not invoke stage-skills.sh: %s", body)
	}
	if !strings.Contains(string(body), "--diff") {
		t.Fatalf("--diff not forwarded: %s", body)
	}
}
