package staging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Stage copies each skill from canonicalRoot/<name>/ to stageRoot/<name>/.
// canonicalRoot is typically ~/projects/agent-skills/skills.
// stageRoot is typically <repo>/.scion/skills-staging/<harness>.
//
// Stage clears stageRoot first (idempotent), then copies each ref.
// Refuses to follow directory symlinks — scion can't follow them and
// the bash stage-skills.sh script also avoids them by design.
func Stage(refs []string, canonicalRoot, stageRoot string) error {
	if err := os.RemoveAll(stageRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return err
	}
	for _, ref := range refs {
		name := refName(ref)
		src := filepath.Join(canonicalRoot, name)
		dst := filepath.Join(stageRoot, name)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("skill source missing: %s (%w)", src, err)
		}
		if err := copyTree(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// refName returns the basename component of an APM-style skill ref.
// E.g. "danmestas/agent-skills/skills/ousterhout" -> "ousterhout".
func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// copyTree mirrors src into dst. Refuses to follow symlinks.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode())
		case info.Mode()&os.ModeSymlink != 0:
			return errors.New("refusing to follow symlink: " + p)
		default:
			return copyFile(p, target, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
