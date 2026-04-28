package main

import (
	"fmt"
	"os"
	"os/exec"
)

// runSubstrateScript reads a substrate-relative script via the resolver,
// extracts it to a temp file with exec permissions, runs it with the
// given args, and cleans up the temp file. Stdout and stderr are
// inherited so the user sees script progress in-place.
//
// Path is substrate-relative (e.g. "scripts/stage-creds.sh"), not an
// OS path. The resolver layers flag → env → user → project → embedded.
func runSubstrateScript(substratePath string, args []string) error {
	body, err := substrateResolver().ReadFile(substratePath)
	if err != nil {
		return fmt.Errorf("substrate script %s: %w", substratePath, err)
	}

	tmp, err := os.CreateTemp("", "darken-script-*.sh")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}

	c := exec.Command("bash", append([]string{tmp.Name()}, args...)...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// runSubstrateScriptCaptured is the test-only variant that returns
// combined stdout+stderr as a string. Production callers use
// runSubstrateScript above, which inherits stdout/stderr.
func runSubstrateScriptCaptured(substratePath string, args []string) (string, error) {
	body, err := substrateResolver().ReadFile(substratePath)
	if err != nil {
		return "", fmt.Errorf("substrate script %s: %w", substratePath, err)
	}

	tmp, err := os.CreateTemp("", "darken-script-*.sh")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", err
	}

	out, err := exec.Command("bash", append([]string{tmp.Name()}, args...)...).CombinedOutput()
	return string(out), err
}
