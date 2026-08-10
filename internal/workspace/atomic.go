package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// dirPerm and filePerm are the restrictive permissions applied when a file
// or directory is created. They are honored on POSIX filesystems; on
// Windows, os.MkdirAll/os.OpenFile largely ignore the Unix mode bits (the
// effective ACL is inherited from the parent directory), so this is
// "restrictive where portable" rather than a portable guarantee.
const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

// ErrConflict is returned when an optimistic-concurrency base digest is
// stale, or when an immutable artifact already exists with different
// content.
var ErrConflict = errors.New("workspace: conflict")

// ErrNotFound is returned when a requested draft or config file does not
// exist yet.
var ErrNotFound = errors.New("workspace: not found")

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create directory: %w", SanitizeFSError(err))
	}
	if runtime.GOOS != "windows" {
		// Best effort: MkdirAll does not narrow permissions on directories
		// that already existed with looser modes.
		_ = os.Chmod(dir, dirPerm)
	}
	return nil
}

// SanitizeFSError strips any embedded absolute filesystem path from a
// standard-library filesystem error (*fs.PathError, which os.MkdirAll,
// os.CreateTemp, os.Open, os.ReadFile, os.Link, and friends all return, and
// *os.LinkError, which os.Link/os.Rename can return with two paths)
// before it is wrapped into a message any caller might ever forward
// directly into a sanitized API error response or CLI output. errors.Is
// against the underlying sentinel (fs.ErrNotExist, fs.ErrPermission, ...)
// keeps working because the original *Err is preserved via %w; only the
// embedded Path/Old/New fields — which is where the workspace root or any
// other absolute path would leak — are dropped.
func SanitizeFSError(err error) error {
	if err == nil {
		return nil
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("%s: %w", pathErr.Op, pathErr.Err)
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return fmt.Errorf("%s: %w", linkErr.Op, linkErr.Err)
	}
	return err
}

// atomicWriteJSON marshals value as indented JSON terminated by a single
// trailing newline and atomically replaces path with the result: the file
// is written to a temporary sibling file, flushed, and renamed into place
// so readers never observe a partially written file.
func atomicWriteJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(dir, ".gamelib-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", SanitizeFSError(err))
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(filePerm); err != nil && runtime.GOOS != "windows" {
		temp.Close()
		return fmt.Errorf("restrict temporary file permissions: %w", SanitizeFSError(err))
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary file: %w", SanitizeFSError(err))
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary file: %w", SanitizeFSError(err))
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", SanitizeFSError(err))
	}
	if err := atomicRename(tempPath, path); err != nil {
		return fmt.Errorf("publish file: %w", SanitizeFSError(err))
	}
	return nil
}

// readJSONIfExists decodes path into value. It reports found=false with a
// nil error when the file does not exist.
func readJSONIfExists(path string, value any) (found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read file: %w", SanitizeFSError(err))
	}
	if err := json.Unmarshal(data, value); err != nil {
		return false, fmt.Errorf("decode file: %w", err)
	}
	return true, nil
}

// writeImmutable writes value as an immutable, create-if-absent JSON
// artifact at path. If the file already exists, its content is compared
// with the would-be new content: byte-identical content is treated as an
// idempotent success (created=false, err=nil); any difference is reported
// as ErrConflict and the existing file is left untouched. This uses a
// temporary-file-plus-hard-link pattern (rather than rename) because
// os.Link fails when the destination already exists on both Windows and
// POSIX platforms, giving true create-if-absent semantics; os.Rename does
// not offer that guarantee on POSIX, where rename(2) always replaces an
// existing destination atomically.
func writeImmutable(path string, value any) (created bool, err error) {
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(dir, ".gamelib-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create temporary file: %w", SanitizeFSError(err))
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(filePerm); err != nil && runtime.GOOS != "windows" {
		temp.Close()
		return false, fmt.Errorf("restrict temporary file permissions: %w", SanitizeFSError(err))
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return false, fmt.Errorf("write temporary file: %w", SanitizeFSError(err))
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return false, fmt.Errorf("sync temporary file: %w", SanitizeFSError(err))
	}
	if err := temp.Close(); err != nil {
		return false, fmt.Errorf("close temporary file: %w", SanitizeFSError(err))
	}

	if err := os.Link(tempPath, path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return false, fmt.Errorf("publish artifact: %w", SanitizeFSError(err))
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return false, fmt.Errorf("read existing artifact: %w", SanitizeFSError(readErr))
		}
		if string(existing) == string(data) {
			return false, nil
		}
		return false, fmt.Errorf("%w: immutable artifact already exists with different content", ErrConflict)
	}
	return true, nil
}
