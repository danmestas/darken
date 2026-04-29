package substrate

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// TestOrchestratorModeSkill_SubharnessDefault asserts the new C4 rule:
// subharness is the DEFAULT dispatch path; Agent is a FALLBACK with
// four named conditions.
func TestOrchestratorModeSkill_SubharnessDefault(t *testing.T) {
	body, err := fs.ReadFile(EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err != nil {
		t.Fatalf("orchestrator-mode skill missing: %v", err)
	}
	s := string(body)

	for _, phrase := range []string{
		"DEFAULT",
		"FALLBACK",
		"substrate unavailable",
		"no role matches",
		"operator override",
		"already-spawned",
	} {
		if !strings.Contains(s, phrase) {
			t.Errorf("orchestrator-mode skill missing required phrase: %q", phrase)
		}
	}
}

// TestSubagentToSubharnessSkill_SubharnessDefault asserts the matching
// rule in the companion skill.
func TestSubagentToSubharnessSkill_SubharnessDefault(t *testing.T) {
	body, err := fs.ReadFile(EmbeddedFS(), "data/skills/subagent-to-subharness/SKILL.md")
	if err != nil {
		t.Fatalf("subagent-to-subharness skill missing: %v", err)
	}
	s := string(body)

	for _, phrase := range []string{
		"DEFAULT",
		"FALLBACK",
		"substrate unavailable",
		"no role matches",
		"operator override",
		"already-spawned",
	} {
		if !strings.Contains(s, phrase) {
			t.Errorf("subagent-to-subharness skill missing required phrase: %q", phrase)
		}
	}
}

// TestWritingPlansSkill_ExistsAndHasBonesRepo asserts Bug 12: the
// vendor writing-plans skill exists in the embed and routes plan output
// through the bones repo CI when BONES_REPO is set.
func TestWritingPlansSkill_ExistsAndHasBonesRepo(t *testing.T) {
	body, err := fs.ReadFile(EmbeddedFS(), "data/skills/writing-plans/SKILL.md")
	if err != nil {
		t.Fatalf("writing-plans vendor skill missing: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "BONES_REPO") {
		t.Error("writing-plans skill must reference BONES_REPO env var")
	}
}

// TestOrchestratorModeSkill_ArchitectureAxis asserts Bug 13: architecture
// is the 4th deferral axis in the escalation gate alongside taste, ethics,
// and reversibility. Both the orchestrator-mode and subagent-to-subharness
// skills must reflect this. Both the canonical .claude/skills/ copies and
// the embedded data/skills/ copies are checked.
func TestOrchestratorModeSkill_ArchitectureAxis(t *testing.T) {
	axes := []string{"taste", "ethics", "reversibility", "architecture"}
	skills := []string{"orchestrator-mode", "subagent-to-subharness"}

	// Embedded copies.
	for _, skill := range skills {
		body, err := fs.ReadFile(EmbeddedFS(), "data/skills/"+skill+"/SKILL.md")
		if err != nil {
			t.Fatalf("embedded %s skill missing: %v", skill, err)
		}
		s := string(body)
		for _, axis := range axes {
			if !strings.Contains(s, axis) {
				t.Errorf("embedded %s skill missing deferral axis %q", skill, axis)
			}
		}
	}

	// Canonical source copies (../../.claude/skills/ relative to package).
	for _, skill := range skills {
		path := "../../.claude/skills/" + skill + "/SKILL.md"
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("canonical .claude/skills/%s/SKILL.md missing: %v", skill, err)
		}
		s := string(body)
		for _, axis := range axes {
			if !strings.Contains(s, axis) {
				t.Errorf("canonical .claude/skills/%s missing deferral axis %q", skill, axis)
			}
		}
	}
}
