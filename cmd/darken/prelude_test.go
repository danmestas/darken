package main

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/danmestas/darken/internal/substrate"
)

// TestPrelude_SkillsDirPermissions verifies that every claude-backend prelude
// writes a ~/.claude/settings.json that grants auto-allow for .claude/skills
// writes. Without this, spawned agents block on a TUI dialog when editing
// skill files even when --dangerously-skip-permissions is set.
func TestPrelude_SkillsDirPermissions(t *testing.T) {
	// All four backends use the claude harness image; only claude is covered
	// here because codex/pi/gemini use non-claude CLI and have no settings.json
	// concept. The marker must appear in the embedded prelude for "claude".
	markers := []string{
		".claude/skills",
		"settings.json",
		"permissions",
	}

	efs := substrate.EmbeddedFS()
	path := "data/images/claude/darkish-prelude.sh"
	body, err := fs.ReadFile(efs, path)
	if err != nil {
		t.Fatalf("cannot read embedded claude prelude at %s: %v", path, err)
	}
	text := string(body)
	for _, m := range markers {
		if !strings.Contains(text, m) {
			t.Errorf("claude prelude missing skills-permission marker %q", m)
		}
	}
}

// TestPrelude_NonClaudeHookBlockAbsent verifies that codex, gemini, and pi
// preludes do not contain the Claude hook setup block. That block writes
// ~/.claude/settings.json hook config, which is meaningless on non-Claude
// harnesses. Ousterhout review fixup F5: strip it from non-Claude images.
func TestPrelude_NonClaudeHookBlockAbsent(t *testing.T) {
	backends := []string{"codex", "gemini", "pi"}
	// Markers exclusive to the Claude hook setup block.
	markers := []string{
		"DARKEN_HOOK_SCRIPT",
		"notify-operator.sh",
		"DARKISH_HOOK_EVENT",
	}

	efs := substrate.EmbeddedFS()
	for _, b := range backends {
		path := "data/images/" + b + "/darkish-prelude.sh"
		body, err := fs.ReadFile(efs, path)
		if err != nil {
			t.Fatalf("cannot read embedded prelude for %s at %s: %v", b, path, err)
		}
		text := string(body)
		for _, m := range markers {
			if strings.Contains(text, m) {
				t.Errorf("prelude %s must not contain Claude hook marker %q (fixup F5)", b, m)
			}
		}
	}
}

// TestPrelude_SkillsPermissionsJqMerge verifies that the claude prelude uses
// jq-merge semantics for .claude/skills permissions, not full-file replacement.
// Full replacement destroys operator-configured permissions and hooks.
// Ousterhout review fixup F6.
func TestPrelude_SkillsPermissionsJqMerge(t *testing.T) {
	efs := substrate.EmbeddedFS()
	path := "data/images/claude/darkish-prelude.sh"
	body, err := fs.ReadFile(efs, path)
	if err != nil {
		t.Fatalf("cannot read embedded claude prelude at %s: %v", path, err)
	}
	text := string(body)

	// Must NOT use a bare cat-replace for the skills settings block.
	if strings.Contains(text, "cat > \"${CLAUDE_SETTINGS}\"") {
		t.Error("claude prelude must not use cat > to fully replace settings.json; use jq merge (fixup F6)")
	}

	// Must use jq to merge the allow rules.
	if !strings.Contains(text, "permissions.allow") {
		t.Error("claude prelude must use jq to merge .permissions.allow for skills permissions (fixup F6)")
	}
}

// TestPrelude_HookInsertionIdempotent verifies that the claude prelude guards
// against duplicate hook entries before appending. Without this guard,
// persistent container homes accumulate duplicate hooks on every prelude run.
// Ousterhout review fixup F7.
func TestPrelude_HookInsertionIdempotent(t *testing.T) {
	efs := substrate.EmbeddedFS()
	path := "data/images/claude/darkish-prelude.sh"
	body, err := fs.ReadFile(efs, path)
	if err != nil {
		t.Fatalf("cannot read embedded claude prelude at %s: %v", path, err)
	}
	text := string(body)

	// Expect a jq de-duplication guard — any() or map(select(...)) pattern.
	dedupeMarkers := []string{"any(", "map(select("}
	found := false
	for _, m := range dedupeMarkers {
		if strings.Contains(text, m) {
			found = true
			break
		}
	}
	if !found {
		t.Error("claude prelude hook insertion must be idempotent; expected jq any() or map(select()) guard (fixup F7)")
	}
}

// TestPreludePreCloneBlockPropagated verifies that the three non-claude
// prelude scripts contain the pre-clone workaround block that is
// authoritative in images/claude/darkish-prelude.sh. Each prelude must
// gate the git-clone on SCION_GIT_CLONE_URL, GITHUB_TOKEN, and the
// absence of /workspace/.git — the same three conditions as the claude
// reference.
func TestPreludePreCloneBlockPropagated(t *testing.T) {
	backends := []string{"codex", "gemini", "pi"}
	// These markers must all appear in each target prelude.
	markers := []string{
		"SCION_GIT_CLONE_URL",
		"GITHUB_TOKEN",
		"/workspace/.git",
		"pre-clone",
	}

	efs := substrate.EmbeddedFS()
	for _, b := range backends {
		path := "data/images/" + b + "/darkish-prelude.sh"
		body, err := fs.ReadFile(efs, path)
		if err != nil {
			t.Fatalf("cannot read embedded prelude for %s at %s: %v", b, path, err)
		}
		text := string(body)
		for _, m := range markers {
			if !strings.Contains(text, m) {
				t.Errorf("prelude %s missing pre-clone marker %q", b, m)
			}
		}
	}
}
