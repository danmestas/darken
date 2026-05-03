package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// makeGroveTestDir creates a temp dir with a .scion/grove-id file.
// It sets DARKEN_REPO_ROOT to dir so repoRoot() resolves deterministically
// without shelling out to git. Returns dir and its base name (slug).
func makeGroveTestDir(t *testing.T) (dir, slug string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".scion"), 0o755); err != nil {
		t.Fatal(err)
	}
	slug = filepath.Base(dir)
	t.Setenv("DARKEN_REPO_ROOT", dir)
	return dir, slug
}

// TestCheckGroveStatus_PassesWhenOk verifies the check passes when scion
// grove list reports the current project's grove as "ok".
func TestCheckGroveStatus_PassesWhenOk(t *testing.T) {
	dir, slug := makeGroveTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".scion", "grove-id"),
		[]byte("aaaabbbb-0000-0000-0000-000000000001"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{
		groveListJSONOut: `[{"name":"` + slug + `","status":"ok","grove_id":"x","type":"git","agent_count":0}]`,
	}
	setDefaultClient(t, mc)

	if err := checkGroveStatus(); err != nil {
		t.Fatalf("expected nil when grove status is ok, got: %v", err)
	}
}

// TestCheckGroveStatus_FailsWhenOrphaned verifies the check returns an error
// when scion grove list classifies the project's grove as "orphaned".
// This is the primary acceptance test for issue #59.
func TestCheckGroveStatus_FailsWhenOrphaned(t *testing.T) {
	dir, slug := makeGroveTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".scion", "grove-id"),
		[]byte("aaaabbbb-0000-0000-0000-000000000002"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{
		groveListJSONOut: `[{"name":"` + slug + `","status":"orphaned","grove_id":"x","type":"git","agent_count":0}]`,
	}
	setDefaultClient(t, mc)

	err := checkGroveStatus()
	if err == nil {
		t.Fatal("expected error when grove status is orphaned, got nil")
	}
	if !containsAll(err.Error(), []string{slug, "orphaned", "darken up"}) {
		t.Errorf("error message should name slug, status, and remediation; got: %v", err)
	}
}

// TestCheckGroveStatus_SkipsWhenNoGroveID verifies the check is a no-op when
// the project has not been grove-init'd (no .scion/grove-id).
func TestCheckGroveStatus_SkipsWhenNoGroveID(t *testing.T) {
	dir, _ := makeGroveTestDir(t)
	// No grove-id file written.
	_ = dir

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := checkGroveStatus(); err != nil {
		t.Fatalf("expected nil when grove not init'd, got: %v", err)
	}
}

// TestCheckGroveStatus_SkipsWhenNotInRepo verifies the check is a no-op when
// DARKEN_REPO_ROOT is unset and cwd is not inside a git repo.
func TestCheckGroveStatus_SkipsWhenNotInRepo(t *testing.T) {
	// Ensure DARKEN_REPO_ROOT is unset and cwd is a non-repo temp dir.
	t.Setenv("DARKEN_REPO_ROOT", "")
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := checkGroveStatus(); err != nil {
		t.Fatalf("expected nil when not in a git repo, got: %v", err)
	}
}

// TestCheckGroveStatus_SkipsWhenScionUnreachable verifies the check is a no-op
// when scion grove list itself fails (server down — a different check covers that).
func TestCheckGroveStatus_SkipsWhenScionUnreachable(t *testing.T) {
	dir, _ := makeGroveTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".scion", "grove-id"),
		[]byte("aaaabbbb-0000-0000-0000-000000000003"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{
		groveListJSONErr: errors.New("connection refused"),
	}
	setDefaultClient(t, mc)

	if err := checkGroveStatus(); err != nil {
		t.Fatalf("expected nil when scion unreachable, got: %v", err)
	}
}

// TestCheckGroveStatus_SkipsWhenSlugNotInList verifies the check is a no-op when
// the project's slug is not present in scion grove list output (e.g. newly
// init'd grove not yet synced to the list).
func TestCheckGroveStatus_SkipsWhenSlugNotInList(t *testing.T) {
	dir, _ := makeGroveTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".scion", "grove-id"),
		[]byte("aaaabbbb-0000-0000-0000-000000000004"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{
		groveListJSONOut: `[{"name":"other-project","status":"ok","grove_id":"y","type":"git","agent_count":0}]`,
	}
	setDefaultClient(t, mc)

	if err := checkGroveStatus(); err != nil {
		t.Fatalf("expected nil when slug absent from list, got: %v", err)
	}
}

// TestDoctorBroadChecks_IncludesGroveStatusCheck verifies the grove-status
// check is present in the doctorBroadChecks registry with the correct ID
// and severity.
func TestDoctorBroadChecks_IncludesGroveStatusCheck(t *testing.T) {
	var found bool
	for _, dc := range doctorBroadChecks() {
		if dc.ID != "grove-status" {
			continue
		}
		found = true
		if dc.Severity != SeverityFail {
			t.Errorf("grove-status check: want SeverityFail, got %q", dc.Severity)
		}
		if dc.Remediation == "" {
			t.Error("grove-status check: Remediation must not be empty")
		}
		if dc.Run == nil {
			t.Error("grove-status check: Run must not be nil")
		}
		break
	}
	if !found {
		t.Error("grove-status check not found in doctorBroadChecks registry")
	}
}

// containsAll reports whether s contains every needle in needles.
func containsAll(s string, needles []string) bool {
	for _, n := range needles {
		if !contains(s, n) {
			return false
		}
	}
	return true
}

// contains is a simple substring helper (strings.Contains is fine but this
// avoids importing strings in the test file when it's otherwise not needed).
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
