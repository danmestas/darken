package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func runBootstrap(args []string) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"docker daemon reachable", checkDocker},
		{"scion CLI present", checkScion},
		{"scion server running", ensureScionServer},
		{"darken images built", ensureImages},
		{"hub secrets pushed", ensureHubSecrets},
		{"per-harness skills staged", ensureAllSkillsStaged},
		{"final doctor", finalDoctor},
	}
	for i, s := range steps {
		fmt.Printf("[%d/%d] %s ...\n", i+1, len(steps), s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("step %q failed: %w", s.name, err)
		}
	}
	fmt.Println("bootstrap: OK")
	return nil
}

// ensureScionServer starts the scion server if not already running.
func ensureScionServer() error {
	if err := exec.Command("scion", "server", "status").Run(); err == nil {
		return nil
	}
	return exec.Command("scion", "server", "start").Run()
}

// ensureImages builds any missing darken images via `make -C images <backend>`.
func ensureImages() error {
	for _, b := range []string{"claude", "codex", "pi", "gemini"} {
		if imageExists("local/darkish-" + b + ":latest") {
			continue
		}
		c := exec.Command("make", "-C", "images", b)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("make %s: %w", b, err)
		}
	}
	return nil
}

func ensureHubSecrets() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	return runShell(filepath.Join(root, "scripts", "stage-creds.sh"), "all")
}

// ensureAllSkillsStaged runs stage-skills.sh per harness directory.
// Soft-fails per-harness so one missing skill canon doesn't abort
// the whole bootstrap.
func ensureAllSkillsStaged() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	dirs, err := os.ReadDir(filepath.Join(root, ".scion", "templates"))
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == "base" {
			continue
		}
		if err := runShell(filepath.Join(root, "scripts", "stage-skills.sh"), d.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap: stage-skills %s failed: %v\n", d.Name(), err)
		}
	}
	return nil
}

func finalDoctor() error {
	report, err := doctorBroad()
	fmt.Println(report)
	if err != nil {
		return errors.New("post-bootstrap doctor reported failures")
	}
	return nil
}
