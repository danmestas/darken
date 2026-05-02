// Package main — `darken setup` is the deprecated alias for `darken up`.
// The runSetup function lives in up.go now; this file holds the
// helpers shared between up and bootstrap.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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
//
// Precondition: scion's local template store contains every canonical
// role. ensureAllSkillsStaged satisfies this by calling
// ImportAllTemplates during bootstrap. Without that import, push fails
// with "template not found locally" because scion looks up templates by
// name in its own store and refuses to push what it has never seen.
func uploadAllTemplatesToHub() error {
	for _, role := range canonicalRoles {
		fmt.Printf("uploading template %s to Hub (user scope) ...\n", role)
		if err := defaultScionClient.PushTemplate(role); err != nil {
			return fmt.Errorf("upload template %s: %w", role, err)
		}
	}
	return nil
}
