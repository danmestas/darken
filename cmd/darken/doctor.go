package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danmestas/darken/internal/substrate"
)

type check struct {
	name string
	run  func() error
}

func runDoctor(args []string) error {
	// New: --init triggers per-init scaffold verification (Phase 6).
	for _, a := range args {
		if a == "--init" {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			report, err := runInitDoctor(root)
			fmt.Print(report)
			return err
		}
	}

	// Existing dispatch follows below — no change.
	if len(args) >= 1 {
		report, err := doctorHarness(args[0])
		fmt.Println(report)
		return err
	}
	report, err := doctorBroad()
	fmt.Println(report)
	return err
}

func doctorBroad() (string, error) {
	checks := []check{
		{"docker daemon reachable", checkDocker},
		{"scion CLI present", checkScion},
		{"scion server status", checkScionServer},
		{"hub secrets present", checkHubSecrets},
		{"darken images built", checkImages},
	}

	var sb strings.Builder
	var failed []string
	for _, c := range checks {
		if err := c.run(); err != nil {
			fmt.Fprintf(&sb, "FAIL  %s — %v\n", c.name, err)
			fmt.Fprintf(&sb, "      remediation: %s\n", remediationFor(c.name, err))
			failed = append(failed, c.name)
		} else {
			fmt.Fprintf(&sb, "OK    %s\n", c.name)
		}
	}
	if len(failed) > 0 {
		return sb.String(), fmt.Errorf("%d checks failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return sb.String(), nil
}

func checkDocker() error {
	out, err := exec.Command("docker", "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker info: %s", string(out))
	}
	return nil
}

func checkScion() error {
	out, err := exec.Command("scion", "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("scion not on PATH: %s", string(out))
	}
	return nil
}

func checkScionServer() error {
	out, err := exec.Command("scion", "server", "status").CombinedOutput()
	if err != nil {
		return fmt.Errorf("server not running: %s", string(out))
	}
	return nil
}

func checkHubSecrets() error {
	out, err := exec.Command("scion", "hub", "secret", "list").CombinedOutput()
	if err != nil {
		return fmt.Errorf("hub secret list: %s", string(out))
	}
	for _, want := range []string{"claude_auth", "codex_auth"} {
		if !strings.Contains(string(out), want) {
			return fmt.Errorf("missing hub secret: %s", want)
		}
	}
	return nil
}

func checkImages() error {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		return err
	}
	for _, want := range []string{
		"local/darkish-claude:latest",
		"local/darkish-codex:latest",
		"local/darkish-pi:latest",
		"local/darkish-gemini:latest",
	} {
		if !strings.Contains(string(out), want) {
			return fmt.Errorf("missing image: %s", want)
		}
	}
	return nil
}

// remediationFor returns the §9 failure-mode remediation hint for a
// failed check. Dispatches on the structured check name, not the
// error message, so a future error-string change can't silently break
// the mapping.
func remediationFor(check string, err error) string {
	switch check {
	case "docker daemon reachable":
		return "start Docker Desktop / podman / colima"
	case "scion CLI present":
		return "make install in ~/projects/scion"
	case "scion server status":
		return "scion server start"
	case "hub secrets present", "secret":
		return "scripts/stage-creds.sh"
	case "darken images built", "image":
		return "make -C images"
	case "staging", "staging-mismatch":
		return "darken skills <harness>"
	}
	// Fallback for callers passing free-form check names.
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "is a directory") || strings.Contains(msg, "directory symlink"):
			return "Switch to copy-staging via `darken skills <harness>` (never use directory symlinks)"
		case strings.Contains(msg, "caveman tier mismatch"):
			return "Update <harness>/system-prompt.md Communication section; flag to darwin"
		}
	}
	return "see spec §9 failure modes"
}

func doctorHarness(name string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}

	manifestRel := ".scion/templates/" + name + "/scion-agent.yaml"
	resolver := substrateResolver()
	_, manifestLayer, _ := resolver.Lookup(manifestRel)
	body, err := resolver.ReadFile(manifestRel)
	if err != nil {
		if substrate.IsMiss(err) {
			return "", fmt.Errorf("no template defined for harness %s (override missing; embed comes in Phase 2)", name)
		}
		return "", fmt.Errorf("manifest read: %w", err)
	}
	backend := scanField(string(body), "default_harness_config:")
	skills := scanList(string(body), "skills:")

	var sb strings.Builder
	var failed []string

	fmt.Fprintf(&sb, "OK    manifest %s served from %s layer\n", name, manifestLayer)

	imgTag := fmt.Sprintf("local/darkish-%s:latest", backend)
	if !imageExists(imgTag) {
		fmt.Fprintf(&sb, "FAIL  image %s missing — remediation: %s\n",
			imgTag, remediationFor("image", fmt.Errorf("missing image: %s", imgTag)))
		failed = append(failed, "image")
	} else {
		fmt.Fprintf(&sb, "OK    image %s present\n", imgTag)
	}

	wantSecret := map[string]string{
		"claude": "claude_auth", "codex": "codex_auth",
		"pi": "OPENROUTER_API_KEY", "gemini": "gemini_auth",
	}[backend]
	if wantSecret == "" {
		fmt.Fprintf(&sb, "FAIL  unknown backend %q in manifest\n", backend)
		failed = append(failed, "backend")
	} else {
		out, _ := exec.Command("scion", "hub", "secret", "list").CombinedOutput()
		if !strings.Contains(string(out), wantSecret) {
			fmt.Fprintf(&sb, "FAIL  hub secret %s missing — remediation: %s\n",
				wantSecret, remediationFor("secret", fmt.Errorf("missing hub secret: %s", wantSecret)))
			failed = append(failed, "secret")
		} else {
			fmt.Fprintf(&sb, "OK    hub secret %s present\n", wantSecret)
		}
	}

	stageDir := filepath.Join(root, ".scion", "skills-staging", name)
	if _, err := os.Stat(stageDir); err != nil {
		fmt.Fprintf(&sb, "FAIL  skills-staging dir missing at %s — remediation: %s\n",
			stageDir, remediationFor("staging", fmt.Errorf("skills-staging missing: %s", stageDir)))
		failed = append(failed, "staging")
	} else {
		stagingFailed := false
		for _, ref := range skills {
			n := ref[strings.LastIndex(ref, "/")+1:]
			if _, err := os.Stat(filepath.Join(stageDir, n)); err != nil {
				fmt.Fprintf(&sb, "FAIL  manifest declares %q but skills-staging is missing it\n", n)
				failed = append(failed, "staging-mismatch")
				stagingFailed = true
			}
		}
		if !stagingFailed {
			fmt.Fprintf(&sb, "OK    skills-staging matches manifest\n")
		}
	}

	if len(failed) > 0 {
		return sb.String(), fmt.Errorf("%d harness checks failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return sb.String(), nil
}

func postMortemFor(logPath string) string {
	body, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Sprintf("post-mortem: cannot read %s: %v", logPath, err)
	}
	var sb strings.Builder
	patterns := []struct{ needle, reason, fix string }{
		{"auth resolution failed:", "missing hub secret", "Run `scripts/stage-creds.sh <backend>` then re-spawn"},
		{"pull access denied", "image not built locally", "Run `make -C images <backend>`"},
		{"is a directory", "skills symlink-to-directory regression", "Use `darken skills <harness>` (copy-staging)"},
		{"no such image", "darken image missing", "Run `make -C images all`"},
	}
	for _, p := range patterns {
		if strings.Contains(string(body), p.needle) {
			fmt.Fprintf(&sb, "MATCH %q — %s. Remediation: %s\n", p.needle, p.reason, p.fix)
		}
	}
	if sb.Len() == 0 {
		fmt.Fprintf(&sb, "post-mortem: no known patterns in %s\n", logPath)
	}
	return sb.String()
}

// scanField reads a single YAML scalar field's value from a manifest.
//
// Hand-rolled to avoid a YAML dep per constitution §I.
//
// Assumptions about the manifest shape:
//   - top-level scalar (no nesting under another key)
//   - no comments on the value line (no `field: value  # comment`)
//   - no quoted scalars (no `"foo"` or `'foo'`)
//   - no block scalars (no `|` or `>`)
//
// If a manifest field doesn't fit these assumptions, do not extend
// this function — switch to `scion templates show <name> --local
// --format json` instead.
func scanField(body, prefix string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(t, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// scanList reads a YAML block sequence under a given header.
//
// Hand-rolled to avoid a YAML dep per constitution §I.
//
// Assumptions about the manifest shape:
//   - only single-level block sequence under the header
//   - assumes 2-space indentation
//   - terminates on first non-indented line OR blank line
//   - does not handle nested lists, flow-style sequences, or
//     list-on-same-line as header (no `header: [a, b]`)
//
// If a manifest list doesn't fit these assumptions, do not extend
// this function — switch to `scion templates show <name> --local
// --format json` instead.
func scanList(body, header string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, header) {
			in = true
			continue
		}
		if !in {
			continue
		}
		if v, ok := strings.CutPrefix(t, "- "); ok {
			out = append(out, strings.TrimSpace(v))
			continue
		}
		if t == "" || !strings.HasPrefix(line, "  ") {
			break
		}
	}
	return out
}
