package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSubstrateScript_ExtractsAndExecs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Provide an env-layer override so the resolver finds the script.
	// (Project layer only resolves .scion/templates/* paths; env layer
	// accepts any path, which matches how a user override directory works.)
	overrides := filepath.Join(tmp, "overrides")
	dir := filepath.Join(overrides, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptBody := "#!/bin/sh\necho hello-from-stub\n"
	scriptPath := filepath.Join(dir, "demo-script.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKEN_SUBSTRATE_OVERRIDES", overrides)

	// runSubstrateScript should find the script via the resolver, write it
	// to a temp file with exec permissions, and run it. The body should
	// flow back to the caller via stdout.
	out, err := runSubstrateScriptCaptured("scripts/demo-script.sh", []string{})
	if err != nil {
		t.Fatalf("runSubstrateScript failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "hello-from-stub") {
		t.Fatalf("expected stub output, got %q", out)
	}
}

func TestRunSubstrateScript_FailsCleanly_OnMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	err := runSubstrateScript("scripts/no-such-script-anywhere.sh", []string{})
	if err == nil {
		t.Fatal("expected error for non-existent script")
	}
}
