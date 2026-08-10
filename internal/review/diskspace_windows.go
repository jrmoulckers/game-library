//go:build windows

package review

import (
	"errors"
	"syscall"
	"unsafe"
)

// modkernel32/procGetDiskFreeSpaceExW are resolved lazily using only the
// standard library (syscall.NewLazyDLL/NewProc), matching the pattern
// internal/workspace/rename_windows.go already uses for MoveFileExW: no
// third-party dependency is added for this stdlib-only platform call.
var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = modkernel32.NewProc("GetDiskFreeSpaceExW")
)

// platformDiskFreeBytes calls GetDiskFreeSpaceExW for the volume containing
// path and returns the caller's free byte count (lpFreeBytesAvailable,
// which already accounts for per-user quotas, unlike the raw
// lpTotalNumberOfFreeBytes). See
// https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw
func platformDiskFreeBytes(path string) (int64, bool) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r1, _, callErr := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		// Call always returns a non-nil error (it reports the thread's last
		// errno even on success), so r1 is the only reliable success signal.
		// The errno is inspected for clarity; callers only get the bool.
		var errno syscall.Errno
		_ = errors.As(callErr, &errno)
		return 0, false
	}
	if freeBytesAvailable > 1<<62 {
		// Defensive: never return an implausible value that could look
		// negative once converted to int64.
		return 0, false
	}
	return int64(freeBytesAvailable), true
}
