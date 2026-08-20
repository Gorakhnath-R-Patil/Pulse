//go:build !linux

package socket

import "github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"

// Loader mirrors the Linux implementation's API on every other
// platform: every method fails with an error wrapping
// ebpf.ErrUnsupportedPlatform without touching any OS resource. See
// internal/ebpf's Loader for the rationale behind this pattern.
type Loader struct{}

// NewLoader returns a Loader. Construction never fails; every method it
// exposes does, immediately, since eBPF has no equivalent on this
// platform.
func NewLoader() *Loader { return &Loader{} }

func (l *Loader) Load() error {
	return ebpf.CheckSupport()
}

func (l *Loader) Attach() error {
	return ebpf.CheckSupport()
}

func (l *Loader) Read() (CloseEvent, error) {
	return CloseEvent{}, ebpf.CheckSupport()
}

// Close always succeeds: there is never anything to release.
func (l *Loader) Close() error {
	return nil
}
