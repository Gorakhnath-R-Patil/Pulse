//go:build ignore

// process.c implements Pulse's process discovery: process start (exec)
// and process exit lifecycle events, with PID, parent PID, and the
// kernel-truncated command name captured in-kernel at the moment each
// event occurs. Both are read here rather than resolved later in
// userspace, because a process's task_struct is only guaranteed to
// still exist at the moment of the event itself -- this matters
// especially for process exit: by the time userspace processes the
// event, the process is already gone.
//
// The executable's full path is deliberately NOT captured here -- see
// docs/design/process-discovery.md for why, and
// internal/process.ResolveExecutable for the best-effort userspace
// enrichment used instead.
#include "common.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define TASK_COMM_LEN 16

struct process_event {
	u64 timestamp_ns;
	u32 pid;
	u32 ppid;
	u8 comm[TASK_COMM_LEN];
	u8 event_type; // 0 = process start (exec), 1 = process exit
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 16); // 64 KiB
} process_events SEC(".maps");

// Minimal CO-RE struct: only the two fields this program actually
// reads. __attribute__((preserve_access_index)) lets clang emit CO-RE
// relocations for these field accesses without vendoring a full
// vmlinux.h -- see docs/design/process-discovery.md.
struct task_struct {
	int tgid;
	struct task_struct *real_parent;
} __attribute__((preserve_access_index));

static __always_inline u32 get_ppid(void) {
	struct task_struct *task = (struct task_struct *)bpf_get_current_task();
	struct task_struct *parent = NULL;
	u32 ppid = 0;

	// task came from bpf_get_current_task(), not a BTF-trusted context
	// argument, so the verifier requires bpf_probe_read_kernel to
	// dereference it rather than a plain pointer access.
	bpf_probe_read_kernel(&parent, sizeof(parent), &task->real_parent);
	if (parent)
		bpf_probe_read_kernel(&ppid, sizeof(ppid), &parent->tgid);

	return ppid;
}

static __always_inline void emit(u8 event_type) {
	struct process_event *event;

	event = bpf_ringbuf_reserve(&process_events, sizeof(*event), 0);
	if (!event) {
		// Ring buffer full: drop the event. No backpressure to the
		// traced process, matching foundation.c's precedent.
		return;
	}

	event->timestamp_ns = bpf_ktime_get_ns();
	event->pid = (u32)(bpf_get_current_pid_tgid() >> 32);
	event->ppid = get_ppid();
	bpf_get_current_comm(&event->comm, sizeof(event->comm));
	event->event_type = event_type;

	bpf_ringbuf_submit(event, 0);
}

SEC("tracepoint/sched/sched_process_exec")
int on_process_exec(void *ctx) {
	emit(0);
	return 0;
}

SEC("tracepoint/sched/sched_process_exit")
int on_process_exit(void *ctx) {
	emit(1);
	return 0;
}
