package ebpf_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

// These tests exercise the platform-independent contract: on any OS
// other than Linux, every operation fails closed with
// ErrUnsupportedPlatform rather than compiling away or panicking. The
// real Linux behavior (loading, attaching, receiving, detaching for
// real) is covered by loader_linux_test.go, which only builds and runs
// on Linux.

func TestCheckSupport_UnsupportedOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("CheckSupport is expected to succeed on a compatible Linux kernel; see loader_linux_test.go")
	}

	err := ebpf.CheckSupport()
	if !errors.Is(err, ebpf.ErrUnsupportedPlatform) {
		t.Fatalf("CheckSupport() on %s: error = %v, want it to wrap ErrUnsupportedPlatform", runtime.GOOS, err)
	}
}

func TestLoader_FailsClosedOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Loader is expected to work on a compatible Linux kernel with sufficient privilege; see loader_linux_test.go")
	}

	l := ebpf.NewLoader()
	defer l.Close()

	if err := l.Load(); !errors.Is(err, ebpf.ErrUnsupportedPlatform) {
		t.Errorf("Load() error = %v, want it to wrap ErrUnsupportedPlatform", err)
	}
	if err := l.Attach(); !errors.Is(err, ebpf.ErrUnsupportedPlatform) {
		t.Errorf("Attach() error = %v, want it to wrap ErrUnsupportedPlatform", err)
	}
	if _, err := l.Read(); !errors.Is(err, ebpf.ErrUnsupportedPlatform) {
		t.Errorf("Read() error = %v, want it to wrap ErrUnsupportedPlatform", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil (nothing was ever opened)", err)
	}
}
