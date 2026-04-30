// Package main — `darken setup` is the fresh-repo onboarding shortcut.
// Composes runInit + runBootstrap + template upload. Single flag (--force)
// passes through to runInit for CLAUDE.md overwrite.
//
// For the existing-repo / post-`brew upgrade darken` path, see
// runUpgradeInit instead.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func runSetup(args []string) error {
	// Resolve the setup target once so that ensureGroveInit operates on the
	// same directory that runInit writes to, regardless of cwd.
	target, err := resolveInitTarget(args)
	if err != nil {
		return err
	}
	if err := runInit(args); err != nil {
		return err
	}
	if err := ensureGroveInit(target); err != nil {
		return err
	}
	if err := runBootstrap(nil); err != nil {
		return err
	}
	return uploadAllTemplatesToHub()
}

// resolveInitTarget parses the same flag set as runInit and returns the
// absolute path of the target directory. This is intentionally kept in sync
// with runInit's flag definitions so the resolved target is identical.
func resolveInitTarget(args []string) (string, error) {
	fs := flag.NewFlagSet("setup-target", flag.ContinueOnError)
	fs.Bool("dry-run", false, "")
	fs.Bool("force", false, "")
	fs.Bool("refresh", false, "")
	fs.SetOutput(io.Discard) // suppress duplicate usage output
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	target := "."
	if pos := fs.Args(); len(pos) > 0 {
		target = pos[0]
	}
	return filepath.Abs(target)
}

// ensureGroveInit registers targetDir as a project-scoped scion grove by
// running scion grove init. It is idempotent: if .scion/grove-id already
// exists the call is skipped entirely, avoiding a redundant round-trip to the
// hub and preventing grove_id from being replaced.
func ensureGroveInit(targetDir string) error {
	groveIDPath := filepath.Join(targetDir, ".scion", "grove-id")
	if info, err := os.Stat(groveIDPath); err == nil && !info.IsDir() {
		// Grove already initialised for this project; skip.
		return nil
	}
	fmt.Println("initialising project grove ...")
	return defaultScionClient.GroveInit(targetDir)
}

// uploadAllTemplatesToHub pushes all 14 canonical templates to the Hub at
// user (global) scope via ScionClient.PushTemplate.
// Runs after bootstrap so the scion server is guaranteed to be running.
func uploadAllTemplatesToHub() error {
	for _, role := range canonicalRoles {
		fmt.Printf("uploading template %s to Hub (user scope) ...\n", role)
		if err := defaultScionClient.PushTemplate(role); err != nil {
			return fmt.Errorf("upload template %s: %w", role, err)
		}
	}
	return nil
}
