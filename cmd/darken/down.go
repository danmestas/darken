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

	steps := []func() error{
		stopProjectAgents,
		deleteProjectGrove,
		uninstallInitFiles,
	}
	if !*noBones {
		steps = append(steps, chainBonesDown)
	}
	if *purge {
		steps = append(steps, purgeHostState)
	}

	for _, step := range steps {
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

// stopProjectAgents stops any agents in this project's grove. Best-effort:
// if scion list fails the step returns nil so other teardown can proceed.
func stopProjectAgents() error {
	agents, err := scionListAgents()
	if err != nil {
		fmt.Fprintf(os.Stderr, "darken down: skipping agent-stop (scion list failed: %v)\n", err)
		return nil
	}
	if len(agents) == 0 {
		return nil
	}
	fmt.Printf("darken down: stopping %d agent(s) in this grove ...\n", len(agents))
	for _, a := range agents {
		_ = scionCmd([]string{"stop", a.Name, "-y"}).Run()
		_ = scionCmd([]string{"delete", a.Name, "-y"}).Run()
	}
	return nil
}

// deleteProjectGrove invokes `scion grove delete` for the project grove
// if .scion/grove-id is present. Best-effort.
func deleteProjectGrove() error {
	root, err := repoRoot()
	if err != nil {
		return nil
	}
	if _, err := os.Stat(root + "/.scion/grove-id"); err != nil {
		return nil
	}
	fmt.Println("darken down: deleting project grove ...")
	cmd := scionCmd([]string{"grove", "delete", "-y"})
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// uninstallInitFiles delegates to runUninstallInit with --yes so the
// scaffolds darken init wrote (CLAUDE.md, .claude/skills, .gitignore
// lines, .scion/init-manifest.json) are removed.
func uninstallInitFiles() error {
	return runUninstallInit([]string{"--yes"})
}

// chainBonesDown runs `bones down --yes` if bones is on PATH. If missing,
// no-op (matches the up-side fallback).
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
	if err := scionCmd([]string{"server", "stop"}).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "darken down: scion server stop failed: %v\n", err)
	}
	for _, role := range canonicalRoles {
		_ = scionCmd([]string{"--global", "templates", "delete", role, "-y"}).Run()
	}
	return nil
}
