package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	dfmt "github.com/JSLEEKR/difyctl/internal/fmt"
	"github.com/JSLEEKR/difyctl/internal/parse"
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
	src, err := readFileCapped(path, parse.MaxFileSize)
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

// readFileCapped reads up to limit+1 bytes from path and errors if the file
// exceeds the cap. Mirrors the Cycle A protection in parse.LoadFile so that
// `fmt -w` on a hostile 40 MiB file cannot OOM the process; without this,
// `difyctl fmt` was the last unguarded read path in the CLI.
func readFileCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, statErr := f.Stat(); statErr == nil {
		if fi.IsDir() {
			return nil, fmt.Errorf("read %s: is a directory", path)
		}
		if fi.Size() > limit {
			return nil, fmt.Errorf("read %s: file is %d bytes, exceeds cap of %d", path, fi.Size(), limit)
		}
	}
	// Hard cap with LimitReader in case Stat was unreliable (pipes, special files).
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", path, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("read %s: file exceeds %d-byte cap", path, limit)
	}
	return b, nil
}

// writeFileAtomic writes data to path via a temp file + rename, preserving the
// original file's permission bits. A crash between write and rename leaves the
// original file untouched (rather than truncated to zero bytes, which is what
// os.WriteFile would do).
//
// Symlinks: if path is a symlink, we follow it and write to the target file.
// The atomic rename therefore replaces the TARGET, not the symlink itself —
// which is the behavior users intuitively expect from `fmt -w my-link.yml`
// (the symlink should remain a symlink pointing at the updated target).
// Previously, os.Rename(tmp, path) would silently replace the symlink with a
// regular file, which is surprising and corrupts the working-tree shape.
func writeFileAtomic(path string, data []byte) error {
	// Resolve symlinks so the rename replaces the ACTUAL file, not the link.
	// We use EvalSymlinks only when the path is in fact a symlink, so that
	// nonexistent paths still yield the original clean error at the caller.
	target := path
	if li, err := os.Lstat(path); err == nil && li.Mode()&os.ModeSymlink != 0 {
		resolved, rerr := filepath.EvalSymlinks(path)
		if rerr != nil {
			return rerr
		}
		target = resolved
	}
	dir := filepath.Dir(target)
	// Capture original mode so permissions survive the rename. Use Stat
	// (follows symlinks) so permissions come from the resolved target.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
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
	if err := os.Rename(tmpName, target); err != nil {
		cleanup()
		return err
	}
	return nil
}
