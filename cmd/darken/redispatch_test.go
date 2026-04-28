package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubScionForRedispatch plants a fake `scion` that:
//   - returns a JSON list with the named agent + its template, when
//     called as `scion list --format json`
//   - records every invocation to a log file (one line per invocation,
//     args space-separated)
//
// It also plants a no-op `bash` stub so runSpawn's stage-creds.sh /
// stage-skills.sh shellouts don't mutate the operator's working tree.
func stubScionForRedispatch(t *testing.T, agentName, template string) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "scion.log")

	body := `#!/bin/sh
echo "$@" >> ` + logPath + `
case "$1" in
  list)
    cat <<EOF
[{"name":"` + agentName + `","phase":"running","template":"` + template + `"}]
EOF
    ;;
  stop) exit 0 ;;
  start) exit 0 ;;
  hub) exit 0 ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	bashStub := `#!/bin/sh
exit 0
`
	if err := os.WriteFile(filepath.Join(stubDir, "bash"), []byte(bashStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

func TestRedispatch_StopThenRespawn(t *testing.T) {
	logPath := stubScionForRedispatch(t, "r1", "researcher")
	t.Setenv("DARKEN_SPAWN_READY_TIMEOUT", "2s")

	if err := runRedispatch([]string{"r1"}); err != nil {
		t.Fatalf("runRedispatch returned error: %v", err)
	}

	body, _ := os.ReadFile(logPath)
	got := string(body)
	stopIdx := strings.Index(got, "stop r1")
	startIdx := strings.Index(got, "start r1")
	if stopIdx < 0 {
		t.Fatalf("expected `scion stop r1` invocation, got log:\n%s", got)
	}
	if startIdx < 0 {
		t.Fatalf("expected `scion start r1` invocation, got log:\n%s", got)
	}
	if stopIdx >= startIdx {
		t.Fatalf("stop must precede start, got log:\n%s", got)
	}
	if !strings.Contains(got, "--type researcher") {
		t.Fatalf("expected `--type researcher` from list lookup, got log:\n%s", got)
	}
}

func TestRedispatch_AgentNotInList(t *testing.T) {
	stubDir := t.TempDir()
	body := `#!/bin/sh
case "$1" in
  list) echo "[]" ;;
  *) exit 0 ;;
esac
`
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := runRedispatch([]string{"ghost"})
	if err == nil {
		t.Fatal("expected error when agent not in scion list")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should mention not found: %v", err)
	}
	if !strings.Contains(err.Error(), "darken spawn") {
		t.Fatalf("error should hint at darken spawn: %v", err)
	}
}

func TestRedispatch_RequiresAgentArg(t *testing.T) {
	if err := runRedispatch(nil); err == nil {
		t.Fatal("expected error when no agent name given")
	}
}
