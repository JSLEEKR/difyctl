package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/JSLEEKR/difyctl/internal/lint"
	"github.com/JSLEEKR/difyctl/internal/parse"
)

// runLint implements `difyctl lint`.
// Returns (exitCode, err) — err is non-nil only on internal/usage failures;
// lint findings translate to exitCode=1 with err=nil.
func runLint(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json")
	difyVersion := fs.String("dify-version", "", "expected Dify DSL version (informational; unused in v1)")
	_ = difyVersion
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2, fmt.Errorf("lint requires exactly one file argument")
	}
	if *format != "text" && *format != "json" {
		return 2, fmt.Errorf("unknown --format %q (want text or json)", *format)
	}

	path := fs.Arg(0)
	wf, err := parse.LoadFile(path)
	if err != nil {
		return 3, err
	}

	rules := lint.DefaultRules()
	findings := lint.Run(rules, wf)

	switch *format {
	case "json":
		buf, _ := json.MarshalIndent(map[string]any{
			"path":     path,
			"findings": findings,
			"summary":  lint.CountBySeverity(findings),
		}, "", "  ")
		fmt.Fprintln(stdout, string(buf))
	default:
		for _, f := range findings {
			fmt.Fprintln(stdout, f.Format())
		}
		c := lint.CountBySeverity(findings)
		fmt.Fprintf(stdout, "\n%d errors, %d warnings\n", c[lint.SeverityError], c[lint.SeverityWarning])
	}
	if lint.HasErrors(findings) {
		return 1, nil
	}
	return 0, nil
}
