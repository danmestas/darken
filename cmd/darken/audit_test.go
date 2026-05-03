package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeWorkspace creates a temporary darken workspace with a .scion/grove-id
// sentinel and returns the workspace root path.
func makeWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	scionDir := filepath.Join(root, ".scion")
	if err := os.MkdirAll(scionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scionDir, "grove-id"), []byte("test-grove\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// makeWorktree creates a subdirectory simulating a git worktree under a
// darken workspace. The worktree does NOT have its own .scion/grove-id.
func makeWorktree(t *testing.T, workspaceRoot string) string {
	t.Helper()
	wt := filepath.Join(workspaceRoot, ".claude", "worktrees", "test-wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	return wt
}

func TestAuditAppend_WritesToWorkspaceRoot(t *testing.T) {
	root := makeWorkspace(t)
	// Invoke from workspace root — should write to <root>/.scion/audit.jsonl.
	err := runAuditAppendFromDir(root, []string{"dec-001", "dispatch", `{"target_role":"researcher"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	logPath := filepath.Join(root, ".scion", "audit.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("audit.jsonl not created: %v", err)
	}
	if !strings.Contains(string(data), "dec-001") {
		t.Fatalf("expected decision_id in log, got: %s", data)
	}
}

func TestAuditAppend_WritesFromWorktree(t *testing.T) {
	root := makeWorkspace(t)
	wt := makeWorktree(t, root)

	// Simulate cwd == worktree (not workspace root).
	err := runAuditAppendFromDir(wt, []string{"dec-002", "route", `{"tier":"heavy"}`})
	if err != nil {
		t.Fatalf("unexpected error from worktree: %v", err)
	}

	// Entry must be in workspace-root audit log.
	logPath := filepath.Join(root, ".scion", "audit.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("audit.jsonl not at workspace root: %v", err)
	}
	if !strings.Contains(string(data), "dec-002") {
		t.Fatalf("expected entry in workspace-root audit log, got: %s", data)
	}

	// No stray .scion/ created inside the worktree.
	if _, err := os.Stat(filepath.Join(wt, ".scion", "audit.jsonl")); err == nil {
		t.Fatal("stray .scion/audit.jsonl created inside worktree")
	}
}

func TestAuditAppend_CreatesFileLazily(t *testing.T) {
	root := makeWorkspace(t)
	logPath := filepath.Join(root, ".scion", "audit.jsonl")

	// File must not exist yet.
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("audit.jsonl should not exist before first write")
	}

	err := runAuditAppendFromDir(root, []string{"dec-003", "escalate", `{"axis":"reversibility"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("audit.jsonl should exist after first write: %v", err)
	}
}

func TestAuditAppend_ExitsNonZeroOutsideWorkspace(t *testing.T) {
	// A plain temp dir with no .scion/grove-id anywhere above it.
	tmp := t.TempDir()
	err := runAuditAppendFromDir(tmp, []string{"dec-004", "route", `{}`})
	if err == nil {
		t.Fatal("expected non-zero error outside workspace")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("error should mention workspace, got: %v", err)
	}
}

func TestAuditAppend_WritesValidJSON(t *testing.T) {
	root := makeWorkspace(t)
	err := runAuditAppendFromDir(root, []string{"dec-005", "dispatch", `{"target_role":"sme","agent_name":"s1"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	logPath := filepath.Join(root, ".scion", "audit.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	// Each line must be valid JSON.
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		// Must contain required fields.
		for _, field := range []string{"timestamp", "decision_id", "type"} {
			if _, ok := m[field]; !ok {
				t.Fatalf("missing field %q in entry: %s", field, line)
			}
		}
	}
}

func TestAuditAppend_AppendsMulitpleEntries(t *testing.T) {
	root := makeWorkspace(t)
	for i, args := range [][]string{
		{"dec-010", "route", `{"tier":"light"}`},
		{"dec-011", "dispatch", `{"target_role":"researcher"}`},
		{"dec-012", "escalate", `{"axis":"trust"}`},
	} {
		if err := runAuditAppendFromDir(root, args); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	logPath := filepath.Join(root, ".scion", "audit.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 entries, got %d:\n%s", len(lines), data)
	}
}

func TestAuditAppend_DARKEN_WORKSPACE_ROOT_Override(t *testing.T) {
	root := makeWorkspace(t)
	// Set env override — no grove-id walk needed.
	t.Setenv("DARKEN_WORKSPACE_ROOT", root)

	// Invoke from a completely unrelated temp dir.
	unrelated := t.TempDir()
	err := runAuditAppendFromDir(unrelated, []string{"dec-020", "route", `{}`})
	if err != nil {
		t.Fatalf("unexpected error with DARKEN_WORKSPACE_ROOT set: %v", err)
	}
	logPath := filepath.Join(root, ".scion", "audit.jsonl")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected audit.jsonl at DARKEN_WORKSPACE_ROOT: %v", err)
	}
}

func TestAuditAppend_MissingArgs(t *testing.T) {
	root := makeWorkspace(t)
	// Too few args — should return usage error.
	err := runAuditAppendFromDir(root, []string{"dec-030"})
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestAuditAppend_HistoryReadsCanonicalPath(t *testing.T) {
	root := makeWorkspace(t)
	wt := makeWorktree(t, root)

	// Write from the worktree.
	if err := runAuditAppendFromDir(wt, []string{"dec-040", "route", `{"tier":"heavy"}`}); err != nil {
		t.Fatalf("append from worktree: %v", err)
	}

	// darken history must find the entry via the workspace root.
	t.Setenv("DARKEN_WORKSPACE_ROOT", root)
	out, err := captureStdout(func() error { return runHistory(nil) })
	if err != nil {
		t.Fatalf("history error: %v", err)
	}
	// History output shows type column, not decision_id. Verify the entry
	// written from the worktree appears by its type field.
	if !strings.Contains(out, "route") {
		t.Fatalf("history should show entry written from worktree, got:\n%s", out)
	}
}
