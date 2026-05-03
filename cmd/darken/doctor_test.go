package main

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danmestas/darken/internal/substrate"
)

// plantOrchestratorSkill writes the embedded orchestrator-mode SKILL.md
// (or a custom body) to <root>/.claude/skills/orchestrator-mode/SKILL.md
// and sets DARKEN_REPO_ROOT to root. Returns the file path.
func plantOrchestratorSkill(t *testing.T, body string) (root, skillPath string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	skillDir := filepath.Join(root, ".claude", "skills", "orchestrator-mode")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath = filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, skillPath
}

// readEmbeddedOrchestratorSkill returns the embedded SKILL.md body —
// useful for the "in-sync" test case.
func readEmbeddedOrchestratorSkill(t *testing.T) string {
	t.Helper()
	body, err := fs.ReadFile(substrate.EmbeddedFS(), "data/skills/orchestrator-mode/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestSubstrateDrift_InSync(t *testing.T) {
	embedded := readEmbeddedOrchestratorSkill(t)
	plantOrchestratorSkill(t, embedded)

	out, err := checkSubstrateDrift()
	if err != nil {
		t.Fatalf("expected nil error on in-sync, got %v", err)
	}
	if !strings.Contains(out, "in sync") {
		t.Fatalf("expected 'in sync', got: %s", out)
	}
}

func TestSubstrateDrift_Diverged(t *testing.T) {
	plantOrchestratorSkill(t, "# orchestrator-mode\n\nwildly different body\n")

	out, err := checkSubstrateDrift()
	if err != nil {
		t.Fatalf("drift should not error (warning only), got %v", err)
	}
	if !strings.Contains(out, "drift") {
		t.Fatalf("expected drift warning, got: %s", out)
	}
	if !strings.Contains(out, "darken upgrade-init") {
		t.Fatalf("expected remediation hint, got: %s", out)
	}
}

func TestSubstrateDrift_SkillMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	// Don't plant the skill.

	out, err := checkSubstrateDrift()
	if err != nil {
		t.Fatalf("missing skill should not error, got %v", err)
	}
	if !strings.Contains(out, "not initialized") && !strings.Contains(out, "darken init") {
		t.Fatalf("expected init hint, got: %s", out)
	}
}

func TestDoctorReportsMissingScion(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "scion")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	report, err := doctorBroad()
	if err == nil {
		t.Fatalf("doctor should report failure when scion is broken")
	}
	if !strings.Contains(report, "scion") {
		t.Fatalf("report must mention scion, got %q", report)
	}
}

func TestDoctorHarnessChecksImageSecretAndStaging(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", dir)

	// Plant stubs so the test doesn't depend on the dev box's real docker / scion state.
	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir)

	hd := filepath.Join(dir, ".scion", "templates", "sme")
	os.MkdirAll(hd, 0o755)
	os.WriteFile(filepath.Join(hd, "scion-agent.yaml"),
		[]byte("default_harness_config: codex\nskills:\n  - danmestas/agent-skills/skills/ousterhout\n"), 0o644)

	report, err := doctorHarness("sme")
	if err == nil {
		t.Fatalf("expected per-harness preflight to fail without staging dir")
	}
	if !strings.Contains(report, "skills-staging") {
		t.Fatalf("report should call out missing staging dir, got %q", report)
	}
}

func TestDoctorHarnessReadsUserOverridesLayer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// User-scope override directory in the same tmp tree (so we can mutate
	// it without touching the real ~/.config/darken/overrides/).
	overrideHome := filepath.Join(tmp, "fakehome")
	t.Setenv("HOME", overrideHome)

	overrideDir := filepath.Join(overrideHome, ".config", "darken", "overrides", ".scion", "templates", "smeoverride")
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overrideDir, "scion-agent.yaml"),
		[]byte("default_harness_config: claude\nskills: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plant docker/scion stubs so doctorHarness's image+secret checks don't
	// hang on real binaries.
	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir)

	report, _ := doctorHarness("smeoverride")
	if !strings.Contains(report, "user layer") {
		t.Fatalf("expected report to identify user layer; got: %q", report)
	}
}

func TestDoctorHarnessPostMortemMapsAuthError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", dir)
	logDir := filepath.Join(dir, ".scion", "agents", "smoke-1")
	os.MkdirAll(logDir, 0o755)
	os.WriteFile(filepath.Join(logDir, "agent.log"),
		[]byte("broker: auth resolution failed: codex\n"), 0o644)

	report := postMortemFor(filepath.Join(logDir, "agent.log"))
	if !strings.Contains(report, "stage-creds.sh") {
		t.Fatalf("post-mortem should map auth error to stage-creds remediation, got %q", report)
	}
}

func TestInitDoctor_PassesOnCompleteInit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)

	// Plant a complete init scaffold (matches what runInit produces).
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".scion"), 0o755)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken orchestrator-mode\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md"),
		[]byte("---\nname: orchestrator-mode\n---\n# body\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness", "SKILL.md"),
		[]byte("---\nname: subagent-to-subharness\n---\n# body\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "settings.local.json"),
		[]byte(`{"statusLine":{"command":"darken status","type":"command"}}`), 0o644)
	os.WriteFile(filepath.Join(tmp, ".gitignore"),
		[]byte(".scion/agents/\n.scion/skills-staging/\n.scion/audit.jsonl\n.claude/worktrees/\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".scion", "audit.jsonl"), []byte(""), 0o644)

	report, err := runInitDoctor(tmp)
	if err != nil {
		t.Fatalf("expected init doctor to pass on complete scaffold; got: %v\nreport:\n%s", err, report)
	}
	for _, want := range []string{"OK", "CLAUDE.md", "orchestrator-mode", "subagent-to-subharness", "statusLine", ".gitignore"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestInitDoctor_FailsOnMissingCLAUDE(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// no CLAUDE.md planted
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when CLAUDE.md missing")
	}
	if !strings.Contains(report, "CLAUDE.md") {
		t.Fatalf("report should call out CLAUDE.md: %s", report)
	}
	if !strings.Contains(report, "darken init") {
		t.Fatalf("report should suggest `darken init`: %s", report)
	}
}

func TestInitDoctor_FailsOnMissingSkills(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)
	// skills NOT scaffolded
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when skills missing")
	}
	if !strings.Contains(report, "orchestrator-mode") {
		t.Fatalf("report should call out orchestrator-mode skill: %s", report)
	}
}

func TestDoctor_InitFlagDispatchesToInitDoctor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	// minimal init: just CLAUDE.md, missing skills → doctor --init should FAIL
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)

	out, err := captureStdout(func() error { return runDoctor([]string{"--init"}) })
	if err == nil {
		t.Fatalf("expected runDoctor --init to fail with missing skills; got nil err\noutput:\n%s", out)
	}
	if !strings.Contains(out, "orchestrator-mode") {
		t.Fatalf("expected output to call out missing orchestrator-mode skill, got: %s", out)
	}
}

func TestInitDoctor_FailsOnMissingStatusLine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", tmp)
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# darken\n"), 0o644)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "orchestrator-mode", "SKILL.md"), []byte("name: orchestrator-mode"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".claude", "skills", "subagent-to-subharness", "SKILL.md"), []byte("name: subagent-to-subharness"), 0o644)
	// no settings.local.json planted
	report, err := runInitDoctor(tmp)
	if err == nil {
		t.Fatalf("expected error when settings.local.json missing")
	}
	if !strings.Contains(report, "statusLine") && !strings.Contains(report, "settings.local.json") {
		t.Fatalf("report should call out missing statusLine config: %s", report)
	}
}

// Note: TestCheckScion_UsesScionClient, TestCheckScionServer_UsesScionClient,
// and TestCheckHubSecrets_UsesScionClient live in scion_client_test.go where
// the mockScionClient infrastructure is defined.

// TestCheckScion_CLIPresentDaemonDown verifies that when scion CLI is on PATH
// but the daemon is unreachable, checkScion passes (CLI present) while the
// broader doctor report emits the daemon-down message, not a CLI-missing message.
func TestCheckScion_CLIPresentDaemonDown(t *testing.T) {
	// Put a scion stub that exits 0 (CLI present) on PATH.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	// checkScion must pass: CLI is present.
	if err := checkScion(); err != nil {
		t.Fatalf("checkScion should pass when scion binary is on PATH, got: %v", err)
	}

	// checkScionServer uses a mock that simulates daemon down.
	setDefaultClient(t, &mockScionClient{
		serverStatusErr: errors.New("connection refused"),
	})
	err := checkScionServer()
	if err == nil {
		t.Fatal("expected checkScionServer to fail when ServerStatus errors")
	}
	if strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("daemon-down error must not say 'not on PATH', got: %v", err)
	}
}

func TestDoctorBroad_FooterMentionsUpOnFailure(t *testing.T) {
	// Stub scion to exit non-zero so checkScion fails.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Stub docker too so checkDocker doesn't fail with a different
	// error pattern that distracts from the test.
	if err := os.WriteFile(filepath.Join(stubDir, "docker"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	report, err := doctorBroad()
	if err == nil {
		t.Fatal("expected doctor to fail when scion is broken")
	}
	if !strings.Contains(report, "darken up") {
		t.Fatalf("failure report should mention `darken up`:\n%s", report)
	}
}

// B3: scion server liveness check tests.

// TestCheckScionServerLiveness_HealthzOK confirms a 200 /healthz response passes.
func TestCheckScionServerLiveness_HealthzOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("DARKEN_HUB_ENDPOINT", srv.URL)

	if err := checkScionServerLiveness(); err != nil {
		t.Fatalf("expected nil when /healthz returns 200, got: %v", err)
	}
}

// TestCheckScionServerLiveness_HealthzFailsFallsThroughToDaemonLine confirms
// that when healthz is unreachable and scion status shows a running daemon,
// the check passes.
func TestCheckScionServerLiveness_HealthzFailsFallsThroughToDaemonLine(t *testing.T) {
	// Close the server immediately so the healthz probe fails fast.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()
	t.Setenv("DARKEN_HUB_ENDPOINT", addr)

	setDefaultClient(t, &mockScionClient{
		serverStatusOut: "Status: ok\nDaemon: running (pid 42)\nGroves: 1\n",
	})

	if err := checkScionServerLiveness(); err != nil {
		t.Fatalf("expected nil when daemon line reports running, got: %v", err)
	}
}

// TestCheckScionServerLiveness_DaemonStopped confirms the check fails when
// the daemon line reports a non-running state.
func TestCheckScionServerLiveness_DaemonStopped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()
	t.Setenv("DARKEN_HUB_ENDPOINT", addr)

	setDefaultClient(t, &mockScionClient{
		serverStatusOut: "Status: stopped\nDaemon: stopped\n",
	})

	if err := checkScionServerLiveness(); err == nil {
		t.Fatal("expected error when daemon line reports stopped")
	}
}

// TestCheckScionServerLiveness_InDoctorBroad confirms doctorBroad includes
// the liveness check in its output.
func TestCheckScionServerLiveness_InDoctorBroad(t *testing.T) {
	stubDir := t.TempDir()
	// Stub scion to fail so doctorBroad terminates early and we can inspect output.
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "docker"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	report, _ := doctorBroad()
	if !strings.Contains(report, "liveness") {
		t.Fatalf("doctorBroad report should mention liveness check, got:\n%s", report)
	}
}

// B7: /etc/hosts host.docker.internal check.

// TestCheckHostsDockerInternal_Present confirms no error when entry exists.
func TestCheckHostsDockerInternal_Present(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hosts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("127.0.0.1\tlocalhost\n")
	f.WriteString("127.0.0.1\thost.docker.internal\n")

	if err := checkHostsDockerInternalFile(f.Name()); err != nil {
		t.Fatalf("expected nil when entry present, got: %v", err)
	}
}

// TestCheckHostsDockerInternal_Missing confirms error with remediation when entry absent.
func TestCheckHostsDockerInternal_Missing(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hosts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("127.0.0.1\tlocalhost\n")
	f.WriteString("::1\t\tlocalhost\n")

	err = checkHostsDockerInternalFile(f.Name())
	if err == nil {
		t.Fatal("expected error when host.docker.internal missing from /etc/hosts")
	}
	if !strings.Contains(err.Error(), "host.docker.internal") {
		t.Fatalf("error should name the missing host, got: %v", err)
	}
}

// TestCheckHostsDockerInternal_CommentedLine confirms a commented-out entry
// is not counted as present.
func TestCheckHostsDockerInternal_CommentedLine(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hosts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("# 127.0.0.1 host.docker.internal\n")

	err = checkHostsDockerInternalFile(f.Name())
	if err == nil {
		t.Fatal("expected error: commented-out entry should not satisfy the check")
	}
}

// TestCheckHostsDockerInternal_RemediationInReport confirms doctorBroad
// includes the remediation text when the entry is missing.
func TestCheckHostsDockerInternal_RemediationInReport(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hosts")
	if err != nil {
		t.Fatal(err)
	}
	// empty hosts file
	f.Close()

	// Override the hosts path via the injectable function.
	orig := hostsFilePath
	t.Cleanup(func() { hostsFilePath = orig })
	hostsFilePath = f.Name()

	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	os.WriteFile(filepath.Join(stubDir, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	report, _ := doctorBroad()
	if !strings.Contains(report, "host.docker.internal") {
		t.Fatalf("report should mention host.docker.internal, got:\n%s", report)
	}
	if !strings.Contains(report, "sudo tee") {
		t.Fatalf("report should include sudo tee remediation, got:\n%s", report)
	}
}

// B5: go-git FUSE sniff-test.

// TestCheckGoGitFUSE_CleanMount confirms no error on a non-FUSE mount.
func TestCheckGoGitFUSE_CleanMount(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "mounts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cwd := t.TempDir()
	// Standard ext4 mount covering cwd.
	f.WriteString("overlay " + cwd + " ext4 rw 0 0\n")
	f.WriteString("proc /proc proc rw 0 0\n")

	if err := checkGoGitFUSEMounts(f.Name(), cwd); err != nil {
		t.Fatalf("expected nil on non-FUSE mount, got: %v", err)
	}
}

// TestCheckGoGitFUSE_GrpcFuseDetected confirms error when cwd is on grpcfuse.
func TestCheckGoGitFUSE_GrpcFuseDetected(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "mounts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cwd := "/workspace"
	f.WriteString("grpcfuse " + cwd + " fuse.grpcfuse rw,nosuid,nodev 0 0\n")

	err = checkGoGitFUSEMounts(f.Name(), cwd)
	if err == nil {
		t.Fatal("expected error on grpcfuse mount")
	}
	if !strings.Contains(err.Error(), "fuse") {
		t.Fatalf("error should mention fuse, got: %v", err)
	}
	if !strings.Contains(err.Error(), "go-git") {
		t.Fatalf("error should mention go-git, got: %v", err)
	}
}

// TestCheckGoGitFUSE_VirtiofsDetected confirms error when cwd is on virtiofs.
func TestCheckGoGitFUSE_VirtiofsDetected(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "mounts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cwd := "/workspace"
	f.WriteString("virtiofs " + cwd + " virtiofs rw 0 0\n")

	err = checkGoGitFUSEMounts(f.Name(), cwd)
	if err == nil {
		t.Fatal("expected error on virtiofs mount")
	}
	if !strings.Contains(err.Error(), "go-git") {
		t.Fatalf("error should mention go-git, got: %v", err)
	}
}

// TestCheckGoGitFUSE_NoMountsFile confirms no error when /proc/mounts absent.
func TestCheckGoGitFUSE_NoMountsFile(t *testing.T) {
	if err := checkGoGitFUSEMounts("/nonexistent/path/mounts", "/workspace"); err != nil {
		t.Fatalf("expected nil when mounts file absent, got: %v", err)
	}
}

// TestCheckGoGitFUSE_RemediationInDoctorBroad confirms doctorBroad includes
// the FUSE check in its output label.
func TestCheckGoGitFUSE_RemediationInDoctorBroad(t *testing.T) {
	stubDir := t.TempDir()
	os.WriteFile(filepath.Join(stubDir, "scion"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	os.WriteFile(filepath.Join(stubDir, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	report, _ := doctorBroad()
	if !strings.Contains(report, "go-git") || !strings.Contains(report, "FUSE") {
		t.Fatalf("doctorBroad should mention go-git FUSE check, got:\n%s", report)
	}
}

// issue #57: scion secret-type enum smoke checks.

// TestCheckScionSecretTypeSupport_PassesWhenEnvInHelp confirms the check
// passes when scion --help output contains "environment".
func TestCheckScionSecretTypeSupport_PassesWhenEnvInHelp(t *testing.T) {
	stubDir := t.TempDir()
	// Stub scion to emit a --help block that includes the accepted enum.
	stub := "#!/bin/sh\necho '--type string   Secret type: environment (default), variable, file'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	if err := checkScionSecretTypeSupport(); err != nil {
		t.Fatalf("expected nil when 'environment' in help, got: %v", err)
	}
}

// TestCheckScionSecretTypeSupport_FailsWhenEnvAbsentFromHelp confirms the
// check fails when scion's --help output does not contain "environment" —
// signalling an enum drift that would break stage-creds.
func TestCheckScionSecretTypeSupport_FailsWhenEnvAbsentFromHelp(t *testing.T) {
	stubDir := t.TempDir()
	// Stub scion to emit a --help block missing "environment" (simulates old
	// scion that only knew --type env, or a future rename).
	stub := "#!/bin/sh\necho '--type string   Secret type: env (default), variable, file'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	if err := checkScionSecretTypeSupport(); err == nil {
		t.Fatal("expected error when 'environment' absent from scion --help, got nil")
	}
}

// TestCheckScionSecretTypeSupport_AbsentScionSkipsGracefully confirms that
// when scion is not on PATH (already caught by scion-cli check), the secret-
// type check returns nil rather than double-reporting the same root cause.
func TestCheckScionSecretTypeSupport_AbsentScionSkipsGracefully(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir: no scion
	if err := checkScionSecretTypeSupport(); err != nil {
		t.Fatalf("expected nil when scion absent (scion-cli check owns that failure), got: %v", err)
	}
}

// TestDoctorBroadChecks_ContainsSecretTypeEnum confirms the new check is
// registered in doctorBroadChecks so it actually runs during `darken doctor`.
func TestDoctorBroadChecks_ContainsSecretTypeEnum(t *testing.T) {
	found := false
	for _, dc := range doctorBroadChecks() {
		if dc.ID == "scion-secret-type-enum" {
			found = true
			if dc.Run == nil {
				t.Error("scion-secret-type-enum check has nil Run")
			}
			if dc.Remediation == "" {
				t.Error("scion-secret-type-enum check has empty Remediation")
			}
			break
		}
	}
	if !found {
		t.Error("doctorBroadChecks does not contain 'scion-secret-type-enum' check")
	}
}
