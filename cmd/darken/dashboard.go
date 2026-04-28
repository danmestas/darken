// Package main — `darken dashboard` opens scion's web UI in the
// operator's default browser. Thin wrapper: parse scion server
// status, compute URL, exec `open` (macOS) or `xdg-open` (Linux).
package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// dashboardURL is scion's default web port when --workstation or
// --enable-web is set. Today this is hardcoded; future Phase 9 may
// parse it out of scion server status output if scion exposes a
// configurable port via a known field.
const dashboardURL = "http://localhost:8080"

func runDashboard(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken dashboard")
	}

	// Confirm scion server is running before opening the URL — saves
	// the operator from staring at a connection-refused page.
	if err := exec.Command("scion", "server", "status").Run(); err != nil {
		return fmt.Errorf("scion server not running (run `scion server start --workstation`): %w", err)
	}

	// Open the URL via the platform's default opener.
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	c := exec.Command(opener, dashboardURL)
	return c.Run()
}
