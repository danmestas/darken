package main

import (
	"os"
	"os/exec"
)

const defaultHubEndpoint = "http://host.docker.internal:8080"

// scionCmdFn is the package-level function used to construct scion commands.
// It defaults to scionCmd and can be overridden in tests to record invocations.
var scionCmdFn = scionCmd

// scionCmd returns an *exec.Cmd for invoking scion with the given args.
// It centralizes all scion invocations so env propagation is consistent:
//   - SCION_HUB_ENDPOINT is set from DARKEN_HUB_ENDPOINT, defaulting
//     to http://host.docker.internal:8080 (preserves v0.1.15 behavior).
//   - DARKEN_REPO_ROOT is set via the same logic as scriptEnv().
//
// Stdout and stderr are NOT wired — callers set them as needed.
func scionCmd(args []string) *exec.Cmd {
	cmd := exec.Command("scion", args...)
	cmd.Env = scionCmdEnv()
	return cmd
}

// scionCmdEnv returns the environment for scion invocations: the full
// parent env plus SCION_HUB_ENDPOINT and DARKEN_REPO_ROOT overrides.
func scionCmdEnv() []string {
	env := os.Environ()

	hubEndpoint := os.Getenv("DARKEN_HUB_ENDPOINT")
	if hubEndpoint == "" {
		hubEndpoint = defaultHubEndpoint
	}
	env = append(env, "SCION_HUB_ENDPOINT="+hubEndpoint)

	if root, err := repoRoot(); err == nil {
		env = append(env, "DARKEN_REPO_ROOT="+root)
	}
	return env
}
