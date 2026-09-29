package publishing

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrmoulckers/game-library/internal/workspace"
)

// safePath rejects links in every existing ancestor, including the root.
// It deliberately does not follow links even when they point inside the root.
func safePath(root, relative string, allowAbsent bool) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", fmt.Errorf("absolute local root required")
	}
	for _, part := range strings.Split(relative, "/") {
		for _, r := range part {
			if r < 32 {
				return "", fmt.Errorf("control characters are not allowed in filenames")
			}
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if part == "." || strings.ContainsAny(part, ":<>\"|?*") || strings.HasSuffix(part, ".") ||
			strings.HasSuffix(part, " ") || stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" ||
			(len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
			return "", fmt.Errorf("unsafe portable filename")
		}
	}
	full, err := workspace.Contain(root, relative)
	if err != nil {
		return "", err
	}
	current := full
	for {
		info, err := os.Lstat(current)
		if err != nil {
			if !os.IsNotExist(err) || !allowAbsent {
				return "", workspace.SanitizeFSError(err)
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symbolic links are not allowed")
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return full, nil
}

func hashFile(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", workspace.SanitizeFSError(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a readable regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", workspace.SanitizeFSError(err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func observedHash(root, relative string) (string, error) {
	name, err := safePath(root, relative, true)
	if err != nil {
		return "", err
	}
	hash, err := hashFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return hash, err
}

// copyAtomic never opens a destination for in-place writing. The old bytes
// remain intact if reading, hashing, flushing or replacement fails.
func copyAtomic(source, destination, expected string) error {
	input, err := os.Open(source)
	if err != nil {
		return workspace.SanitizeFSError(err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return workspace.SanitizeFSError(err)
	}
	output, err := os.CreateTemp(filepath.Dir(destination), ".gamelib-copy-*.tmp")
	if err != nil {
		return workspace.SanitizeFSError(err)
	}
	defer os.Remove(output.Name())
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, h), input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return workspace.SanitizeFSError(copyErr)
	}
	if syncErr != nil {
		return workspace.SanitizeFSError(syncErr)
	}
	if closeErr != nil {
		return workspace.SanitizeFSError(closeErr)
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return fmt.Errorf("source changed while copying; destination untouched")
	}
	return workspace.ReplaceFile(output.Name(), destination)
}

func overlaps(left, right string) bool {
	leftNative, rightNative := filepath.Clean(left), filepath.Clean(right)
	left = strings.ToLower(leftNative)
	right = strings.ToLower(rightNative)
	separator := string(filepath.Separator)
	leftPrefix := strings.TrimRight(left, separator) + separator
	rightPrefix := strings.TrimRight(right, separator) + separator
	if left == right || strings.HasPrefix(left, rightPrefix) ||
		strings.HasPrefix(right, leftPrefix) {
		return true
	}
	// Directory identity also catches drive aliases and alternate Windows
	// path spellings that lexical containment alone cannot distinguish.
	return ancestorIs(leftNative, rightNative) || ancestorIs(rightNative, leftNative)
}

func ancestorIs(descendant, ancestor string) bool {
	expected, err := os.Stat(ancestor)
	if err != nil {
		return false
	}
	for current := descendant; ; current = filepath.Dir(current) {
		if info, err := os.Stat(current); err == nil && os.SameFile(info, expected) {
			return true
		}
		if filepath.Dir(current) == current {
			return false
		}
	}
}

func collisionCheck(root string, files []File) error {
	wanted := map[string]string{}
	for _, file := range files {
		if file.Blocked {
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(file.Path, filepath.Ext(file.Path)))
		if previous, ok := wanted[key]; ok {
			return fmt.Errorf("export filename/role collision: %s and %s", previous, file.Path)
		}
		wanted[key] = file.Path
	}
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return workspace.SanitizeFSError(err)
	}
	for _, entry := range entries {
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg", ".ico", ".webp", ".gif":
		default:
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if intended, ok := wanted[key]; ok && entry.Name() != intended {
			return fmt.Errorf("target filename/extension collision with %s", intended)
		}
	}
	return nil
}
