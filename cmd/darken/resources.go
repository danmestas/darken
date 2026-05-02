// Package main — lifecycle resource model for `darken up` / `darken down`.
//
// A Resource represents one piece of state darken manages: the scion server
// running, the broker provided to a grove, hub secrets staged, agent
// worktrees created, etc. Each resource owns both directions of its
// lifecycle (Ensure + Release), so the symmetry that bug-class #45
// (forgotten BrokerWithdraw) made obvious is enforced by the type system —
// a resource that forgets one direction won't compile.
//
// The package-level `lifecycle` slice declares the order. `darken up`
// walks forward calling Ensure(); `darken down` walks reverse calling
// Release(); `darken doctor` (Phase H) walks forward calling Observe()
// on resources that opt into the Observer interface. Three commands,
// one world model.
//
// See docs/darken-lifecycle-refactor.md for the full design.
package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Resource is one piece of state darken manages. Implementations own
// both directions of the lifecycle:
//
//   - Ensure makes the resource ready (idempotent — must be safe to call
//     when the resource is already ready).
//   - Release makes the resource clean (idempotent — must be safe to call
//     when the resource is already clean, or never made ready).
//
// Resources that have nothing to do on one side return nil from that
// method. Both methods staying on the interface (rather than being
// nil-able fields on a struct) is intentional: a resource that forgets
// to consider both directions fails to compile.
type Resource interface {
	Name() string
	Ensure() error
	Release() error
}

// lifecycle is the canonical, ordered list of resources darken manages.
// The slice order encodes the topology: each resource may depend on
// resources earlier in the slice. `darken up` walks forward; `darken
// down` walks reverse. Phases B–C grow this list to ~10 entries.
var lifecycle = []Resource{
	DockerDaemon{},
	ScionCLI{},
}

// ensureAll walks resources forward calling Ensure(). The first error
// stops the walk and is returned with the resource name attached. The
// progress prefix (`[N/M] <name> ...`) matches the existing runBootstrap
// output so operator-facing UX is unchanged across the migration.
func ensureAll(resources []Resource) error {
	for i, r := range resources {
		fmt.Printf("[%d/%d] %s ...\n", i+1, len(resources), r.Name())
		if err := r.Ensure(); err != nil {
			return fmt.Errorf("ensure %q: %w", r.Name(), err)
		}
	}
	return nil
}

// releaseAll walks resources in reverse calling Release(). Best-effort:
// errors are logged to stderr and the walk continues, mirroring the
// pre-refactor `darken down` policy. Teardown should not abort midway
// because one resource is in an unexpected state — it should make
// progress on every other resource the operator asked to release.
func releaseAll(resources []Resource) {
	for i := len(resources) - 1; i >= 0; i-- {
		r := resources[i]
		fmt.Printf("[%d/%d] %s ...\n", len(resources)-i, len(resources), r.Name())
		if err := r.Release(); err != nil {
			fmt.Fprintf(os.Stderr, "darken down: release %q: %v (continuing)\n", r.Name(), err)
		}
	}
}

// DockerDaemon ensures the docker daemon is reachable. Release is a no-op
// — docker is host infrastructure, shared across projects, never stopped
// by darken.
type DockerDaemon struct{}

func (DockerDaemon) Name() string { return "docker daemon reachable" }
func (DockerDaemon) Ensure() error {
	out, err := exec.Command("docker", "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker info: %s", string(out))
	}
	return nil
}
func (DockerDaemon) Release() error { return nil }

// ScionCLI ensures the scion binary is on PATH. Release is a no-op —
// the binary is installed via brew, not by darken, and persists across
// projects.
type ScionCLI struct{}

func (ScionCLI) Name() string { return "scion CLI present" }
func (ScionCLI) Ensure() error {
	if _, err := exec.LookPath("scion"); err != nil {
		return fmt.Errorf("scion not found on PATH: %w", err)
	}
	return nil
}
func (ScionCLI) Release() error { return nil }
