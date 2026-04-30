// Package main — `darken up` is the canonical bring-up command.
//
// Composes runInit + ensureGroveInit + runBootstrap + template upload,
// then chains to `bones up` unless --no-bones is set. Replaces the
// older `darken setup` which is preserved as a deprecated alias.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runUp(args []string) error {
	// Strip --no-bones from the args without interfering with the
	// init-side flag parsing. Anything else passes through verbatim
	// to runInit / resolveInitTarget so --force, --dry-run, --refresh,
	// and the optional positional target keep working.
	noBones := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--no-bones" || a == "-no-bones" {
			noBones = true
			continue
		}
		rest = append(rest, a)
	}

	target, err := resolveInitTarget(rest)
	if err != nil {
		return err
	}
	if err := runInit(rest); err != nil {
		return err
	}
	if err := ensureGroveInit(target); err != nil {
		return err
	}
	if err := runBootstrap(nil); err != nil {
		return err
	}
	if err := uploadAllTemplatesToHub(); err != nil {
		return err
	}
	if noBones {
		fmt.Println("darken up: skipping bones up (--no-bones)")
		return nil
	}
	return chainBonesUp()
}

// runSetup is the deprecated alias for `darken up`. Prints a
// one-line deprecation notice and forwards to runUp. Removal target:
// after one release cycle.
func runSetup(args []string) error {
	fmt.Fprintln(os.Stderr, "darken setup: deprecated alias for `darken up` (will be removed in a future release)")
	return runUp(args)
}

// chainBonesUp runs `bones up` if bones is on PATH. If missing,
// prompts the operator to install (interactive TTY only) and either
// continues without bones or aborts. In a non-interactive context
// (no TTY) it warns and continues — matches the operator-stated
// fallback for option (c → b).
func chainBonesUp() error {
	if _, err := exec.LookPath("bones"); err != nil {
		return handleBonesMissing()
	}
	fmt.Println("darken up: chaining `bones up` ...")
	cmd := exec.Command("bones", "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bones up failed: %w (re-run with --no-bones to skip the chain)", err)
	}
	return nil
}

// handleBonesMissing implements the operator's option (c→b) policy:
// in an interactive shell, prompt the operator to install bones; if
// they decline (or stdin is not a terminal), warn and continue.
func handleBonesMissing() error {
	fi, err := os.Stdin.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		fmt.Fprintln(os.Stderr, "darken up: bones not found on PATH; continuing without bones (use --no-bones to silence this warning)")
		return nil
	}
	fmt.Print("bones is not installed. Install via `brew install bones` and re-run, or continue without bones now? [I]nstall now / [c]ontinue without / [a]bort: ")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "i", "install":
		return errors.New("install bones (e.g. `brew install bones`) and re-run `darken up`")
	case "c", "continue":
		fmt.Fprintln(os.Stderr, "darken up: continuing without bones")
		return nil
	default:
		return errors.New("aborted")
	}
}
