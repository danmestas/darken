package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// initCheck describes a single required-file check inside an init'd repo.
// failHint is shown when the check fails — should be a one-liner with the
// remediation command the operator should run.
type initCheck struct {
	name     string
	path     string
	check    func(absPath string) error
	failHint string
}

// runInitDoctor runs the per-init verification pass against the given
// target directory. Returns a formatted report and an error if any
// check failed. Mirrors the doctorBroad/doctorHarness pattern.
func runInitDoctor(target string) (string, error) {
	checks := []initCheck{
		{
			name:     "CLAUDE.md present",
			path:     "CLAUDE.md",
			check:    fileNonEmpty,
			failHint: "run `darken init " + target + "` to scaffold",
		},
		{
			name:     "orchestrator-mode skill scaffolded",
			path:     ".claude/skills/orchestrator-mode/SKILL.md",
			check:    fileNonEmpty,
			failHint: "run `darken init --refresh` to extract from binary",
		},
		{
			name:     "subagent-to-subharness skill scaffolded",
			path:     ".claude/skills/subagent-to-subharness/SKILL.md",
			check:    fileNonEmpty,
			failHint: "run `darken init --refresh` to extract from binary",
		},
		{
			name:     "settings.local.json with statusLine command",
			path:     ".claude/settings.local.json",
			check:    statusLineConfigValid,
			failHint: "run `darken init --refresh` to recreate",
		},
		{
			name:     ".gitignore has darken entries",
			path:     ".gitignore",
			check:    gitignoreHasDarkenEntries,
			failHint: "run `darken init --refresh` to append entries",
		},
	}

	var sb strings.Builder
	var failed []string
	for _, c := range checks {
		abs := filepath.Join(target, c.path)
		if err := c.check(abs); err != nil {
			fmt.Fprintf(&sb, "FAIL  %s — %v\n", c.name, err)
			fmt.Fprintf(&sb, "      remediation: %s\n", c.failHint)
			failed = append(failed, c.name)
		} else {
			fmt.Fprintf(&sb, "OK    %s\n", c.name)
		}
	}

	if len(failed) > 0 {
		return sb.String(), fmt.Errorf("%d init checks failed: %s",
			len(failed), strings.Join(failed, ", "))
	}
	return sb.String(), nil
}

// fileNonEmpty asserts the path exists and is non-zero size.
func fileNonEmpty(absPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("missing or inaccessible: %s", absPath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("empty file: %s", absPath)
	}
	return nil
}

// statusLineConfigValid asserts settings.local.json parses as JSON and
// has a statusLine.command field set.
func statusLineConfigValid(absPath string) error {
	body, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("missing settings.local.json")
	}
	var cfg struct {
		StatusLine struct {
			Command string `json:"command"`
			Type    string `json:"type"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if cfg.StatusLine.Command == "" {
		return fmt.Errorf("statusLine.command not set")
	}
	return nil
}

// prereqTool describes a binary the operator needs on PATH for darken
// init to produce a working orchestrator-mode setup.
type prereqTool struct {
	name        string
	installHint string
}

// verifyInitPrereqs returns a non-nil error listing all missing
// prerequisite tools, with a one-liner install hint per tool. Called
// at the top of runInit so failures surface upfront, not mid-spawn.
func verifyInitPrereqs() error {
	tools := []prereqTool{
		{name: "bones", installHint: "brew install danmestas/tap/bones"},
		{name: "scion", installHint: "see https://github.com/GoogleCloudPlatform/scion (make install)"},
		{name: "docker", installHint: "install Docker Desktop or colima or podman"},
	}
	var missing []string
	for _, t := range tools {
		if _, err := exec.LookPath(t.name); err != nil {
			missing = append(missing, fmt.Sprintf("  - %s not on PATH; install via: %s", t.name, t.installHint))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("darken init prereqs missing:\n%s", strings.Join(missing, "\n"))
	}
	return nil
}

// gitignoreHasDarkenEntries asserts the canonical darken-related
// gitignore entries are present.
func gitignoreHasDarkenEntries(absPath string) error {
	body, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("missing .gitignore")
	}
	want := []string{
		".scion/agents/",
		".scion/skills-staging/",
		".claude/worktrees/",
	}
	var missing []string
	for _, w := range want {
		if !strings.Contains(string(body), w) {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing entries: %s", strings.Join(missing, ", "))
	}
	return nil
}
