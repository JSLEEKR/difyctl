package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/JSLEEKR/difyctl/internal/diff"
	"github.com/JSLEEKR/difyctl/internal/parse"
)

// runDiff implements `difyctl diff`.
func runDiff(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json")
	failOnBreaking := fs.Bool("fail-on-breaking", false, "exit 1 when any BREAKING change is detected")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2, fmt.Errorf("diff requires exactly two file arguments")
	}
	if *format != "text" && *format != "json" {
		return 2, fmt.Errorf("unknown --format %q (want text or json)", *format)
	}

	a, err := parse.LoadFile(fs.Arg(0))
	if err != nil {
		if *format == "json" {
			emitDiffErrorJSON(stdout, err)
		}
		return 3, err
	}
	b, err := parse.LoadFile(fs.Arg(1))
	if err != nil {
		if *format == "json" {
			emitDiffErrorJSON(stdout, err)
		}
		return 3, err
	}

	changes := diff.Compute(a, b)

	switch *format {
	case "json":
		if err := diff.RenderJSON(stdout, changes); err != nil {
			return 1, err
		}
	default:
		diff.RenderText(stdout, changes)
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "summary: "+diff.Summary(changes))
	}

	if *failOnBreaking && diff.HasBreaking(changes) {
		return 1, nil
	}
	return 0, nil
}

// emitDiffErrorJSON writes a single-element error envelope to stdout so that
// callers parsing JSON output don't get empty input on IO/parse failure.
func emitDiffErrorJSON(w io.Writer, err error) {
	env, _ := json.MarshalIndent(map[string]any{
		"error":   err.Error(),
		"changes": []diff.Change{},
	}, "", "  ")
	fmt.Fprintln(w, string(env))
}
