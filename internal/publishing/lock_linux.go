//go:build linux

package publishing

import (
	"fmt"
	"os"
	"syscall"
)

func lockFile(name string) (func(), error) {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("publishing workspace is inaccessible")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("publishing workspace is locked: %w", err)
	}
	return func() { _ = file.Close() }, nil
}
