// Package main — `darken setup` is the fresh-repo onboarding shortcut.
// Composes runInit + runBootstrap. Single flag (--force) passes
// through to runInit for CLAUDE.md overwrite.
//
// For the existing-repo / post-`brew upgrade darken` path, see
// runUpgradeInit instead.
package main

func runSetup(args []string) error {
	if err := runInit(args); err != nil {
		return err
	}
	return runBootstrap(nil)
}
