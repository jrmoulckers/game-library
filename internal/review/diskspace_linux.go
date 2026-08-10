//go:build linux

package review

import "syscall"

// platformDiskFreeBytes uses syscall.Statfs (available in the standard
// library on Linux) to report the free space available to an unprivileged
// caller (Bavail, not the raw Bfree, which can include space reserved for
// root) on the filesystem containing path.
func platformDiskFreeBytes(path string) (int64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, false
	}
	// Bsize/Bavail widths vary across the linux/GOARCH build matrix (int64
	// vs int32/uint32 depending on architecture); converting through
	// uint64 keeps this file portable across all of them without per-arch
	// build tags.
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if available > 1<<62 {
		return 0, false
	}
	return int64(available), true
}
