// Package main — `darken audit` manages workspace audit-log entries.
//
// Subcommands:
//
//	append <decision_id> <type> <payload-json>
//	    Write a JSONL entry to <workspace-root>/.scion/audit.jsonl.
//	    Resolves the workspace root by (in priority order):
//	      1. $DARKEN_WORKSPACE_ROOT env var
//	      2. Walking up from cwd until a directory containing .scion/grove-id
//	         is found.
//	    Exits non-zero if no workspace root can be located.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// runAudit dispatches darken audit <subcommand>.
func runAudit(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: darken audit <subcommand>\n  subcommands: append")
	}
	switch args[0] {
	case "append":
		return runAuditAppend(args[1:])
	default:
		return fmt.Errorf("audit: unknown subcommand %q (available: append)", args[0])
	}
}

// runAuditAppend implements `darken audit append <decision_id> <type> <payload-json>`.
func runAuditAppend(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("audit append: cannot determine cwd: %w", err)
	}
	return runAuditAppendFromDir(cwd, args)
}

// runAuditAppendFromDir is the testable core: runs audit-append logic as if
// cwd were dir. Tests call this directly to simulate different working
// directories without actually changing the process cwd.
func runAuditAppendFromDir(dir string, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: darken audit append <decision_id> <type> <payload-json>")
	}
	decisionID := args[0]
	entryType := args[1]
	payloadRaw := args[2]

	// Validate payload is JSON.
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		return fmt.Errorf("audit append: payload-json is not valid JSON: %w", err)
	}

	wsRoot, err := findWorkspaceRoot(dir)
	if err != nil {
		return err
	}

	return writeAuditEntry(wsRoot, decisionID, entryType, payload)
}

// findWorkspaceRoot locates the darken workspace root starting from dir.
// It checks $DARKEN_WORKSPACE_ROOT first, then walks up looking for
// a directory containing .scion/grove-id.
func findWorkspaceRoot(dir string) (string, error) {
	if v := os.Getenv("DARKEN_WORKSPACE_ROOT"); v != "" {
		return v, nil
	}
	return walkToWorkspaceRoot(dir)
}

// walkToWorkspaceRoot walks from dir up to the filesystem root, returning
// the first directory that contains .scion/grove-id.
func walkToWorkspaceRoot(dir string) (string, error) {
	current := dir
	for {
		sentinel := filepath.Join(current, ".scion", "grove-id")
		if _, err := os.Stat(sentinel); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding grove-id.
			break
		}
		current = parent
	}
	return "", fmt.Errorf("audit append: no darken workspace found (no .scion/grove-id above %s); not inside a darken-managed workspace", dir)
}

// writeAuditEntry appends a JSONL entry to <wsRoot>/.scion/audit.jsonl,
// creating the file (and .scion/ directory) lazily on first write.
func writeAuditEntry(wsRoot, decisionID, entryType string, payload map[string]interface{}) error {
	type entry struct {
		Timestamp  string                 `json:"timestamp"`
		DecisionID string                 `json:"decision_id"`
		Harness    string                 `json:"harness,omitempty"`
		Type       string                 `json:"type"`
		Payload    map[string]interface{} `json:"payload,omitempty"`
	}
	e := entry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		DecisionID: decisionID,
		Type:       entryType,
		Payload:    payload,
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit append: marshal failed: %w", err)
	}

	logPath := filepath.Join(wsRoot, ".scion", "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("audit append: cannot create .scion dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("audit append: open failed: %w", err)
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	if err != nil {
		return fmt.Errorf("audit append: write failed: %w", err)
	}
	return nil
}
