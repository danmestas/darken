// Package main — `darken status` produces a single-line summary suitable
// for Claude Code's statusLine.command. Must be fast — called every
// prompt. Avoid external commands (no scion list, no docker calls).
package main

import (
	"errors"
	"fmt"

	"github.com/danmestas/darken/internal/substrate"
)

func runStatus(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken status")
	}
	hash := substrate.EmbeddedHash()
	if len(hash) > 12 {
		hash = hash[:12]
	}
	fmt.Printf("[darken: orchestrator-mode | substrate %s]\n", hash)
	return nil
}
