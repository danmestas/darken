package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdoutModes captures stdout from fn.
func captureStdoutModes(t *testing.T, fn func() error) string {
	t.Helper()
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	if err := fn(); err != nil {
		t.Fatalf("fn: %v", err)
	}
	w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestModesList_PrintsAllModes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	dir := filepath.Join(root, ".scion", "modes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"alpha.yaml": "description: \"A mode\"\nskills: []\n",
		"beta.yaml":  "description: \"B mode\"\nskills:\n  - x\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := captureStdoutModes(t, func() error { return runModesList() })
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Errorf("expected both mode names in output; got:\n%s", out)
	}
	if !strings.Contains(out, "A mode") || !strings.Contains(out, "B mode") {
		t.Errorf("expected both descriptions in output; got:\n%s", out)
	}
}

func TestModesShow_PrintsResolvedSkills(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	dir := filepath.Join(root, ".scion", "modes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"base.yaml":  "description: \"base\"\nskills:\n  - a\n  - b\n",
		"child.yaml": "description: \"child\"\nextends: base\nskills:\n  - c\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := captureStdoutModes(t, func() error { return runModesShow("child") })
	for _, want := range []string{"a", "b", "c", "extends: base"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%s", want, out)
		}
	}
}

func TestModes_UnknownSubcommand(t *testing.T) {
	err := runModes([]string{"foo"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention 'unknown': %v", err)
	}
}
