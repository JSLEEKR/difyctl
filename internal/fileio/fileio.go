// Package fileio centralises the "read a DSL file safely" logic shared by
// lint, diff, and fmt. It exists because previous cycles repeatedly hit the
// same class of cascade bug: a protection (file-size cap, directory reject,
// non-UTF-8 BOM rejection) was added to one command but silently skipped on
// another path. The very first cycle-A work capped parse.LoadFile at 32 MiB
// but fmt had its own os.ReadFile, which Cycle F later had to patch; Cycle E
// rejected UTF-16/32 BOMs in fmt but lint/diff (which both went through
// parse.LoadFile) silently ASCII-stripped such input and lied to the user.
//
// All three subcommands now MUST route their file reads through ReadCapped
// so that any future guard is uniformly applied. Do not add new open/read
// call sites elsewhere; extend this package instead.
package fileio

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxFileSize caps how large a DSL file may be. 32 MiB — well above any
// realistic Dify export (real exports are ~50 KB) and small enough that a
// hostile file cannot OOM the process.
const MaxFileSize int64 = 32 * 1024 * 1024

// ErrNonUTF8BOM is returned when input carries a UTF-16 or UTF-32 byte-order
// mark. yaml.v3 does NOT decode such inputs — it silently slurps the ASCII
// subset, which would cause `fmt -w` (or any re-serialising caller) to write
// the stripped remainder back, corrupting the file. Lint/diff with this input
// would report rules against a mangled document. We refuse up-front.
var ErrNonUTF8BOM = errors.New("non-UTF-8 input detected (yaml.v3 only decodes UTF-8)")

// ReadCapped opens path, rejects directories, enforces MaxFileSize, and
// rejects non-UTF-8 byte-order marks. On success it returns the full bytes.
//
// The return errors are plain (not wrapped with a sentinel); callers that
// want to wrap with their own domain error (e.g. parse.ErrIO) should do so
// at the call site.
func ReadCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if fi, statErr := f.Stat(); statErr == nil {
		if fi.IsDir() {
			return nil, fmt.Errorf("%s: is a directory", path)
		}
		if fi.Size() > MaxFileSize {
			return nil, fmt.Errorf("%s: file is %d bytes, exceeds cap of %d", path, fi.Size(), MaxFileSize)
		}
	}
	// Hard cap via LimitReader in case Stat was unreliable (pipes, special files).
	b, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", path, err)
	}
	if int64(len(b)) > MaxFileSize {
		return nil, fmt.Errorf("%s: file exceeds %d-byte cap", path, MaxFileSize)
	}
	if HasNonUTF8BOM(b) {
		return nil, fmt.Errorf("%s: %w", path, ErrNonUTF8BOM)
	}
	return b, nil
}

// HasNonUTF8BOM reports whether src starts with a UTF-16 or UTF-32 BOM.
// The UTF-8 BOM (EF BB BF) is accepted — yaml.v3 handles it natively.
//
// Check order matters: UTF-32 LE (FF FE 00 00) must be checked before
// UTF-16 LE (FF FE) because they share a prefix.
func HasNonUTF8BOM(src []byte) bool {
	// UTF-32 BE: 00 00 FE FF
	if len(src) >= 4 && src[0] == 0x00 && src[1] == 0x00 && src[2] == 0xFE && src[3] == 0xFF {
		return true
	}
	// UTF-32 LE: FF FE 00 00  (check BEFORE UTF-16 LE since they overlap in prefix)
	if len(src) >= 4 && src[0] == 0xFF && src[1] == 0xFE && src[2] == 0x00 && src[3] == 0x00 {
		return true
	}
	// UTF-16 BE: FE FF
	if len(src) >= 2 && src[0] == 0xFE && src[1] == 0xFF {
		return true
	}
	// UTF-16 LE: FF FE
	if len(src) >= 2 && src[0] == 0xFF && src[1] == 0xFE {
		return true
	}
	return false
}
