// Package discovery resolves service identity metadata — today, which
// container (and, on Kubernetes, which pod) a process belongs to — from
// information already available on the host, without calling out to a
// container runtime or the Kubernetes API. See
// docs/design/service-identity.md for what that limits it to, and what
// Day 19's Kubernetes integration adds on top.
package discovery

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// containerIDPattern matches a container runtime ID: a 64-character hex
// string, the convention Docker, containerd, and CRI-O all follow
// regardless of cgroup driver (cgroupfs or systemd) — it appears
// somewhere in the cgroup path either way, just with different
// surrounding punctuation the ID pattern itself doesn't need to care
// about.
var containerIDPattern = regexp.MustCompile(`[0-9a-f]{64}`)

// podUIDPattern matches a Kubernetes pod UID embedded in a cgroup path.
// kubelet writes it two ways depending on cgroup driver: dash-separated
// under the cgroupfs driver ("pod12345678-90ab-...") and underscore-
// separated under the systemd driver ("pod12345678_90ab_...", since
// systemd unit names can't contain dashes in this position) — both are
// the same UUID, just escaped differently, which this single pattern
// (accepting either separator) and a normalization pass below account
// for.
var podUIDPattern = regexp.MustCompile(`pod([0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12})`)

// ResolveContainer resolves the container pid belongs to by reading its
// cgroup membership from /proc/<pid>/cgroup. It returns a nil
// *model.Container, not an error, when pid is running directly on the
// host rather than inside a recognized container cgroup — that's the
// common and expected case for most processes, not a failure.
func ResolveContainer(pid uint32) (*model.Container, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return nil, fmt.Errorf("discovery: reading cgroup for pid %d: %w", pid, err)
	}
	return parseCgroup(data), nil
}

// parseCgroup extracts container identity from the raw contents of a
// /proc/<pid>/cgroup file. It handles both the single-line cgroup v2
// unified hierarchy ("0::/path") and the multi-line cgroup v1 format
// (one controller per line, "hierarchy-id:controllers:path") by
// pattern-matching across the whole file rather than parsing per-line
// structure — every line for a given process shares the same cgroup
// path suffix, so this is simpler and just as reliable as picking one
// specific line to parse.
//
// This recognizes the common Docker/containerd/Kubernetes cgroup path
// conventions on both cgroupfs and systemd drivers. It does not attempt
// to cover every container runtime or custom cgroup layout — see
// docs/design/service-identity.md's Limitations.
func parseCgroup(data []byte) *model.Container {
	text := string(data)

	id := containerIDPattern.FindString(text)
	if id == "" {
		return nil
	}

	container := &model.Container{ID: id}
	if m := podUIDPattern.FindStringSubmatch(text); m != nil {
		container.PodUID = strings.ReplaceAll(m[1], "_", "-")
	}
	return container
}
