package main

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/danmestas/darken/internal/substrate"
)

// version is overridden at build time via -ldflags="-X main.version=v0.1.0"
// when goreleaser produces release archives + the homebrew formula. For
// `go install` paths, version stays "dev" because the ldflag isn't applied —
// resolvedVersion() falls through to runtime/debug.ReadBuildInfo() and
// reports the module version that go modules resolved.
var version = "dev"

func runVersion(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken version")
	}
	hash := substrate.EmbeddedHash()
	if len(hash) > 12 {
		hash = hash[:12]
	}
	fmt.Printf("darken %s (substrate sha256:%s)\n", resolvedVersion(), hash)
	return nil
}

// resolvedVersion returns the build-time-injected version when present;
// otherwise falls back to the module version recorded by `go install`
// (debug.ReadBuildInfo). Returns "dev" only for source-tree builds where
// neither path produces a real version.
func resolvedVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
