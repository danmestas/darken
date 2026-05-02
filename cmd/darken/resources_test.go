package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerDaemon_EnsurePassesWhenInfoSucceeds(t *testing.T) {
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "docker"),
		[]byte("#!/bin/sh\necho 'Server: ok'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	if err := (DockerDaemon{}).Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
}

func TestDockerDaemon_EnsureFailsWhenInfoErrors(t *testing.T) {
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "docker"),
		[]byte("#!/bin/sh\necho 'cannot connect' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir)
	err := (DockerDaemon{}).Ensure()
	if err == nil {
		t.Fatal("expected error when docker info fails")
	}
	if !strings.Contains(err.Error(), "cannot connect") {
		t.Fatalf("error should surface docker output: %v", err)
	}
}

func TestDockerDaemon_ReleaseIsNoOp(t *testing.T) {
	if err := (DockerDaemon{}).Release(); err != nil {
		t.Fatalf("Release should never error for shared infra: %v", err)
	}
}

func TestScionCLI_EnsurePassesWhenOnPath(t *testing.T) {
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	if err := (ScionCLI{}).Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
}

func TestScionCLI_EnsureFailsWhenAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir on PATH
	if err := (ScionCLI{}).Ensure(); err == nil {
		t.Fatal("expected error when scion not on PATH")
	}
}

func TestScionCLI_ReleaseIsNoOp(t *testing.T) {
	if err := (ScionCLI{}).Release(); err != nil {
		t.Fatalf("Release should never error for installed binary: %v", err)
	}
}

// fakeResource lets tests exercise the walker without depending on
// stub binaries. ensureErr/releaseErr are returned from the matching
// methods; the call counters let assertions check the order in which
// the walker visits each resource.
type fakeResource struct {
	name        string
	ensureErr   error
	releaseErr  error
	ensureCalls *[]string
	releaseCalls *[]string
}

func (f fakeResource) Name() string { return f.name }
func (f fakeResource) Ensure() error {
	*f.ensureCalls = append(*f.ensureCalls, f.name)
	return f.ensureErr
}
func (f fakeResource) Release() error {
	*f.releaseCalls = append(*f.releaseCalls, f.name)
	return f.releaseErr
}

func TestEnsureAll_VisitsResourcesInOrder(t *testing.T) {
	var seen []string
	rs := []Resource{
		fakeResource{name: "a", ensureCalls: &seen, releaseCalls: &seen},
		fakeResource{name: "b", ensureCalls: &seen, releaseCalls: &seen},
		fakeResource{name: "c", ensureCalls: &seen, releaseCalls: &seen},
	}
	out, err := captureStdout(func() error { return ensureAll(rs) })
	if err != nil {
		t.Fatalf("ensureAll: %v\n%s", err, out)
	}
	if got, want := strings.Join(seen, ","), "a,b,c"; got != want {
		t.Fatalf("Ensure order: want %q, got %q", want, got)
	}
}

func TestEnsureAll_StopsAtFirstError(t *testing.T) {
	var seen []string
	bang := errors.New("bang")
	rs := []Resource{
		fakeResource{name: "a", ensureCalls: &seen, releaseCalls: &seen},
		fakeResource{name: "b", ensureCalls: &seen, releaseCalls: &seen, ensureErr: bang},
		fakeResource{name: "c", ensureCalls: &seen, releaseCalls: &seen},
	}
	out, err := captureStdout(func() error { return ensureAll(rs) })
	if err == nil {
		t.Fatalf("expected error, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), `ensure "b"`) {
		t.Fatalf("error should name failing resource: %v", err)
	}
	if got, want := strings.Join(seen, ","), "a,b"; got != want {
		t.Fatalf("walker should stop at first error: want %q, got %q", want, got)
	}
}

func TestReleaseAll_VisitsResourcesInReverse(t *testing.T) {
	var seen []string
	rs := []Resource{
		fakeResource{name: "a", ensureCalls: &seen, releaseCalls: &seen},
		fakeResource{name: "b", ensureCalls: &seen, releaseCalls: &seen},
		fakeResource{name: "c", ensureCalls: &seen, releaseCalls: &seen},
	}
	_, err := captureStdout(func() error { releaseAll(rs); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(seen, ","), "c,b,a"; got != want {
		t.Fatalf("Release order: want %q, got %q", want, got)
	}
}

func TestReleaseAll_BestEffortContinuesPastErrors(t *testing.T) {
	var seen []string
	bang := errors.New("bang")
	rs := []Resource{
		fakeResource{name: "a", ensureCalls: &seen, releaseCalls: &seen},
		fakeResource{name: "b", ensureCalls: &seen, releaseCalls: &seen, releaseErr: bang},
		fakeResource{name: "c", ensureCalls: &seen, releaseCalls: &seen},
	}
	_, stderr := captureBoth(func() error { releaseAll(rs); return nil })
	if got, want := strings.Join(seen, ","), "c,b,a"; got != want {
		t.Fatalf("walker should visit all resources despite errors: want %q, got %q", want, got)
	}
	if !strings.Contains(stderr, `release "b"`) {
		t.Fatalf("expected error logged for b, got stderr: %q", stderr)
	}
}

// captureBoth returns (stdout, stderr) — stdout is discarded as
// progress prefix noise; stderr carries the best-effort error log.
func captureBoth(fn func() error) (string, string) {
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	_ = fn()
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outBuf := readAll(rOut)
	errBuf := readAll(rErr)
	return outBuf, errBuf
}

func readAll(r *os.File) string {
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return sb.String()
}
