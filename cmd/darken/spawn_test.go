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
	for _, b := range []string{"scion", "bash"} {
		stub := filepath.Join(dir, b)
		if err := os.WriteFile(stub, []byte(
			"#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
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
