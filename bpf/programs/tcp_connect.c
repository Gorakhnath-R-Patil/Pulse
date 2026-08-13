//go:build ignore

// tcp_connect.c captures outbound IPv4 TCP connection attempts: which
// process initiated a connection, from which local address/port, to
// which remote address/port, and whether the connect ultimately
// succeeded (ret == 0) or failed (e.g. ECONNREFUSED, ETIMEDOUT).
//
// This uses an fexit (BPF_TRACE_FEXIT, "tracing") program rather than a
// kprobe/kretprobe pair. fexit gives both tcp_v4_connect's original
// arguments and its return value in a single, BTF-typed callback, and
// -- because its `sk` argument is a BTF-trusted pointer -- lets socket
// fields be read directly, without bpf_probe_read_kernel. That avoids
// both a kprobe/kretprobe correlation map and the architecture-specific
// pt_regs handling process.c's kernel-side counterpart never needed
// either. IPv6 (tcp_v6_connect) is not supported yet -- see
// docs/design/network-connect.md.
#include "common.h"
#include "bpf_tracing.h"

char __license[] SEC("license") = "Dual MIT/GPL";

// Minimal CO-RE structs: only the fields this program reads. See
// process.c for the same pattern applied to task_struct.
struct sock_common {
	union {
		struct {
			// Network byte order (big-endian on every arch);
			// read directly as raw address bytes in Go, no
			// conversion needed there either.
			__be32 skc_daddr;
			__be32 skc_rcv_saddr;
		};
	};
	union {
		struct {
			// Network byte order: the *destination* port is
			// stored big-endian.
			__be16 skc_dport;
			// Host byte order: unlike skc_dport, the kernel
			// keeps the *local* port in host-native order
			// because it compares this field directly in hot
			// paths. This asymmetry is real kernel behavior,
			// not a mistake -- see docs/design/network-connect.md.
			__u16 skc_num;
		};
	};
	short unsigned int skc_family;
} __attribute__((preserve_access_index));

struct sock {
	struct sock_common __sk_common;
} __attribute__((preserve_access_index));

struct connect_event {
	u64 timestamp_ns;
	u32 pid;
	u32 saddr; // raw network-byte-order bytes; decode as 4 address octets, not an integer
	u32 daddr; // same
	u16 sport; // host byte order
	u16 dport; // network byte order
	u8 comm[16];
	u8 success; // 1 if tcp_v4_connect returned 0, else 0
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 16); // 64 KiB
} connect_events SEC(".maps");

SEC("fexit/tcp_v4_connect")
int BPF_PROG(on_tcp_v4_connect, struct sock *sk, struct sockaddr *uaddr, int addr_len, int ret) {
	struct connect_event *event;

	event = bpf_ringbuf_reserve(&connect_events, sizeof(*event), 0);
	if (!event) {
		// Ring buffer full: drop the event. No backpressure to the
		// connecting process, matching every other program here.
		return 0;
	}

	event->timestamp_ns = bpf_ktime_get_ns();
	event->pid = (u32)(bpf_get_current_pid_tgid() >> 32);
	event->success = (ret == 0) ? 1 : 0;
	event->saddr = sk->__sk_common.skc_rcv_saddr;
	event->daddr = sk->__sk_common.skc_daddr;
	event->sport = sk->__sk_common.skc_num;
	event->dport = sk->__sk_common.skc_dport;
	bpf_get_current_comm(&event->comm, sizeof(event->comm));

	bpf_ringbuf_submit(event, 0);
	return 0;
}
