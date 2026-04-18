// Command difyctl is a CLI for linting, diffing, and formatting Dify workflow DSL.
//
// Exit codes:
//
//	0 — OK
//	1 — lint found issues (or diff found BREAKING with --fail-on-breaking)
//	2 — arg / usage error
//	3 — IO / parse error
package main

import (
	"errors"
	"fmt"
	"os"
)

// usage prints the global help text to stderr.
func usage() {
	fmt.Fprintln(os.Stderr, `difyctl — lint, diff, and format Dify workflow DSL files.

Usage:
  difyctl lint <file.yml> [--format text|json]
  difyctl diff <a.yml> <b.yml> [--format text|json] [--fail-on-breaking]
  difyctl fmt  <file.yml>     [-w]
  difyctl version

Exit codes:
  0 OK
  1 lint issues (or BREAKING with --fail-on-breaking)
  2 usage error
  3 io / parse error`)
}

func main() {
	code, err := realMain(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "difyctl: "+err.Error())
	}
	os.Exit(code)
}

// realMain is testable — it parses args and dispatches without calling os.Exit.
func realMain(args []string) (int, error) {
	if len(args) == 0 {
		usage()
		return 2, errors.New("no subcommand")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "lint":
		return runLint(rest, os.Stdout, os.Stderr)
	case "diff":
		return runDiff(rest, os.Stdout, os.Stderr)
	case "fmt":
		return runFmt(rest, os.Stdout, os.Stderr)
	case "version":
		fmt.Fprintln(os.Stdout, Version)
		return 0, nil
	case "-h", "--help", "help":
		usage()
		return 0, nil
	default:
		usage()
		return 2, fmt.Errorf("unknown subcommand: %s", sub)
	}
}
