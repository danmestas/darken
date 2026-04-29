// Package main — `darken setup` is the fresh-repo onboarding shortcut.
// Composes runInit + runBootstrap + template upload. Single flag (--force)
// passes through to runInit for CLAUDE.md overwrite.
//
// For the existing-repo / post-`brew upgrade darken` path, see
// runUpgradeInit instead.
package main

import (
	"fmt"
)

func runSetup(args []string) error {
	if err := runInit(args); err != nil {
		return err
	}
	if err := runBootstrap(nil); err != nil {
		return err
	}
	return uploadAllTemplatesToHub()
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
