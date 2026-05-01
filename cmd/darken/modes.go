package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/danmestas/darken/internal/substrate"
	"gopkg.in/yaml.v3"
)

// runModes dispatches `darken modes list` and `darken modes show <name>`.
func runModes(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: darken modes list | show <name>")
	}
	switch args[0] {
	case "list":
		return runModesList()
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: darken modes show <name>")
		}
		return runModesShow(args[1])
	default:
		return fmt.Errorf("unknown subcommand: modes %s", args[0])
	}
}

// modesDir locates the modes directory: prefers project-local
// .scion/modes/ if the cwd is a darkish-factory checkout; otherwise
// falls back to the embedded copy by extracting just the modes tree.
// Returns (path, cleanup, err). Cleanup is a no-op for project-local.
func modesDir() (string, func(), error) {
	noop := func() {}
	if root, err := repoRoot(); err == nil {
		dir := filepath.Join(root, ".scion", "modes")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			return dir, noop, nil
		}
	}
	// Fall back to embedded by routing through the substrate extraction
	// helper used by bootstrap.
	_, modesPath, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		return "", noop, fmt.Errorf("locate modes dir: %w", err)
	}
	if modesPath == "" {
		cleanup()
		return "", noop, fmt.Errorf("modes dir not available")
	}
	return modesPath, cleanup, nil
}

func runModesList() error {
	dir, cleanup, err := modesDir()
	if err != nil {
		return err
	}
	defer cleanup()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read modes dir %s: %w", dir, err)
	}

	type row struct{ name, desc string }
	var rows []row
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			rows = append(rows, row{name, "(unreadable)"})
			continue
		}
		var m struct {
			Description string `yaml:"description"`
		}
		_ = yaml.Unmarshal(body, &m)
		rows = append(rows, row{name, m.Description})
	}

	// Pad name column for readability.
	w := 0
	for _, r := range rows {
		if len(r.name) > w {
			w = len(r.name)
		}
	}
	for _, r := range rows {
		fmt.Printf("  %-*s  %s\n", w, r.name, r.desc)
	}
	return nil
}

func runModesShow(name string) error {
	dir, cleanup, err := modesDir()
	if err != nil {
		return err
	}
	defer cleanup()

	path := filepath.Join(dir, name+".yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("mode %q: %w", name, err)
	}

	fmt.Printf("# %s\n%s\n", path, body)

	resolved, err := substrate.ResolveSkillsFromFS(os.DirFS(dir), name)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", name, err)
	}
	fmt.Printf("# resolved skills (after extends expansion):\n")
	if len(resolved) == 0 {
		fmt.Printf("  (none)\n")
		return nil
	}
	for _, s := range resolved {
		fmt.Printf("  %s\n", s)
	}
	return nil
}

// Suppress unused import warning if fs.FS becomes unused.
var _ = fs.ValidPath
