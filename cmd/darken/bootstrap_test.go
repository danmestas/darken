package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureScionServer_UsesScionClient asserts ensureScionServer routes
// through ScionClient.ServerStatus so hub-endpoint env propagation applies.
func TestEnsureScionServer_UsesScionClient(t *testing.T) {
	mc := &mockScionClient{serverStatusOut: "Status: ok\n"}
	setDefaultClient(t, mc)
	if err := ensureScionServer(); err != nil {
		t.Fatalf("ensureScionServer: %v", err)
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
	want := []string{"server", "make", "stage-creds.sh", "stage-skills.sh"}
	pos := -1
	for _, w := range want {
		i := strings.Index(string(body), w)
		if i < pos || i == -1 {
			t.Fatalf("step %q out of order or missing in: %s", w, body)
		}
		pos = i
	}
}

// TestEnsureBrokerProvide_UsesScionClient asserts ensureBrokerProvide routes
// through ScionClient.BrokerProvide.
func TestEnsureBrokerProvide_UsesScionClient(t *testing.T) {
	mc := &mockScionClient{}
	setDefaultClient(t, mc)
	if err := ensureBrokerProvide(); err != nil {
		t.Fatalf("expected nil, got: %v", err)
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
