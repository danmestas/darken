// Package main — `darken upgrade-init` is the post-`brew upgrade
// darken` convenience: refresh the project's scaffolds against the
// new binary's embedded substrate, then verify with doctor --init.
//
// Equivalent to: `darken init --refresh && darken doctor --init`.
package main

import (
	"errors"
)

func runUpgradeInit(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken upgrade-init")
	}
	if err := runInit([]string{"--refresh"}); err != nil {
		return err
	}
	return runDoctor([]string{"--init"})
}
