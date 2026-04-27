package substrate

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbedded_ContainsResearcherManifest(t *testing.T) {
	body, err := fs.ReadFile(EmbeddedFS(), "data/.scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatalf("embedded researcher manifest missing: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("embedded researcher manifest is empty")
	}
}

func TestEmbedded_ContainsAllRoles(t *testing.T) {
	want := []string{
		"orchestrator", "researcher", "designer",
		"planner-t1", "planner-t2", "planner-t3", "planner-t4",
		"tdd-implementer", "verifier", "reviewer",
		"sme", "admin", "darwin",
	}
	for _, role := range want {
		path := "data/.scion/templates/" + role + "/scion-agent.yaml"
		_, err := fs.Stat(EmbeddedFS(), path)
		if err != nil {
			t.Errorf("role %s manifest missing from embed: %v", role, err)
		}
	}
}

func TestEmbedded_ContainsHostSkills(t *testing.T) {
	for _, skill := range []string{"orchestrator-mode", "subagent-to-subharness"} {
		path := "data/skills/" + skill + "/SKILL.md"
		body, err := fs.ReadFile(EmbeddedFS(), path)
		if err != nil {
			t.Errorf("skill %s missing: %v", skill, err)
			continue
		}
		if !strings.Contains(string(body), "name: "+skill) {
			t.Errorf("skill %s body missing frontmatter name", skill)
		}
	}
}

func TestEmbedded_HasStableHash(t *testing.T) {
	h := EmbeddedHash()
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars: %q", len(h), h)
	}
	// Confirm it's stable: second call returns the same value (sync.Once cached).
	if h2 := EmbeddedHash(); h != h2 {
		t.Fatalf("EmbeddedHash returned different values on consecutive calls: %s vs %s", h, h2)
	}
}
