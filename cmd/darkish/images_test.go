package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImagesForwardsToMake(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	os.WriteFile(filepath.Join(dir, "make"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runImages([]string{"claude"})
	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "claude") {
		t.Fatalf("make claude not invoked: %s", body)
	}
}
