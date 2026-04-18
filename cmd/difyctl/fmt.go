package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	dfmt "github.com/JSLEEKR/difyctl/internal/fmt"
)

// runFmt implements `difyctl fmt`.
func runFmt(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("w", false, "write changes back to file")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2, fmt.Errorf("fmt requires exactly one file argument")
	}
	path := fs.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		return 3, err
	}
	out, err := dfmt.Format(src)
	if err != nil {
		return 3, err
	}
	if *write {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return 3, err
		}
		return 0, nil
	}
	if _, err := stdout.Write(out); err != nil {
		return 1, err
	}
	return 0, nil
}
