package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/danmestas/darken/internal/substrate"
)

// runOrchestrate prints the host-mode orchestrator skill body to stdout.
// Operators pipe this into a fresh Claude Code session to prime as the
// orchestrator without relying on the project's CLAUDE.md auto-load.
//
// Lookup order:
//  1. <repo>/.claude/skills/orchestrator-mode/SKILL.md (project copy)
//  2. ~/projects/agent-skills/skills/orchestrator-mode/SKILL.md (canonical)
//  3. embedded substrate (data/skills/orchestrator-mode/SKILL.md)
func runOrchestrate(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: darken orchestrate")
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

	// Phase 2 fallback: embedded
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err == nil {
		_, err = os.Stdout.Write(body)
		return err
	}

	return fmt.Errorf("orchestrator skill not found in project, agent-skills, or embedded substrate")
}
