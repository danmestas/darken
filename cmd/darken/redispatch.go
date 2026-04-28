// Package main — `darken redispatch <agent>` kills the named agent
// (via scion stop) and re-spawns it with the same role. The role is
// looked up from `scion list --format json`. Worker worktree is
// preserved by scion across stop/start; redispatch treats it as a
// fresh start (commits are the durable state).
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func runRedispatch(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: darken redispatch <agent>")
	}
	name := args[0]

	// Look up the role from scion list --format json.
	agents, err := scionListAgents()
	if err != nil {
		return fmt.Errorf("scion list failed: %w", err)
	}
	var role string
	for _, a := range agents {
		if a.Name == name {
			role = a.Template
			break
		}
	}
	if role == "" {
		return fmt.Errorf("agent %q not found in scion list (use `darken spawn %s --type <role>` to start fresh)", name, name)
	}

	// Stop the agent. Tolerate "already stopped" — scion stop returns 0
	// on a missing agent in current versions; if that changes, treat
	// non-zero as a soft failure (we still want to attempt re-spawn).
	stop := exec.Command("scion", "stop", name)
	stop.Stdout = os.Stdout
	stop.Stderr = os.Stderr
	if err := stop.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "redispatch: scion stop %s returned %v (continuing)\n", name, err)
	}

	// Re-spawn via the existing spawn path. This invokes the readiness
	// poll, prints per-phase progress, etc.
	return runSpawn([]string{name, "--type", role})
}
