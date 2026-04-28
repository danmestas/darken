// Package main — `darken uninstall-init` removes the project scaffolds
// `darken init` wrote, preserving operator-customized files and the
// .scion/ runtime tree. Reads the per-project manifest at
// .scion/init-manifest.json to compare bytes, falling back to the
// binary's current Body() output if the manifest is missing.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	statePristine   = "PRISTINE"
	stateCustomized = "CUSTOMIZED"
	stateMissing    = "MISSING"
)

// classified pairs an artifact with its disposition for the current run.
type classified struct {
	Art    artifact
	State  string
	Reason string
}

func runUninstallInit(args []string) error {
	fs := flag.NewFlagSet("uninstall-init", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print manifest, exit without prompting")
	yes := fs.Bool("yes", false, "skip interactive prompt")
	force := fs.Bool("force", false, "also remove CUSTOMIZED artifacts")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not in an init'd repo (run from a directory where 'darken init' was run): %w", err)
	}

	if !looksInitd(root) {
		return errors.New("not in an init'd repo (no CLAUDE.md or .claude/ found)")
	}

	manifest, err := readInitManifest(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uninstall-init: manifest read failed (%v); falling back to Body() comparison\n", err)
		manifest = nil
	}

	arts := initArtifacts(root)
	classes := make([]classified, 0, len(arts))
	for _, art := range arts {
		c := classifyArtifact(root, art, manifest)
		classes = append(classes, c)
	}

	printUninstallManifest(root, manifest, classes)

	if *dryRun {
		return nil
	}

	if !*yes {
		ok, err := confirmTTY()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	failed := applyUninstall(root, classes, *force)

	for _, d := range []string{
		filepath.Join(root, ".claude", "skills", "orchestrator-mode"),
		filepath.Join(root, ".claude", "skills", "subagent-to-subharness"),
		filepath.Join(root, ".claude", "skills"),
		filepath.Join(root, ".claude"),
	} {
		_ = os.Remove(d)
	}

	mp := filepath.Join(root, ".scion", "init-manifest.json")
	if err := os.Remove(mp); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "uninstall-init: failed to remove manifest: %v\n", err)
		failed++
	}

	removed, kept := 0, 0
	for _, c := range classes {
		switch c.State {
		case statePristine:
			removed++
		case stateCustomized:
			if !*force {
				kept++
			} else {
				removed++
			}
		}
	}
	fmt.Printf("removed %d files, patched .gitignore, kept %d customized\n", removed, kept)
	if failed > 0 {
		return fmt.Errorf("%d artifacts failed to remove (see stderr)", failed)
	}
	return nil
}

// looksInitd is a cheap heuristic: at least one expected scaffold
// exists at the project root. Avoids the surprise of running
// uninstall-init in a non-darken dir and getting a noisy manifest.
func looksInitd(root string) bool {
	for _, p := range []string{"CLAUDE.md", ".claude"} {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			return true
		}
	}
	return false
}

// classifyArtifact determines PRISTINE / CUSTOMIZED / MISSING for one artifact.
func classifyArtifact(root string, art artifact, manifest *initManifest) classified {
	dst := filepath.Join(root, art.RelPath)
	body, err := os.ReadFile(dst)
	if errors.Is(err, os.ErrNotExist) {
		return classified{Art: art, State: stateMissing, Reason: "not present"}
	}
	if err != nil {
		return classified{Art: art, State: stateCustomized, Reason: fmt.Sprintf("read error: %v", err)}
	}

	switch art.Kind {
	case "file":
		if manifest != nil {
			for _, ma := range manifest.Artifacts {
				if ma.Path == art.RelPath {
					h := sha256.Sum256(body)
					if hex.EncodeToString(h[:]) == ma.SHA256 {
						return classified{Art: art, State: statePristine, Reason: "matches recorded hash"}
					}
					return classified{Art: art, State: stateCustomized, Reason: "differs from recorded hash"}
				}
			}
		}
		want, err := art.Body()
		if err != nil {
			return classified{Art: art, State: stateCustomized, Reason: fmt.Sprintf("Body() error: %v", err)}
		}
		if bytes.Equal(body, want) {
			return classified{Art: art, State: statePristine, Reason: "matches embedded body"}
		}
		return classified{Art: art, State: stateCustomized, Reason: "differs from embedded body"}

	case "gitignore-lines":
		all := true
		for _, line := range gitignoreLines {
			if !lineInBody(body, line) {
				all = false
				break
			}
		}
		if all {
			return classified{Art: art, State: statePristine, Reason: "will strip 7 darken-managed lines"}
		}
		return classified{Art: art, State: stateCustomized, Reason: "darken lines edited or partially removed"}
	}

	return classified{Art: art, State: stateCustomized, Reason: "unknown kind"}
}

// lineInBody returns true if the given line (after TrimSpace) appears
// as a TrimSpace'd line in body.
func lineInBody(body []byte, target string) bool {
	target = strings.TrimSpace(target)
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == target {
			return true
		}
	}
	return false
}

// printUninstallManifest formats the disposition table to stdout.
func printUninstallManifest(root string, manifest *initManifest, classes []classified) {
	fmt.Printf("darken uninstall-init — manifest for %s\n", root)
	if manifest != nil {
		fmt.Printf("init-manifest: .scion/init-manifest.json (darken %s)\n\n", manifest.DarkenVersion)
	} else {
		fmt.Printf("init-manifest: (none — falling back to embedded Body() comparison)\n\n")
	}

	var nRemove, nPatch, nKeep int
	for _, c := range classes {
		var verb string
		switch {
		case c.Art.Kind == "gitignore-lines" && c.State == statePristine:
			verb = "PATCH"
			nPatch++
		case c.State == statePristine:
			verb = "REMOVE"
			nRemove++
		case c.State == stateCustomized:
			verb = "KEEP"
			nKeep++
		case c.State == stateMissing:
			verb = "MISS"
		}
		fmt.Printf("%-7s  %-50s  (%s)\n", verb, c.Art.RelPath, c.Reason)
	}
	fmt.Printf("\n%d files to remove, %d file to patch, %d customized file kept.\n", nRemove, nPatch, nKeep)
}

// confirmTTY prints the prompt and reads a y/N answer from stdin.
// Errors if stdin is not a terminal — callers running non-interactively
// must pass --yes.
func confirmTTY() (bool, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("stdin stat: %w", err)
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return false, errors.New("non-interactive context: pass --yes to confirm")
	}
	fmt.Print("Proceed? [y/N]: ")
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}

// applyUninstall performs the deletions. Returns the count of failed removals.
func applyUninstall(root string, classes []classified, force bool) int {
	failed := 0
	for _, c := range classes {
		shouldRemove := c.State == statePristine || (c.State == stateCustomized && force)
		if !shouldRemove {
			continue
		}
		dst := filepath.Join(root, c.Art.RelPath)
		switch c.Art.Kind {
		case "file":
			if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "uninstall: failed to remove %s: %v\n", dst, err)
				failed++
			}
		case "gitignore-lines":
			if err := stripGitignoreLines(dst, gitignoreLines); err != nil {
				fmt.Fprintf(os.Stderr, "uninstall: failed to patch %s: %v\n", dst, err)
				failed++
			}
		}
	}
	return failed
}

// stripGitignoreLines reads the file, drops any line whose TrimSpace
// equals one of the targets, and atomic-writes the result back.
func stripGitignoreLines(path string, targets []string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	skip := make(map[string]bool, len(targets))
	for _, t := range targets {
		skip[strings.TrimSpace(t)] = true
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if skip[strings.TrimSpace(line)] {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return err
	}
	return nil
}
