package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapStepsAreOrdered(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	for _, b := range []string{"scion", "docker", "make", "bash"} {
		os.WriteFile(filepath.Join(dir, b),
			[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runBootstrap([]string{})

	body, _ := os.ReadFile(log)
	want := []string{"server", "make", "stage-creds.sh", "stage-skills.sh"}
	pos := -1
	for _, w := range want {
		i := strings.Index(string(body), w)
		if i < pos || i == -1 {
			t.Fatalf("step %q out of order or missing in: %s", w, body)
		}
		pos = i
	}
}
