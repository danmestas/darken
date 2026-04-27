package substrate

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// mustWrite is a tiny test helper: t.Fatal on any setup error so a
// fixture-creation failure surfaces clearly instead of cascading into
// a misleading assertion miss later.
func mustWrite(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dir), err)
	}
	if err := os.WriteFile(dir, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", dir, err)
	}
}

func TestResolver_FlagOverrideWins(t *testing.T) {
	tmp := t.TempDir()
	flagDir := filepath.Join(tmp, "flag")
	envDir := filepath.Join(tmp, "env")
	userDir := filepath.Join(tmp, "user")

	mustWrite(t, filepath.Join(flagDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "FLAG")
	mustWrite(t, filepath.Join(envDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "ENV")
	mustWrite(t, filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "USER")

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

	mustWrite(t, filepath.Join(envDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "ENV")
	mustWrite(t, filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "USER")
	mustWrite(t, filepath.Join(projectDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "PROJECT")

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
	mustWrite(t, filepath.Join(projectDir, "scripts", "stage-creds.sh"), "PROJECT")

	r := New(Config{ProjectRoot: projectDir})

	// Non-template files do NOT resolve from project root.
	_, err := r.ReadFile("scripts/stage-creds.sh")
	if err == nil {
		t.Fatalf("expected miss for scripts/* in project root (templates-only), got hit")
	}
}

func TestResolver_MissesReturnMissError(t *testing.T) {
	r := New(Config{})
	_, err := r.ReadFile(".scion/templates/researcher/scion-agent.yaml")
	if err == nil {
		t.Fatal("expected error on miss")
	}
	if !IsMiss(err) {
		t.Fatalf("expected IsMiss(err)==true, got false (err=%v)", err)
	}
}

func TestResolver_OpenAndStat(t *testing.T) {
	tmp := t.TempDir()
	userDir := filepath.Join(tmp, "user")
	mustWrite(t, filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "USER")

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

// TestResolver_LookupReturnsLayerName guards against a regression on
// the layer string ("flag"|"env"|"user"|"project"). darken doctor
// reports it directly to the operator; a silent rename here would
// break the layer-attribution output without any test catching it.
func TestResolver_LookupReturnsLayerName(t *testing.T) {
	tmp := t.TempDir()
	userDir := filepath.Join(tmp, "user")
	mustWrite(t, filepath.Join(userDir, ".scion", "templates", "researcher", "scion-agent.yaml"), "USER")

	r := New(Config{UserOverrideDir: userDir})
	path, layer, err := r.Lookup(".scion/templates/researcher/scion-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if layer != "user" {
		t.Fatalf("expected layer=%q, got %q", "user", layer)
	}
	if path == "" {
		t.Fatal("expected non-empty resolved path")
	}
}
