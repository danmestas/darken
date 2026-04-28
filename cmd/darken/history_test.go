package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleAuditLog = `{"timestamp":"2026-04-28T07:14:32Z","decision_id":"uuid-1","harness":"orchestrator","type":"route","outcome":"ratified","payload":{"tier":"heavy"}}
{"timestamp":"2026-04-28T07:14:35Z","decision_id":"uuid-2","harness":"orchestrator","type":"dispatch","outcome":"ratified","payload":{"target_role":"researcher","agent_name":"r1"}}
{"timestamp":"2026-04-28T07:18:01Z","decision_id":"uuid-3","harness":"orchestrator","type":"escalate","outcome":"escalated","payload":{"axis":"reversibility"}}
`

func plantAuditLog(t *testing.T, body string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	scionDir := filepath.Join(tmp, ".scion")
	os.MkdirAll(scionDir, 0o755)
	logPath := filepath.Join(scionDir, "audit.jsonl")
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return logPath
}

func TestHistory_PrintsTabularSummary(t *testing.T) {
	plantAuditLog(t, sampleAuditLog)

	out, err := captureStdout(func() error { return runHistory(nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"2026-04-28T07:14:32Z",
		"orchestrator",
		"route",
		"ratified",
		"dispatch",
		"escalate",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestHistory_LastNLimit(t *testing.T) {
	plantAuditLog(t, sampleAuditLog)

	out, err := captureStdout(func() error { return runHistory([]string{"--last", "1"}) })
	if err != nil {
		t.Fatal(err)
	}
	// --last 1 should show only the most-recent entry (the escalate).
	if !strings.Contains(out, "escalate") {
		t.Fatalf("expected most-recent entry, got:\n%s", out)
	}
	// Should NOT contain the older entries (route or dispatch).
	if strings.Contains(out, "route") {
		t.Fatalf("--last 1 should exclude older entries, got:\n%s", out)
	}
}

func TestHistory_FormatJSON(t *testing.T) {
	plantAuditLog(t, sampleAuditLog)

	out, err := captureStdout(func() error { return runHistory([]string{"--format", "json"}) })
	if err != nil {
		t.Fatal(err)
	}
	// JSON format should be raw JSONL (each line a parseable object).
	if !strings.Contains(out, `"decision_id":"uuid-1"`) {
		t.Fatalf("expected raw JSONL output, got:\n%s", out)
	}
}

func TestHistory_EmptyLogReturnsZeroEntries(t *testing.T) {
	plantAuditLog(t, "")

	out, err := captureStdout(func() error { return runHistory(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no audit entries") {
		t.Fatalf("expected friendly empty message, got:\n%s", out)
	}
}

func TestHistory_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// Don't plant the file.

	_, err := captureStdout(func() error { return runHistory(nil) })
	if err == nil {
		t.Fatal("expected error when audit log missing")
	}
	if !strings.Contains(err.Error(), "audit") {
		t.Fatalf("error should mention audit log: %v", err)
	}
}
