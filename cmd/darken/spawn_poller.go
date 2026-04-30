package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// agentInfo is a partial mirror of scion's pkg/api/types.go AgentInfo.
// Only the fields Phase 7's poller needs.
type agentInfo struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Template string `json:"template"` // role/template name used at spawn time (e.g. "researcher")
}

// pollUntilReady runs `scion list --format json` in a tick loop, looking
// for an agent whose name matches the given one and whose phase has
// transitioned to "running" (success) or "error" (failure). Returns the
// terminal phase string and a nil error on running, or the phase + a
// non-nil error on error/timeout/scion-CLI-missing.
//
// timeout: max wall-clock to wait. interval: time between polls.
//
// onPhaseChange is invoked once per distinct phase observed (skipped if
// nil). Call sites use this to print progress to stderr without
// polluting the poller with formatting concerns.
//
// Caller is expected to have already invoked `scion start <name> ...`
// before calling pollUntilReady — this function only watches for the
// state transition; it doesn't dispatch the agent itself.
func pollUntilReady(agentName string, timeout, interval time.Duration, onPhaseChange func(phase string)) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastPhase string
	for {
		agents, err := scionListAgents()
		if err != nil {
			return "", fmt.Errorf("scion list failed: %w", err)
		}
		for _, a := range agents {
			if a.Name != agentName {
				continue
			}
			if a.Phase != lastPhase {
				lastPhase = a.Phase
				if onPhaseChange != nil {
					onPhaseChange(a.Phase)
				}
			}
			switch a.Phase {
			case "running":
				return "running", nil
			case "error":
				return "error", fmt.Errorf("agent %q transitioned to error phase", agentName)
			}
		}
		if time.Now().After(deadline) {
			return lastPhase, fmt.Errorf("timeout waiting for agent %q to reach running phase (last seen: %q)", agentName, lastPhase)
		}
		time.Sleep(interval)
	}
}

// scionListAgents shells out to `scion list --format json` and parses
// the result into agentInfo slices.
//
// scion sometimes prepends non-JSON diagnostic lines (e.g. a dev-auth
// WARNING) before the JSON array. jsonStart strips those lines so the
// unmarshal does not fail on the prefix.
func scionListAgents() ([]agentInfo, error) {
	out, err := exec.Command("scion", "list", "--format", "json").Output()
	if err != nil {
		return nil, err
	}
	jsonBytes := jsonStart(out)
	var agents []agentInfo
	if err := json.Unmarshal(jsonBytes, &agents); err != nil {
		return nil, fmt.Errorf("parse scion list output: %w", err)
	}
	return agents, nil
}

// jsonStart returns the slice of b starting at the first '[' or '{'
// that appears as the first non-whitespace byte of a line.
// Lines before that are silently discarded. Leading spaces and tabs on
// the matching line are also stripped so the caller receives a slice
// that begins with the JSON delimiter itself.
// If no such line exists, the original slice is returned unchanged so
// that the caller surfaces a descriptive json.Unmarshal error.
func jsonStart(b []byte) []byte {
	for len(b) > 0 {
		// Find the first non-space/tab byte on this line.
		i := 0
		for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
			i++
		}
		if i < len(b) && (b[i] == '[' || b[i] == '{') {
			return b[i:]
		}
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			return b
		}
		b = b[nl+1:]
	}
	return b
}
