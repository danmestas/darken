package main

import (
	"fmt"
	"os"
)

// SkillMetadata holds the parsed front-matter fields from a SKILL.md.
// It is the single typed representation of skill identity used by staging,
// doctor, and future skill commands.
type SkillMetadata struct {
	// Roles is the list of harness roles that may see this skill.
	// Empty slice with HasRoles=true means the skill was explicitly locked
	// to zero roles. Empty slice with HasRoles=false means no roles field
	// was present — the skill is visible to all roles.
	Roles    []string
	HasRoles bool
}

// loadSkillMetadata reads path, parses its YAML front-matter, and returns
// a typed SkillMetadata. Any I/O or parse error is returned directly to the
// caller; there is no silent keep-on-error fallback.
//
// Callers that want "non-skill directory" semantics should handle
// os.IsNotExist separately before calling this function.
func loadSkillMetadata(path string) (SkillMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return SkillMetadata{}, fmt.Errorf("loadSkillMetadata: open %s: %w", path, err)
	}
	defer f.Close()

	roles, hasRoles, err := parseFrontmatterRoles(f)
	if err != nil {
		return SkillMetadata{}, fmt.Errorf("loadSkillMetadata: parse %s: %w", path, err)
	}
	return SkillMetadata{Roles: roles, HasRoles: hasRoles}, nil
}
