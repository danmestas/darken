package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danmestas/darken/internal/substrate"
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
	return runSubstrateScript("scripts/stage-creds.sh", []string{"all"})
}

// ensureAllSkillsStaged runs stage-skills.sh per harness directory.
// Soft-fails per-harness so one missing skill canon doesn't abort
// the whole bootstrap.
func ensureAllSkillsStaged() error {
	templatesDir, cleanup, err := resolveTemplatesDir()
	if err != nil {
		return err
	}
	defer cleanup()

	return withTemplatesDirEnv(templatesDir, func() error {
		dirs, err := os.ReadDir(templatesDir)
		if err != nil {
			return fmt.Errorf("read templates dir %s: %w", templatesDir, err)
		}
		for _, d := range dirs {
			if !d.IsDir() || d.Name() == "base" {
				continue
			}
			if err := runSubstrateScript("scripts/stage-skills.sh", []string{d.Name()}); err != nil {
				fmt.Fprintf(os.Stderr, "bootstrap: stage-skills %s failed: %v\n", d.Name(), err)
			}
		}
		return nil
	})
}

// withTemplatesDirEnv runs fn with DARKEN_TEMPLATES_DIR set to dir,
// restoring the previous value (or unsetting) afterward. Used by
// callers that need stage-skills.sh to read manifests from a specific
// path (typically the resolved project-local-or-embedded location).
func withTemplatesDirEnv(dir string, fn func() error) error {
	prev, hadPrev := os.LookupEnv("DARKEN_TEMPLATES_DIR")
	os.Setenv("DARKEN_TEMPLATES_DIR", dir)
	defer func() {
		if hadPrev {
			os.Setenv("DARKEN_TEMPLATES_DIR", prev)
		} else {
			os.Unsetenv("DARKEN_TEMPLATES_DIR")
		}
	}()
	return fn()
}

// resolveTemplatesDir returns a path containing per-harness manifest
// dirs (each with scion-agent.yaml). Prefers the operator's project
// templates if present; otherwise extracts the embedded substrate
// templates to a tmpdir. The returned cleanup func is a no-op for the
// project case and an os.RemoveAll for the embedded case.
func resolveTemplatesDir() (string, func(), error) {
	noop := func() {}

	if root, err := repoRoot(); err == nil {
		projectDir := filepath.Join(root, ".scion", "templates")
		if info, statErr := os.Stat(projectDir); statErr == nil && info.IsDir() {
			return projectDir, noop, nil
		}
	}

	return extractEmbeddedTemplates()
}

// extractEmbeddedTemplates copies the embedded data/.scion/templates
// tree to a tmpdir and returns its path. The cleanup func removes the
// tmpdir; callers should defer it.
func extractEmbeddedTemplates() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "darken-templates-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	const root = "data/.scion/templates"
	walkErr := fs.WalkDir(substrate.EmbeddedFS(), root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel := strings.TrimPrefix(path, root+"/")
		dst := filepath.Join(tmpDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		body, err := fs.ReadFile(substrate.EmbeddedFS(), path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, body, 0o644)
	})
	if walkErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("extract embedded templates: %w", walkErr)
	}
	return tmpDir, cleanup, nil
}

func finalDoctor() error {
	report, err := doctorBroad()
	fmt.Println(report)
	if err != nil {
		return errors.New("post-bootstrap doctor reported failures")
	}
	return nil
}
