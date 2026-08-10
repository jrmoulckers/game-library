package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

// WriteArtifact writes value as an immutable, create-if-absent JSON
// artifact under paths.Artifacts. name is a slash-separated relative path
// (for example "gate-reviews/<id>.json") and is contained within the
// artifacts directory before anything is written. Re-submitting the exact
// same content is idempotent (created=false, err=nil); submitting
// different content for an existing name returns ErrConflict.
//
// The returned path is an absolute filesystem path: it exists so callers in
// this process (for example this package's own tests, or a caller that
// needs to read the file back) can open it directly. It must never be
// forwarded verbatim into an API response — callers that need to hand a
// symbolic reference to an untrusted client should use the already-known
// relative name (or RelativeArtifactName) instead.
func WriteArtifact(paths Paths, name string, value any) (created bool, path string, err error) {
	path, err = Contain(paths.Artifacts, name)
	if err != nil {
		return false, "", fmt.Errorf("artifact name %q is invalid: %w", name, err)
	}
	created, err = writeImmutable(path, value)
	if err != nil {
		return false, path, err
	}
	return created, path, nil
}

// ReadArtifact decodes the immutable JSON artifact named name (relative to
// paths.Artifacts, contained the same way WriteArtifact contains it) into
// value. It reports found=false with a nil error when no such artifact
// exists yet.
func ReadArtifact(paths Paths, name string, value any) (found bool, err error) {
	path, err := Contain(paths.Artifacts, name)
	if err != nil {
		return false, fmt.Errorf("artifact name %q is invalid: %w", name, err)
	}
	return readJSONIfExists(path, value)
}

// RelativeArtifactName converts an absolute artifact path (as returned by
// WriteArtifact) back into the slash-separated name relative to
// paths.Artifacts that was originally passed in. It is the only safe way to
// turn a WriteArtifact result into something that may be returned to an API
// client: the result never contains the workspace root or any other
// absolute filesystem detail.
func RelativeArtifactName(paths Paths, absPath string) (string, error) {
	rel, err := filepath.Rel(paths.Artifacts, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve artifact reference: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", fmt.Errorf("resolve artifact reference: path is not contained within the artifacts directory")
	}
	return rel, nil
}
