//go:build !windows

package metadata

import "os"

// openReadOnly deliberately uses os.Open on non-Windows systems.  O_RDONLY
// does not create, truncate, lock, or update a file, which is the only
// behaviour this reader needs on these platforms.
func openReadOnly(name string) (*os.File, error) {
	return os.Open(name)
}

func isReadBusy(error) bool { return false }
