package main

import (
	"strings"
	"testing"
)

func TestScionCmd_SetsHubEndpointFromEnv(t *testing.T) {
	t.Setenv("DARKEN_HUB_ENDPOINT", "http://example:9090")

	cmd := scionCmd([]string{"server", "status"})

	found := false
	for _, e := range cmd.Env {
		if e == "SCION_HUB_ENDPOINT=http://example:9090" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("SCION_HUB_ENDPOINT=http://example:9090 not in env; got: %v", cmd.Env)
	}
}

func TestScionCmd_DefaultsHubEndpoint(t *testing.T) {
	t.Setenv("DARKEN_HUB_ENDPOINT", "")

	cmd := scionCmd([]string{"server", "status"})

	found := false
	for _, e := range cmd.Env {
		if e == "SCION_HUB_ENDPOINT=http://host.docker.internal:8080" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("default SCION_HUB_ENDPOINT not set; env: %v", cmd.Env)
	}
}

func TestScionCmd_PropagatesRepoRoot(t *testing.T) {
	cmd := scionCmd([]string{"server", "status"})

	found := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "DARKEN_REPO_ROOT=") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DARKEN_REPO_ROOT not propagated; env: %v", cmd.Env)
	}
}

func TestScionCmd_ArgsForwardedVerbatim(t *testing.T) {
	args := []string{"spawn", "name", "--type", "researcher", "do thing"}
	cmd := scionCmd(args)

	// cmd.Args includes the executable as args[0].
	if len(cmd.Args) != len(args)+1 {
		t.Fatalf("Args length: want %d, got %d: %v", len(args)+1, len(cmd.Args), cmd.Args)
	}
	if cmd.Args[0] != "scion" {
		t.Errorf("Args[0]: want scion, got %q", cmd.Args[0])
	}
	for i, want := range args {
		if cmd.Args[i+1] != want {
			t.Errorf("Args[%d]: want %q, got %q", i+1, want, cmd.Args[i+1])
		}
	}
}
