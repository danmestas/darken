package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ScionClient exposes the deep operations that darken performs against the
// scion CLI. Env propagation, output policy, and error mapping are owned by
// the implementation; callers receive typed results.
//
// The six methods correspond to the six operation classes used across
// doctor.go, spawn.go, setup.go, and bootstrap.go.
type ScionClient interface {
	// ServerStatus returns the raw output of `scion server status`.
	// Returns a non-nil error if scion is not on PATH or the command fails.
	ServerStatus() (string, error)

	// SecretList returns the raw output of `scion hub secret list`.
	SecretList() (string, error)

	// StartAgent starts an agent with the given name, harness type inferred from
	// args. args is the full argument list passed after the agent name to
	// `scion start`.
	StartAgent(name string, args []string) error

	// BrokerProvide registers the current grove with the local broker.
	// Idempotent; returns nil when the grove is already registered.
	BrokerProvide() error

	// PushTemplate uploads a role template to the hub at user (global) scope.
	PushTemplate(role string) error

	// ImportAllTemplates copies every template subdirectory under dir into
	// scion's local store at user (global) scope. Idempotent: re-importing
	// the same template overwrites the prior copy. Bodies survive deletion
	// of the source dir, so the caller can clean up an extracted tmpdir
	// immediately after this returns.
	ImportAllTemplates(dir string) error

	// GroveInit registers targetDir as a project-scoped scion grove.
	// Idempotent at the caller level: callers check for .scion/grove-id before
	// invoking this method. targetDir is used as the working directory so that
	// grove init applies to the correct project even when cwd differs.
	GroveInit(targetDir string) error

	// LookAgent returns the raw terminal output of `scion look <name>`.
	// ANSI stripping is the caller's responsibility.
	LookAgent(name string, extraArgs []string) ([]byte, error)
}

// execScionClient is the production ScionClient that delegates to the scion binary.
// It routes all invocations through scionCmdEnv() so env variables are
// consistent across all operations.
type execScionClient struct{}

func (c *execScionClient) ServerStatus() (string, error) {
	out, err := scionCmdWithEnv([]string{"server", "status"}).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("scion server status: %w", err)
	}
	return string(out), nil
}

func (c *execScionClient) SecretList() (string, error) {
	out, err := scionCmdWithEnv([]string{"hub", "secret", "list"}).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("scion hub secret list: %w", err)
	}
	return string(out), nil
}

func (c *execScionClient) StartAgent(name string, args []string) error {
	full := append([]string{"start", name}, args...)
	cmd := scionCmdWithEnv(full)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *execScionClient) BrokerProvide() error {
	cmd := scionCmdWithEnv([]string{"broker", "provide"})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *execScionClient) PushTemplate(role string) error {
	cmd := scionCmdWithEnv([]string{"--global", "templates", "push", role})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *execScionClient) ImportAllTemplates(dir string) error {
	cmd := scionCmdWithEnv([]string{"--global", "templates", "import", "--all", dir})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *execScionClient) GroveInit(targetDir string) error {
	cmd := scionCmdWithEnv([]string{"grove", "init"})
	cmd.Dir = targetDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *execScionClient) LookAgent(name string, extraArgs []string) ([]byte, error) {
	full := append([]string{"look", name}, extraArgs...)
	out, err := scionCmdWithEnv(full).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("scion look %s: %w\n%s", name, err, ee.Stderr)
		}
		return out, fmt.Errorf("scion look %s: %w", name, err)
	}
	return out, nil
}

// defaultScionClient is the package-level ScionClient used by doctor, spawn,
// setup, and bootstrap. Tests replace it with a mockScionClient.
var defaultScionClient ScionClient = &execScionClient{}

// scionCmdWithEnv is a thin helper that builds an exec.Cmd for scion with the
// canonical darken environment applied. It is the internal implementation
// detail for execScionClient; callers outside that struct should use
// defaultScionClient methods.
func scionCmdWithEnv(args []string) *exec.Cmd {
	cmd := exec.Command("scion", args...)
	cmd.Env = scionCmdEnv()
	return cmd
}

// secretListContains reports whether SecretList output contains all wanted secrets.
func secretListContains(output string, secrets []string) (missing []string) {
	for _, s := range secrets {
		if !strings.Contains(output, s) {
			missing = append(missing, s)
		}
	}
	return missing
}
