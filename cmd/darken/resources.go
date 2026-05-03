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
	"path/filepath"
	"strings"
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

// Observer is an optional capability for Resources that can cheaply
// report their current state without mutating it. Used by `darken
// doctor` (Phase H) to walk the same lifecycle slice that `darken up`
// and `darken down` use, producing a read-only state report.
//
// Resources that genuinely have observable state implement this; ones
// that don't (e.g., HubSecrets — verifying secrets without staging
// them is non-trivial) skip the interface and are reported as
// "(no observer)" in doctor output.
//
// Status convention: "ok" / "missing" / "stopped" / "drift" / etc.
// Detail is a human-readable line — e.g. version strings, paths.
type Observer interface {
	Resource
	Observe() (status, detail string)
}

// LifecycleObservation is the doctor-friendly shape of a single
// Observer.Observe() call. Used by lifecycleObservations to package
// resource state for the doctor renderer without leaking the
// Resource/Observer types into doctor.go.
type LifecycleObservation struct {
	Name   string
	Status string
	Detail string
}

// lifecycleObservations walks `lifecycle` and returns one
// LifecycleObservation per resource that implements Observer.
// Resources without Observe() are skipped — `darken doctor` handles
// the "no observer" case as a Skip entry, separate from the main
// DoctorCheck registry to keep severity/remediation handling intact.
func lifecycleObservations() []LifecycleObservation {
	out := make([]LifecycleObservation, 0, len(lifecycle))
	for _, r := range lifecycle {
		if obs, ok := r.(Observer); ok {
			status, detail := obs.Observe()
			out = append(out, LifecycleObservation{
				Name:   r.Name(),
				Status: status,
				Detail: detail,
			})
		}
	}
	return out
}

// lifecycle is the canonical, ordered list of resources darken manages.
// The slice order encodes the topology: each resource may depend on
// resources earlier in the slice. `darken up` walks forward; `darken
// down` walks reverse. Down-side ordering is critical: AgentWorktrees
// and ProjectAgents must be released BEFORE Grove (they read from
// .scion/agents/ and the grove registry, which Grove.Release destroys).
var lifecycle = []Resource{
	DockerDaemon{},
	ScionCLI{},
	ScionServer{},
	Grove{},
	GroveBroker{},
	DarkenImages{},
	HubSecrets{},
	Substrate{},
	ProjectAgents{},
	AgentWorktrees{},
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
func (DockerDaemon) Observe() (string, string) {
	if err := (DockerDaemon{}).Ensure(); err != nil {
		return "missing", "docker info failed"
	}
	return "ok", ""
}

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
func (ScionCLI) Observe() (string, string) {
	path, err := exec.LookPath("scion")
	if err != nil {
		return "missing", "scion not on PATH"
	}
	return "ok", path
}

// ScionServer ensures the scion daemon is running. Idempotent: if status
// reports OK, Ensure is a no-op. Release is a no-op — leaving the server
// running between projects is the right default; --purge handles the
// cross-project case explicitly.
type ScionServer struct{}

func (ScionServer) Name() string { return "scion server running" }
func (ScionServer) Ensure() error {
	if _, err := defaultScionClient.ServerStatus(); err == nil {
		return nil
	}
	return defaultScionClient.StartServer()
}
func (ScionServer) Release() error { return nil }
func (ScionServer) Observe() (string, string) {
	if _, err := defaultScionClient.ServerStatus(); err != nil {
		return "stopped", "scion server status returned error"
	}
	return "ok", ""
}

// GroveBroker registers the local broker as a provider for the project
// grove (Ensure) or removes that registration (Release). This is the
// canonical example of the symmetric pair the Resource interface
// enforces — bug class #45 (forgotten BrokerWithdraw) is structurally
// impossible because both methods are required.
type GroveBroker struct{}

func (GroveBroker) Name() string   { return "broker provided to grove" }
func (GroveBroker) Ensure() error  { return defaultScionClient.BrokerProvide() }
func (GroveBroker) Release() error { return defaultScionClient.BrokerWithdraw() }

// DarkenImages ensures every per-backend image is built. Idempotent
// per-backend via imageExists. Release is a no-op — the image cache is
// host-wide, persisting across projects (and re-builds are cheap if the
// cache is gone).
type DarkenImages struct{}

func (DarkenImages) Name() string { return "darken images built" }
func (DarkenImages) Ensure() error {
	for _, b := range []string{"claude", "codex", "pi", "gemini"} {
		if imageExists("local/darkish-" + b + ":latest") {
			continue
		}
		c := exec.Command("make", "-C", "images", b)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("make %s: %w", b, err)
		}
	}
	return nil
}
func (DarkenImages) Release() error { return nil }

// HubSecrets stages credentials for each backend into the Hub via
// stageHubCreds (cmd/darken/creds.go). Release is a no-op — secrets are
// hub-wide, shared across projects, removed only via --purge.
type HubSecrets struct{}

func (HubSecrets) Name() string   { return "hub secrets pushed" }
func (HubSecrets) Ensure() error  { return stageHubCreds("all") }
func (HubSecrets) Release() error { return nil }

// Substrate stages per-role skill bundles and imports the templates into
// scion's local store. Combines the two halves of the old
// ensureAllSkillsStaged: per-role stage-skills.sh runs (which Phase G
// will replace with native Go) plus the ImportAllTemplates call.
// Release is a no-op — the Grove resource handles cleanup via scion clean.
type Substrate struct{}

func (Substrate) Name() string { return "substrate staged + imported" }
func (Substrate) Ensure() error {
	templatesDir, _, cleanup, err := resolveSubstrateDirs()
	if err != nil {
		return err
	}
	defer cleanup()

	dirs, err := os.ReadDir(templatesDir)
	if err != nil {
		return fmt.Errorf("read templates dir %s: %w", templatesDir, err)
	}
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == "base" {
			continue
		}
		if err := stageSkillsNative(d.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "substrate: stage-skills %s failed: %v\n", d.Name(), err)
		}
	}
	return defaultScionClient.ImportAllTemplates(templatesDir)
}
func (Substrate) Release() error { return nil }

// Grove ensures the project-scoped scion grove is initialized (Ensure)
// or removed (Release). Pulled into the lifecycle from the old
// runUp → ensureGroveInit flow so darken up is one walker call rather
// than init + bootstrap separately. Idempotent: ensureGroveInit checks
// for .scion/grove-id before running scion grove init.
type Grove struct{}

func (Grove) Name() string { return "project grove" }
func (Grove) Ensure() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	return ensureGroveInit(root)
}
func (Grove) Release() error {
	root, err := repoRoot()
	if err != nil {
		return nil
	}
	return defaultScionClient.CleanGrove(root)
}
func (Grove) Observe() (string, string) {
	root, err := repoRoot()
	if err != nil {
		return "missing", "no repo root"
	}
	groveID := filepath.Join(root, ".scion", "grove-id")
	body, err := os.ReadFile(groveID)
	if err != nil {
		return "missing", "no .scion/grove-id"
	}
	return "ok", strings.TrimSpace(string(body))
}

// ProjectAgents is a down-only resource: agents are spawned via
// `darken spawn`, not by the up lifecycle. Release stops + deletes
// every agent in the project grove. Best-effort per-agent — one stuck
// agent shouldn't block teardown of the rest.
type ProjectAgents struct{}

func (ProjectAgents) Name() string  { return "project agents" }
func (ProjectAgents) Ensure() error { return nil }
func (ProjectAgents) Release() error {
	agents, err := scionListAgents()
	if err != nil {
		fmt.Fprintf(os.Stderr, "project agents: scion list failed: %v (skipping)\n", err)
		return nil
	}
	if len(agents) == 0 {
		return nil
	}
	fmt.Printf("project agents: stopping %d agent(s) ...\n", len(agents))
	for _, a := range agents {
		_ = defaultScionClient.StopAgent(a.Name)
		_ = defaultScionClient.DeleteAgent(a.Name)
	}
	return nil
}

// AgentWorktrees is a down-only resource: git worktrees under
// .scion/agents/ are created on darken spawn. Release enumerates them
// via `git worktree list --porcelain`, removes each, and runs
// `git worktree prune` to clean orphan registry entries. Best-effort —
// an operator who manually deleted a worktree dir shouldn't block
// teardown.
type AgentWorktrees struct{}

func (AgentWorktrees) Name() string  { return "agent worktrees" }
func (AgentWorktrees) Ensure() error { return nil }
func (AgentWorktrees) Release() error {
	paths, err := listAgentWorktreePaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent worktrees: list failed: %v (skipping)\n", err)
		return nil
	}
	for _, p := range paths {
		if err := exec.Command("git", "worktree", "remove", "--force", p).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "agent worktrees: remove %s: %v (continuing)\n", p, err)
		}
	}
	_ = exec.Command("git", "worktree", "prune").Run()
	return nil
}

// listAgentWorktreePaths runs `git worktree list --porcelain` and
// returns paths under .scion/agents/. Returns an empty slice (and nil
// error) if there are no worktrees registered, which is normal on a
// project that never spawned an agent.
func listAgentWorktreePaths() ([]string, error) {
	out, err := exec.Command("git", "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		path := strings.TrimPrefix(line, "worktree ")
		if strings.Contains(path, "/.scion/agents/") {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
