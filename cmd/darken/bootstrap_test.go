package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureScionServer_UsesScionCmdFn asserts ensureScionServer routes
// through scionCmdFn so hub-endpoint env propagation applies.
func TestEnsureScionServer_UsesScionCmdFn(t *testing.T) {
	called := false
	orig := scionCmdFn
	t.Cleanup(func() { scionCmdFn = orig })
	scionCmdFn = func(args []string) *exec.Cmd {
		called = true
		// Simulate scion server status exits 0 (server running).
		return exec.Command("true")
	}
	_ = ensureScionServer()
	if !called {
		t.Error("ensureScionServer did not call scionCmdFn")
	}
}

func TestBootstrapStepsAreOrdered(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	for _, b := range []string{"scion", "docker", "make"} {
		os.WriteFile(filepath.Join(dir, b),
			[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	}
	// bash stub: log args + dump the script body. Bootstrap now extracts
	// embedded substrate scripts to a temp file, so the file name is
	// random — but the body's own header comment names the script
	// (e.g. "# stage-creds.sh — ..."), which we can grep for.
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755)
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
