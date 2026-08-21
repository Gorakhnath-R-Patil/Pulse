package discovery

import (
	"testing"
)

func TestParseCgroup_PlainHostProcess(t *testing.T) {
	// A process running directly on the host, not in any container.
	data := []byte("0::/user.slice/user-1000.slice/session-2.scope\n")

	got := parseCgroup(data)
	if got != nil {
		t.Errorf("parseCgroup() = %+v, want nil for a non-containerized process", got)
	}
}

func TestParseCgroup_DockerCgroupfs_V2(t *testing.T) {
	data := []byte("0::/docker/e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1\n")

	got := parseCgroup(data)
	if got == nil {
		t.Fatal("parseCgroup() = nil, want a resolved container")
	}
	if got.ID != "e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1" {
		t.Errorf("ID = %q, want the 64-hex container ID", got.ID)
	}
	if got.PodUID != "" {
		t.Errorf("PodUID = %q, want empty for a non-Kubernetes container", got.PodUID)
	}
}

func TestParseCgroup_DockerSystemd_V1(t *testing.T) {
	// cgroup v1: one line per controller, all sharing the same path
	// suffix.
	data := []byte(`12:pids:/system.slice/docker-e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1.scope
11:memory:/system.slice/docker-e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1.scope
1:name=systemd:/system.slice/docker-e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1.scope
`)

	got := parseCgroup(data)
	if got == nil {
		t.Fatal("parseCgroup() = nil, want a resolved container")
	}
	if got.ID != "e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1" {
		t.Errorf("ID = %q, want the 64-hex container ID", got.ID)
	}
}

func TestParseCgroup_KubernetesCgroupfs(t *testing.T) {
	data := []byte("0::/kubepods/burstable/pod12345678-90ab-cdef-1234-567890abcdef/e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1\n")

	got := parseCgroup(data)
	if got == nil {
		t.Fatal("parseCgroup() = nil, want a resolved container")
	}
	if got.ID != "e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1" {
		t.Errorf("ID = %q, want the 64-hex container ID", got.ID)
	}
	if got.PodUID != "12345678-90ab-cdef-1234-567890abcdef" {
		t.Errorf("PodUID = %q, want the dash-separated pod UID", got.PodUID)
	}
}

func TestParseCgroup_KubernetesSystemd(t *testing.T) {
	// systemd unit names can't contain dashes in this position, so
	// kubelet substitutes underscores for the pod UID's dashes here.
	data := []byte("0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod12345678_90ab_cdef_1234_567890abcdef.slice/cri-containerd-e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1.scope\n")

	got := parseCgroup(data)
	if got == nil {
		t.Fatal("parseCgroup() = nil, want a resolved container")
	}
	if got.ID != "e3f1c2a9b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1" {
		t.Errorf("ID = %q, want the 64-hex container ID", got.ID)
	}
	// Normalized back to the canonical dash-separated form, matching
	// what the cgroupfs driver would have produced for the same pod.
	if got.PodUID != "12345678-90ab-cdef-1234-567890abcdef" {
		t.Errorf("PodUID = %q, want the normalized dash-separated pod UID", got.PodUID)
	}
}

func TestParseCgroup_Empty(t *testing.T) {
	got := parseCgroup([]byte(""))
	if got != nil {
		t.Errorf("parseCgroup(\"\") = %+v, want nil", got)
	}
}
