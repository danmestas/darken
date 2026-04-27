package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// repoRoot returns the repo root, preferring DARKEN_REPO_ROOT for tests.
func repoRoot() (string, error) {
	if v := os.Getenv("DARKEN_REPO_ROOT"); v != "" {
		return v, nil
	}
	return findRepoRoot()
}

// findRepoRoot shells out to git to locate the repository root.
func findRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repo: %w", err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// imageExists reports whether a docker image is present locally.
func imageExists(tag string) bool {
	out, err := exec.Command("docker", "images", "-q", tag).Output()
	return err == nil && len(out) > 0
}
