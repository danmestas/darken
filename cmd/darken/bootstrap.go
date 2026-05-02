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
		{"grove registered with broker", ensureBrokerProvide},
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
	if _, err := defaultScionClient.ServerStatus(); err == nil {
		return nil
	}
	// Server not running: start it. Server start is a bootstrap-only
	// imperative not exposed on ScionClient (callers need check-only or
	// ensure-running; expose the latter here via a direct exec).
	cmd := scionCmdWithEnv([]string{"server", "start"})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureBrokerProvide registers the current grove with the local broker so
// agents can be dispatched here. Idempotent — scion broker provide is a
// no-op when the grove is already registered.
func ensureBrokerProvide() error {
	return defaultScionClient.BrokerProvide()
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
	templatesDir, modesDir, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		return err
	}
	defer cleanup()

	if err := withSubstrateDirsEnv(templatesDir, modesDir, func() error {
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
	}); err != nil {
		return err
	}

	// Register every canonical role with scion's local template store so
	// uploadAllTemplatesToHub can push them. The deferred cleanup above
	// removes the source dir; scion's import copies bodies into its own
	// store, so post-cleanup push works.
	if err := defaultScionClient.ImportAllTemplates(templatesDir); err != nil {
		return fmt.Errorf("import templates: %w", err)
	}
	return nil
}

// withSubstrateDirsEnv runs fn with DARKEN_TEMPLATES_DIR and
// DARKEN_MODES_DIR set, restoring previous values (or unsetting)
// afterward. modesDir may be empty when only the templates dir is
// known; in that case DARKEN_MODES_DIR is left unchanged.
func withSubstrateDirsEnv(templatesDir, modesDir string, fn func() error) error {
	restoreT := setEnvWithRestore("DARKEN_TEMPLATES_DIR", templatesDir)
	defer restoreT()
	if modesDir != "" {
		restoreM := setEnvWithRestore("DARKEN_MODES_DIR", modesDir)
		defer restoreM()
	}
	return fn()
}

// withModeOverride runs fn with DARKEN_MODE_OVERRIDE set to mode if mode is
// non-empty; the previous value (or unset state) is restored on return. When
// mode is empty the env var is left untouched, so callers that don't want to
// override the role's default_mode pay no cost. Bootstrap doesn't take an
// override; only spawn does, so this wrapper is intentionally separate from
// withSubstrateDirsEnv.
func withModeOverride(mode string, fn func() error) error {
	if mode == "" {
		return fn()
	}
	restore := setEnvWithRestore("DARKEN_MODE_OVERRIDE", mode)
	defer restore()
	return fn()
}

func setEnvWithRestore(key, val string) func() {
	prev, hadPrev := os.LookupEnv(key)
	os.Setenv(key, val)
	return func() {
		if hadPrev {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	}
}

// resolveTemplatesDir is the legacy single-dir entry point preserved for
// callers that don't need the modes dir. Internally delegates to
// resolveSubstrateDirs and discards the modes path.
func resolveTemplatesDir() (string, func(), error) {
	t, _, cleanup, err := resolveSubstrateDirs()
	return t, cleanup, err
}

// withTemplatesDirEnv is the legacy env-only-templates wrapper. Prefer
// withSubstrateDirsEnv when modes are also needed.
func withTemplatesDirEnv(dir string, fn func() error) error {
	return withSubstrateDirsEnv(dir, "", fn)
}

// resolveSubstrateDirs returns paths to the templates and modes dirs.
// Prefers the operator's project layout (.scion/templates + .scion/modes)
// when it has at least one role subdir; otherwise extracts the embedded
// substrate to a tmpdir laid out as <tmp>/templates/ and <tmp>/modes/.
// The returned cleanup func is a no-op for the project case and an
// os.RemoveAll for the embedded case.
//
// Why the role-subdir guard: a workspace bootstrapped by `darken init`
// (or hand-created) may have an empty .scion/templates/ before any
// role canon is staged in. Returning that empty dir downstream causes
// `scion templates import --all` to fail with "no importable agent
// definitions". Falling back to the embedded substrate keeps `darken up`
// working for fresh workspaces.
func resolveSubstrateDirs() (string, string, func(), error) {
	noop := func() {}

	if root, err := repoRoot(); err == nil {
		projectTemplates := filepath.Join(root, ".scion", "templates")
		projectModes := filepath.Join(root, ".scion", "modes")
		if info, statErr := os.Stat(projectTemplates); statErr == nil && info.IsDir() && hasRoleSubdirs(projectTemplates) {
			// Modes dir may not exist yet on a project mid-migration;
			// pass through whatever's there and let the script error if
			// it actually needs it.
			return projectTemplates, projectModes, noop, nil
		}
	}

	return extractEmbeddedSubstrate()
}

// hasRoleSubdirs reports whether dir contains at least one subdirectory
// other than "base". `base` is the shared common-skill bundle, not a
// canonical role, so a templates dir holding only `base/` is still
// effectively empty for import purposes.
func hasRoleSubdirs(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "base" {
			return true
		}
	}
	return false
}

// extractEmbeddedSubstrate copies both data/.scion/templates and
// data/.scion/modes to a single tmpdir, side-by-side, and returns
// (templatesDir, modesDir, cleanup). Layout:
//
//	<tmp>/templates/<role>/scion-agent.yaml
//	<tmp>/modes/<name>.yaml
//
// The cleanup func removes the parent tmpdir.
func extractEmbeddedSubstrate() (string, string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "darken-substrate-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	templatesDir := filepath.Join(tmpDir, "templates")
	modesDir := filepath.Join(tmpDir, "modes")

	if err := extractEmbeddedTree("data/.scion/templates", templatesDir); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("extract embedded templates: %w", err)
	}
	if err := extractEmbeddedTree("data/.scion/modes", modesDir); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("extract embedded modes: %w", err)
	}
	return templatesDir, modesDir, cleanup, nil
}

// extractEmbeddedTree walks an embed root and writes each file to
// dstRoot, expanding manifest placeholders as it goes.
func extractEmbeddedTree(srcRoot, dstRoot string) error {
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(substrate.EmbeddedFS(), srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == srcRoot {
			return nil
		}
		rel := strings.TrimPrefix(path, srcRoot+"/")
		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		body, err := fs.ReadFile(substrate.EmbeddedFS(), path)
		if err != nil {
			return err
		}
		content := string(body)
		if strings.HasSuffix(path, "scion-agent.yaml") {
			content = expandManifest(content)
		}
		return os.WriteFile(dst, []byte(content), 0o644)
	})
}

func finalDoctor() error {
	report, err := doctorBroad()
	fmt.Println(report)
	if err != nil {
		return errors.New("post-bootstrap doctor reported failures")
	}
	return nil
}
