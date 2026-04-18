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

	"github.com/JSLEEKR/difyctl/internal/parse"
)

// exitErr signals the exit code for a CLI failure.
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string { return e.err.Error() }
func (e *exitErr) Unwrap() error { return e.err }

func newExitErr(code int, err error) *exitErr { return &exitErr{code: code, err: err} }

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

// exitCodeFor maps a low-level error to our documented exit codes.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		return ee.code
	}
	if errors.Is(err, parse.ErrIO) {
		return 3
	}
	if errors.Is(err, parse.ErrParse) {
		return 3
	}
	return 1
}
