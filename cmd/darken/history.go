// Package main — `darken history` reads .scion/audit.jsonl and prints
// a tabular summary or raw JSON. Filters: --last N (most-recent N),
// --since DUR (Go duration), --format text|json.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// auditEntry mirrors docs/AUDIT_LOG_SCHEMA.md. Fields are loose
// because we tolerate unknown payload shapes.
type auditEntry struct {
	Timestamp  string                 `json:"timestamp"`
	DecisionID string                 `json:"decision_id"`
	Harness    string                 `json:"harness"`
	Type       string                 `json:"type"`
	Outcome    string                 `json:"outcome"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
}

func runHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	last := fs.Int("last", 0, "show only the most-recent N entries (0 = all)")
	since := fs.String("since", "", "show entries since the given Go duration ago (e.g. '1h', '24h')")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not in an init'd repo: %w", err)
	}
	logPath := filepath.Join(root, ".scion", "audit.jsonl")

	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("audit log read failed at %s: %w", logPath, err)
	}
	defer f.Close()

	var entries []auditEntry
	var rawLines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e auditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			fmt.Fprintf(os.Stderr, "history: skipping malformed entry: %v\n", err)
			continue
		}
		entries = append(entries, e)
		rawLines = append(rawLines, line)
	}

	// Apply --since filter.
	if *since != "" {
		dur, err := time.ParseDuration(*since)
		if err != nil {
			return fmt.Errorf("--since: invalid duration %q: %w", *since, err)
		}
		cutoff := time.Now().Add(-dur)
		var filtered []auditEntry
		var filteredRaw []string
		for i, e := range entries {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err != nil {
				continue // skip entries with unparseable timestamps
			}
			if t.After(cutoff) {
				filtered = append(filtered, e)
				filteredRaw = append(filteredRaw, rawLines[i])
			}
		}
		entries = filtered
		rawLines = filteredRaw
	}

	// Apply --last filter (after --since).
	if *last > 0 && len(entries) > *last {
		entries = entries[len(entries)-*last:]
		rawLines = rawLines[len(rawLines)-*last:]
	}

	if len(entries) == 0 {
		fmt.Println("no audit entries")
		return nil
	}

	switch *format {
	case "json":
		for _, line := range rawLines {
			fmt.Println(line)
		}
	case "text":
		fmt.Printf("%-21s  %-14s  %-9s  %-10s  %s\n", "TIMESTAMP", "HARNESS", "TYPE", "OUTCOME", "DETAIL")
		for _, e := range entries {
			detail := summarizePayload(e.Type, e.Payload)
			fmt.Printf("%-21s  %-14s  %-9s  %-10s  %s\n", e.Timestamp, e.Harness, e.Type, e.Outcome, detail)
		}
	default:
		return fmt.Errorf("--format: must be text or json, got %q", *format)
	}

	return nil
}

// summarizePayload returns a short string describing the most-relevant
// payload field for the given decision type. Best-effort; payload
// structure isn't strictly enforced (per AUDIT_LOG_SCHEMA stability
// note). Unknown types fall through to JSON-marshalled payload.
func summarizePayload(decisionType string, payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	switch decisionType {
	case "route":
		if tier, ok := payload["tier"].(string); ok {
			return "tier=" + tier
		}
	case "dispatch":
		role, _ := payload["target_role"].(string)
		name, _ := payload["agent_name"].(string)
		if role != "" && name != "" {
			return name + " <- " + role
		}
	case "escalate":
		if axis, ok := payload["axis"].(string); ok {
			return "axis=" + axis
		}
	case "ratify":
		if axis, ok := payload["axis"].(string); ok {
			return "axis=" + axis
		}
	case "apply":
		if id, ok := payload["recommendation_id"].(string); ok {
			return id
		}
	}
	// Fallback: compact JSON.
	if b, err := json.Marshal(payload); err == nil {
		return string(b)
	}
	return ""
}
