//go:build ignore

// foundation.c exercises Pulse's eBPF load -> attach -> receive -> detach
// lifecycle end to end. It attaches to a real, frequently-firing
// tracepoint and emits a real event through a real BPF ring buffer, but
// deliberately extracts no process, network, or other telemetry
// semantics from it — the event payload is just a sequence number and a
// kernel timestamp. Turning "a process called execve" into a Pulse
// telemetry event is Day 04's job (process discovery); this file only
// proves the plumbing that day will build on. See
// docs/design/ebpf-foundation.md.
#include "common.h"

char __license[] SEC("license") = "Dual MIT/GPL";

struct heartbeat_event {
	u64 timestamp_ns;
	u64 sequence;
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 16); // 64 KiB
} heartbeats SEC(".maps");

// sequence is reset every time this program is loaded (i.e. every
// pulse-agent start) — it is not a persistent counter and carries no
// meaning across restarts.
u64 sequence = 0;

SEC("tracepoint/syscalls/sys_enter_execve")
int on_sys_enter_execve(void *ctx) {
	struct heartbeat_event *event;

	event = bpf_ringbuf_reserve(&heartbeats, sizeof(*event), 0);
	if (!event) {
		// Ring buffer full, or the map isn't ready yet: drop the
		// event. There is no backpressure signal from kernel to
		// producer here by design — the ring buffer's fixed size is
		// the only bound, matching the "bounded buffers" requirement
		// for kernel-level capture.
		return 0;
	}

	event->timestamp_ns = bpf_ktime_get_ns();
	event->sequence = __sync_fetch_and_add(&sequence, 1);

	bpf_ringbuf_submit(event, 0);
	return 0;
}
