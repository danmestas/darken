package main

import (
	"fmt"
	"io"
	"os"
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

// runLookInto fetches the raw terminal output for the named agent via
// ScionClient.LookAgent, strips ANSI escape sequences, and writes clean
// text to w. Routing through ScionClient ensures env propagation is
// consistent with all other scion operations and avoids hardcoding flags
// such as --no-hub.
func runLookInto(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: darken look <agent>")
	}
	agentName := args[0]
	raw, err := defaultScionClient.LookAgent(agentName, args[1:])
	if err != nil {
		return err
	}
	clean := stripANSI(raw)
	_, werr := w.Write(clean)
	return werr
}
