// Package main — credential staging for the four backend types.
//
// Pre-Phase-F this lived in scripts/stage-creds.sh; the bash is now a
// container-side concern only (still embedded for spawn.sh inside
// containers). Host-side staging goes through the ScionClient interface
// so it shares env propagation, stderr triage, and mockability with
// the rest of the surface.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runCreds is the operator-facing `darken creds` command. Defaults to
// staging all four backends when no specific one is named.
func runCreds(args []string) error {
	what := "all"
	if len(args) > 0 {
		what = args[0]
	}
	return stageHubCreds(what)
}

// stageHubCreds pushes credentials for the given backend (or "all")
// to the Hub via ScionClient.PushFileSecret / PushEnvSecret.
//
// Soft-fails per-backend in "all" mode: a missing keychain entry, env
// var, or file skips that backend only — other backends still get
// staged. Returns nil after logging failures so the caller (HubSecrets,
// runCreds, runSpawn) can continue. Specific-backend mode returns the
// underlying error so the operator sees what went wrong.
func stageHubCreds(what string) error {
	switch what {
	case "claude":
		return stageClaudeCreds()
	case "codex":
		return stageCodexCreds()
	case "pi":
		return stagePiCreds()
	case "gemini":
		return stageGeminiCreds()
	case "all", "":
		stagers := []struct {
			name string
			fn   func() error
		}{
			{"claude", stageClaudeCreds},
			{"codex", stageCodexCreds},
			{"pi", stagePiCreds},
			{"gemini", stageGeminiCreds},
		}
		for _, s := range stagers {
			if err := s.fn(); err != nil {
				fmt.Fprintf(os.Stderr, "stage-creds: WARNING — %s: %v\n", s.name, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown backend %q (want claude|codex|pi|gemini|all)", what)
	}
}

// stageClaudeCreds reads the Claude credentials from the macOS Keychain
// (the canonical install path for Claude Code) and pushes them as a
// file-type secret. Returns an error on non-macOS hosts (no `security`
// CLI) or when the Keychain entry is absent — the caller decides
// whether that's fatal.
func stageClaudeCreds() error {
	if _, err := exec.LookPath("security"); err != nil {
		return fmt.Errorf("security CLI unavailable (non-macOS host)")
	}
	out, err := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return fmt.Errorf("Keychain entry 'Claude Code-credentials' not found")
	}
	tmp, err := os.CreateTemp("", "darken-claude-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	_ = os.Chmod(tmp.Name(), 0o600)
	if err := defaultScionClient.PushFileSecret(
		"claude_auth",
		"/home/scion/.claude/.credentials.json",
		tmp.Name(),
	); err != nil {
		return err
	}
	fmt.Println("stage-creds: claude_auth pushed (file -> /home/scion/.claude/.credentials.json)")
	return nil
}

// stageCodexCreds pushes ~/.codex/auth.json as a file-type secret.
// Soft-fails when the file is absent (codex not installed locally).
func stageCodexCreds() error {
	home, _ := os.UserHomeDir()
	src := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("%s not found", src)
	}
	if err := defaultScionClient.PushFileSecret(
		"codex_auth",
		"/home/scion/.codex/auth.json",
		src,
	); err != nil {
		return err
	}
	fmt.Println("stage-creds: codex_auth pushed (file -> /home/scion/.codex/auth.json)")
	return nil
}

// stagePiCreds pushes OPENROUTER_API_KEY as an env-type secret.
// Soft-fails when the env var is unset (operator hasn't enabled
// OpenRouter routing).
func stagePiCreds() error {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return fmt.Errorf("OPENROUTER_API_KEY not set")
	}
	if err := defaultScionClient.PushEnvSecret("OPENROUTER_API_KEY", key); err != nil {
		return err
	}
	fmt.Println("stage-creds: OPENROUTER_API_KEY pushed (env)")
	return nil
}

// stageGeminiCreds prefers the OAuth flow (~/.gemini/oauth_creds.json)
// and falls back to the API-key flow (GEMINI_API_KEY env). Mirrors the
// bash precedence so behavior is identical across the migration.
func stageGeminiCreds() error {
	home, _ := os.UserHomeDir()
	src := filepath.Join(home, ".gemini", "oauth_creds.json")
	if _, err := os.Stat(src); err == nil {
		if err := defaultScionClient.PushFileSecret(
			"gemini_auth",
			"/home/scion/.gemini/oauth_creds.json",
			src,
		); err != nil {
			return err
		}
		fmt.Println("stage-creds: gemini_auth pushed (file -> /home/scion/.gemini/oauth_creds.json)")
		return nil
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		if err := defaultScionClient.PushEnvSecret("GEMINI_API_KEY", key); err != nil {
			return err
		}
		fmt.Println("stage-creds: GEMINI_API_KEY pushed (env)")
		return nil
	}
	return fmt.Errorf("neither ~/.gemini/oauth_creds.json nor GEMINI_API_KEY found")
}
