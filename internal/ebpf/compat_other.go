//go:build !linux

package ebpf

import (
	"fmt"
	"runtime"
)

// CheckSupport always fails on non-Linux platforms: eBPF is a Linux
// kernel feature with no equivalent elsewhere.
func CheckSupport() error {
	return fmt.Errorf("%w: eBPF requires Linux, running on %s", ErrUnsupportedPlatform, runtime.GOOS)
}
