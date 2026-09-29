//go:build windows

package publishing

import (
	"fmt"
	"syscall"
)

// A no-sharing handle also excludes another dashboard process. Windows
// releases it after a crash, so recovery never requires deleting a stale lock.
func lockFile(name string) (func(), error) {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(ptr, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("publishing workspace is locked or inaccessible: %w", err)
	}
	return func() { _ = syscall.CloseHandle(handle) }, nil
}
