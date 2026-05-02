// Package main — `darken down` is the canonical teardown command.
//
// Mirror of `darken up`: stop running agents, drop the project grove,
// run uninstall-init to remove scaffolds, then chain to `bones down`
// unless --no-bones is set. The goal is "appear as if darken was never
// there" within reason — global hub state and the scion server itself
// are left untouched unless --purge is set.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func runDown(args []string) error {
	fs := flag.NewFlagSet("down", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	noBones := fs.Bool("no-bones", false, "skip the bones down chain")
	purge := fs.Bool("purge", false, "also stop the scion server and remove user-scope hub templates (host-wide; use with care)")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if !*yes {
		ok, err := confirmDown()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	// Lifecycle resources (broker withdraw, grove clean, agent stop+
	// delete, worktree prune) are walked in reverse via releaseAll.
	// uninstallInitFiles, chainBonesDown, and purgeHostState are
	// darken-specific concerns outside the lifecycle model — they run
	// as explicit best-effort steps after the resource walk.
	releaseAll(lifecycle)

	postLifecycle := []func() error{uninstallInitFiles}
	if !*noBones {
		postLifecycle = append(postLifecycle, chainBonesDown)
	}
	if *purge {
		postLifecycle = append(postLifecycle, purgeHostState)
	}
	for _, step := range postLifecycle {
		if err := step(); err != nil {
			fmt.Fprintf(os.Stderr, "darken down: step failed: %v (continuing best-effort)\n", err)
		}
	}
	return nil
}

func confirmDown() (bool, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("stdin stat: %w", err)
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return false, errors.New("non-interactive context: pass --yes to confirm")
	}
	fmt.Print("darken down will stop project agents, delete the project grove, and remove darken scaffolds. Proceed? [y/N]: ")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

// uninstallInitFiles delegates to runUninstallInit with --yes so the
// scaffolds darken init wrote (CLAUDE.md, .claude/skills, .gitignore
// lines, .scion/init-manifest.json) are removed.
func uninstallInitFiles() error {
	return runUninstallInit([]string{"--yes"})
}

// chainBonesDown runs `bones down --yes` if bones is on PATH. If missing,
// no-op (matches the up-side fallback).
//
// Note: bones starts a hub momentarily during teardown to deregister
// state before destruction. The "bones: starting hub for workspace ..."
// line in the output is bones' own logging and is expected — not a
// contradiction with the teardown intent.
func chainBonesDown() error {
	if _, err := exec.LookPath("bones"); err != nil {
		return nil
	}
	fmt.Println("darken down: chaining `bones down --yes` ...")
	cmd := exec.Command("bones", "down", "--yes")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// purgeHostState stops the scion server and removes user-scope hub
// templates. Host-wide; gated behind --purge so a single project's
// teardown doesn't trample shared state.
func purgeHostState() error {
	fmt.Println("darken down --purge: stopping scion server ...")
	if err := defaultScionClient.StopServer(); err != nil {
		fmt.Fprintf(os.Stderr, "darken down: scion server stop failed: %v\n", err)
	}
	for _, role := range canonicalRoles {
		_ = defaultScionClient.DeleteTemplate(role)
	}
	return nil
}
