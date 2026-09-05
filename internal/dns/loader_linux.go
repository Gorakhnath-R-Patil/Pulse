//go:build linux

package dns

import (
	"errors"
	"fmt"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

// Regenerate with `go generate ./...` after changing
// bpf/programs/dns_telemetry.c. Requires clang with a BPF target; see
// docs/development/getting-started.md.
//
// "DNS" is capitalized deliberately, exporting the generated loader
// function as LoadDNSObjects — see the equivalent note in
// internal/ebpf/loader_linux.go for why that matters.
//go:generate go tool bpf2go -cc clang -no-strip -tags linux DNS ../../bpf/programs/dns_telemetry.c -- -I../../bpf/headers

// pendingQueryTTL bounds how long a query waits for a matching response
// before queryCorrelator forgets it, so a query that's never answered
// doesn't accumulate forever.
const pendingQueryTTL = 5 * time.Second

// Loader owns the lifecycle of Pulse's DNS telemetry program: load,
// attach (to the sendto and paired recvfrom-enter/exit tracepoints),
// receive, detach. See internal/ebpf's Loader, which this mirrors, for
// the calling contract: Load, Attach, and Read in that order, Close
// always safe.
//
// A Loader is not safe for concurrent use by multiple goroutines.
type Loader struct {
	objs              DNSObjects
	sendtoLink        link.Link
	recvfromEnterLink link.Link
	recvfromExitLink  link.Link
	rd                *ringbuf.Reader

	loaded   bool
	attached bool

	refMonotonicNS uint64
	refWallClock   time.Time

	correlator *queryCorrelator
}

// NewLoader returns an unloaded Loader. Load must be called before
// Attach, and Attach before Read.
func NewLoader() *Loader {
	return &Loader{correlator: newQueryCorrelator(pendingQueryTTL)}
}

// Load checks kernel compatibility, then loads the DNS telemetry
// program and its maps into the kernel. It does not attach the
// program — no events are observed until Attach is called.
func (l *Loader) Load() error {
	if err := ebpf.CheckSupport(); err != nil {
		return err
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("dns: raising memlock limit: %w", err)
	}

	if err := LoadDNSObjects(&l.objs, nil); err != nil {
		return fmt.Errorf("dns: loading dns_telemetry program: %w", err)
	}

	l.refMonotonicNS, l.refWallClock = ebpf.MonotonicReference()

	l.loaded = true
	return nil
}

// Attach attaches the loaded programs to sys_enter_sendto,
// sys_enter_recvfrom, and sys_exit_recvfrom, and opens a reader on
// their shared ring buffer. Load must have succeeded first.
func (l *Loader) Attach() error {
	if !l.loaded {
		return ErrNotLoaded
	}

	sendtoLink, err := link.Tracepoint("syscalls", "sys_enter_sendto", l.objs.OnSysEnterSendto, nil)
	if err != nil {
		return fmt.Errorf("dns: attaching sys_enter_sendto: %w", err)
	}
	l.sendtoLink = sendtoLink

	recvEnterLink, err := link.Tracepoint("syscalls", "sys_enter_recvfrom", l.objs.OnSysEnterRecvfrom, nil)
	if err != nil {
		sendtoLink.Close()
		l.sendtoLink = nil
		return fmt.Errorf("dns: attaching sys_enter_recvfrom: %w", err)
	}
	l.recvfromEnterLink = recvEnterLink

	recvExitLink, err := link.Tracepoint("syscalls", "sys_exit_recvfrom", l.objs.OnSysExitRecvfrom, nil)
	if err != nil {
		recvEnterLink.Close()
		sendtoLink.Close()
		l.recvfromEnterLink = nil
		l.sendtoLink = nil
		return fmt.Errorf("dns: attaching sys_exit_recvfrom: %w", err)
	}
	l.recvfromExitLink = recvExitLink

	rd, err := ringbuf.NewReader(l.objs.DnsEvents)
	if err != nil {
		recvExitLink.Close()
		recvEnterLink.Close()
		sendtoLink.Close()
		l.recvfromExitLink = nil
		l.recvfromEnterLink = nil
		l.sendtoLink = nil
		return fmt.Errorf("dns: opening ring buffer reader: %w", err)
	}
	l.rd = rd

	l.attached = true
	return nil
}

// Read blocks until the next event that actually parses as a DNS
// message is available, or until Close interrupts it. Ring buffer
// records that don't parse (see parseMessage) are silently skipped —
// expected, given this package captures every UDP sendto/recvfrom
// payload without kernel-side DNS filtering (see
// bpf/programs/dns_telemetry.c) — so Read may consume more than one
// record per call. A response event carries a real Latency if its
// matching query was observed and hasn't been evicted as stale (see
// queryCorrelator). Attach must have succeeded first.
func (l *Loader) Read() (DNSEvent, error) {
	if !l.attached {
		return DNSEvent{}, ErrNotLoaded
	}

	for {
		record, err := l.rd.Read()
		if err != nil {
			return DNSEvent{}, fmt.Errorf("dns: reading ring buffer: %w", err)
		}

		raw, err := decodeRawEvent(record.RawSample)
		if err != nil {
			return DNSEvent{}, err
		}

		msg, err := parseMessage(raw.Data)
		if err != nil {
			continue
		}

		timestamp := l.refWallClock.Add(time.Duration(int64(raw.TimestampNS) - int64(l.refMonotonicNS)))

		event := DNSEvent{
			Timestamp:    timestamp,
			PID:          raw.PID,
			Comm:         raw.Comm,
			IsResponse:   msg.isResponse,
			Name:         msg.name,
			QType:        msg.qtype,
			ResponseCode: msg.responseCode,
			AnswerCount:  msg.answerCount,
		}

		if msg.isResponse {
			if latency, ok := l.correlator.observeResponse(raw.PID, msg.id, timestamp); ok {
				event.Latency = latency
			}
		} else {
			l.correlator.observeQuery(raw.PID, msg.id, timestamp)
		}

		return event, nil
	}
}

// Close detaches every program (whichever attached) and releases every
// kernel resource Load/Attach acquired (if any), collecting every error
// encountered rather than stopping at the first. Safe to call multiple
// times and even when Load or Attach never succeeded.
func (l *Loader) Close() error {
	var errs []error

	if l.rd != nil {
		if err := l.rd.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing ring buffer reader: %w", err))
		}
		l.rd = nil
	}
	if l.recvfromExitLink != nil {
		if err := l.recvfromExitLink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("detaching sys_exit_recvfrom: %w", err))
		}
		l.recvfromExitLink = nil
	}
	if l.recvfromEnterLink != nil {
		if err := l.recvfromEnterLink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("detaching sys_enter_recvfrom: %w", err))
		}
		l.recvfromEnterLink = nil
	}
	if l.sendtoLink != nil {
		if err := l.sendtoLink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("detaching sys_enter_sendto: %w", err))
		}
		l.sendtoLink = nil
	}
	if l.loaded {
		if err := l.objs.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing program/map handles: %w", err))
		}
		l.loaded = false
	}
	l.attached = false

	return errors.Join(errs...)
}
