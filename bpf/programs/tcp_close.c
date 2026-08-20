//go:build ignore

// tcp_close.c captures IPv4 TCP connection lifecycle end and the byte
// counters accumulated over the connection's life: which process owned
// the socket, its endpoints (so an event here can be correlated with
// the tcp_connect.c event that started the same connection), how many
// bytes were sent and received, and any pending socket error at close
// time.
//
// Like tcp_connect.c, this uses fentry (BPF_TRACE_FENTRY) rather than a
// kprobe: tcp_close's sk argument is a BTF-trusted pointer, so its
// fields (and, via bpf_skc_to_tcp_sock, the wider tcp_sock it's embedded
// in) can be read directly. This mirrors cilium/ebpf's own tcprtt.c
// example almost exactly -- see docs/design/socket-data.md.
#include "common.h"
#include "bpf_tracing.h"

char __license[] SEC("license") = "Dual MIT/GPL";

// Minimal CO-RE structs: only the fields this program reads. See
// tcp_connect.c for sock_common; sk_err and tcp_sock's byte counters
// are new here.
struct sock_common {
	union {
		struct {
			__be32 skc_daddr;
			__be32 skc_rcv_saddr;
		};
	};
	union {
		struct {
			__be16 skc_dport;
			__u16 skc_num;
		};
	};
	short unsigned int skc_family;
} __attribute__((preserve_access_index));

struct sock {
	struct sock_common __sk_common;
	int sk_err;
} __attribute__((preserve_access_index));

// tcp_sock embeds sock (and several layers between it and sock in the
// real kernel struct) as its first member, which is what makes
// bpf_skc_to_tcp_sock's cast from `struct sock *` valid -- see the
// kernel's own struct tcp_sock definition.
struct tcp_sock {
	u64 bytes_sent;
	u64 bytes_received;
} __attribute__((preserve_access_index));

struct close_event {
	u64 timestamp_ns;
	u32 pid;
	u32 saddr;
	u32 daddr;
	u16 sport;
	u16 dport;
	u64 bytes_sent;
	u64 bytes_received;
	u8 comm[16];
	// 0 if the socket had no pending error at close, else sk->sk_err
	// (a positive errno value, e.g. ECONNRESET).
	u32 sk_err;
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 16); // 64 KiB
} close_events SEC(".maps");

SEC("fentry/tcp_close")
int BPF_PROG(on_tcp_close, struct sock *sk) {
	struct tcp_sock *ts = bpf_skc_to_tcp_sock(sk);
	if (!ts) {
		// Not actually a TCP socket (or the kernel couldn't validate
		// it as one) -- nothing to report.
		return 0;
	}

	struct close_event *event;
	event = bpf_ringbuf_reserve(&close_events, sizeof(*event), 0);
	if (!event) {
		// Ring buffer full: drop the event, same as every other
		// program here.
		return 0;
	}

	event->timestamp_ns = bpf_ktime_get_ns();
	event->pid = (u32)(bpf_get_current_pid_tgid() >> 32);
	event->saddr = sk->__sk_common.skc_rcv_saddr;
	event->daddr = sk->__sk_common.skc_daddr;
	event->sport = sk->__sk_common.skc_num;
	event->dport = sk->__sk_common.skc_dport;
	event->bytes_sent = ts->bytes_sent;
	event->bytes_received = ts->bytes_received;
	event->sk_err = (u32)sk->sk_err;
	bpf_get_current_comm(&event->comm, sizeof(event->comm));

	bpf_ringbuf_submit(event, 0);
	return 0;
}
