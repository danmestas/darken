package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeleteProjectGrove_RoutesThroughCleanGrove is the regression guard
// for the `darken down` cobra-Usage-dump bug: the previous implementation
// invoked `scion grove delete -y`, a subcommand that does not exist.
// CleanGrove maps to `scion clean --yes`, the canonical teardown call.
func TestDeleteProjectGrove_RoutesThroughCleanGrove(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	if err := os.MkdirAll(filepath.Join(root, ".scion"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".scion", "grove-id"), []byte("test-grove\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := deleteProjectGrove(); err != nil {
		t.Fatalf("deleteProjectGrove: %v", err)
	}
	if got := len(mc.cleanGroveCalls); got != 1 {
		t.Fatalf("expected exactly 1 CleanGrove call; got %d: %v", got, mc.cleanGroveCalls)
	}
	if mc.cleanGroveCalls[0] != root {
		t.Errorf("CleanGrove called with %q; want %q", mc.cleanGroveCalls[0], root)
	}
}

// TestDeleteProjectGrove_NoOpWithoutGroveID confirms the step is a clean
// no-op (no client call, no error) when .scion/grove-id is absent — i.e.
// the workspace was never bootstrapped, so there's nothing to clean.
func TestDeleteProjectGrove_NoOpWithoutGroveID(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := deleteProjectGrove(); err != nil {
		t.Fatalf("deleteProjectGrove should no-op without grove-id, got: %v", err)
	}
	if len(mc.cleanGroveCalls) != 0 {
		t.Fatalf("CleanGrove should not be called when grove-id missing; got: %v", mc.cleanGroveCalls)
	}
}

// TestWithdrawBroker_RoutesThroughBrokerWithdraw verifies the new
// teardown step calls BrokerWithdraw exactly once when the project has
// a grove-id (i.e. broker provide ran during darken up).
func TestWithdrawBroker_RoutesThroughBrokerWithdraw(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	if err := os.MkdirAll(filepath.Join(root, ".scion"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".scion", "grove-id"), []byte("g\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := withdrawBroker(); err != nil {
		t.Fatalf("withdrawBroker: %v", err)
	}
	if mc.brokerWithdrawCalls != 1 {
		t.Fatalf("expected 1 BrokerWithdraw call; got %d", mc.brokerWithdrawCalls)
	}
}

// TestWithdrawBroker_SwallowsErrors confirms that a failure from
// BrokerWithdraw (e.g., broker never provided, hub unreachable) does
// NOT propagate up — teardown must be best-effort. The error is logged
// to stderr but withdrawBroker still returns nil.
func TestWithdrawBroker_SwallowsErrors(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	if err := os.MkdirAll(filepath.Join(root, ".scion"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".scion", "grove-id"), []byte("g\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc := &mockScionClient{brokerWithdrawErr: errBrokerNotProvided{}}
	setDefaultClient(t, mc)

	stderr, err := captureStderr(func() error { return withdrawBroker() })
	if err != nil {
		t.Fatalf("withdrawBroker should swallow errors, got: %v", err)
	}
	if !strings.Contains(stderr, "broker withdraw skipped") {
		t.Fatalf("expected skip message on stderr, got: %q", stderr)
	}
}

// TestWithdrawBroker_NoOpWithoutGroveID mirrors the deleteProjectGrove
// no-op test: nothing to withdraw if the workspace never bootstrapped.
func TestWithdrawBroker_NoOpWithoutGroveID(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := withdrawBroker(); err != nil {
		t.Fatal(err)
	}
	if mc.brokerWithdrawCalls != 0 {
		t.Fatal("BrokerWithdraw should not be called without grove-id")
	}
}

// errBrokerNotProvided is a sentinel error type for the swallowing test —
// using a real error type makes the intent ("simulate scion's not-provided
// failure") clearer than fmt.Errorf in the test.
type errBrokerNotProvided struct{}

func (errBrokerNotProvided) Error() string { return "broker not provided for grove" }
