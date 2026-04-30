// Package main — `darken setup` is the fresh-repo onboarding shortcut.
// Composes runInit + runBootstrap + template upload. Single flag (--force)
// passes through to runInit for CLAUDE.md overwrite.
//
// For the existing-repo / post-`brew upgrade darken` path, see
// runUpgradeInit instead.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func runSetup(args []string) error {
	if err := runInit(args); err != nil {
		return err
	}
	if err := ensureGroveInit(); err != nil {
		return err
	}
	if err := runBootstrap(nil); err != nil {
		return err
	}
	return uploadAllTemplatesToHub()
}

// ensureGroveInit registers the current directory as a project-scoped scion
// grove by running scion grove init. It is idempotent: if .scion/grove-id
// already exists the call is skipped entirely, avoiding a redundant round-trip
// to the hub and preventing grove_id from being replaced.
func ensureGroveInit() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("ensureGroveInit: getwd: %w", err)
	}
	groveIDPath := filepath.Join(cwd, ".scion", "grove-id")
	if info, err := os.Stat(groveIDPath); err == nil && !info.IsDir() {
		// Grove already initialised for this project; skip.
		return nil
	}
	fmt.Println("initialising project grove ...")
	return defaultScionClient.GroveInit()
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
