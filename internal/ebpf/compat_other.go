//go:build !linux

package ebpf

import (
	"fmt"
	"runtime"
)

func unsupportedPlatform() error {
	return fmt.Errorf("%w: eBPF requires Linux, running on %s", ErrUnsupportedPlatform, runtime.GOOS)
}

// CheckSupport, HaveRingBuffer, HaveTracepoints, and HaveTracing all
// fail identically on non-Linux platforms: eBPF is a Linux kernel
// feature with no equivalent elsewhere, so there's nothing more
// specific to report per-feature.

func CheckSupport() error    { return unsupportedPlatform() }
func HaveRingBuffer() error  { return unsupportedPlatform() }
func HaveTracepoints() error { return unsupportedPlatform() }
func HaveTracing() error     { return unsupportedPlatform() }
