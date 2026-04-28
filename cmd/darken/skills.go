package main

import (
	"errors"
)

func runSkills(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: darken skills <harness> [--diff|--add SKILL|--remove SKILL]")
	}
	templatesDir, cleanup, err := resolveTemplatesDir()
	if err != nil {
		return err
	}
	defer cleanup()
	return withTemplatesDirEnv(templatesDir, func() error {
		return runSubstrateScript("scripts/stage-skills.sh", args)
	})
}
