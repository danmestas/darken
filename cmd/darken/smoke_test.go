// Package main — C5 wave e2e smoke tests.
//
// TestSmoke_SpawnRunMessageStopDelete_Integration exercises the full
// darken spawn -> agent-running -> message -> stop -> delete lifecycle
// via Go-side wrappers and a fake scion binary in PATH.
// Full container e2e (Docker image pull, real Claude session) is
// deferred to CI.
//
// TestSmoke_SubstrateManifestExpansion exercises the full
// substrateResolver -> expandManifest chain for all 14 canonical roles,
// asserting that every embedded template expands cleanly when
// DARKEN_HUB_ENDPOINT is set.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmoke_SpawnRunMessageStopDelete_Integration is the C5 e2e smoke.
// Phases:
//  1. spawn smoke-c5 --type researcher --no-stage -> assert running
//  2. scionListAgents -> confirm phase=running
//  3. scion message smoke-c5 "commit sentinel file" -> assert sentinel written
//  4. scion stop smoke-c5
//  5. scion delete smoke-c5
//  6. scionListAgents -> assert smoke-c5 absent (teardown clean)
//  7. assert all expected scion invocations were recorded
func TestSmoke_SpawnRunMessageStopDelete_Integration(t *testing.T) {
	fakeDir := t.TempDir()

	// Build the fake scion binary with fakeDir baked into the script so
	// it can maintain state without requiring any extra env var injection.
	fakeScionScript := fmt.Sprintf(`#!/bin/sh
FAKE="%s"
CMD="$1"; shift
case "$CMD" in
  start)
    NAME="$1"
    echo "$NAME" > "$FAKE/agent.name"
    echo "start $NAME $*" >> "$FAKE/invocations.txt"
    exit 0
    ;;
  list)
    if [ ! -f "$FAKE/agent.name" ] || [ -f "$FAKE/deleted" ]; then
      printf '[]\n'
    else
      N=$(cat "$FAKE/agent.name")
      PH="running"
      [ -f "$FAKE/stopped" ] && PH="stopped"
      printf '[{"name":"%%s","phase":"%%s","template":"researcher"}]\n' "$N" "$PH"
    fi
    exit 0
    ;;
  message)
    MN="$1"; shift
    echo "message $MN $*" >> "$FAKE/invocations.txt"
    printf 'sentinel\n' > "$FAKE/sentinel.committed"
    exit 0
    ;;
  stop)
    echo "stop $*" >> "$FAKE/invocations.txt"
    touch "$FAKE/stopped"
    exit 0
    ;;
  delete)
    echo "delete $*" >> "$FAKE/invocations.txt"
    touch "$FAKE/deleted"
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`, fakeDir)

	if err := os.WriteFile(filepath.Join(fakeDir, "scion"), []byte(fakeScionScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// Prepend fakeDir to PATH so both scionCmd (via exec.Command LookPath)
	// and scionListAgents (direct exec.Command) find the fake binary.
	t.Setenv("PATH", fakeDir+":"+os.Getenv("PATH"))

	const agentName = "smoke-c5"
	const role = "researcher"

	// Phase 1 - spawn: assert no error; poller sees phase=running on first tick.
	if err := runSpawn([]string{agentName, "--type", role, "--no-stage", "smoke task"}); err != nil {
		t.Fatalf("Phase 1 spawn: %v", err)
	}

	// Phase 2 - assert agent in running phase.
	agents, err := scionListAgents()
	if err != nil {
		t.Fatalf("Phase 2 list: %v", err)
	}
	var agentRunning bool
	for _, a := range agents {
		if a.Name == agentName && a.Phase == "running" {
			agentRunning = true
		}
	}
	if !agentRunning {
		t.Errorf("Phase 2: agent %q not in running phase; got: %+v", agentName, agents)
	}

	// Phase 3 - message: simulate commit of sentinel file.
	msgCmd := scionCmd([]string{"message", agentName, "commit sentinel file to confirm substrate"})
	msgCmd.Stdout = io.Discard
	msgCmd.Stderr = io.Discard
	if err := msgCmd.Run(); err != nil {
		t.Fatalf("Phase 3 message: %v", err)
	}

	// Phase 4 - assert sentinel committed.
	if _, err := os.Stat(filepath.Join(fakeDir, "sentinel.committed")); err != nil {
		t.Errorf("Phase 4: sentinel not written after message: %v", err)
	}

	// Phase 5 - stop agent.
	stopCmd := scionCmd([]string{"stop", agentName})
	stopCmd.Stdout = io.Discard
	stopCmd.Stderr = io.Discard
	if err := stopCmd.Run(); err != nil {
		t.Fatalf("Phase 5 stop: %v", err)
	}

	// Phase 6 - delete agent.
	delCmd := scionCmd([]string{"delete", agentName})
	delCmd.Stdout = io.Discard
	delCmd.Stderr = io.Discard
	if err := delCmd.Run(); err != nil {
		t.Fatalf("Phase 6 delete: %v", err)
	}

	// Phase 7 - teardown clean: agent must not appear in list after delete.
	agents, err = scionListAgents()
	if err != nil {
		t.Fatalf("Phase 7 list after delete: %v", err)
	}
	for _, a := range agents {
		if a.Name == agentName {
			t.Errorf("Phase 7: agent %q still listed after delete (phase=%s)", agentName, a.Phase)
		}
	}

	// Assert all expected scion invocations were recorded in the fake binary.
	invPath := filepath.Join(fakeDir, "invocations.txt")
	invBytes, err := os.ReadFile(invPath)
	if err != nil {
		t.Fatalf("invocations.txt not found: %v", err)
	}
	inv := string(invBytes)
	for _, want := range []string{
		"start " + agentName,
		"message " + agentName,
		"stop " + agentName,
		"delete " + agentName,
	} {
		if !strings.Contains(inv, want) {
			t.Errorf("expected invocation %q missing from:\n%s", want, inv)
		}
	}
}

// TestSmoke_SubstrateManifestExpansion exercises the full
// substrateResolver -> expandManifest chain for all 14 canonical roles.
// Uses the embedded substrate (no filesystem dependency).
// Asserts: every template expands with no leftover DARKEN_ placeholders,
// no hardcoded host.docker.internal, and DARKEN_HUB_ENDPOINT is
// substituted with the test value.
func TestSmoke_SubstrateManifestExpansion(t *testing.T) {
	t.Setenv("DARKEN_HUB_ENDPOINT", "http://smoke-hub:9191")

	r := substrateResolver()

	for _, role := range canonicalRoles {
		role := role
		t.Run(role, func(t *testing.T) {
			path := ".scion/templates/" + role + "/scion-agent.yaml"
			body, err := r.ReadFile(path)
			if err != nil {
				t.Fatalf("read embedded template %s: %v", path, err)
			}
			expanded := expandManifest(string(body))
			if strings.Contains(expanded, "${DARKEN_") {
				t.Errorf("unexpanded DARKEN_ placeholder remains: %s", expanded)
			}
			if strings.Contains(expanded, "host.docker.internal") {
				t.Errorf("hardcoded host.docker.internal survived expansion: %s", expanded)
			}
			if !strings.Contains(expanded, "http://smoke-hub:9191") {
				t.Errorf("DARKEN_HUB_ENDPOINT not substituted; expanded:\n%s", expanded)
			}
		})
	}
}
