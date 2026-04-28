// Package main is the darken operator CLI.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/danmestas/darken/internal/substrate"
)

var globalFlags struct {
	substrateOverrides string
}

type subcommand struct {
	name string
	desc string
	run  func(args []string) error
}

var subcommands = []subcommand{
	{"doctor", "preflight + post-mortem health checks", runDoctor},
	{"spawn", "stage creds + skills + scion start", runSpawn},
	{"bootstrap", "first-time machine setup", runBootstrap},
	{"apply", "review + apply darwin recommendations", runApply},
	{"create-harness", "scaffold a new harness directory", runCreateHarness},
	{"skills", "manage staged skills", runSkills},
	{"creds", "refresh hub secrets", runCreds},
	{"images", "wrap make -C images", runImages},
	{"list", "wrap scion list", runList},
	{"orchestrate", "print host-mode orchestrator skill body", runOrchestrate},
	{"redispatch", "kill + re-spawn an agent with the same role", runRedispatch},
	{"init", "scaffold CLAUDE.md in a target dir", runInit},
	{"version", "print binary version + embedded substrate hash", runVersion},
	{"status", "print one-line status (statusLine-friendly)", runStatus},
	{"dashboard", "open scion's web UI in the default browser", runDashboard},
	{"history", "tabular view of .scion/audit.jsonl", runHistory},
}

func main() {
	// flag.Parse exits non-zero on --help; intercept first so the test runner sees a clean exit.
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" || a == "help" {
			printUsage()
			os.Exit(0)
		}
	}

	fs := flag.NewFlagSet("darken", flag.ContinueOnError)
	fs.StringVar(&globalFlags.substrateOverrides, "substrate-overrides", "",
		"path to substrate override directory (overrides $DARKEN_SUBSTRATE_OVERRIDES and ~/.config/darken/overrides/)")
	fs.Usage = printUsage
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	args := fs.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(2)
	}

	for _, sc := range subcommands {
		if sc.name == args[0] {
			if err := sc.run(args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, "darken:", err)
				os.Exit(1)
			}
			return
		}
	}

	fmt.Fprintf(os.Stderr, "darken: unknown subcommand %q\n", args[0])
	printUsage()
	os.Exit(2)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: darken <subcommand> [flags] [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Subcommands:")
	for _, sc := range subcommands {
		fmt.Fprintf(os.Stderr, "  %-16s %s\n", sc.name, sc.desc)
	}
}

// substrateResolver builds a *substrate.Resolver from globalFlags +
// the user's home dir + the project root (best-effort). Subcommands
// call this on demand rather than caching a singleton, so per-test
// env-var changes flow through.
func substrateResolver() *substrate.Resolver {
	cfg := substrate.Config{
		FlagOverride: globalFlags.substrateOverrides,
	}
	if home, err := os.UserHomeDir(); err == nil {
		cfg.UserOverrideDir = filepath.Join(home, ".config", "darken", "overrides")
	}
	if root, err := repoRoot(); err == nil {
		cfg.ProjectRoot = root
	}
	return substrate.New(cfg)
}
