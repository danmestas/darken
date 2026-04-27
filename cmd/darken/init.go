package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
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
		return nil
	}

	body, err := renderCLAUDE(target)
	if err != nil {
		return err
	}
	if err := os.WriteFile(claudePath, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", claudePath)
	return nil
}

// renderCLAUDE renders the embedded CLAUDE.md template with the
// target dir's basename as RepoName.
func renderCLAUDE(targetDir string) (string, error) {
	body, err := readEmbeddedTemplate("data/templates/CLAUDE.md.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("claude").Parse(string(body))
	if err != nil {
		return "", err
	}
	data := struct{ RepoName string }{
		RepoName: filepath.Base(targetDir),
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
