package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicWriteJSONCreatesFileWithTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "value.json")
	if err := atomicWriteJSON(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("expected trailing LF newline, got %q", data)
	}
	if string(data)[len(data)-2] == '\r' {
		t.Fatalf("expected LF-only line ending, found CR: %q", data)
	}
	var round map[string]int
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if round["a"] != 1 {
		t.Fatalf("round trip mismatch: %+v", round)
	}
}

func TestAtomicWriteJSONOverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	if err := atomicWriteJSON(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteJSON(path, map[string]int{"a": 2}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]int
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if round["a"] != 2 {
		t.Fatalf("expected overwritten value 2, got %+v", round)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file after overwrite (no leaked temp files), found %d", len(entries))
	}
}

func TestAtomicWriteJSONRestrictsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits are not portable to Windows ACLs")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	if err := atomicWriteJSON(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected no group/other permission bits, got %o", info.Mode().Perm())
	}
}

func TestReadJSONIfExistsReportsAbsence(t *testing.T) {
	dir := t.TempDir()
	var value map[string]int
	found, err := readJSONIfExists(filepath.Join(dir, "missing.json"), &value)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected found=false for a missing file")
	}
}

func TestWriteImmutableCreatesOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	created, err := writeImmutable(path, map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected created=true for a new artifact")
	}
}

func TestWriteImmutableIsIdempotentForIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if _, err := writeImmutable(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	created, err := writeImmutable(path, map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
	if created {
		t.Fatal("expected created=false for a repeated identical write")
	}
}

func TestWriteImmutableConflictsOnDifferentContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if _, err := writeImmutable(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	_, err := writeImmutable(path, map[string]int{"a": 2})
	if err == nil {
		t.Fatal("expected a conflict error for differing content")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var round map[string]int
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if round["a"] != 1 {
		t.Fatalf("expected original content to remain untouched, got %+v", round)
	}
}

// TestSanitizeFSErrorStripsEmbeddedPath covers issue #1's "never an
// absolute path, even in a sanitized error" requirement at its source: a
// real *fs.PathError (produced by attempting to read a directory as a
// regular file, which always fails and always embeds the path) must have
// that path stripped, while remaining a non-empty, still-useful message.
func TestSanitizeFSErrorStripsEmbeddedPath(t *testing.T) {
	dir := t.TempDir()
	_, err := os.ReadFile(dir)
	if err == nil {
		t.Fatal("expected an error reading a directory as a file")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("test setup assumption failed: expected the raw error to contain the path, got %q", err.Error())
	}
	sanitized := SanitizeFSError(err)
	if strings.Contains(sanitized.Error(), dir) {
		t.Fatalf("expected the path to be stripped, got %q", sanitized.Error())
	}
	if sanitized.Error() == "" {
		t.Fatal("expected a non-empty sanitized message")
	}
}

// TestAtomicWriteJSONErrorsNeverContainAPath covers the same requirement
// end to end through atomicWriteJSON's own error paths: forcing ensureDir
// to fail (by making an ordinary file where a directory is required) must
// not leak the underlying directory's path into the returned error.
func TestAtomicWriteJSONErrorsNeverContainAPath(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(blocker, "sub", "value.json")
	err := atomicWriteJSON(target, map[string]int{"a": 1})
	if err == nil {
		t.Fatal("expected an error when the parent path is blocked by a regular file")
	}
	if strings.Contains(err.Error(), dir) {
		t.Fatalf("atomicWriteJSON error must never contain the underlying path: %v", err)
	}
}

// TestWriteImmutableErrorsNeverContainAPath mirrors
// TestAtomicWriteJSONErrorsNeverContainAPath for the create-if-absent
// artifact path.
func TestWriteImmutableErrorsNeverContainAPath(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(blocker, "sub", "artifact.json")
	_, err := writeImmutable(target, map[string]int{"a": 1})
	if err == nil {
		t.Fatal("expected an error when the parent path is blocked by a regular file")
	}
	if strings.Contains(err.Error(), dir) {
		t.Fatalf("writeImmutable error must never contain the underlying path: %v", err)
	}
}
