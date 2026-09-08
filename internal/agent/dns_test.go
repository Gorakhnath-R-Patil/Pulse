package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/dns"
)

// fakeDNSLoader mirrors fakeProcessLoader in process_test.go — see its
// doc comment for the block-channel rationale.
type fakeDNSLoader struct {
	loadErr     error
	attachErr   error
	closeErr    error
	events      []dns.DNSEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeDNSLoader) Load() error   { return f.loadErr }
func (f *fakeDNSLoader) Attach() error { return f.attachErr }

func (f *fakeDNSLoader) Close() error {
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
	}
	return f.closeErr
}

func (f *fakeDNSLoader) Read() (dns.DNSEvent, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	if f.block != nil {
		<-f.block
	}
	return dns.DNSEvent{}, f.terminalErr
}

func TestDNSSource_Read_NormalizesEvent(t *testing.T) {
	fake := &fakeDNSLoader{events: []dns.DNSEvent{
		{PID: 100, Comm: "systemd-resolve", Name: "example.com", QType: 1},
	}}
	src := dnsSource{loader: fake, nodeName: "pulse-node-1"}

	event, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if event.Type != "dns.query" {
		t.Errorf("Type = %q, want %q", event.Type, "dns.query")
	}
	if event.Attributes["dns.name"] != "example.com" {
		t.Errorf(`Attributes["dns.name"] = %q, want "example.com"`, event.Attributes["dns.name"])
	}
}

func TestDNSSource_Read_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("read failed")
	src := dnsSource{loader: &fakeDNSLoader{terminalErr: wantErr}, nodeName: "pulse-node-1"}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}

func TestDNSPipeline_LogsEventsEndToEnd(t *testing.T) {
	app, buf := testApp()
	fake := &fakeDNSLoader{
		events: []dns.DNSEvent{
			{PID: 100, Comm: "systemd-resolve", Name: "example.com", QType: 1},
		},
		block: make(chan struct{}),
	}

	p := app.newDNSPipeline(fake, testCorrelatingProcessor())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for !strings.Contains(buf.String(), `"pid":100`) {
		select {
		case <-deadline:
			t.Fatalf("log output never contained the observed event's pid: %s", buf.String())
		case <-time.After(time.Millisecond):
		}
	}

	if !strings.Contains(buf.String(), "dns.query") {
		t.Errorf("log output missing the event type: %s", buf.String())
	}

	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after the loader was closed")
	}
}
