//go:build !windows

package workspace

import "os"

// atomicRename replaces newpath with oldpath. On POSIX platforms rename(2)
// already atomically replaces an existing destination, so this is a thin
// pass-through; see rename_windows.go for the Windows-specific durable
// replace-existing implementation this mirrors.
func atomicRename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
