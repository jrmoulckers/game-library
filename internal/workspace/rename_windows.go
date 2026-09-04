//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// moveFileExW is resolved lazily from kernel32.dll using only the standard
// library (syscall.NewLazyDLL/NewProc), so this file adds no third-party
// dependency.
var (
	modkernel32     = syscall.NewLazyDLL("kernel32.dll")
	procMoveFileExW = modkernel32.NewProc("MoveFileExW")
)

// Flags for the dwFlags parameter of MoveFileExW. See
// https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-movefileexw
const (
	movefileReplaceExisting = 0x00000001
	movefileWriteThrough    = 0x00000008
)

// atomicRename replaces newpath with oldpath. Its error messages
// deliberately never include oldpath/newpath: both are absolute
// filesystem paths under the workspace root, and this function's errors
// can end up wrapped into a message a caller forwards toward an API
// response (see workspace.atomicWriteJSON and sanitizeFSError).
//
// Go's os.Rename on Windows calls MoveFileExW with only
// MOVEFILE_REPLACE_EXISTING (via internal/syscall/windows), which is not
// guaranteed to flush the rename's directory-entry update to stable storage
// before returning. This local replacement additionally sets
// MOVEFILE_WRITE_THROUGH so the call does not return until the rename is
// durable, matching the durability atomicWriteJSON already gets from
// File.Sync on the temporary file's contents before the rename. Using our
// own stdlib-only syscall (rather than relying on os.Rename's internal
// behavior, which is not part of any Go compatibility promise) keeps this
// guarantee independent of future stdlib changes.
func atomicRename(oldpath, newpath string) error {
	oldptr, err := syscall.UTF16PtrFromString(oldpath)
	if err != nil {
		return fmt.Errorf("encode rename source path: %w", err)
	}
	newptr, err := syscall.UTF16PtrFromString(newpath)
	if err != nil {
		return fmt.Errorf("encode rename destination path: %w", err)
	}
	r1, _, callErr := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(oldptr)),
		uintptr(unsafe.Pointer(newptr)),
		uintptr(movefileReplaceExisting|movefileWriteThrough),
	)
	if r1 == 0 {
		var errno syscall.Errno
		if errors.As(callErr, &errno) && errno != 0 {
			return fmt.Errorf("rename: %w", errno)
		}
		return fmt.Errorf("rename: MoveFileExW failed")
	}
	return nil
}
