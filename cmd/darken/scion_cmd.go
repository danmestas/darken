package main

import (
	"os"
	"os/exec"
)

const defaultHubEndpoint = "http://host.docker.internal:8080"

// skillsCanonical returns the canonical skills source directory.
// It reads DARKEN_SKILLS_CANONICAL from the environment, falling back
// to ~/projects/agent-config/skills (the standard agent-config layout).
func skillsCanonical() string {
	if v := os.Getenv("DARKEN_SKILLS_CANONICAL"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return home + "/projects/agent-config/skills"
}

// scionCmd returns an *exec.Cmd for invoking scion with the given args.
// Used by execScionClient (via scionCmdWithEnv) and by bootstrap's
// server-start path, which is the one operation not represented on
// ScionClient (start-server is a bootstrap-only imperative).
func scionCmd(args []string) *exec.Cmd {
	cmd := exec.Command("scion", args...)
	cmd.Env = scionCmdEnv()
	return cmd
}

// scionCmdEnv returns the environment for scion invocations: the full
// parent env plus SCION_HUB_ENDPOINT, DARKEN_REPO_ROOT, and
// DARKEN_SKILLS_CANONICAL overrides.
func scionCmdEnv() []string {
	env := os.Environ()

	hubEndpoint := os.Getenv("DARKEN_HUB_ENDPOINT")
	if hubEndpoint == "" {
		hubEndpoint = defaultHubEndpoint
	}
	env = envOverride(env, "SCION_HUB_ENDPOINT", hubEndpoint)

	if root, err := repoRoot(); err == nil {
		env = envOverride(env, "DARKEN_REPO_ROOT", root)
	}

	env = envOverride(env, "DARKEN_SKILLS_CANONICAL", skillsCanonical())
	return env
}

// envOverride sets key=value in env, replacing an existing entry if
// present or appending if not. This avoids duplicate keys that arise
// when the parent env already contains a key we want to override.
func envOverride(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
