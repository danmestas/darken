package staging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageCopiesSkills(t *testing.T) {
	tmp := t.TempDir()
	canon := filepath.Join(tmp, "canon", "skills")
	stage := filepath.Join(tmp, "stage")
	os.MkdirAll(filepath.Join(canon, "ousterhout"), 0o755)
	os.WriteFile(filepath.Join(canon, "ousterhout", "SKILL.md"), []byte("hi"), 0o644)

	if err := Stage([]string{"danmestas/agent-skills/skills/ousterhout"}, canon, stage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "ousterhout", "SKILL.md")); err != nil {
		t.Fatalf("ousterhout not staged: %v", err)
	}
}
