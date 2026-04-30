package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUp_NoBonesFlagSkipsChain verifies --no-bones suppresses `bones up`
// even when bones is on PATH.
func TestUp_NoBonesFlagSkipsChain(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	logPath := stubAllBinariesForSetup(t)

	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runUp([]string{"--no-bones"}); err != nil {
		t.Fatalf("runUp: %v", err)
	}

	body, _ := os.ReadFile(logPath)
	if strings.Contains(string(body), "bones up") {
		t.Errorf("--no-bones should suppress `bones up`; log:\n%s", body)
	}
}

// TestUp_ChainsBonesWhenAvailable verifies the default path invokes
// `bones up` once.
func TestUp_ChainsBonesWhenAvailable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	logPath := stubAllBinariesForSetup(t)

	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runUp(nil); err != nil {
		t.Fatalf("runUp: %v", err)
	}

	body, _ := os.ReadFile(logPath)
	if !strings.Contains(string(body), "bones up") {
		t.Errorf("default path must chain `bones up`; log:\n%s", body)
	}
}

// TestSetup_DeprecationNotice verifies the alias prints a deprecation
// line on stderr and still succeeds.
func TestSetup_DeprecationNotice(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	stubAllBinariesForSetup(t)

	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	out, err := captureCombined(func() error { return runSetup([]string{"--no-bones"}) })
	if err != nil {
		t.Fatalf("runSetup: %v", err)
	}
	if !strings.Contains(out, "deprecated") {
		t.Errorf("expected deprecation notice on stderr, got:\n%s", out)
	}
}

// TestDown_NoBonesFlagSkipsChain asserts --no-bones suppresses
// `bones down` invocation during teardown.
func TestDown_NoBonesFlagSkipsChain(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKEN_REPO_ROOT", root)
	logPath := stubAllBinariesForSetup(t)

	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runDown([]string{"--yes", "--no-bones"}); err != nil {
		t.Fatalf("runDown: %v", err)
	}

	body, _ := os.ReadFile(logPath)
	if strings.Contains(string(body), "bones down") {
		t.Errorf("--no-bones should suppress `bones down`; log:\n%s", body)
	}
}
