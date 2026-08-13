//go:build linux

package ebpf

import (
	"fmt"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
)

// HaveRingBuffer reports whether the running kernel supports BPF ring
// buffer maps (Linux 5.8+), the event transport every program under
// bpf/programs/ uses.
func HaveRingBuffer() error {
	if err := features.HaveMapType(cebpf.RingBuf); err != nil {
		return fmt.Errorf("%w: BPF ring buffer maps: %v", ErrUnsupportedKernel, err)
	}
	return nil
}

// HaveTracepoints reports whether the running kernel supports
// tracepoint-attached BPF programs, as used by
// bpf/programs/foundation.c and bpf/programs/process.c.
func HaveTracepoints() error {
	if err := features.HaveProgramType(cebpf.TracePoint); err != nil {
		return fmt.Errorf("%w: tracepoint programs: %v", ErrUnsupportedKernel, err)
	}
	return nil
}

// HaveTracing reports whether the running kernel supports BTF-based
// "tracing" programs — fentry/fexit/fmod_ret — as used by
// bpf/programs/tcp_connect.c. This is a newer kernel feature (5.5+)
// than tracepoint support, so it is checked separately rather than
// folded into CheckSupport, which existing tracepoint-only programs
// call and shouldn't be made to fail on a kernel that only lacks this.
func HaveTracing() error {
	if err := features.HaveProgramType(cebpf.Tracing); err != nil {
		return fmt.Errorf("%w: fentry/fexit tracing programs: %v", ErrUnsupportedKernel, err)
	}
	return nil
}

// CheckSupport verifies the running kernel supports what
// bpf/programs/foundation.c and bpf/programs/process.c need: ring
// buffers and tracepoint programs. It probes the running kernel
// directly via cilium/ebpf's feature-detection package (which attempts
// the real map/program creation the kernel would reject if unsupported)
// rather than parsing a kernel version string — version numbers are an
// unreliable proxy for what a specific kernel build actually supports
// (backports, distro patches, disabled configs).
//
// A caller whose program needs a different combination of features
// (e.g. bpf/programs/tcp_connect.c, which needs HaveRingBuffer and
// HaveTracing but not HaveTracepoints) should call the more specific
// Have* functions directly instead of this one.
func CheckSupport() error {
	if err := HaveRingBuffer(); err != nil {
		return err
	}
	return HaveTracepoints()
}
