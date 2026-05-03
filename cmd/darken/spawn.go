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
	// Handle --help / -h before any other parsing so subcommand-specific
	// docs are printed rather than falling through to top-level help.
	// Must be checked before the positional-name extraction below.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			// Build flagset solely to render PrintDefaults.
			fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
			fs.String("type", "", "harness role (e.g. researcher)")
			fs.String("backend", "", "backend override (claude|codex|pi|gemini)")
			fs.String("mode", "", "skill-set mode override (defaults to role's default_mode)")
			fs.Bool("no-stage", false, "skip stage-creds and stage-skills")
			fs.Bool("watch", false, "block + attach to the agent's session (legacy behavior)")
			fmt.Fprintln(os.Stderr, "Usage: darken spawn <name> --type <role> [flags] [task...]")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Stage credentials and skills, then start a scion harness agent.")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Flags:")
			fs.PrintDefaults()
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Examples:")
			fmt.Fprintln(os.Stderr, "  darken spawn researcher-1 --type researcher \"analyze the API surface\"")
			fmt.Fprintln(os.Stderr, "  darken spawn impl-2 --type implementer --backend codex \"refactor auth module\"")
			fmt.Fprintln(os.Stderr, "  darken spawn rev-3 --type reviewer --no-stage \"review PR #42\"")
			return nil
		}
	}

	if len(args) < 1 {
		return errors.New("usage: darken spawn <name> --type <role> [...]")
	}
	name := args[0]

	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	harnessType := fs.String("type", "", "harness role (e.g. researcher)")
	backend := fs.String("backend", "", "backend override (claude|codex|pi|gemini)")
	mode := fs.String("mode", "", "skill-set mode override (defaults to role's default_mode)")
	noStage := fs.Bool("no-stage", false, "skip stage-creds and stage-skills")
	watch := fs.Bool("watch", false, "block + attach to the agent's session (legacy behavior)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *harnessType == "" {
		return errors.New("--type is required")
	}
	posArgs := fs.Args()

	if !*noStage {
		if err := stageHubCreds("all"); err != nil {
			fmt.Fprintln(os.Stderr, "spawn: stage-creds non-fatal:", err)
		}
		if err := withModeOverride(*mode, func() error {
			return stageSkillsForRole(*harnessType)
		}); err != nil {
			return fmt.Errorf("spawn: %w", err)
		}
	}

	// Validate the manifest before attempting to start the agent. A parse
	// error in a known role's manifest is a configuration mistake that must
	// be fixed; silently degrading would let a broken manifest produce an
	// agent with missing context or wrong backend.
	// A missing manifest (ErrNotExist) is non-fatal: the role may not have a
	// local template tree in this workspace.
	if _, err := loadManifestForRole(*harnessType); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("spawn: manifest for role %q: %w", *harnessType, err)
	}
	// TODO(upstream-scion): manifest command_args are intentionally not
	// forwarded to scion start. scion start has no --betas flag; passing raw
	// harness flags through the orchestration CLI crosses an abstraction
	// boundary. Wire m.CommandArgs here once upstream scion exposes a
	// harness-level flag routing mechanism.

	startArgs := []string{"--type", *harnessType}
	if *backend != "" {
		image := fmt.Sprintf("local/darkish-%s:latest", *backend)
		startArgs = append(startArgs, "--harness", *backend, "--image", image)
	}
	if len(posArgs) > 0 {
		startArgs = append(startArgs, posArgs...)
	}
	if *watch {
		startArgs = append(startArgs, "--attach")
		// Legacy mode: StartAgent with --attach blocks until agent exits.
		return defaultScionClient.StartAgent(name, startArgs)
	}

	// Hybrid: surface scion start's own immediate failures (template
	// not found, bad args) directly. Post-dispatch failures (image
	// pull, container init) get caught by the readiness poll below.
	if err := defaultScionClient.StartAgent(name, startArgs); err != nil {
		return err
	}

	// Read timeout override from env; default 15s.
	timeout := 15 * time.Second
	if v := os.Getenv("DARKEN_SPAWN_READY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	// Operator-visible labels per scion lifecycle phase. The "running"
	// phase is omitted intentionally — runSpawn prints the final
	// "ready (X.Xs)" line itself with elapsed time.
	phaseLabels := map[string]string{
		"created":      "queued",
		"provisioning": "provisioning",
		"cloning":      "cloning workspace",
		"starting":     "container starting",
	}

	// Poll for ready (or error / timeout). Per-phase progress goes to
	// stderr via the callback; the final ready line is printed below.
	start := time.Now()
	phase, err := pollUntilReady(name, timeout, 500*time.Millisecond, func(p string) {
		if p == "running" {
			return // runSpawn prints the final "ready (X.Xs)" line
		}
		label, ok := phaseLabels[p]
		if !ok {
			label = p
		}
		fmt.Fprintf(os.Stderr, "[spawning %s] %s\n", name, label)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[spawning %s] FAILED at phase=%s — %v\n", name, phase, err)
		return fmt.Errorf("agent %s did not reach ready: %w", name, err)
	}
	fmt.Fprintf(os.Stderr, "[spawning %s] ready (%.1fs)\n", name, time.Since(start).Seconds())
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
