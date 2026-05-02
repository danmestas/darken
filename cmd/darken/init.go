package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// minBonesVersion is the lowest bones release that omits the legacy ASCII
// banner from `bones init` output. Older versions still work but produce
// noisy output during `darken up`. Bumped by hand when bones evolves.
const minBonesVersion = "0.6.2"

func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "print actions without executing")
	force := flags.Bool("force", false, "overwrite existing CLAUDE.md")
	refresh := flags.Bool("refresh", false, "re-scaffold skills/statusLine/.gitignore without clobbering CLAUDE.md (use --force with --refresh to also regenerate CLAUDE.md)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if err := verifyInitPrereqs(); err != nil {
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

	arts := initArtifacts(target)

	// Decision: should we (re)write CLAUDE.md?
	claudePath := filepath.Join(target, "CLAUDE.md")
	_, claudeExists := statResult(claudePath)
	var writeCLAUDE bool
	switch {
	case *refresh && *force:
		writeCLAUDE = true
	case *refresh:
		writeCLAUDE = false
	case *force:
		writeCLAUDE = true
	case !claudeExists:
		writeCLAUDE = true
	default:
		writeCLAUDE = false
	}

	if *dryRun {
		for _, art := range arts {
			dst := filepath.Join(target, art.RelPath)
			if art.RelPath == "CLAUDE.md" {
				if writeCLAUDE {
					fmt.Printf("would create %s\n", dst)
				} else {
					fmt.Printf("would skip %s (already exists; use --force to overwrite)\n", dst)
				}
				continue
			}
			fmt.Printf("would write %s\n", dst)
		}
		return nil
	}

	// Write each artifact. CLAUDE.md is critical (hard fail); other
	// artifacts are best-effort (log + continue) — matches the
	// pre-refactor contract.
	for _, art := range arts {
		if err := writeArtifact(target, art, writeCLAUDE, *refresh); err != nil {
			if art.RelPath == "CLAUDE.md" {
				return fmt.Errorf("write CLAUDE.md: %w", err)
			}
			fmt.Fprintf(os.Stderr, "init: %s: %v\n", art.RelPath, err)
		}
	}

	// Persist the manifest after all artifacts are written. Best-effort:
	// a manifest write failure shouldn't abort init — uninstall will fall
	// back to comparing against the binary's current Body() output.
	if err := writeInitManifest(target, arts); err != nil {
		fmt.Fprintf(os.Stderr, "init: manifest write failed: %v\n", err)
	}

	// bones init (unchanged)
	if err := runBonesInit(target); err != nil {
		fmt.Fprintf(os.Stderr, "init: bones init failed: %v\n", err)
	} else if _, err := exec.LookPath("bones"); err == nil {
		fmt.Println("ran `bones init` for workspace bootstrap")
	}

	return nil
}

// writeArtifact dispatches on art.Kind to write a file or append the
// gitignore-lines block. Idempotent for gitignore-lines (skips lines
// already present).
func writeArtifact(target string, art artifact, writeCLAUDE, refresh bool) error {
	dst := filepath.Join(target, art.RelPath)
	switch art.Kind {
	case "file":
		if art.RelPath == "CLAUDE.md" {
			if !writeCLAUDE {
				if _, exists := statResult(dst); exists {
					if refresh {
						fmt.Printf("preserved %s (use --refresh --force to regenerate)\n", dst)
					} else {
						fmt.Printf("skipped %s (already exists; use --force to overwrite)\n", dst)
					}
				}
				return nil
			}
			body, err := art.Body()
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, body, 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", dst)
			return nil
		}
		if art.RelPath == ".claude/settings.local.json" {
			// Don't clobber existing settings (operator may have added other keys).
			if _, exists := statResult(dst); exists {
				return nil
			}
		}
		body, err := art.Body()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return err
		}
		fmt.Printf("scaffolded %s\n", art.RelPath)
		return nil

	case "gitignore-lines":
		// Append only lines not already present (idempotent).
		var existing []byte
		if b, err := os.ReadFile(dst); err == nil {
			existing = b
		}
		var add []string
		for _, line := range gitignoreLines {
			if !strings.Contains(string(existing), line) {
				add = append(add, line)
			}
		}
		if len(add) == 0 {
			return nil
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			f.WriteString("\n")
		}
		for _, line := range add {
			f.WriteString(line + "\n")
		}
		fmt.Println("appended darken entries to .gitignore")
		return nil

	default:
		return fmt.Errorf("unknown artifact kind: %s", art.Kind)
	}
}

// statResult is a tiny helper: reports (info, exists) without an error
// for the caller to handle. Existence is the only signal we need at
// these call sites.
func statResult(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}

// runBonesInit shells out to `bones init` in the target dir if bones is
// on PATH. Soft-fail: bones being missing is not fatal — operator
// without bones still gets a usable orchestrator session.
//
// If bones exits non-zero but its stderr contains "already initialized",
// the workspace is already bootstrapped and the call is treated as a
// clean no-op — no error is returned and no warning is emitted.
func runBonesInit(targetDir string) error {
	if _, err := exec.LookPath("bones"); err != nil {
		return nil // soft-fail; bones not on PATH
	}
	warnIfBonesOutdated()
	var stderrBuf strings.Builder
	c := exec.Command("bones", "init")
	c.Dir = targetDir
	c.Stdout = os.Stdout
	c.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if err := c.Run(); err != nil {
		if strings.Contains(stderrBuf.String(), "already initialized") {
			return nil // idempotent no-op
		}
		return err
	}
	return nil
}

// warnIfBonesOutdated emits a stderr nudge when the installed bones is
// older than minBonesVersion. Always best-effort — a parse failure or a
// failed `bones --version` call is silently ignored. The warning names
// the brew tap because that is the canonical install path.
func warnIfBonesOutdated() {
	out, err := exec.Command("bones", "--version").Output()
	if err != nil {
		return
	}
	ver := parseBonesVersion(string(out))
	if ver == "" || !semverLess(ver, minBonesVersion) {
		return
	}
	fmt.Fprintf(os.Stderr,
		"warning: bones %s is older than recommended %s; "+
			"run `brew upgrade danmestas/tap/bones` for cleaner output\n",
		ver, minBonesVersion)
}

// parseBonesVersion extracts the X.Y.Z token from `bones --version`
// output. Expected format: "bones 0.6.2 (commit ..., built ...)".
// Returns the empty string on any parse failure so callers can treat
// "unknown version" as "skip the check".
func parseBonesVersion(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) < 2 || fields[0] != "bones" {
		return ""
	}
	return fields[1]
}

// semverLess reports whether a < b for dotted X.Y.Z version strings.
// Non-numeric components compare as 0. Intentionally minimal: this is
// the warn-if-old path, not a release-blocking semver check.
func semverLess(a, b string) bool {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := range 3 {
		var ai, bi int
		if i < len(aParts) {
			ai, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bi, _ = strconv.Atoi(bParts[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return false
}
