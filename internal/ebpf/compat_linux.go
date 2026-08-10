//go:build linux

package ebpf

import (
	"fmt"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
)

// CheckSupport verifies the running kernel supports what this package's
// foundation program needs: BPF ring buffers (Linux 5.8+) as the event
// transport, and tracepoint programs as the attachment mechanism. It
// probes the running kernel directly via cilium/ebpf's feature-detection
// package (which attempts the real syscall and inspects the result)
// rather than parsing a kernel version string — version numbers are an
// unreliable proxy for what a specific kernel build actually supports
// (backports, distro patches, disabled configs).
func CheckSupport() error {
	if err := features.HaveMapType(cebpf.RingBuf); err != nil {
		return fmt.Errorf("%w: BPF ring buffer maps: %v", ErrUnsupportedKernel, err)
	}
	if err := features.HaveProgramType(cebpf.TracePoint); err != nil {
		return fmt.Errorf("%w: tracepoint programs: %v", ErrUnsupportedKernel, err)
	}
	return nil
}
