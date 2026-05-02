package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockScionClient is a configurable ScionClient for unit tests.
type mockScionClient struct {
	serverStatusOut    string
	serverStatusErr    error
	secretListOut      string
	secretListErr      error
	startAgentErr      error
	brokerProvideErr   error
	brokerWithdrawErr  error
	pushTemplateErr    error
	groveInitErr       error
	cleanGroveErr      error
	lookAgentOut       []byte
	lookAgentErr       error

	importAllTemplatesErr   error
	importAllTemplatesCalls []string

	startAgentCalls     [][]string
	pushTemplateCalls   []string
	groveInitCalls      int
	groveInitDir        string
	cleanGroveCalls     []string
	brokerWithdrawCalls int
	lookAgentCalls      []string
}

func (m *mockScionClient) ServerStatus() (string, error) {
	return m.serverStatusOut, m.serverStatusErr
}
func (m *mockScionClient) SecretList() (string, error) {
	return m.secretListOut, m.secretListErr
}
func (m *mockScionClient) StartAgent(name string, args []string) error {
	m.startAgentCalls = append(m.startAgentCalls, append([]string{name}, args...))
	return m.startAgentErr
}
func (m *mockScionClient) BrokerProvide() error {
	return m.brokerProvideErr
}
func (m *mockScionClient) PushTemplate(role string) error {
	m.pushTemplateCalls = append(m.pushTemplateCalls, role)
	return m.pushTemplateErr
}
func (m *mockScionClient) ImportAllTemplates(dir string) error {
	m.importAllTemplatesCalls = append(m.importAllTemplatesCalls, dir)
	return m.importAllTemplatesErr
}
func (m *mockScionClient) GroveInit(targetDir string) error {
	m.groveInitCalls++
	m.groveInitDir = targetDir
	return m.groveInitErr
}
func (m *mockScionClient) CleanGrove(targetDir string) error {
	m.cleanGroveCalls = append(m.cleanGroveCalls, targetDir)
	return m.cleanGroveErr
}
func (m *mockScionClient) BrokerWithdraw() error {
	m.brokerWithdrawCalls++
	return m.brokerWithdrawErr
}
func (m *mockScionClient) LookAgent(name string, extraArgs []string) ([]byte, error) {
	m.lookAgentCalls = append(m.lookAgentCalls, name)
	return m.lookAgentOut, m.lookAgentErr
}

// setDefaultClient replaces defaultScionClient for the duration of the test.
func setDefaultClient(t *testing.T, c ScionClient) {
	t.Helper()
	orig := defaultScionClient
	t.Cleanup(func() { defaultScionClient = orig })
	defaultScionClient = c
}

// TestCheckScion_UsesScionClient asserts checkScion passes when scion binary is on PATH.
func TestCheckScion_UsesScionClient(t *testing.T) {
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	if err := checkScion(); err != nil {
		t.Fatalf("checkScion: %v", err)
	}
}

// TestCheckScion_FailsWhenNotOnPath asserts checkScion fails when scion binary
// is absent from PATH.
func TestCheckScion_FailsWhenNotOnPath(t *testing.T) {
	stubDir := t.TempDir()
	// Empty dir: no scion binary present.
	t.Setenv("PATH", stubDir)
	if err := checkScion(); err == nil {
		t.Fatal("expected error when scion not on PATH, got nil")
	}
}

// TestCheckScionServer_UsesScionClient asserts checkScionServer routes through ScionClient.
func TestCheckScionServer_UsesScionClient(t *testing.T) {
	mc := &mockScionClient{serverStatusOut: "Daemon: running\n"}
	setDefaultClient(t, mc)
	if err := checkScionServer(); err != nil {
		t.Fatalf("checkScionServer: %v", err)
	}
}

// TestCheckHubSecrets_UsesScionClient asserts checkHubSecrets routes through ScionClient.SecretList.
func TestCheckHubSecrets_UsesScionClient(t *testing.T) {
	mc := &mockScionClient{secretListOut: "claude_auth\ncodex_auth\n"}
	setDefaultClient(t, mc)
	if err := checkHubSecrets(); err != nil {
		t.Fatalf("checkHubSecrets: %v", err)
	}
}

// TestCheckHubSecrets_FailsOnMissingSecret confirms the check fails when a
// required secret is absent from the SecretList output.
func TestCheckHubSecrets_FailsOnMissingSecret(t *testing.T) {
	mc := &mockScionClient{secretListOut: "some_other_key\n"}
	setDefaultClient(t, mc)
	if err := checkHubSecrets(); err == nil {
		t.Fatal("expected error when secrets missing, got nil")
	}
}

// TestUploadAllTemplatesToHub_UsesPushTemplate asserts uploadAllTemplatesToHub
// calls ScionClient.PushTemplate for each canonical role.
func TestUploadAllTemplatesToHub_UsesPushTemplate(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	out, err := captureStdout(func() error {
		return uploadAllTemplatesToHub()
	})
	if err != nil {
		t.Fatalf("uploadAllTemplatesToHub: %v\nout: %s", err, out)
	}
	if len(mc.pushTemplateCalls) != len(canonicalRoles) {
		t.Fatalf("PushTemplate called %d times; want %d\ncalls: %v",
			len(mc.pushTemplateCalls), len(canonicalRoles), mc.pushTemplateCalls)
	}
	for i, role := range canonicalRoles {
		if mc.pushTemplateCalls[i] != role {
			t.Errorf("PushTemplate[%d]: want %q, got %q", i, role, mc.pushTemplateCalls[i])
		}
	}
}

// TestImportAllTemplates_SuppressesUsageDumpOnKnownError stubs scion to
// emulate the "no importable agent definitions found" failure (which scion
// follows with a full cobra Usage block on stderr). The wrapped client must
// (1) return a clear, operator-friendly error and (2) NOT surface the cobra
// Usage block to stderr.
func TestImportAllTemplates_SuppressesUsageDumpOnKnownError(t *testing.T) {
	stubDir := t.TempDir()
	scionStub := "#!/bin/sh\n" +
		"cat >&2 <<'EOF'\n" +
		"Error: no importable agent definitions found in /tmp/empty\n\n" +
		"Usage:\n" +
		"  scion templates import <source> [flags]\n\n" +
		"Flags:\n" +
		"      --all              Import all discovered agents\n" +
		"EOF\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	c := &execScionClient{}
	stderr, err := captureStderr(func() error {
		return c.ImportAllTemplates("/tmp/empty")
	})
	if err == nil {
		t.Fatal("expected error from ImportAllTemplates, got nil")
	}
	if !strings.Contains(err.Error(), "no agent definitions in /tmp/empty") {
		t.Fatalf("error should name the empty dir, got: %v", err)
	}
	if strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("error should not contain cobra Usage block, got: %v", err)
	}
	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr should not surface cobra Usage block, got: %q", stderr)
	}
	if strings.Contains(stderr, "--all") {
		t.Fatalf("stderr should not surface flag list, got: %q", stderr)
	}
}

// TestImportAllTemplates_SurfacesUnknownStderr confirms that on UNRECOGNIZED
// failures the wrapped client still surfaces scion's stderr — we only filter
// the one known noisy mode, not all errors.
func TestImportAllTemplates_SurfacesUnknownStderr(t *testing.T) {
	stubDir := t.TempDir()
	scionStub := "#!/bin/sh\necho 'Error: connection to broker refused' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(scionStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	c := &execScionClient{}
	stderr, err := captureStderr(func() error {
		return c.ImportAllTemplates("/tmp/whatever")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(stderr, "broker refused") {
		t.Fatalf("unknown errors should pass through to stderr, got: %q", stderr)
	}
}

// TestSpawn_UsesScionClientStartAgent verifies that runSpawn calls
// ScionClient.StartAgent instead of the raw scion binary for the start operation.
func TestSpawn_UsesScionClientStartAgent(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	// Bash stub for the substrate scripts (stage-creds, stage-skills).
	// Scion stub for the readiness poller (scion list --format json).
	stubDir := t.TempDir()
	logFile := filepath.Join(stubDir, "calls.log")
	os.WriteFile(filepath.Join(stubDir, "bash"),
		[]byte("#!/bin/sh\ncat \"$1\" >> "+logFile+"\n"), 0o755)
	// Scion stub: log invocations; respond to list with running phase.
	os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\necho \"$@\" >> "+logFile+"\n"+
			"case \"$1\" in\n"+
			"  list) echo '[{\"name\":\"sa-test\",\"phase\":\"running\"}]'; exit 0 ;;\n"+
			"  *) exit 0 ;;\n"+
			"esac\n"), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	if err := runSpawn([]string{"sa-test", "--type", "researcher", "do work"}); err != nil {
		t.Fatalf("runSpawn: %v", err)
	}
	if len(mc.startAgentCalls) == 0 {
		t.Fatal("StartAgent was not called by runSpawn")
	}
	if mc.startAgentCalls[0][0] != "sa-test" {
		t.Errorf("StartAgent first call: want agent name sa-test, got %v", mc.startAgentCalls[0])
	}
	// Scion binary must not have been invoked for the start operation.
	// (list is OK — that is the poller, not the start.)
	body, _ := os.ReadFile(logFile)
	if strings.Contains(string(body), "start sa-test") {
		t.Errorf("scion binary was invoked for start; StartAgent should own the call: %s", body)
	}
}
