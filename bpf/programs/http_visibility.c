//go:build ignore

// http_visibility.c captures cleartext HTTP request/response lines by
// inspecting the first bytes passed to write(2): if they look like an
// HTTP request line (starts with a known method) or a status line
// (starts with "HTTP/"), a bounded prefix is captured for userspace to
// parse into structured fields (method, path, status) — the raw bytes
// are never persisted beyond that parse step (see internal/httpvis),
// and non-HTTP writes are never even captured (see classify_prefix).
// TLS-encrypted HTTPS traffic is invisible to this technique entirely
// — see docs/design/http-visibility.md.
//
// This hooks sys_enter_write (a tracepoint) rather than tcp_sendmsg or
// vfs_write (fentry/kprobe on a kernel function taking a struct msghdr
// or file*): the syscall's own arguments hand over a userspace buffer
// pointer and length directly, with no msghdr/iov_iter internals to
// navigate. See ADR-008 in docs/design/decisions.md for why this
// project specifically preferred a tracepoint here over repeating
// Day 05/06's fentry approach.
#include "common.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define CAPTURE_LEN 256
#define PREFIX_LEN 8

struct http_event {
	u64 timestamp_ns;
	u32 pid;
	u32 size;         // total bytes passed to write(), which may exceed what's captured
	u16 captured_len; // how many bytes of data[] are meaningfully captured
	u8 comm[16];
	u8 data[CAPTURE_LEN];
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 18); // 256 KiB: these events are larger than this project's others
} http_events SEC(".maps");

// trace_event_raw_sys_enter mirrors every sys_enter_* tracepoint's
// stable, common layout (ftrace's syscall-entry format): a fixed header
// followed by up to six raw syscall arguments. This needs no CO-RE —
// it isn't a kernel-internal struct subject to layout changes across
// configs, it's ftrace's own long-stable tracing ABI, the same one
// foundation.c and process.c already rely on (they just didn't need to
// read args[] to do it).
struct trace_event_raw_sys_enter {
	u16 common_type;
	u8 common_flags;
	u8 common_preempt_count;
	s32 common_pid;
	s64 __syscall_nr;
	u64 args[6];
};

static __always_inline int classify_prefix(const char prefix[PREFIX_LEN]) {
	if (prefix[0] == 'G' && prefix[1] == 'E' && prefix[2] == 'T' && prefix[3] == ' ')
		return 1;
	if (prefix[0] == 'P' && prefix[1] == 'O' && prefix[2] == 'S' && prefix[3] == 'T' && prefix[4] == ' ')
		return 1;
	if (prefix[0] == 'P' && prefix[1] == 'U' && prefix[2] == 'T' && prefix[3] == ' ')
		return 1;
	if (prefix[0] == 'H' && prefix[1] == 'T' && prefix[2] == 'T' && prefix[3] == 'P' && prefix[4] == '/')
		return 1;
	return 0;
}

// write(int fd, const void *buf, size_t count): args[0]=fd, args[1]=buf,
// args[2]=count.
SEC("tracepoint/syscalls/sys_enter_write")
int on_sys_enter_write(struct trace_event_raw_sys_enter *ctx) {
	const char *buf = (const char *)ctx->args[1];
	u64 count = ctx->args[2];

	if (count < PREFIX_LEN) {
		// Too short to be a meaningful HTTP request/status line.
		return 0;
	}

	char prefix[PREFIX_LEN];
	if (bpf_probe_read_user(&prefix, sizeof(prefix), buf) < 0) {
		// buf isn't readable right now (e.g. this write() call isn't
		// actually backed by a plain user buffer) -- nothing to report.
		return 0;
	}

	if (!classify_prefix(prefix)) {
		return 0;
	}

	struct http_event *event = bpf_ringbuf_reserve(&http_events, sizeof(*event), 0);
	if (!event) {
		// Ring buffer full: drop the event, same as every other
		// program here.
		return 0;
	}

	event->timestamp_ns = bpf_ktime_get_ns();
	event->pid = (u32)(bpf_get_current_pid_tgid() >> 32);
	event->size = (u32)count;

	u64 to_capture = count < CAPTURE_LEN ? count : CAPTURE_LEN;
	if (bpf_probe_read_user(&event->data, to_capture, buf) < 0) {
		event->captured_len = 0;
	} else {
		event->captured_len = (u16)to_capture;
	}

	bpf_get_current_comm(&event->comm, sizeof(event->comm));

	bpf_ringbuf_submit(event, 0);
	return 0;
}
