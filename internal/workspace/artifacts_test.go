package workspace

import (
	"errors"
	"strings"
	"testing"
)

func TestWriteArtifactCreatesOnce(t *testing.T) {
	paths := testPaths(t)
	created, path, err := WriteArtifact(paths, "gate-reviews/example.json", map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected created=true for a new artifact")
	}
	if path == "" {
		t.Fatal("expected a non-empty resolved path")
	}
}

func TestWriteArtifactIsIdempotent(t *testing.T) {
	paths := testPaths(t)
	if _, _, err := WriteArtifact(paths, "example.json", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	created, _, err := WriteArtifact(paths, "example.json", map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if created {
		t.Fatal("expected created=false on a repeated identical write")
	}
}

func TestWriteArtifactConflictsOnDifferentContent(t *testing.T) {
	paths := testPaths(t)
	if _, _, err := WriteArtifact(paths, "example.json", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteArtifact(paths, "example.json", map[string]int{"a": 2}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestWriteArtifactRejectsTraversalName(t *testing.T) {
	paths := testPaths(t)
	if _, _, err := WriteArtifact(paths, "../escape.json", map[string]int{"a": 1}); err == nil {
		t.Fatal("expected an error for a traversal artifact name")
	}
}

func TestReadArtifactRoundTrips(t *testing.T) {
	paths := testPaths(t)
	if _, _, err := WriteArtifact(paths, "gate-reviews/a/example.json", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	var value map[string]int
	found, err := ReadArtifact(paths, "gate-reviews/a/example.json", &value)
	if err != nil {
		t.Fatal(err)
	}
	if !found || value["a"] != 1 {
		t.Fatalf("found=%v value=%v", found, value)
	}
}

func TestReadArtifactReportsNotFound(t *testing.T) {
	paths := testPaths(t)
	var value map[string]int
	found, err := ReadArtifact(paths, "gate-reviews/a/missing.json", &value)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected found=false for a nonexistent artifact")
	}
}

func TestRelativeArtifactNameStripsTheWorkspaceRoot(t *testing.T) {
	paths := testPaths(t)
	_, absPath, err := WriteArtifact(paths, "plans/import-plan/op-123.json", map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := RelativeArtifactName(paths, absPath)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "plans/import-plan/op-123.json" {
		t.Fatalf("rel = %q", rel)
	}
	if strings.Contains(rel, paths.Root) {
		t.Fatal("relative artifact name must never contain the workspace root")
	}
}

func TestRelativeArtifactNameRejectsPathsOutsideArtifacts(t *testing.T) {
	paths := testPaths(t)
	if _, err := RelativeArtifactName(paths, paths.Root); err == nil {
		t.Fatal("expected an error for a path outside the artifacts directory")
	}
}
