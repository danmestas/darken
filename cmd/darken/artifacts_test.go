package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danmestas/darken/internal/substrate"
)

func TestInitArtifacts_ListIsCompleteAndStable(t *testing.T) {
	target := t.TempDir()
	arts := initArtifacts(target)

	if len(arts) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(arts))
	}

	wantPaths := map[string]string{
		"CLAUDE.md": "file",
		".claude/skills/orchestrator-mode/SKILL.md":      "file",
		".claude/skills/subagent-to-subharness/SKILL.md": "file",
		".claude/settings.local.json":                    "file",
		".gitignore":                                     "gitignore-lines",
	}
	for _, art := range arts {
		wantKind, ok := wantPaths[art.RelPath]
		if !ok {
			t.Errorf("unexpected artifact: %q", art.RelPath)
			continue
		}
		if art.Kind != wantKind {
			t.Errorf("artifact %q: expected kind %q, got %q", art.RelPath, wantKind, art.Kind)
		}
		delete(wantPaths, art.RelPath)
	}
	for missing := range wantPaths {
		t.Errorf("missing artifact: %q", missing)
	}
}

func TestInitArtifacts_BodyMatchesEmbeddedSkill(t *testing.T) {
	target := t.TempDir()
	arts := initArtifacts(target)

	var found bool
	for _, art := range arts {
		if art.RelPath != ".claude/skills/orchestrator-mode/SKILL.md" {
			continue
		}
		got, err := art.Body()
		if err != nil {
			t.Fatalf("Body() error: %v", err)
		}
		want, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
		if err != nil {
			t.Fatalf("read embedded: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("orchestrator-mode SKILL.md Body() differs from embedded:\nwant len=%d\ngot len=%d", len(want), len(got))
		}
		found = true
	}
	if !found {
		t.Fatal("orchestrator-mode artifact not in list")
	}
}

func TestInitArtifacts_CLAUDEBodyTemplatedWithRepoName(t *testing.T) {
	target := filepath.Join(t.TempDir(), "myproject")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	arts := initArtifacts(target)

	for _, art := range arts {
		if art.RelPath != "CLAUDE.md" {
			continue
		}
		body, err := art.Body()
		if err != nil {
			t.Fatalf("Body() error: %v", err)
		}
		s := string(body)
		if !strings.Contains(s, "myproject") {
			t.Fatalf("CLAUDE.md should reference target basename 'myproject', got:\n%s", s)
		}
		if !strings.Contains(s, substrate.EmbeddedHash()[:12]) {
			t.Fatalf("CLAUDE.md should contain first 12 chars of embedded hash, got:\n%s", s)
		}
		return
	}
	t.Fatal("CLAUDE.md artifact not in list")
}

func TestInitArtifacts_GitignoreLinesContainsExpectedSet(t *testing.T) {
	arts := initArtifacts(t.TempDir())
	for _, art := range arts {
		if art.RelPath != ".gitignore" {
			continue
		}
		body, err := art.Body()
		if err != nil {
			t.Fatalf("Body() error: %v", err)
		}
		s := string(body)
		for _, want := range []string{
			".scion/agents/",
			".scion/skills-staging/",
			".scion/audit.jsonl",
			".claude/worktrees/",
			".claude/settings.local.json",
			".superpowers/",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("gitignore Body() missing %q:\n%s", want, s)
			}
		}
		return
	}
	t.Fatal(".gitignore artifact not in list")
}
