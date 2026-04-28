package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func runSpawn(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: darken spawn <name> --type <role> [...]")
	}
	name := args[0]

	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	harnessType := fs.String("type", "", "harness role (e.g. researcher)")
	backend := fs.String("backend", "", "backend override (claude|codex|pi|gemini)")
	noStage := fs.Bool("no-stage", false, "skip stage-creds and stage-skills")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *harnessType == "" {
		return errors.New("--type is required")
	}
	posArgs := fs.Args()

	if !*noStage {
		if err := runSubstrateScript("scripts/stage-creds.sh", []string{"all"}); err != nil {
			fmt.Fprintln(os.Stderr, "spawn: stage-creds non-fatal:", err)
		}
		if err := runSubstrateScript("scripts/stage-skills.sh", []string{*harnessType}); err != nil {
			return fmt.Errorf("stage-skills failed: %w", err)
		}
	}

	cmd := []string{"start", name, "--type", *harnessType}
	if *backend != "" {
		image := fmt.Sprintf("local/darkish-%s:latest", *backend)
		cmd = append(cmd, "--harness", *backend, "--image", image)
	}
	if len(posArgs) > 0 {
		cmd = append(cmd, "--notify", posArgs[0])
	}

	c := exec.Command("scion", cmd...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	// Hybrid: surface scion start's own immediate failures (template
	// not found, bad args) directly. Post-dispatch failures (image
	// pull, container init) get caught by the readiness poll below.
	if err := c.Run(); err != nil {
		return err
	}

	// Read timeout override from env; default 15s.
	timeout := 15 * time.Second
	if v := os.Getenv("DARKEN_SPAWN_READY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	// Poll for ready (or error / timeout). Print one-line progress to stderr.
	fmt.Fprintf(os.Stderr, "[spawning %s] container starting\n", name)
	phase, err := pollUntilReady(name, timeout, 500*time.Millisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[spawning %s] FAILED at phase=%s — %v\n", name, phase, err)
		return fmt.Errorf("agent %s did not reach ready: %w", name, err)
	}
	fmt.Fprintf(os.Stderr, "[spawning %s] ready\n", name)
	return nil
}

// runShell invokes a shell script via bash. Stdout/stderr are inherited
// so the user sees script progress in-place.
//
// TODO: remove once all callers (bootstrap.go, creds.go, skills.go,
// apply.go) migrate to runSubstrateScript via subsequent Phase 5
// tasks. Spawn.go is the first migration; bootstrap.go is Task 2.
func runShell(script string, args ...string) error {
	c := exec.Command("bash", append([]string{script}, args...)...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
