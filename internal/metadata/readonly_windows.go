//go:build windows

package metadata

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	genericRead         = 0x80000000
	fileShareRead       = 0x00000001
	fileShareWrite      = 0x00000002
	fileShareDelete     = 0x00000004
	openExisting        = 3
	fileAttributeNormal = 0x80
	invalidHandleValue  = ^uintptr(0)
)

var (
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	createFileW = kernel32.NewProc("CreateFileW")
	closeHandle = kernel32.NewProc("CloseHandle")
)

// openReadOnly is intentionally not os.Open on Windows.  Playnite can have
// its database open while this process is reading it, so the handle must share
// read, write, and delete, and must never have CREATE/WRITE semantics.
func openReadOnly(name string) (*os.File, error) {
	path, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}

	handle, _, callErr := createFileW.Call(
		uintptr(unsafe.Pointer(path)),
		genericRead,
		fileShareRead|fileShareWrite|fileShareDelete,
		0,
		openExisting,
		fileAttributeNormal,
		0,
	)
	if handle == invalidHandleValue {
		return nil, callErr
	}
	file := os.NewFile(handle, name)
	if file == nil {
		// Best-effort cleanup: the open already failed, so a close failure
		// cannot be reported any more usefully than EINVAL.
		_, _, _ = closeHandle.Call(handle)
		return nil, syscall.EINVAL
	}
	return file, nil
}

func isReadBusy(err error) bool {
	return errors.Is(err, syscall.Errno(32)) ||
		errors.Is(err, syscall.Errno(33))
}
