//go:build ignore

// dns_telemetry.c captures DNS queries and responses sent/received over
// UDP. Queries are captured at sys_enter_sendto — the payload is
// already in the caller's buffer, ready to send, at that point.
// Responses need sys_enter_recvfrom (to stash the destination buffer
// pointer) paired with sys_exit_recvfrom: unlike sendto, recvfrom's
// buffer doesn't hold anything meaningful until the syscall actually
// returns, and a sys_exit_* tracepoint's own context carries only a
// return value, not the original arguments — so the buffer pointer has
// to be captured at enter time and looked back up at exit time. This is
// the same shape as a kprobe/kretprobe correlation map, just built on
// tracepoints instead — see ADR-008 in docs/design/decisions.md for why
// this project prefers tracepoints here.
//
// No attempt is made to filter to port-53 traffic in-kernel: a UDP
// socket connect()ed once to a resolver, then used with plain
// send()/recv(), never passes a destination address to sendto/recvfrom
// at all, so port-based filtering in-kernel would miss exactly that
// case. Filtering happens entirely in userspace instead, by attempting
// to parse every captured UDP payload as a DNS message and discarding
// what doesn't parse — see docs/design/dns-telemetry.md's Tradeoffs for
// the overhead this accepts, and internal/dns for the actual parsing.
#include "common.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define CAPTURE_LEN 512

struct dns_event {
	u64 timestamp_ns;
	u32 pid;
	u16 size;
	u16 captured_len;
	u8 comm[16];
	u8 data[CAPTURE_LEN];
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 18); // 256 KiB
} dns_events SEC(".maps");

// Correlates sys_enter_recvfrom's destination buffer pointer with the
// matching sys_exit_recvfrom, keyed by the calling thread's pid_tgid —
// the two fire on the same thread's stack, back to back, with nothing
// else able to run recvfrom on that thread in between.
struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1 << 14);
	__type(key, u64);   // pid_tgid
	__type(value, u64); // buf pointer, stashed at sys_enter_recvfrom
} pending_recvfrom SEC(".maps");

struct trace_event_raw_sys_enter {
	u16 common_type;
	u8 common_flags;
	u8 common_preempt_count;
	s32 common_pid;
	s64 __syscall_nr;
	u64 args[6];
};

struct trace_event_raw_sys_exit {
	u16 common_type;
	u8 common_flags;
	u8 common_preempt_count;
	s32 common_pid;
	s64 __syscall_nr;
	s64 ret;
};

static __always_inline void emit_dns_event(const char *buf, u32 size) {
	struct dns_event *event = bpf_ringbuf_reserve(&dns_events, sizeof(*event), 0);
	if (!event) {
		// Ring buffer full: drop the event, same as every other
		// program here.
		return;
	}

	u32 to_capture = size < CAPTURE_LEN ? size : CAPTURE_LEN;
	if (bpf_probe_read_user(&event->data, to_capture, buf) < 0) {
		// buf isn't readable right now; discard rather than submit a
		// half-populated event. A reserved-but-unresolved record must
		// be either submitted or discarded, never abandoned -- an
		// abandoned one would block every later record behind it.
		bpf_ringbuf_discard(event, 0);
		return;
	}

	event->timestamp_ns = bpf_ktime_get_ns();
	event->pid = (u32)(bpf_get_current_pid_tgid() >> 32);
	event->size = (u16)size;
	event->captured_len = (u16)to_capture;
	bpf_get_current_comm(&event->comm, sizeof(event->comm));

	bpf_ringbuf_submit(event, 0);
}

// sendto(int fd, const void *buf, size_t len, ...): args[1]=buf,
// args[2]=len.
SEC("tracepoint/syscalls/sys_enter_sendto")
int on_sys_enter_sendto(struct trace_event_raw_sys_enter *ctx) {
	u64 len = ctx->args[2];
	if (len < 12 || len > CAPTURE_LEN) {
		// Shorter than a DNS header, or larger than anything captured
		// anyway -- not worth reserving ring buffer space for.
		return 0;
	}
	emit_dns_event((const char *)ctx->args[1], (u32)len);
	return 0;
}

// recvfrom(int fd, void *buf, size_t len, ...): args[1]=buf. Stashed
// here; actually read once on_sys_exit_recvfrom confirms it's populated.
SEC("tracepoint/syscalls/sys_enter_recvfrom")
int on_sys_enter_recvfrom(struct trace_event_raw_sys_enter *ctx) {
	u64 pid_tgid = bpf_get_current_pid_tgid();
	u64 buf = ctx->args[1];
	bpf_map_update_elem(&pending_recvfrom, &pid_tgid, &buf, BPF_ANY);
	return 0;
}

SEC("tracepoint/syscalls/sys_exit_recvfrom")
int on_sys_exit_recvfrom(struct trace_event_raw_sys_exit *ctx) {
	u64 pid_tgid = bpf_get_current_pid_tgid();
	u64 *bufp = bpf_map_lookup_elem(&pending_recvfrom, &pid_tgid);
	if (!bufp) {
		return 0;
	}
	u64 buf = *bufp;
	bpf_map_delete_elem(&pending_recvfrom, &pid_tgid);

	s64 ret = ctx->ret;
	if (ret < 12 || ret > CAPTURE_LEN) {
		// Negative/zero return means recvfrom failed or read nothing;
		// too-short/too-long means not worth treating as DNS.
		return 0;
	}

	emit_dns_event((const char *)buf, (u32)ret);
	return 0;
}
