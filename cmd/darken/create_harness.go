package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runCreateHarness(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: darken create-harness <role> --backend X --model Y --skills A,B --description '...' [--max-turns N --axes 'taste,reversibility']")
	}
	role := args[0]

	fs := flag.NewFlagSet("create-harness", flag.ContinueOnError)
	backend := fs.String("backend", "claude", "claude|codex|pi|gemini")
	model := fs.String("model", "", "model id (e.g. claude-sonnet-4-6)")
	skills := fs.String("skills", "", "comma-separated APM-style skill refs")
	desc := fs.String("description", "", "one-sentence description")
	maxTurns := fs.Int("max-turns", 50, "scion max_turns for the manifest")
	axes := fs.String("axes", "(none)", "escalation-axis affinity (e.g. taste,reversibility)")
	scope := fs.String("scope", "user", "where to scaffold: user|project")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *model == "" || *desc == "" {
		return errors.New("--model and --description are required")
	}
	if *scope != "user" && *scope != "project" {
		return fmt.Errorf("--scope must be user|project, got %q", *scope)
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}
	var dir string
	switch *scope {
	case "user":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".config", "darken", "overrides", ".scion", "templates", role)
	case "project":
		dir = filepath.Join(root, ".scion", "templates", role)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	skillList := splitNonEmpty(*skills, ",")
	manifest := buildManifest(role, *backend, *model, *desc, skillList, *maxTurns)
	if err := os.WriteFile(filepath.Join(dir, "scion-agent.yaml"), []byte(manifest), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "agents.md"), []byte(agentsTemplate(role)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "system-prompt.md"), []byte(promptTemplate(role, *desc)), 0o644); err != nil {
		return err
	}

	rosterPath := filepath.Join(root, ".design", "harness-roster.md")
	body, err := os.ReadFile(rosterPath)
	if err != nil {
		return err
	}
	// 8 cells matching .design/harness-roster.md header:
	// Role | Backend | Model | Max turns | Max duration | Detached | Axes | One-line role.
	row := fmt.Sprintf("| `%s` | %s | %s | %d | %s | false | %s | %s |\n",
		role, *backend, *model, *maxTurns, "1h", *axes, *desc)
	out, err := insertAfterTableSeparator(string(body), row)
	if err != nil {
		return fmt.Errorf("%s: %w", rosterPath, err)
	}
	return os.WriteFile(rosterPath, []byte(out), 0o644)
}

// insertAfterTableSeparator finds the first markdown table separator
// line (e.g. "|---|---|---|") in body and inserts row immediately
// after it. This places new entries inside the table body rather than
// above its header. Returns an error if no separator is found —
// silent fall-through would corrupt the roster.
func insertAfterTableSeparator(body, row string) (string, error) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if isMarkdownTableSeparator(strings.TrimSpace(line)) {
			newRow := strings.TrimRight(row, "\n")
			out := append([]string{}, lines[:i+1]...)
			out = append(out, newRow)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n"), nil
		}
	}
	return "", errors.New("missing markdown table separator (|---|---|...)")
}

// isMarkdownTableSeparator reports whether line is a GitHub-flavored
// markdown table separator: pipe-delimited cells of dashes (with
// optional alignment colons), e.g. "|---|---|---|" or "|:---|---:|".
func isMarkdownTableSeparator(line string) bool {
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") || len(line) < 5 {
		return false
	}
	inner := strings.Trim(line, "|")
	cells := strings.Split(inner, "|")
	for _, c := range cells {
		c = strings.TrimSpace(c)
		c = strings.TrimLeft(c, ":")
		c = strings.TrimRight(c, ":")
		if len(c) < 3 || strings.Trim(c, "-") != "" {
			return false
		}
	}
	return true
}

func splitNonEmpty(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildManifest(role, backend, model, desc string, skills []string, maxTurns int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "schema_version: \"1\"\n")
	fmt.Fprintf(&sb, "description: %q\n", desc)
	fmt.Fprintf(&sb, "agent_instructions: agents.md\n")
	fmt.Fprintf(&sb, "system_prompt: system-prompt.md\n")
	fmt.Fprintf(&sb, "default_harness_config: %s\n", backend)
	fmt.Fprintf(&sb, "image: local/darkish-%s:latest\n", backend)
	fmt.Fprintf(&sb, "model: %s\n", model)
	fmt.Fprintf(&sb, "max_turns: %d\n", maxTurns)
	fmt.Fprintf(&sb, "max_duration: \"1h\"\n")
	fmt.Fprintf(&sb, "detached: false\n")
	if len(skills) > 0 {
		fmt.Fprintln(&sb, "skills:")
		for _, s := range skills {
			fmt.Fprintf(&sb, "  - %s\n", s)
		}
		fmt.Fprintln(&sb, "volumes:")
		fmt.Fprintf(&sb, "  - source: ./.scion/skills-staging/%s/\n", role)
		fmt.Fprintf(&sb, "    target: /home/scion/skills/role/\n")
		fmt.Fprintf(&sb, "    read_only: true\n")
	}
	return sb.String()
}

func agentsTemplate(role string) string {
	return fmt.Sprintf(`# %s — agent instructions

Worker protocol. See README §5.1 for the role definition.

## Communication tier

- To orchestrator: caveman standard.
- To sub-agents (if any): caveman ultra.
`, role)
}

func promptTemplate(role, desc string) string {
	return fmt.Sprintf(`# %s

%s

Fill in this prompt with role-specific identity, constraints, and
output format expectations. Operator must complete this stub.
`, role, desc)
}
