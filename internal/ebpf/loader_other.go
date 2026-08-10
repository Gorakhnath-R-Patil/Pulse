//go:build !linux

package ebpf

// Loader mirrors the Linux implementation's API on every other
// platform: every method fails with an error wrapping
// ErrUnsupportedPlatform without touching any OS resource. This lets
// code elsewhere in the module construct and hold a *Loader on any OS
// without its own build tags — only this package needs to know eBPF is
// Linux-only.
type Loader struct{}

// NewLoader returns a Loader. Construction never fails; every method it
// exposes does, immediately, since eBPF has no equivalent on this
// platform.
func NewLoader() *Loader { return &Loader{} }

func (l *Loader) Load() error {
	return CheckSupport()
}

func (l *Loader) Attach() error {
	return CheckSupport()
}

func (l *Loader) Read() (HeartbeatEvent, error) {
	return HeartbeatEvent{}, CheckSupport()
}

// Close always succeeds: there is never anything to release.
func (l *Loader) Close() error {
	return nil
}
