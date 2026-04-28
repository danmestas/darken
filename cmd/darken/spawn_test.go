package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpawnInvokesStageThenScion(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// scion stub: log invocation args (used to assert `start smoke-1`).
	// Also handle `list --format json` so the post-start poller sees
	// phase=running and exits promptly. (Phase 7 Task 2 wires the poll
	// after scion start; without the list branch the poller would
	// fail to parse empty stdout and runSpawn would error.)
	scionStub := filepath.Join(dir, "scion")
	if err := os.WriteFile(scionStub, []byte(
		"#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"+
			"case \"$1\" in\n"+
			"  start) exit 0 ;;\n"+
			"  list)  echo '[{\"name\":\"smoke-1\",\"phase\":\"running\"}]'; exit 0 ;;\n"+
			"  *)     exit 0 ;;\n"+
			"esac\n"), 0o755); err != nil {
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

func TestSpawnReturnsAfterReady(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// scion stub: scion start logs the call AND scion list returns running
	scionStub := `#!/bin/sh
echo "$0 $@" >> ` + log + `
case "$1" in
  start) exit 0 ;;
  list)  echo '[{"name":"smoke-1","phase":"running"}]'; exit 0 ;;
  *)     exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	start := time.Now()
	if err := runSpawn([]string{"smoke-1", "--type", "researcher", "task..."}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Should return promptly because phase=running on first poll.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("spawn returned too slowly (%s); should poll fast and exit on first running tick", elapsed)
	}

	body, _ := os.ReadFile(log)
	if !strings.Contains(string(body), "start smoke-1") {
		t.Fatalf("scion start not invoked: %s", body)
	}
	if !strings.Contains(string(body), "list --format json") {
		t.Fatalf("scion list not invoked for ready-poll: %s", body)
	}
}
