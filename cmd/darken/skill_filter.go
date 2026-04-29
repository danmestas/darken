package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// filterSkillsForRole removes subdirectories under dir whose SKILL.md
// frontmatter has a non-empty roles list that does not include role.
// Directories without SKILL.md or without a roles field are kept:
// unlocked skills are visible to all roles (backward compatible).
// A non-existent dir is silently skipped.
func filterSkillsForRole(dir, role string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("filterSkillsForRole: read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		visible, ferr := skillVisibleToRole(skillDir, role)
		if ferr != nil {
			// Fail closed: SKILL.md exists but is malformed or unreadable.
			// Remove the skill rather than granting inadvertent visibility.
			_ = os.RemoveAll(skillDir) // best-effort; ignore remove error
			continue
		}
		if !visible {
			if rerr := os.RemoveAll(skillDir); rerr != nil {
				return fmt.Errorf("filterSkillsForRole: remove %s: %w", skillDir, rerr)
			}
		}
	}
	return nil
}

// skillVisibleToRole reports whether the skill directory is visible to role.
// A skill is visible when its SKILL.md has no roles field, an empty roles
// list, or lists role in its roles field.
//
// Non-skill directories (no SKILL.md) are always visible.
// Any other I/O or parse error is returned to the caller.
func skillVisibleToRole(dir, role string) (bool, error) {
	path := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true, nil // no SKILL.md -> non-skill dir, keep
	}
	meta, err := loadSkillMetadata(path)
	if err != nil {
		return false, err
	}
	if !meta.HasRoles || len(meta.Roles) == 0 {
		return true, nil // no roles constraint -> visible to all
	}
	for _, r := range meta.Roles {
		if r == role {
			return true, nil
		}
	}
	return false, nil
}

// parseFrontmatterRoles reads YAML front matter from r and returns the
// roles list, whether a roles key was found, and any scan error.
//
// Supported forms:
//
//	roles:
//	  - foo
//	  - bar
//
//	roles: [foo, bar]
//
//	roles: []
func parseFrontmatterRoles(r io.Reader) (roles []string, hasRoles bool, err error) {
	scanner := bufio.NewScanner(r)
	inFrontmatter := false
	inRolesList := false

	for scanner.Scan() {
		line := scanner.Text()
		if !inFrontmatter {
			if line == "---" {
				inFrontmatter = true
			}
			continue
		}
		if line == "---" {
			break // end of front matter
		}
		if inRolesList {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				roles = append(roles, strings.TrimPrefix(trimmed, "- "))
				continue
			}
			inRolesList = false // non-list line ends the block
		}
		if strings.HasPrefix(line, "roles:") {
			hasRoles = true
			rest := strings.TrimSpace(strings.TrimPrefix(line, "roles:"))
			if rest == "" {
				inRolesList = true
				continue
			}
			// Inline form: roles: [foo, bar] or roles: []
			rest = strings.Trim(rest, "[]")
			rest = strings.TrimSpace(rest)
			if rest == "" {
				continue // empty list
			}
			for _, item := range strings.Split(rest, ",") {
				if v := strings.TrimSpace(item); v != "" {
					roles = append(roles, v)
				}
			}
		}
	}
	return roles, hasRoles, scanner.Err()
}
