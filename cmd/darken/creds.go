package main

import "path/filepath"

func runCreds(args []string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = []string{"all"}
	}
	return runShell(filepath.Join(root, "scripts", "stage-creds.sh"), args...)
}
