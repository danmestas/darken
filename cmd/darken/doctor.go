package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/danmestas/darken/internal/substrate"
)

// Severity constants for DoctorCheck. Fail checks contribute to the overall
// failure count and cause doctor to exit 1. Warn checks are reported but do
// not cause a non-zero exit.
const (
	SeverityFail = "fail"
	SeverityWarn = "warn"
)

// DoctorCheck is a single preflight check with all its metadata inlined.
// Using a registry of DoctorCheck values replaces the old stringly-typed
// remediationFor dispatch.
type DoctorCheck struct {
	ID          string        // machine-readable identifier (stable across versions)
	Label       string        // human-readable label printed in the report
	Severity    string        // SeverityFail or SeverityWarn
	Run         func() error  // returns nil on success, non-nil on failure
	Detail      func() string // optional: short detail appended to the OK line (e.g. version string)
	Remediation string        // inline remediation hint; used when Run() returns non-nil
}

func runDoctor(args []string) error {
	// Handle --help / -h before any other parsing so it prints
	// subcommand-specific docs rather than falling through to top-level help.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			printDoctorUsage()
			return nil
		}
	}

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

func printDoctorUsage() {
	fmt.Fprintln(os.Stderr, "Usage: darken doctor [flags] [harness-name]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Run preflight and post-mortem health checks.")
	fmt.Fprintln(os.Stderr, "With no arguments: broad system checks (docker, scion, hub secrets, images, etc.).")
	fmt.Fprintln(os.Stderr, "With a harness-name: per-harness checks for that agent.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --init    verify per-project init scaffolds (CLAUDE.md, skills, audit log, etc.)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  darken doctor              # broad system check")
	fmt.Fprintln(os.Stderr, "  darken doctor --init       # verify init scaffolds in current repo")
	fmt.Fprintln(os.Stderr, "  darken doctor researcher-1 # harness-specific check")
}

// checkSubstrateDrift compares the project's orchestrator-mode SKILL.md
// against the embedded copy. Returns a single human-readable line
// describing one of three states (in sync / drift / not initialized).
//
// Kept for backward compatibility with tests that assert specific output
// strings. New code uses checkSubstrateDriftErr.
func checkSubstrateDrift() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "SKIP  substrate drift — not in an init'd repo (run `darken init`)\n", nil
	}
	projectPath := filepath.Join(root, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	projectBody, err := os.ReadFile(projectPath)
	if err != nil {
		return "SKIP  substrate drift — project skill not initialized at " + projectPath + " (run `darken init`)\n", nil
	}
	embeddedBody, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("embedded skill read failed: %w", err)
	}
	if bytes.Equal(projectBody, embeddedBody) {
		return "OK    substrate skills in sync with binary\n", nil
	}
	return "WARN  substrate drift — project's orchestrator-mode/SKILL.md differs from binary (run `darken upgrade-init` to refresh)\n", nil
}

// checkSubstrateDriftErr is the DoctorCheck-compatible variant of
// checkSubstrateDrift: returns nil when in sync or not initialized,
// non-nil error when drift is detected.
func checkSubstrateDriftErr() error {
	root, err := repoRoot()
	if err != nil {
		return nil // not in an init'd repo — not an error
	}
	projectPath := filepath.Join(root, ".claude", "skills", "orchestrator-mode", "SKILL.md")
	projectBody, err := os.ReadFile(projectPath)
	if err != nil {
		return nil // not initialized — SKIP, not WARN
	}
	embeddedBody, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err != nil {
		return fmt.Errorf("embedded skill read failed: %w", err)
	}
	if bytes.Equal(projectBody, embeddedBody) {
		return nil
	}
	return fmt.Errorf("project orchestrator-mode/SKILL.md differs from binary (run darken upgrade-init to refresh)")
}

// doctorBroadChecks returns the ordered registry of broad preflight checks.
// Each DoctorCheck carries its own remediation string; no external dispatch.
func doctorBroadChecks() []DoctorCheck {
	return []DoctorCheck{
		{
			ID:          "docker-daemon",
			Label:       "docker daemon reachable",
			Severity:    SeverityFail,
			Run:         checkDocker,
			Remediation: "start Docker Desktop / podman / colima",
		},
		{
			ID:          "scion-cli",
			Label:       "scion CLI present",
			Severity:    SeverityFail,
			Run:         checkScion,
			Remediation: "make install in ~/projects/scion",
		},
		{
			ID:          "scion-server-status",
			Label:       "scion server status",
			Severity:    SeverityFail,
			Run:         checkScionServer,
			Remediation: "scion server start",
		},
		{
			ID:          "scion-daemon-liveness",
			Label:       "scion daemon liveness",
			Severity:    SeverityFail,
			Run:         checkScionServerLiveness,
			Remediation: "scion server start",
		},
		{
			ID:          "go-git-fuse",
			Label:       "go-git FUSE compatibility",
			Severity:    SeverityFail,
			Run:         checkGoGitFUSE,
			Remediation: "clone the grove outside the Docker Desktop shared volume",
		},
		{
			ID:          "grove-status",
			Label:       "grove status ok (not orphaned)",
			Severity:    SeverityFail,
			Run:         checkGroveStatus,
			Remediation: "re-run `darken up` to re-register the grove with the local broker",
		},
		{
			ID:          "hub-secrets",
			Label:       "hub secrets present",
			Severity:    SeverityFail,
			Run:         checkHubSecrets,
			Remediation: "scripts/stage-creds.sh",
		},
		{
			ID:          "darken-images",
			Label:       "darken images built",
			Severity:    SeverityFail,
			Run:         checkImages,
			Remediation: "make -C images",
		},
		{
			ID:          "substrate-drift",
			Label:       "substrate skills in sync with binary",
			Severity:    SeverityWarn,
			Run:         checkSubstrateDriftErr,
			Remediation: "darken upgrade-init",
		},
		{
			ID:          "hosts-docker-internal",
			Label:       "host.docker.internal in /etc/hosts",
			Severity:    SeverityWarn,
			Run:         checkHostsDockerInternal,
			Remediation: `echo "127.0.0.1 host.docker.internal" | sudo tee -a /etc/hosts`,
		},
		{
			ID:          "bones-cli",
			Label:       "bones CLI present",
			Severity:    SeverityWarn,
			Run:         checkBones,
			Detail:      bonesVersion,
			Remediation: "brew install bones (or run `darken up --no-bones` to skip the bones chain)",
		},
		{
			ID:       "scion-secret-type-enum",
			Label:    "scion hub secret set --type enum accepts 'environment'",
			Severity: SeverityFail,
			Run:      checkScionSecretTypeSupport,
			// Root cause: darken previously passed --type env; scion's enum is
			// environment | variable | file. Any mismatch here breaks every spawn
			// at the stage-creds step. See issue #57.
			Remediation: "upgrade scion (brew upgrade scion or make install in ~/projects/scion)",
		},
	}
}

func doctorBroad() (string, error) {
	var sb strings.Builder
	var failed []string

	for _, dc := range doctorBroadChecks() {
		err := dc.Run()
		switch {
		case err == nil:
			if dc.Detail != nil {
				if d := dc.Detail(); d != "" {
					fmt.Fprintf(&sb, "OK    %s (%s)\n", dc.Label, d)
					continue
				}
			}
			fmt.Fprintf(&sb, "OK    %s\n", dc.Label)
		case dc.Severity == SeverityWarn:
			fmt.Fprintf(&sb, "WARN  %s — %v\n", dc.Label, err)
			fmt.Fprintf(&sb, "      remediation: %s\n", dc.Remediation)
		default:
			fmt.Fprintf(&sb, "FAIL  %s — %v\n", dc.Label, err)
			fmt.Fprintf(&sb, "      remediation: %s\n", dc.Remediation)
			failed = append(failed, dc.ID)
		}
	}

	if len(failed) > 0 {
		sb.WriteString("\n→ for a fresh project, run `darken up` to bring everything online\n")
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
	_, err := exec.LookPath("scion")
	if err != nil {
		return fmt.Errorf("scion not found on PATH: %w", err)
	}
	return nil
}

// checkBones reports whether the bones CLI is on PATH. Severity is Warn
// because `darken up --no-bones` is a supported path; we just want the
// operator to see when bones is missing or stale.
func checkBones() error {
	_, err := exec.LookPath("bones")
	if err != nil {
		return fmt.Errorf("bones not found on PATH (run `brew install bones`, or use --no-bones to skip the chain)")
	}
	return nil
}

// bonesVersion captures `bones --version` for the Detail line of the
// bones-cli check. Returns empty on any error so the renderer falls back
// to plain "OK". Trims whitespace and keeps the first non-empty line.
func bonesVersion() string {
	out, err := exec.Command("bones", "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func checkScionServer() error {
	_, err := defaultScionClient.ServerStatus()
	if err != nil {
		return fmt.Errorf("server not running: %w", err)
	}
	return nil
}

// checkScionServerLiveness probes the scion daemon directly.
// Primary: HTTP GET DARKEN_HUB_ENDPOINT/healthz (fast, works inside Docker).
// Fallback: parse the "Daemon:" line from scion server status (works on host).
func checkScionServerLiveness() error {
	endpoint := os.Getenv("DARKEN_HUB_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultHubEndpoint
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(endpoint + "/healthz")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		return fmt.Errorf("scion daemon /healthz returned %d", resp.StatusCode)
	}
	// Healthz unreachable; fall through to daemon-line parse.
	out, sErr := defaultScionClient.ServerStatus()
	if sErr != nil {
		return fmt.Errorf("scion server status: %w", sErr)
	}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		lower := strings.ToLower(t)
		if !strings.HasPrefix(lower, "daemon:") {
			continue
		}
		if strings.Contains(lower, "running") {
			return nil
		}
		return fmt.Errorf("scion daemon not running: %s", t)
	}
	// No daemon line — accept the zero exit from scion server status.
	return nil
}

// checkGoGitFUSE is the entry point: reads /proc/mounts and the cwd,
// then delegates to checkGoGitFUSEMounts.
func checkGoGitFUSE() error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	return checkGoGitFUSEMounts("/proc/mounts", cwd)
}

// checkGoGitFUSEMounts sniff-tests for Mac Docker Desktop fakeowner FUSE
// mounts that are incompatible with go-git (used by sciontool internally).
// Returns an error when cwd is on a FUSE filesystem; nil otherwise.
// mountsPath and cwd are injectable for testing.
func checkGoGitFUSEMounts(mountsPath, cwd string) error {
	data, err := os.ReadFile(mountsPath)
	if os.IsNotExist(err) {
		return nil // /proc/mounts absent; skip
	}
	if err != nil {
		return nil // unreadable; skip silently
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPoint := fields[1]
		fsType := strings.ToLower(fields[2])
		if !strings.HasPrefix(cwd, mountPoint) {
			continue
		}
		if strings.Contains(fsType, "fuse") || fsType == "virtiofs" {
			return fmt.Errorf(
				"workspace %q is on %s — go-git (used by sciontool) may fail; clone the grove outside the Docker Desktop shared volume",
				cwd, fields[2],
			)
		}
	}
	return nil
}

// hostsFilePath is the path to the hosts file. Injectable for testing.
var hostsFilePath = "/etc/hosts"

// checkHostsDockerInternal verifies that host.docker.internal has an entry
// in the hosts file. Required so the darken CLI (running on the host) can
// reach the scion hub inside Docker Desktop.
func checkHostsDockerInternal() error {
	return checkHostsDockerInternalFile(hostsFilePath)
}

// checkHostsDockerInternalFile is the testable variant that accepts a custom
// hosts file path.
func checkHostsDockerInternalFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // skip comments
		}
		if strings.Contains(trimmed, "host.docker.internal") {
			return nil
		}
	}
	return fmt.Errorf(
		"host.docker.internal not found in %s; add with: echo \"127.0.0.1 host.docker.internal\" | sudo tee -a %s",
		path, path,
	)
}

// groveEntry is the shape of one entry from `scion grove list --format json`.
type groveEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// checkGroveStatus verifies that the current project's grove is reported as
// "ok" by scion grove list. This catches a known scion issue where
// `scion grove init` with hub disabled creates a local-only grove that
// scion's orphan classifier cannot verify, labelling it "orphaned" even when
// the workspace is healthy.
//
// The check is a no-op (returns nil) when:
//   - cwd is not inside a git repo (not an error — operator is not in a project)
//   - the project has no .scion/grove-id (grove has never been init'd)
//   - scion grove list fails (scion not running — a different check covers that)
func checkGroveStatus() error {
	root, err := repoRoot()
	if err != nil {
		return nil // not in a git repo — skip
	}
	// Only run when the grove has been initialized.
	if _, err := os.Stat(filepath.Join(root, ".scion", "grove-id")); err != nil {
		return nil // grove not init'd — skip
	}
	slug := filepath.Base(root)

	out, err := defaultScionClient.GroveListJSON()
	if err != nil {
		return nil // scion not reachable — covered by scion-server checks
	}

	var entries []groveEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil // can't parse JSON — skip rather than false-alarm
	}

	for _, e := range entries {
		if e.Name != slug {
			continue
		}
		if e.Status != "ok" {
			return fmt.Errorf(
				"grove %q reports status=%q (want ok) — "+
					"scion's orphan classifier did not receive the workspace path; "+
					"this is a known scion issue where grove init with hub disabled "+
					"leaves local_path unset in grove_contributors; "+
					"re-run `darken up` or file an issue against scion if it persists",
				slug, e.Status,
			)
		}
		return nil
	}
	// Grove slug not found in list — either not registered or list is incomplete.
	return nil
}

func checkHubSecrets() error {
	out, err := defaultScionClient.SecretList()
	if err != nil {
		return fmt.Errorf("hub secret list: %w", err)
	}
	for _, want := range []string{"claude_auth", "codex_auth"} {
		if !strings.Contains(out, want) {
			return fmt.Errorf("missing hub secret: %s", want)
		}
	}
	return nil
}

// checkScionSecretTypeSupport probes whether scion's `hub secret set`
// accepts --type environment (the value darken passes for env secrets).
//
// Background: darken previously passed --type env, which scion's current
// enum rejects (accepted: environment | variable | file). That mismatch
// caused every spawn to fail at stage-creds with exit 1 while doctor
// reported all-green. This check surfaces the drift before any spawn is
// attempted.
//
// The probe invokes `scion hub secret set --help` and confirms that
// "environment" appears in the flag description. This is a read-only, no-op
// call — it never writes a secret — so it is safe to run at doctor time.
func checkScionSecretTypeSupport() error {
	out, err := exec.Command("scion", "hub", "secret", "set", "--help").CombinedOutput()
	if err != nil {
		// If scion itself is absent, the scion-cli check already caught it.
		// Return nil here to avoid double-reporting; the scion-cli FAIL is
		// the actionable one.
		return nil
	}
	if !strings.Contains(string(out), "environment") {
		return fmt.Errorf(
			"scion hub secret set --help does not list 'environment' as an accepted --type value; " +
				"darken passes --type environment for env secrets. " +
				"This version of scion may use a different enum — upgrade scion or check `scion hub secret set --help` manually",
		)
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
	manifest, err := loadHarnessManifest(body)
	if err != nil {
		return fmt.Sprintf("FAIL  manifest parse for %s — %v\n", name, err),
			fmt.Errorf("manifest parse: %w", err)
	}
	backend := manifest.Backend
	skills := manifest.Skills

	var sb strings.Builder
	var failed []string

	fmt.Fprintf(&sb, "OK    manifest %s served from %s layer\n", name, manifestLayer)

	imgTag := imageTagFor(backend)
	if !imageExists(imgTag) {
		fmt.Fprintf(&sb, "FAIL  image %s missing — remediation: make -C images\n", imgTag)
		failed = append(failed, "image")
	} else {
		fmt.Fprintf(&sb, "OK    image %s present\n", imgTag)
	}

	wantSecret := harnessSecretFor(backend)
	{
		out, _ := defaultScionClient.SecretList()
		if !strings.Contains(out, wantSecret) {
			fmt.Fprintf(&sb, "FAIL  hub secret %s missing — remediation: scripts/stage-creds.sh\n", wantSecret)
			failed = append(failed, "secret")
		} else {
			fmt.Fprintf(&sb, "OK    hub secret %s present\n", wantSecret)
		}
	}

	stageDir := filepath.Join(root, ".scion", "skills-staging", name)
	if _, err := os.Stat(stageDir); err != nil {
		fmt.Fprintf(&sb, "FAIL  skills-staging dir missing at %s — remediation: darken skills %s\n", stageDir, name)
		failed = append(failed, "staging")
	} else {
		stagingFailed := false
		for _, ref := range skills {
			n := ref[strings.LastIndex(ref, "/")+1:]
			if _, err := os.Stat(filepath.Join(stageDir, n)); err != nil {
				fmt.Fprintf(&sb, "FAIL  manifest declares %q but skills-staging is missing it — remediation: darken skills %s\n", n, name)
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
