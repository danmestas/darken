package main

import (
	"os"
	"strings"
	"testing"
)

func TestApplyDryRun(t *testing.T) {
	f, _ := os.CreateTemp("", "rec-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(`session: test
recommendations:
  - id: rec-001
    target_harness: tdd-implementer
    type: skill_add
    rationale: "evidence shows X"
    proposed_change:
      skill: "danmestas/agent-skills/skills/idiomatic-go"
    confidence: 0.9
    reversibility: trivial
`)
	f.Close()

	out, err := captureStdout(func() error {
		return runApply([]string{"--dry-run", f.Name()})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "rec-001") {
		t.Fatalf("dry-run did not show rec-001: %s", out)
	}
	if !strings.Contains(out, "skill_add") {
		t.Fatalf("dry-run did not show type: %s", out)
	}
}
