// Package main is the darkish operator CLI.
package main

import (
	"flag"
	"fmt"
	"os"
)

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
}

func main() {
	// flag.Parse exits non-zero on --help; intercept first so the test runner sees a clean exit.
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" || a == "help" {
			printUsage()
			os.Exit(0)
		}
	}
	flag.Usage = printUsage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(2)
	}

	for _, sc := range subcommands {
		if sc.name == args[0] {
			if err := sc.run(args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, "darkish:", err)
				os.Exit(1)
			}
			return
		}
	}

	fmt.Fprintf(os.Stderr, "darkish: unknown subcommand %q\n", args[0])
	printUsage()
	os.Exit(2)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: darkish <subcommand> [flags] [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Subcommands:")
	for _, sc := range subcommands {
		fmt.Fprintf(os.Stderr, "  %-16s %s\n", sc.name, sc.desc)
	}
}
