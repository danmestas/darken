package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGrove_ReleaseRoutesThroughCleanGrove is the regression guard for
// the cobra-Usage-dump bug: the pre-Resource implementation invoked
// `scion grove delete -y`, a subcommand that does not exist. Grove.Release
// routes through ScionClient.CleanGrove which maps to `scion clean --yes`.
func TestGrove_ReleaseRoutesThroughCleanGrove(t *testing.T) {
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

	if err := (Grove{}).Release(); err != nil {
		t.Fatalf("Grove.Release: %v", err)
	}
	if got := len(mc.cleanGroveCalls); got != 1 {
		t.Fatalf("expected exactly 1 CleanGrove call; got %d: %v", got, mc.cleanGroveCalls)
	}
	if mc.cleanGroveCalls[0] != root {
		t.Errorf("CleanGrove called with %q; want %q", mc.cleanGroveCalls[0], root)
	}
}

// TestGrove_Release_CleansHubEvenWhenLocalStateMissing is the regression
// test for the bug where Grove.Release silently no-oped when .scion/grove-id
// was absent, leaving the hub grove registration stranded. CleanGrove must
// be called unconditionally so scion clean --yes can decide idempotency.
func TestGrove_Release_CleansHubEvenWhenLocalStateMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	// Intentionally do NOT create .scion/grove-id.

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := (Grove{}).Release(); err != nil {
		t.Fatalf("Grove.Release: %v", err)
	}
	if len(mc.cleanGroveCalls) == 0 {
		t.Fatal("CleanGrove must be called even when .scion/grove-id is absent; got 0 calls")
	}
	if mc.cleanGroveCalls[0] != root {
		t.Errorf("CleanGrove called with %q; want %q", mc.cleanGroveCalls[0], root)
	}
}

// TestGroveBroker_ReleaseRoutesThroughBrokerWithdraw verifies the
// resource calls BrokerWithdraw on Release. (The Ensure side is covered
// in bootstrap_test.go.)
func TestGroveBroker_ReleaseRoutesThroughBrokerWithdraw(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)
	if err := (GroveBroker{}).Release(); err != nil {
		t.Fatalf("GroveBroker.Release: %v", err)
	}
	if mc.brokerWithdrawCalls != 1 {
		t.Fatalf("expected 1 BrokerWithdraw call; got %d", mc.brokerWithdrawCalls)
	}
}

// TestGroveBroker_ReleaseSurfacesErrors confirms that a failure from
// BrokerWithdraw propagates out of Release. The walker (releaseAll) is
// what implements best-effort across resources — individual resources
// should report errors honestly so the walker can log them.
func TestGroveBroker_ReleaseSurfacesErrors(t *testing.T) {
	mc := &mockScionClient{brokerWithdrawErr: errBrokerNotProvided{}}
	setDefaultClient(t, mc)
	if err := (GroveBroker{}).Release(); err == nil {
		t.Fatal("expected error from GroveBroker.Release when underlying call fails")
	}
}

// errBrokerNotProvided is a sentinel error type for the swallowing test —
// using a real error type makes the intent ("simulate scion's not-provided
// failure") clearer than fmt.Errorf in the test.
type errBrokerNotProvided struct{}

func (errBrokerNotProvided) Error() string { return "broker not provided for grove" }
