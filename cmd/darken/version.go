package main

import (
	"errors"
	"fmt"

	"github.com/danmestas/darkish-factory/internal/substrate"
)

// version is overridden at build time via -ldflags="-X main.version=v0.1.0".
// Defaults to "dev" for source-tree builds.
var version = "dev"

func runVersion(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken version")
	}
	hash := substrate.EmbeddedHash()
	if len(hash) > 12 {
		hash = hash[:12]
	}
	fmt.Printf("darken %s (substrate sha256:%s)\n", version, hash)
	return nil
}
