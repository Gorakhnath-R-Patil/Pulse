package ebpf_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

func TestHaveFunctions_UnsupportedOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("expected to succeed on a compatible Linux kernel; see loader_linux_test.go")
	}

	for name, fn := range map[string]func() error{
		"HaveRingBuffer":  ebpf.HaveRingBuffer,
		"HaveTracepoints": ebpf.HaveTracepoints,
		"HaveTracing":     ebpf.HaveTracing,
	} {
		if err := fn(); !errors.Is(err, ebpf.ErrUnsupportedPlatform) {
			t.Errorf("%s() on %s: error = %v, want it to wrap ErrUnsupportedPlatform", name, runtime.GOOS, err)
		}
	}
}
