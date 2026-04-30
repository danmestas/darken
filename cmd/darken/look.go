package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
)

// ansiEscape matches ANSI/VT100 escape sequences of the form ESC[...m
// as well as cursor-control sequences (ESC[<n>J, ESC[H, etc.).
// The pattern covers the common CSI (Control Sequence Introducer)
// family used by terminal TUI toolkits.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// stripANSI removes ANSI escape sequences from b and returns the
// cleaned bytes. The original slice is not modified.
func stripANSI(b []byte) []byte {
	return ansiEscape.ReplaceAll(b, nil)
}

// runLook is the entry point registered in main.go. It delegates to
// runLookInto with os.Stdout so tests can capture output.
func runLook(args []string) error {
	return runLookInto(args, os.Stdout)
}

// runLookInto runs `scion --no-hub look <agent>`, strips ANSI escape
// sequences from the output, and writes clean text to w.
func runLookInto(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: darken look <agent>")
	}
	agentName := args[0]
	cmdArgs := append([]string{"--no-hub", "look", agentName}, args[1:]...)
	cmd := exec.Command("scion", cmdArgs...)
	raw, err := cmd.Output()
	if err != nil {
		// Surface stderr if available.
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return fmt.Errorf("scion look %s: %w\n%s", agentName, err, ee.Stderr)
		}
		return fmt.Errorf("scion look %s: %w", agentName, err)
	}
	clean := stripANSI(raw)
	_, werr := w.Write(clean)
	return werr
}

// init registers the look subcommand so it appears in `darken --help`.
// This is called from the package init chain; the subcommands slice is
// defined in main.go and is safe to append during init.
func init() {
	subcommands = append(subcommands, subcommand{
		name: "look",
		desc: "inspect an agent terminal, ANSI-stripped (wraps scion look)",
		run:  runLook,
	})
}
