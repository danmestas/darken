package staging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillsFromJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "manifest.json")
	os.WriteFile(path, []byte(`{
		"name":"sme","skills":["danmestas/agent-skills/skills/ousterhout","danmestas/agent-skills/skills/hipp"]
	}`), 0o644)
	skills, err := ParseSkillsFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
}
