//go:build !windows && !linux

package publishing

import "fmt"

func lockFile(_ string) (func(), error) {
	return nil, fmt.Errorf("publishing locks are supported only on Windows and Linux")
}
