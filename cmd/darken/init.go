package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/danmestas/darken/internal/substrate"
)

func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "print actions without executing")
	force := flags.Bool("force", false, "overwrite existing CLAUDE.md")
	if err := flags.Parse(args); err != nil {
		return err
	}

	pos := flags.Args()
	target := "."
	if len(pos) > 0 {
		target = pos[0]
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("target dir does not exist: %s", target)
	}

	claudePath := filepath.Join(target, "CLAUDE.md")
	exists := false
	if _, err := os.Stat(claudePath); err == nil {
		exists = true
	}

	if *dryRun {
		if exists && !*force {
			fmt.Printf("would skip %s (already exists; use --force to overwrite)\n", claudePath)
		} else {
			fmt.Printf("would create %s\n", claudePath)
		}
		return nil
	}

	if exists && !*force {
		fmt.Printf("skipped %s (already exists; use --force to overwrite)\n", claudePath)
	} else {
		body, err := renderCLAUDE(target)
		if err != nil {
			return err
		}
		if err := os.WriteFile(claudePath, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", claudePath)
	}

	// Scaffold skills (project-local copies of the embedded host-mode skills)
	for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
		if err := scaffoldSkill(target, skill); err != nil {
			fmt.Fprintf(os.Stderr, "init: skill scaffold %s failed: %v\n", skill, err)
		} else {
			fmt.Printf("scaffolded .claude/skills/%s/SKILL.md\n", skill)
		}
	}

	// Scaffold statusLine + gitignore
	if err := scaffoldStatusLine(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: statusLine scaffold failed: %v\n", err)
	} else {
		fmt.Println("scaffolded .claude/settings.local.json")
	}
	if err := scaffoldGitignore(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: .gitignore append failed: %v\n", err)
	} else {
		fmt.Println("appended darken entries to .gitignore")
	}

	// bones init (soft-fail if bones not on PATH)
	if err := runBonesInit(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: bones init failed: %v\n", err)
	} else if _, err := exec.LookPath("bones"); err == nil {
		fmt.Println("ran `bones init` for workspace bootstrap")
	}

	return nil
}

// renderCLAUDE renders the embedded CLAUDE.md template with the
// target dir's basename as RepoName and the first 12 chars of the
// embedded substrate hash as SubstrateHash12.
func renderCLAUDE(targetDir string) (string, error) {
	body, err := readEmbeddedTemplate("data/templates/CLAUDE.md.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("claude").Parse(string(body))
	if err != nil {
		return "", err
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
		return "", err
	}
	return sb.String(), nil
}

func readEmbeddedTemplate(path string) ([]byte, error) {
	body, err := fs.ReadFile(substrate.EmbeddedFS(), path)
	if err != nil {
		return nil, fmt.Errorf("embedded template not found: %s: %w", path, err)
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

// scaffoldSkill copies a skill from the embedded substrate into
// .claude/skills/<name>/SKILL.md so Claude Code's project-local skill
// discovery picks it up.
func scaffoldSkill(targetDir, name string) error {
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/"+name+"/SKILL.md")
	if err != nil {
		return err
	}
	dst := filepath.Join(targetDir, ".claude", "skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, body, 0o644)
}

// scaffoldStatusLine writes .claude/settings.local.json with a
// statusLine.command pointing at `darken status`. If the file already
// exists, leaves it alone (don't clobber other settings).
func scaffoldStatusLine(targetDir string) error {
	path := filepath.Join(targetDir, ".claude", "settings.local.json")
	if _, err := os.Stat(path); err == nil {
		return nil // existing settings; don't clobber
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := `{
  "statusLine": {
    "command": "darken status",
    "type": "command"
  }
}
`
	return os.WriteFile(path, []byte(body), 0o644)
}

// scaffoldGitignore appends darken-related entries to <target>/.gitignore.
// Idempotent — only appends entries not already present.
func scaffoldGitignore(targetDir string) error {
	path := filepath.Join(targetDir, ".gitignore")
	var existing []byte
	if b, err := os.ReadFile(path); err == nil {
		existing = b
	}
	entries := []string{
		"# darken: scion runtime + per-spawn worktrees + claude-code worktrees",
		".scion/agents/",
		".scion/skills-staging/",
		".scion/audit.jsonl",
		".claude/worktrees/",
		".claude/settings.local.json",
		".superpowers/",
	}
	var add []string
	for _, e := range entries {
		if !strings.Contains(string(existing), e) {
			add = append(add, e)
		}
	}
	if len(add) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		f.WriteString("\n")
	}
	for _, e := range add {
		f.WriteString(e + "\n")
	}
	return nil
}

// runBonesInit shells out to `bones init` in the target dir if bones is
// on PATH. Soft-fail: bones being missing is not fatal — operator
// without bones still gets a usable orchestrator session.
func runBonesInit(targetDir string) error {
	if _, err := exec.LookPath("bones"); err != nil {
		return nil // soft-fail; bones not on PATH
	}
	c := exec.Command("bones", "init")
	c.Dir = targetDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
