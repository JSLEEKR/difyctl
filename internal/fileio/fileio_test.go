package fileio

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCapped_Good(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.yml")
	if err := os.WriteFile(p, []byte("app: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := ReadCapped(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(b) != "app: {}\n" {
		t.Fatalf("got %q", b)
	}
}

func TestReadCapped_Missing(t *testing.T) {
	_, err := ReadCapped("/nonexistent/definitely/not/here.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestReadCapped_Directory guards that directories are rejected with a clean
// message, not left to io.ReadAll (which would return a scary
// "is a directory" wrapped by PathError) or worse, silently succeed if the
// directory's contents happen to be under the size cap.
func TestReadCapped_Directory(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadCapped(dir)
	if err == nil {
		t.Fatal("expected directory rejection")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("error should mention 'is a directory', got: %v", err)
	}
}

// TestReadCapped_TooLarge asserts the 32 MiB cap is enforced.
func TestReadCapped_TooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~32 MiB")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "big.yml")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("a"), 1024)
	for written := int64(0); written <= MaxFileSize; written += int64(len(chunk)) {
		if _, werr := f.Write(chunk); werr != nil {
			t.Fatal(werr)
		}
	}
	_ = f.Close()
	_, err = ReadCapped(p)
	if err == nil {
		t.Fatal("expected oversize rejection")
	}
	if !strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "cap") {
		t.Fatalf("error should mention cap, got: %v", err)
	}
}

// TestReadCapped_UTF16BOMRejected locks the data-loss bug fix: UTF-16 input
// must be refused at the read layer so that neither fmt nor lint nor diff
// ever sees the silently ASCII-stripped content. Before the fileio extraction,
// this rejection lived only in internal/fmt, and lint/diff happily reported
// rules against the half-decoded garbage (Cycle G cascade).
func TestReadCapped_UTF16BOMRejected(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"UTF-16 LE": {0xFF, 0xFE, 'a', 0x00, 'p', 0x00, 'p', 0x00},
		"UTF-16 BE": {0xFE, 0xFF, 0x00, 'a', 0x00, 'p', 0x00, 'p'},
		"UTF-32 LE": {0xFF, 0xFE, 0x00, 0x00, 'a', 0x00, 0x00, 0x00},
		"UTF-32 BE": {0x00, 0x00, 0xFE, 0xFF, 0x00, 0x00, 0x00, 'a'},
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(dir, name+".yml")
			if err := os.WriteFile(p, content, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ReadCapped(p)
			if err == nil {
				t.Fatal("want ErrNonUTF8BOM, got nil — lint/diff/fmt would silently accept garbage")
			}
			if !errors.Is(err, ErrNonUTF8BOM) {
				t.Fatalf("want ErrNonUTF8BOM, got %v", err)
			}
		})
	}
}

// TestReadCapped_UTF8BOMAccepted ensures that a plain UTF-8 BOM (which yaml.v3
// handles natively) is NOT mistakenly rejected. Notepad and some editors prefix
// UTF-8 files with EF BB BF and users legitimately commit such files.
func TestReadCapped_UTF8BOMAccepted(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "utf8bom.yml")
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("app: {}\n")...)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCapped(p); err != nil {
		t.Fatalf("UTF-8 BOM must be accepted: %v", err)
	}
}

// TestHasNonUTF8BOM_Table is a table-driven test for the detector. The order
// of checks inside HasNonUTF8BOM matters (UTF-32 LE shares FF FE prefix with
// UTF-16 LE); this test exercises both.
func TestHasNonUTF8BOM_Table(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", nil, false},
		{"plain ASCII", []byte("app:\n"), false},
		{"UTF-8 BOM", []byte{0xEF, 0xBB, 0xBF, 'a'}, false},
		{"UTF-16 LE", []byte{0xFF, 0xFE, 'a', 0}, true},
		{"UTF-16 BE", []byte{0xFE, 0xFF, 0, 'a'}, true},
		{"UTF-32 LE", []byte{0xFF, 0xFE, 0, 0, 'a', 0, 0, 0}, true},
		{"UTF-32 BE", []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'a'}, true},
		{"short FF", []byte{0xFF}, false}, // single byte, not enough
		{"short FF FE", []byte{0xFF, 0xFE}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasNonUTF8BOM(c.in); got != c.want {
				t.Fatalf("HasNonUTF8BOM(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
