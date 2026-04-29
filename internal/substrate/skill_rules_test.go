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
