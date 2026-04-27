package main

import (
	"errors"
	"path/filepath"
)

func runSkills(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: darken skills <harness> [--diff|--add SKILL|--remove SKILL]")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	return runShell(filepath.Join(root, "scripts", "stage-skills.sh"), args...)
}
