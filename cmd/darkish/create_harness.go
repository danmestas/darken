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
		return errors.New("usage: darkish create-harness <role> --backend X --model Y --skills A,B --description '...' [--max-turns N --axes 'taste,reversibility']")
	}
	role := args[0]

	fs := flag.NewFlagSet("create-harness", flag.ContinueOnError)
	backend := fs.String("backend", "claude", "claude|codex|pi|gemini")
	model := fs.String("model", "", "model id (e.g. claude-sonnet-4-6)")
	skills := fs.String("skills", "", "comma-separated APM-style skill refs")
	desc := fs.String("description", "", "one-sentence description")
	maxTurns := fs.Int("max-turns", 50, "scion max_turns for the manifest")
	axes := fs.String("axes", "(none)", "escalation-axis affinity (e.g. taste,reversibility)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *model == "" || *desc == "" {
		return errors.New("--model and --description are required")
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, ".scion", "templates", role)
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
	row := fmt.Sprintf("| `%s` | %s | %d | %s | false | %s | %s |\n",
		role, *model, *maxTurns, "1h", *axes, *desc)
	// The real harness-roster.md has a blank line after the heading; the
	// anchor "## Roster\n\n" matches both fixtures and the live doc.
	out := strings.Replace(string(body), "## Roster\n\n",
		"## Roster\n\n"+row, 1)
	return os.WriteFile(rosterPath, []byte(out), 0o644)
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
