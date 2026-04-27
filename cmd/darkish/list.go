package main

import (
	"os"
	"os/exec"
)

// runList is a thin passthrough to `scion list` for now.
//
// FUTURE: Spec §12.7 envisions darkish-specific column reformat (e.g.
// template / grove / broker / phase) on top of `scion list --format
// json`. Tracked in Open Questions; this initial implementation
// streams scion output unchanged so operators get parity day-1.
func runList(args []string) error {
	c := exec.Command("scion", append([]string{"list"}, args...)...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
