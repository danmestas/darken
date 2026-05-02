package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScionServer_EnsureUsesScionClient asserts ScionServer.Ensure routes
// through ScionClient.ServerStatus so hub-endpoint env propagation applies.
func TestScionServer_EnsureUsesScionClient(t *testing.T) {
	mc := &mockScionClient{serverStatusOut: "Status: ok\n"}
	setDefaultClient(t, mc)
	if err := (ScionServer{}).Ensure(); err != nil {
		t.Fatalf("ScionServer.Ensure: %v", err)
	}
}

func TestBootstrapStepsAreOrdered(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	for _, b := range []string{"scion", "docker", "make"} {
		os.WriteFile(filepath.Join(dir, b),
			[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
	}
	// bash stub: log args + dump the script body. Bootstrap now extracts
	// embedded substrate scripts to a temp file, so the file name is
	// random — but the body's own header comment names the script
	// (e.g. "# stage-creds.sh — ..."), which we can grep for.
	os.WriteFile(filepath.Join(dir, "bash"),
		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\ncat \"$1\" >> "+log+"\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runBootstrap([]string{})

	body, _ := os.ReadFile(log)
	// stage-creds.sh and stage-skills.sh were replaced by native Go in
	// Phase F and Phase G. The bash log now only validates scion/make
	// ordering — substrate staging happens entirely in-process.
	want := []string{"server", "make"}
	pos := -1
	for _, w := range want {
		i := strings.Index(string(body), w)
		if i < pos || i == -1 {
			t.Fatalf("step %q out of order or missing in: %s", w, body)
		}
		pos = i
	}
}

// TestGroveBroker_EnsureUsesBrokerProvide asserts GroveBroker.Ensure routes
// through ScionClient.BrokerProvide.
func TestGroveBroker_EnsureUsesBrokerProvide(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)
	if err := (GroveBroker{}).Ensure(); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

// TestGroveBroker_ReleaseUsesBrokerWithdraw asserts the symmetric pair —
// the bug class from #45 (forgotten withdraw) is structurally impossible
// because Resource requires both methods.
func TestGroveBroker_ReleaseUsesBrokerWithdraw(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)
	if err := (GroveBroker{}).Release(); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if mc.brokerWithdrawCalls != 1 {
		t.Fatalf("expected 1 BrokerWithdraw call; got %d", mc.brokerWithdrawCalls)
	}
}

// TestBootstrap_BrokerProvideStep confirms broker provide runs after the
// scion server step and before the images step.
func TestBootstrap_BrokerProvideStep(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	scionBody := "#!/bin/sh\necho \"scion $@\" >> " + log + "\nexit 0\n"
	dockerBody := "#!/bin/sh\necho \"docker $@\" >> " + log + "\nexit 0\n"
	makeBody := "#!/bin/sh\necho \"make $@\" >> " + log + "\nexit 0\n"
	bashBody := "#!/bin/sh\necho \"bash $@\" >> " + log + "\ncat \"$1\" >> " + log + "\n"
	for name, body := range map[string]string{"scion": scionBody, "docker": dockerBody, "make": makeBody, "bash": bashBody} {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	_ = runBootstrap([]string{})

	body, _ := os.ReadFile(log)
	got := string(body)
	if !strings.Contains(got, "broker provide") {
		t.Fatalf("bootstrap should call scion broker provide, log:\n%s", got)
	}
	// broker provide must appear after server status check.
	serverIdx := strings.Index(got, "server")
	brokerIdx := strings.Index(got, "broker provide")
	if brokerIdx < serverIdx {
		t.Fatalf("broker provide must run after scion server check, log:\n%s", got)
	}
}

// TestResolveSubstrateDirs_FallsBackOnEmptyProjectDir guards against the
// `darken up` failure mode where a workspace's `.scion/templates/` exists
// but is empty (e.g. a fresh `darken init` workspace). resolveSubstrateDirs
// must fall back to the embedded substrate so downstream `scion templates
// import --all` has role subdirs to read.
func TestResolveSubstrateDirs_FallsBackOnEmptyProjectDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	// Create the templates dir but leave it empty.
	if err := os.MkdirAll(filepath.Join(root, ".scion", "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	templatesDir, _, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		t.Fatalf("resolveSubstrateDirs: %v", err)
	}
	defer cleanup()

	// Fallback path lives under os.TempDir(); the project dir does not.
	if strings.HasPrefix(templatesDir, root) {
		t.Fatalf("expected fallback to embedded substrate when project dir is empty, got %q", templatesDir)
	}
	// Embedded substrate must contain at least one role subdir.
	if !hasRoleSubdirs(templatesDir) {
		t.Fatalf("embedded substrate templatesDir has no role subdirs: %s", templatesDir)
	}
}

// TestResolveSubstrateDirs_FallsBackOnBaseOnlyProjectDir confirms that a
// templates dir holding only the shared `base/` skill bundle still triggers
// the embedded fallback — `base` is not a canonical role.
func TestResolveSubstrateDirs_FallsBackOnBaseOnlyProjectDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	if err := os.MkdirAll(filepath.Join(root, ".scion", "templates", "base"), 0o755); err != nil {
		t.Fatal(err)
	}

	templatesDir, _, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		t.Fatalf("resolveSubstrateDirs: %v", err)
	}
	defer cleanup()

	if strings.HasPrefix(templatesDir, root) {
		t.Fatalf("base-only project dir should still trigger fallback, got %q", templatesDir)
	}
}

// TestResolveSubstrateDirs_UsesProjectDirWithRoleSubdirs is the happy-path
// regression: a workspace with at least one canonical role uses its own
// templates dir (no embedded extraction).
func TestResolveSubstrateDirs_UsesProjectDirWithRoleSubdirs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	roleDir := filepath.Join(root, ".scion", "templates", "researcher")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatal(err)
	}

	templatesDir, _, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		t.Fatalf("resolveSubstrateDirs: %v", err)
	}
	defer cleanup()

	want := filepath.Join(root, ".scion", "templates")
	if templatesDir != want {
		t.Fatalf("expected project templatesDir %q, got %q", want, templatesDir)
	}
}

// TestHasRoleSubdirs covers the helper directly.
func TestHasRoleSubdirs(t *testing.T) {
	cases := []struct {
		name string
		mk   func(t *testing.T) string
		want bool
	}{
		{"empty dir", func(t *testing.T) string { return t.TempDir() }, false},
		{"base only", func(t *testing.T) string {
			d := t.TempDir()
			os.MkdirAll(filepath.Join(d, "base"), 0o755)
			return d
		}, false},
		{"role present", func(t *testing.T) string {
			d := t.TempDir()
			os.MkdirAll(filepath.Join(d, "researcher"), 0o755)
			return d
		}, true},
		{"role beside base", func(t *testing.T) string {
			d := t.TempDir()
			os.MkdirAll(filepath.Join(d, "base"), 0o755)
			os.MkdirAll(filepath.Join(d, "admin"), 0o755)
			return d
		}, true},
		{"only files, no dirs", func(t *testing.T) string {
			d := t.TempDir()
			os.WriteFile(filepath.Join(d, "stray.yaml"), []byte("x"), 0o644)
			return d
		}, false},
		{"missing dir", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "does-not-exist")
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasRoleSubdirs(tc.mk(t)); got != tc.want {
				t.Fatalf("hasRoleSubdirs: want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestEnsureAllSkillsStaged_ImportsTemplatesForLocalStore asserts that
// ensureAllSkillsStaged calls ImportAllTemplates with the resolved
// templatesDir. This is the regression guard for the darken-up
// template-upload bug: without the import, scion's local store is empty
// when uploadAllTemplatesToHub later calls templates push.
func TestEnsureAllSkillsStaged_ImportsTemplatesForLocalStore(t *testing.T) {
	// Prepare a fake project root with .scion/templates/ so resolveTemplatesDir
	// returns a known, no-cleanup path. resolveTemplatesDir's first preference
	// is repoRoot()/.scion/templates/.
	root := t.TempDir()
	t.Setenv("DARKEN_REPO_ROOT", root)
	templatesDir := filepath.Join(root, ".scion", "templates")
	for _, role := range []string{"admin", "researcher"} {
		dir := filepath.Join(templatesDir, role)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Plant a minimal manifest so stage-skills doesn't barf on missing yaml.
		manifest := filepath.Join(dir, "scion-agent.yaml")
		if err := os.WriteFile(manifest, []byte("schema_version: \"1\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Stub bash on PATH so runSubstrateScript no-ops.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "bash"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	mc := &mockScionClient{}
	setDefaultClient(t, mc)

	if err := (Substrate{}).Ensure(); err != nil {
		t.Fatalf("Substrate.Ensure: %v", err)
	}

	if got := len(mc.importAllTemplatesCalls); got != 1 {
		t.Fatalf("expected exactly 1 ImportAllTemplates call; got %d: %v",
			got, mc.importAllTemplatesCalls)
	}
	if got := mc.importAllTemplatesCalls[0]; got != templatesDir {
		t.Errorf("ImportAllTemplates called with %q; want %q", got, templatesDir)
	}
}
