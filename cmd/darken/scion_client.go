package main

import (
	"bytes"
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

	// CleanGrove is the inverse of GroveInit: removes the local .scion/
	// directory and unlinks from the Hub. Equivalent to `scion clean --yes`
	// run inside targetDir. The earlier `scion grove delete` subcommand does
	// not exist; CleanGrove is the canonical teardown call.
	CleanGrove(targetDir string) error

	// BrokerWithdraw removes the local broker as a provider for the
	// current grove. Symmetric counterpart to BrokerProvide. Best-effort:
	// callers should treat "broker not provided" / "no grove" failures as
	// no-ops since teardown shouldn't abort on already-clean state.
	BrokerWithdraw() error

	// LookAgent returns the raw terminal output of `scion look <name>`.
	// ANSI stripping is the caller's responsibility.
	LookAgent(name string, extraArgs []string) ([]byte, error)

	// StartServer starts the scion daemon. Idempotency is the caller's
	// concern — typically gated behind a ServerStatus check.
	StartServer() error

	// StopServer stops the scion daemon. Used by `darken down --purge`
	// for cross-project teardown; ordinary `darken down` leaves the
	// server running.
	StopServer() error

	// StopAgent stops a running agent by name. Idempotent: stopping an
	// already-stopped agent is a no-op on scion's side.
	StopAgent(name string) error

	// DeleteAgent removes an agent registration by name. Should be
	// preceded by StopAgent so no work is in flight.
	DeleteAgent(name string) error

	// DeleteTemplate removes a template from scion's user (global) store.
	// Used by `darken down --purge` to revert template uploads from
	// uploadAllTemplatesToHub.
	DeleteTemplate(role string) error

	// PushFileSecret uploads a credential file to the Hub. target is
	// the path-in-container the agent expects; srcPath is the file on
	// the host to read content from. Maps to:
	//   scion hub secret set --type file --target <target> <name> @<srcPath>
	PushFileSecret(name, target, srcPath string) error

	// PushEnvSecret uploads a value as an env-type secret named for
	// the env var it'll populate inside the container. Maps to:
	//   scion hub secret set --type env --target <name> <name> @<tmpfile>
	// where tmpfile holds value.
	PushEnvSecret(name, value string) error
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
	err, _ := runScionCmd(scionCmdWithEnv(full))
	return err
}

func (c *execScionClient) BrokerProvide() error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"broker", "provide"}))
	return err
}

func (c *execScionClient) PushTemplate(role string) error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"--global", "--non-interactive", "templates", "push", role}))
	return err
}

func (c *execScionClient) ImportAllTemplates(dir string) error {
	cmd := scionCmdWithEnv([]string{"--global", "--non-interactive", "templates", "import", "--all", dir})
	err, suppressed := runScionCmd(cmd, "no importable agent definitions")
	if suppressed {
		return fmt.Errorf("scion templates import: no agent definitions in %s — templates dir is empty or missing role subdirs", dir)
	}
	if err != nil {
		return fmt.Errorf("scion templates import: %w", err)
	}
	return nil
}

func (c *execScionClient) GroveInit(targetDir string) error {
	cmd := scionCmdWithEnv([]string{"grove", "init"})
	cmd.Dir = targetDir
	err, _ := runScionCmd(cmd)
	return err
}

func (c *execScionClient) CleanGrove(targetDir string) error {
	cmd := scionCmdWithEnv([]string{"clean", "--yes"})
	cmd.Dir = targetDir
	err, suppressed := runScionCmd(cmd, "no scion grove found")
	if suppressed {
		return nil
	}
	return err
}

func (c *execScionClient) BrokerWithdraw() error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"broker", "withdraw"}))
	return err
}

// runScionCmd is the shared transport for execScionClient methods that
// pipe scion's stdout/stderr to the operator. Stderr is buffered so we
// can suppress scion's cobra Usage block on known runtime-error messages
// (e.g. "no importable agent definitions found"), which would otherwise
// dump the full Usage as noise. Returns:
//
//   - err = the underlying cmd.Run() error (or nil)
//   - suppressed = true iff err is non-nil AND a knownNoisy substring
//     matched the buffered stderr. Caller wraps the error with a
//     friendly message; the noisy stderr is dropped.
//
// On success, buffered stderr is replayed so progress output reaches
// the operator. On unknown errors, stderr passes through verbatim so
// the operator can debug.
func runScionCmd(cmd *exec.Cmd, knownNoisy ...string) (error, bool) {
	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	stderr := stderrBuf.String()
	if err != nil {
		for _, pat := range knownNoisy {
			if strings.Contains(stderr, pat) {
				return err, true
			}
		}
		os.Stderr.WriteString(stderr)
		return err, false
	}
	if stderr != "" {
		os.Stderr.WriteString(stderr)
	}
	return nil, false
}

func (c *execScionClient) StartServer() error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"server", "start"}))
	return err
}

func (c *execScionClient) StopServer() error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"server", "stop"}))
	return err
}

func (c *execScionClient) StopAgent(name string) error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"stop", name, "-y"}))
	return err
}

func (c *execScionClient) DeleteAgent(name string) error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"delete", name, "-y"}))
	return err
}

func (c *execScionClient) DeleteTemplate(role string) error {
	err, _ := runScionCmd(scionCmdWithEnv([]string{"--global", "templates", "delete", role, "-y"}))
	return err
}

func (c *execScionClient) PushFileSecret(name, target, srcPath string) error {
	cmd := scionCmdWithEnv([]string{
		"hub", "secret", "set",
		"--type", "file",
		"--target", target,
		name, "@" + srcPath,
	})
	err, _ := runScionCmd(cmd)
	return err
}

func (c *execScionClient) PushEnvSecret(name, value string) error {
	tmp, err := os.CreateTemp("", "darken-secret-*")
	if err != nil {
		return fmt.Errorf("create temp for env secret: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(value); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp for env secret: %w", err)
	}
	tmp.Close()
	cmd := scionCmdWithEnv([]string{
		"hub", "secret", "set",
		"--type", "env",
		"--target", name,
		name, "@" + tmp.Name(),
	})
	err2, _ := runScionCmd(cmd)
	return err2
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
