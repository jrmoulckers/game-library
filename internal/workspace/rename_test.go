package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAtomicRenameCreatesFirstDestination covers the "first save" case: the
// destination does not exist yet.
func TestAtomicRenameCreatesFirstDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.tmp")
	dst := filepath.Join(dir, "value.json")
	if err := os.WriteFile(src, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicRename(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one" {
		t.Fatalf("got %q, want %q", data, "one")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected the source temp file to be gone after rename, stat err = %v", err)
	}
}

// TestAtomicRenameReplacesExistingDestinationRepeatedly covers "subsequent
// saves": atomicRename must replace an already-existing destination file
// (not just create a new one), and must keep doing so correctly across many
// sequential writes, on every supported platform including Windows.
func TestAtomicRenameReplacesExistingDestinationRepeatedly(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "value.json")

	for i, want := range []string{"first", "second", "third", "fourth"} {
		src := filepath.Join(dir, "source.tmp")
		if err := os.WriteFile(src, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := atomicRename(src, dst); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
		if string(data) != want {
			t.Fatalf("save %d: got %q, want %q", i, data, want)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("save %d: expected exactly one file (no leaked temp/source files), found %d", i, len(entries))
		}
	}
}

// TestAtomicRenameRejectsMissingSource ensures failures surface as an error
// rather than silently succeeding or panicking.
func TestAtomicRenameRejectsMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := atomicRename(filepath.Join(dir, "missing.tmp"), filepath.Join(dir, "value.json")); err == nil {
		t.Fatal("expected an error when the source file does not exist")
	}
}
