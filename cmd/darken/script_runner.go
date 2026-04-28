package main

import (
	"fmt"
	"os"
	"os/exec"
)

// extractSubstrateScript reads a substrate-relative script via the
// resolver and writes it to a temp file with exec permissions.
// Returns the temp file path and a cleanup func that removes it.
// Caller is responsible for invoking cleanup (typically via defer).
//
// Path is substrate-relative (e.g. "scripts/stage-creds.sh"), not an
// OS path. The resolver layers flag → env → user → project → embedded.
func extractSubstrateScript(substratePath string) (path string, cleanup func(), err error) {
	body, err := substrateResolver().ReadFile(substratePath)
	if err != nil {
		return "", nil, fmt.Errorf("substrate script %s: %w", substratePath, err)
	}

	tmp, err := os.CreateTemp("", "darken-script-*.sh")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.Remove(tmp.Name()) }

	if _, werr := tmp.Write(body); werr != nil {
		tmp.Close()
		cleanup()
		return "", nil, werr
	}
	if cerr := tmp.Close(); cerr != nil {
		cleanup()
		return "", nil, cerr
	}
	if cerr := os.Chmod(tmp.Name(), 0o755); cerr != nil {
		cleanup()
		return "", nil, cerr
	}
	return tmp.Name(), cleanup, nil
}

// runSubstrateScript extracts a substrate-relative script and runs it
// with the given args. Stdout and stderr are inherited so the user
// sees script progress in-place.
//
// Uses the bash on PATH (not /bin/bash) so test stubs that prepend a
// fake bash to PATH work. Future hardening to /bin/bash would require
// updating spawn_test.go's bash stub strategy.
//
// Sets DARKEN_REPO_ROOT in the script's env so scripts can locate the
// operator's working repo even when extracted to a tmpdir. Scripts
// like stage-skills.sh use this to find harness manifests.
func runSubstrateScript(substratePath string, args []string) error {
	tmpPath, cleanup, err := extractSubstrateScript(substratePath)
	if err != nil {
		return err
	}
	defer cleanup()

	c := exec.Command("bash", append([]string{tmpPath}, args...)...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = scriptEnv()
	return c.Run()
}

// runSubstrateScriptCaptured is the test-only variant that returns
// combined stdout+stderr as a string. Production callers use
// runSubstrateScript above, which inherits stdout/stderr.
func runSubstrateScriptCaptured(substratePath string, args []string) (string, error) {
	tmpPath, cleanup, err := extractSubstrateScript(substratePath)
	if err != nil {
		return "", err
	}
	defer cleanup()

	c := exec.Command("bash", append([]string{tmpPath}, args...)...)
	c.Env = scriptEnv()
	out, err := c.CombinedOutput()
	return string(out), err
}

// scriptEnv returns the parent environment plus DARKEN_REPO_ROOT
// pointing at the resolved operator working repo (best-effort; if
// repoRoot fails, leaves DARKEN_REPO_ROOT unset so scripts fall back
// to BASH_SOURCE-relative resolution).
func scriptEnv() []string {
	env := os.Environ()
	if root, err := repoRoot(); err == nil {
		env = append(env, "DARKEN_REPO_ROOT="+root)
	}
	return env
}
