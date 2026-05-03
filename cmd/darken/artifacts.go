// Package main — artifact list for `darken init`.
//
// initArtifacts is the single source of truth for what init scaffolds.
// Both runInit (writer) and runUninstallInit (reader) consume this
// list. Adding a new scaffold means appending one entry here; both
// commands pick it up automatically.
package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/danmestas/darken/internal/substrate"
)

// artifact describes one file or file-region that `darken init` writes
// into a target directory.
type artifact struct {
	// RelPath is the artifact's path relative to the init target dir.
	RelPath string
	// Kind is "file" (whole-file owned) or "gitignore-lines" (line-set
	// appended into a possibly-shared file).
	Kind string
	// Body returns the bytes init would write at the current binary's
	// substrate version. For "gitignore-lines" the bytes are the
	// newline-joined set of lines including the leading comment.
	Body func() ([]byte, error)
}

// gitignoreLines is the canonical line set that init appends to the
// project's .gitignore. Stored as a slice so uninstall can also iterate
// for line-presence checks. The leading comment is the first entry.
var gitignoreLines = []string{
	"# darken: scion runtime + per-spawn worktrees + claude-code worktrees",
	".scion/agents/",
	".scion/skills-staging/",
	".scion/audit.jsonl",
	".claude/worktrees/",
	".claude/settings.local.json",
	".superpowers/",
}

// settingsLocalJSON is the body init writes to .claude/settings.local.json.
const settingsLocalJSON = `{
  "statusLine": {
    "command": "darken status",
    "type": "command"
  }
}
`

// initArtifacts returns the artifact list keyed to the target directory.
// Order is stable across calls and across runs.
func initArtifacts(targetDir string) []artifact {
	return []artifact{
		{
			RelPath: "CLAUDE.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return claudeMdBody(targetDir) },
		},
		{
			RelPath: ".claude/skills/orchestrator-mode/SKILL.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return embeddedSkillBody("orchestrator-mode") },
		},
		{
			RelPath: ".claude/skills/subagent-to-subharness/SKILL.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return embeddedSkillBody("subagent-to-subharness") },
		},
		{
			RelPath: ".claude/settings.local.json",
			Kind:    "file",
			Body:    func() ([]byte, error) { return []byte(settingsLocalJSON), nil },
		},
		{
			RelPath: ".gitignore",
			Kind:    "gitignore-lines",
			Body:    func() ([]byte, error) { return []byte(strings.Join(gitignoreLines, "\n") + "\n"), nil },
		},
		{
			RelPath: ".scion/audit.jsonl",
			Kind:    "touch",
			Body:    func() ([]byte, error) { return nil, nil },
		},
	}
}

// claudeMdBody renders the embedded CLAUDE.md.tmpl with the target
// dir's basename as RepoName and the first 12 chars of the embedded
// substrate hash as SubstrateHash12. Will replace renderCLAUDE in init.go
// after Task 2.
func claudeMdBody(targetDir string) ([]byte, error) {
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/templates/CLAUDE.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("embedded CLAUDE.md.tmpl: %w", err)
	}
	tmpl, err := template.New("claude").Parse(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse CLAUDE.md template: %w", err)
	}
	data := struct {
		RepoName        string
		SubstrateHash12 string
	}{
		RepoName:        filepath.Base(targetDir),
		SubstrateHash12: firstN(substrate.EmbeddedHash(), 12),
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return nil, fmt.Errorf("execute CLAUDE.md template: %w", err)
	}
	return []byte(sb.String()), nil
}

// embeddedSkillBody returns the embedded SKILL.md body for the given
// skill name. Replaces the per-skill read in scaffoldSkill.
func embeddedSkillBody(name string) ([]byte, error) {
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/"+name+"/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("embedded skill %s: %w", name, err)
	}
	return body, nil
}

// firstN returns the first n characters of s, or all of s if shorter.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
