package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
		if err := writeFileAtomic(path, out); err != nil {
			return 3, err
		}
		return 0, nil
	}
	if _, err := stdout.Write(out); err != nil {
		return 1, err
	}
	return 0, nil
}

// writeFileAtomic writes data to path via a temp file + rename, preserving the
// original file's permission bits. A crash between write and rename leaves the
// original file untouched (rather than truncated to zero bytes, which is what
// os.WriteFile would do).
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	// Capture original mode so permissions survive the rename.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".difyctl-fmt-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// On any failure past this point, best-effort remove the temp so we don't leak it.
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
