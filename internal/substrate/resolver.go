// Package substrate resolves substrate files (harness templates, stage
// scripts, Dockerfiles, orchestrator-side skills) through a layered
// override chain. Used by every darken subcommand that needs to read
// substrate state.
//
// Resolution order (first match wins):
//
//  1. Config.FlagOverride        (--substrate-overrides)
//  2. $DARKEN_SUBSTRATE_OVERRIDES (Config.envDir, set in New)
//  3. Config.UserOverrideDir     (~/.config/darken/overrides/)
//  4. Config.ProjectRoot         (CWD; templates only — see comment)
//  5. embedded                   (added in Phase 2; Phase 1 fails through)
//
// Layer 4 is special: only paths starting with ".scion/templates/"
// resolve here. This lets a working repo version-control its own role
// overrides without polluting other override scopes.
//
// Substrate-relative paths must always use forward slashes regardless
// of operating system. The prefix check at layer 4 is a literal string
// match against ".scion/templates/" — callers passing filepath.Join
// results on Windows would slip past it.
package substrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config configures a Resolver. All fields are optional; unset layers
// are skipped during resolution.
type Config struct {
	FlagOverride    string // --substrate-overrides flag value
	UserOverrideDir string // typically ~/.config/darken/overrides/
	ProjectRoot     string // typically CWD (only used for .scion/templates/* paths)
}

// Resolver resolves substrate-relative paths to filesystem files via
// the layered chain documented on the package.
type Resolver struct {
	flagDir    string
	envDir     string
	userDir    string
	projectDir string
}

// New builds a Resolver from the given Config. The
// DARKEN_SUBSTRATE_OVERRIDES env var is snapshotted at construction
// time; subsequent changes are not observed by the returned Resolver.
func New(cfg Config) *Resolver {
	return &Resolver{
		flagDir:    cfg.FlagOverride,
		envDir:     os.Getenv("DARKEN_SUBSTRATE_OVERRIDES"),
		userDir:    cfg.UserOverrideDir,
		projectDir: cfg.ProjectRoot,
	}
}

// embedSentinel marks resolver paths that live in EmbeddedFS rather
// than the host filesystem. ReadFile / Open / Stat / Lookup detect
// this prefix and route accordingly.
const embedSentinel = "embed://"

// ReadFile resolves name through the chain and returns the file contents.
func (r *Resolver) ReadFile(name string) ([]byte, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	if rest, ok := strings.CutPrefix(p, embedSentinel); ok {
		return fs.ReadFile(EmbeddedFS(), "data/"+rest)
	}
	return os.ReadFile(p)
}

// Open resolves name and returns an open file handle. Returns
// fs.File rather than *os.File for forward-compat with Phase 2's
// embedded layer (which yields fs.File but not *os.File). Callers
// must not type-assert to *os.File.
func (r *Resolver) Open(name string) (fs.File, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	if rest, ok := strings.CutPrefix(p, embedSentinel); ok {
		return EmbeddedFS().Open("data/" + rest)
	}
	return os.Open(p)
}

// Stat resolves name and returns its FileInfo.
func (r *Resolver) Stat(name string) (fs.FileInfo, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}
	if rest, ok := strings.CutPrefix(p, embedSentinel); ok {
		return fs.Stat(EmbeddedFS(), "data/"+rest)
	}
	return os.Stat(p)
}

// Lookup reports the absolute path that would be resolved, plus the
// layer name that hit. Returns ("", "", err) on miss. Used by `darken
// doctor` to surface which layer served each role.
func (r *Resolver) Lookup(name string) (path, layer string, err error) {
	for _, c := range r.candidates(name) {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, c.layer, nil
		}
	}
	if _, err := fs.Stat(EmbeddedFS(), "data/"+name); err == nil {
		return embedSentinel + name, "embedded", nil
	}
	return "", "", &MissError{Name: name}
}

func (r *Resolver) resolve(name string) (string, error) {
	for _, c := range r.candidates(name) {
		if _, err := os.Stat(c.path); err == nil {
			return c.path, nil
		}
	}
	// Embedded fallback layer (always present).
	if _, err := fs.Stat(EmbeddedFS(), "data/"+name); err == nil {
		return embedSentinel + name, nil
	}
	return "", &MissError{Name: name}
}

type candidate struct {
	path  string
	layer string
}

func (r *Resolver) candidates(name string) []candidate {
	var out []candidate
	if r.flagDir != "" {
		out = append(out, candidate{filepath.Join(r.flagDir, name), "flag"})
	}
	if r.envDir != "" {
		out = append(out, candidate{filepath.Join(r.envDir, name), "env"})
	}
	if r.userDir != "" {
		out = append(out, candidate{filepath.Join(r.userDir, name), "user"})
	}
	if r.projectDir != "" && strings.HasPrefix(name, ".scion/templates/") {
		out = append(out, candidate{filepath.Join(r.projectDir, name), "project"})
	}
	return out
}

// MissError indicates the resolver could not find a file in any layer.
// Phase 2 will catch this and fall through to the embedded layer.
type MissError struct {
	Name string
}

func (e *MissError) Error() string {
	return fmt.Sprintf("substrate: %s not found in any override layer", e.Name)
}

// IsMiss reports whether err is a MissError (helpful for Phase 2's
// embedded fallback).
func IsMiss(err error) bool {
	var m *MissError
	return errors.As(err, &m)
}
