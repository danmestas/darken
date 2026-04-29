package main

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/danmestas/darken/internal/substrate"
)

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
