package substrate

import (
	"io/fs"
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
