package workspace

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Contain resolves a slash-separated relative path element against base and
// guarantees the resulting path cannot escape base. It rejects backslashes,
// drive letters, UNC-style prefixes, NUL bytes, empty segments, and any ".."
// segment so behavior is identical on Windows and Linux regardless of the
// native path separator or drive-letter conventions. Callers must pass
// forward-slash-separated relative paths (the repository's existing
// safe-path convention); OS-native separators are never accepted from
// untrusted input.
func Contain(base, elem string) (string, error) {
	if elem == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.ContainsRune(elem, 0) {
		return "", fmt.Errorf("path contains a NUL byte")
	}
	if strings.ContainsAny(elem, "\\") {
		return "", fmt.Errorf("path must use forward slashes")
	}
	if strings.HasPrefix(elem, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	if len(elem) >= 2 && elem[1] == ':' {
		return "", fmt.Errorf("path must not contain a drive letter")
	}
	for _, part := range strings.Split(elem, "/") {
		switch part {
		case "":
			return "", fmt.Errorf("path must not contain empty segments")
		case "..":
			return "", fmt.Errorf("path must not contain ..")
		}
	}
	cleanRel := path.Clean(elem)
	if cleanRel == ".." || cleanRel == "." || strings.HasPrefix(cleanRel, "../") {
		return "", fmt.Errorf("path escapes the workspace root")
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	target := filepath.Join(absBase, filepath.FromSlash(cleanRel))
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}

	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return "", fmt.Errorf("resolve relative containment: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace root")
	}
	return absTarget, nil
}
