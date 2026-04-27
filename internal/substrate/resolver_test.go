package substrate

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestResolver_FlagOverrideWins(t *testing.T) {
	tmp := t.TempDir()
	flagDir := filepath.Join(tmp, "flag")
	envDir := filepath.Join(tmp, "env")
	userDir := filepath.Join(tmp, "user")

	for _, d := range []string{flagDir, envDir, userDir} {
		os.MkdirAll(filepath.Join(d, ".scion", "templates", "researcher"), 0o755)
	}
	os.WriteFile(filepath.Join(flagDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("FLAG"), 0o644)
	os.WriteFile(filepath.Join(envDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("ENV"), 0o644)
	os.WriteFile(filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("USER"), 0o644)

	t.Setenv("DARKEN_SUBSTRATE_OVERRIDES", envDir)

	r := New(Config{
		FlagOverride:    flagDir,
		UserOverrideDir: userDir,
		ProjectRoot:     "",
	})

	body, err := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "FLAG" {
		t.Fatalf("expected FLAG to win, got %q", string(body))
	}
}

func TestResolver_EnvBeatsUserBeatsProject(t *testing.T) {
	tmp := t.TempDir()
	envDir := filepath.Join(tmp, "env")
	userDir := filepath.Join(tmp, "user")
	projectDir := filepath.Join(tmp, "project")

	for _, d := range []string{envDir, userDir, projectDir} {
		os.MkdirAll(filepath.Join(d, ".scion", "templates", "researcher"), 0o755)
	}
	os.WriteFile(filepath.Join(envDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("ENV"), 0o644)
	os.WriteFile(filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("USER"), 0o644)
	os.WriteFile(filepath.Join(projectDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("PROJECT"), 0o644)

	t.Setenv("DARKEN_SUBSTRATE_OVERRIDES", envDir)

	r := New(Config{
		UserOverrideDir: userDir,
		ProjectRoot:     projectDir,
	})

	body, _ := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if string(body) != "ENV" {
		t.Fatalf("expected ENV to beat user/project, got %q", string(body))
	}
}

func TestResolver_ProjectOnlyForTemplates(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	os.MkdirAll(filepath.Join(projectDir, "scripts"), 0o755)
	os.WriteFile(filepath.Join(projectDir, "scripts", "stage-creds.sh"), []byte("PROJECT"), 0o644)

	r := New(Config{ProjectRoot: projectDir})

	// Non-template files do NOT resolve from project root.
	_, err := r.ReadFile("scripts/stage-creds.sh")
	if err == nil {
		t.Fatalf("expected miss for scripts/* in project root (templates-only), got hit")
	}
}

func TestResolver_MissesAreErrFsExtMissing(t *testing.T) {
	r := New(Config{})
	_, err := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if err == nil {
		t.Fatal("expected error on miss")
	}
	// IsMiss should report true on this error.
	if !IsMiss(err) {
		t.Fatalf("expected IsMiss(err)==true, got false (err=%v)", err)
	}
}

func TestResolver_OpenAndStat(t *testing.T) {
	tmp := t.TempDir()
	userDir := filepath.Join(tmp, "user")
	os.MkdirAll(filepath.Join(userDir, ".scion", "templates", "researcher"), 0o755)
	os.WriteFile(filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), []byte("USER"), 0o644)

	r := New(Config{UserOverrideDir: userDir})
	f, err := r.Open(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	body, _ := io.ReadAll(f)
	if string(body) != "USER" {
		t.Fatalf("Open returned %q", string(body))
	}

	info, err := r.Stat(".scion/templates/researcher/scion-agent.yaml")
	if err != nil || info.Size() != 4 {
		t.Fatalf("Stat returned size=%d err=%v", info.Size(), err)
	}
}
