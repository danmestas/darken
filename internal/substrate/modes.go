package substrate

import (
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// Mode is a parsed .scion/modes/<name>.yaml file. The mode's name is the
// filename stem; there is no name field in the YAML itself.
type Mode struct {
	Description string   `yaml:"description"`
	Extends     string   `yaml:"extends,omitempty"`
	Skills      []string `yaml:"skills"`
}

// loadModeFromFS reads <name>.yaml from fsys and parses it.
func loadModeFromFS(fsys fs.FS, name string) (*Mode, error) {
	data, err := fs.ReadFile(fsys, name+".yaml")
	if err != nil {
		return nil, fmt.Errorf("mode %q: %w", name, err)
	}
	var m Mode
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("mode %q: parse: %w", name, err)
	}
	return &m, nil
}

// ResolveSkillsFromFS resolves the given mode name into an ordered, deduped
// skill name list. extends chains are walked recursively; first occurrence
// of a duplicate name wins. Cycles are detected and rejected.
func ResolveSkillsFromFS(fsys fs.FS, name string) ([]string, error) {
	visited := map[string]bool{}
	var stack []string
	return resolveRecursive(fsys, name, visited, stack)
}

func resolveRecursive(fsys fs.FS, name string, visited map[string]bool, stack []string) ([]string, error) {
	if visited[name] {
		return nil, fmt.Errorf("mode %q: cycle detected in extends chain: %s -> %s",
			stack[0], joinChain(stack), name)
	}
	visited[name] = true
	stack = append(stack, name)

	m, err := loadModeFromFS(fsys, name)
	if err != nil {
		return nil, err
	}

	var skills []string
	if m.Extends != "" {
		parentSkills, err := resolveRecursive(fsys, m.Extends, visited, stack)
		if err != nil {
			return nil, err
		}
		skills = append(skills, parentSkills...)
	}
	skills = append(skills, m.Skills...)
	return dedupePreserveFirst(skills), nil
}

func dedupePreserveFirst(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func joinChain(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	out := stack[0]
	for _, s := range stack[1:] {
		out += " -> " + s
	}
	return out
}
