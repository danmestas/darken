package main

import (
	"fmt"
	"strings"
)

// knownBackends is the exhaustive set of valid backend values in scion-agent.yaml.
var knownBackends = map[string]bool{
	"claude": true,
	"codex":  true,
	"pi":     true,
	"gemini": true,
}

// HarnessManifest is the typed representation of a scion-agent.yaml manifest.
// It is the single source of truth for backend and skills metadata, shared by
// doctorHarness, staging, setup upload, and spawn.
type HarnessManifest struct {
	// Backend is one of: claude, codex, pi, gemini.
	Backend string
	// Skills is the ordered list of skill refs declared in the manifest.
	Skills []string
}

// loadHarnessManifest parses a scion-agent.yaml body into a typed HarnessManifest.
// It validates Backend against the known enum and returns an error for:
//   - missing or empty default_harness_config
//   - unrecognised backend value
//
// Skills may be empty (nil slice) when the manifest declares no skills.
func loadHarnessManifest(body []byte) (HarnessManifest, error) {
	s := string(body)
	backend := scanField(s, "default_harness_config:")
	if backend == "" {
		return HarnessManifest{}, fmt.Errorf("loadHarnessManifest: default_harness_config missing or empty")
	}
	if !knownBackends[backend] {
		return HarnessManifest{}, fmt.Errorf("loadHarnessManifest: unknown backend %q (valid: claude, codex, pi, gemini)", backend)
	}
	skills := scanList(s, "skills:")
	// Normalise nil to empty slice for callers that range over Skills.
	if skills == nil {
		skills = []string{}
	}
	return HarnessManifest{Backend: backend, Skills: skills}, nil
}

// harnessSecretFor returns the hub secret name required by a given backend.
// Returns empty string for unknown backends (should not happen after loadHarnessManifest validation).
func harnessSecretFor(backend string) string {
	return map[string]string{
		"claude": "claude_auth",
		"codex":  "codex_auth",
		"pi":     "OPENROUTER_API_KEY",
		"gemini": "gemini_auth",
	}[backend]
}

// imageTagFor returns the Docker image tag for a given backend.
func imageTagFor(backend string) string {
	return fmt.Sprintf("local/darkish-%s:latest", strings.ToLower(backend))
}
