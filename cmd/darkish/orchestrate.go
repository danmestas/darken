package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// runOrchestrate prints the host-mode orchestrator skill body to stdout.
// Operators pipe this into a fresh Claude Code session to prime as the
// orchestrator without relying on the project's CLAUDE.md auto-load.
//
// Lookup order:
//  1. <repo>/.claude/skills/orchestrator-mode/SKILL.md (project copy)
//  2. ~/projects/agent-skills/skills/orchestrator-mode/SKILL.md (canonical)
func runOrchestrate(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darkish orchestrate")
	}

	candidates := []string{}
	if root, err := repoRoot(); err == nil {
		candidates = append(candidates, filepath.Join(root, ".claude", "skills", "orchestrator-mode", "SKILL.md"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "projects", "agent-skills", "skills", "orchestrator-mode", "SKILL.md"))
	}

	for _, p := range candidates {
		body, err := os.ReadFile(p)
		if err == nil {
			_, err = os.Stdout.Write(body)
			return err
		}
	}

	return fmt.Errorf("orchestrator skill not found in any of: %v", candidates)
}
