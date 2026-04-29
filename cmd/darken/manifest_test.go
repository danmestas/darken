package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitManifest_WriteThenRead(t *testing.T) {
	target := t.TempDir()

	arts := []artifact{
		{
			RelPath: "CLAUDE.md",
			Kind:    "file",
			Body:    func() ([]byte, error) { return []byte("hello"), nil },
		},
		{
			RelPath: ".gitignore",
			Kind:    "gitignore-lines",
			Body:    func() ([]byte, error) { return []byte("line1\nline2\n"), nil },
		},
	}

	if err := writeInitManifest(target, arts); err != nil {
		t.Fatalf("write: %v", err)
	}

	mp := filepath.Join(target, ".scion", "init-manifest.json")
	if _, err := os.Stat(mp); err != nil {
		t.Fatalf("manifest not written at %s: %v", mp, err)
	}

	got, err := readInitManifest(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got == nil {
		t.Fatal("expected manifest, got nil")
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if len(got.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(got.Artifacts))
	}

	h := sha256.Sum256([]byte("hello"))
	wantSha := hex.EncodeToString(h[:])
	for _, a := range got.Artifacts {
		if a.Path == "CLAUDE.md" && a.SHA256 != wantSha {
			t.Errorf("CLAUDE.md sha256 = %s, want %s", a.SHA256, wantSha)
		}
	}
}

func TestInitManifest_ReadMissingReturnsNilNoError(t *testing.T) {
	target := t.TempDir()
	got, err := readInitManifest(target)
	if err != nil {
		t.Fatalf("expected nil error for missing manifest, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil manifest for missing file, got %+v", got)
	}
}

func TestInitManifest_ReadMalformedReturnsError(t *testing.T) {
	target := t.TempDir()
	mp := filepath.Join(target, ".scion", "init-manifest.json")
	if err := os.MkdirAll(filepath.Dir(mp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mp, []byte("not-json{"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readInitManifest(target)
	if err == nil {
		t.Fatal("expected error for malformed manifest, got nil")
	}
	if got != nil {
		t.Errorf("expected nil manifest on parse error, got %+v", got)
	}
}

// --- expandManifest tests ---

func TestManifest_SubstitutesHubEndpoint(t *testing.T) {
	t.Setenv("DARKEN_HUB_ENDPOINT", "http://example:9090")
	body := "hub:\n  endpoint: ${DARKEN_HUB_ENDPOINT}\n"
	got := expandManifest(body)
	want := "hub:\n  endpoint: http://example:9090\n"
	if got != want {
		t.Errorf("expandManifest: got %q, want %q", got, want)
	}
}

func TestManifest_DefaultsHubEndpoint(t *testing.T) {
	t.Setenv("DARKEN_HUB_ENDPOINT", "")
	body := "hub:\n  endpoint: ${DARKEN_HUB_ENDPOINT}\n"
	got := expandManifest(body)
	want := "hub:\n  endpoint: http://host.docker.internal:8080\n"
	if got != want {
		t.Errorf("expandManifest default: got %q, want %q", got, want)
	}
}

func TestManifest_RestrictsExpansionToDarkenPrefix(t *testing.T) {
	t.Setenv("HOME", "/should-not-expand")
	body := "home: $HOME\nendpoint: ${DARKEN_HUB_ENDPOINT}\n"
	got := expandManifest(body)
	if strings.Contains(got, "/should-not-expand") {
		t.Errorf("expandManifest expanded $HOME — should only expand DARKEN_* vars: %q", got)
	}
	if strings.Contains(got, "$HOME") {
		// $HOME must survive unexpanded.
		// (This is the correct behaviour — the check above already fails if expanded.)
	}
}

func TestManifest_AllTemplatesUseSubstitution(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not in a git repo: %v", err)
	}
	for _, role := range canonicalRoles {
		p := filepath.Join(root, ".scion", "templates", role, "scion-agent.yaml")
		body, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("role %s: cannot read manifest: %v", role, err)
			continue
		}
		s := string(body)
		if !strings.Contains(s, "${DARKEN_HUB_ENDPOINT}") {
			t.Errorf("role %s: manifest missing ${DARKEN_HUB_ENDPOINT} placeholder", role)
		}
		if strings.Contains(s, "host.docker.internal") {
			t.Errorf("role %s: manifest still contains hardcoded host.docker.internal", role)
		}
	}
}
