package metrics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// shutdownTimeout bounds how long Run waits for in-flight scrape
// requests to finish once ctx is canceled, before giving up on a
// graceful shutdown.
const shutdownTimeout = 5 * time.Second

// Server serves r's metrics over HTTP at /metrics, in Prometheus's own
// text exposition format, via promhttp — the standard library this
// project's own dependency (client_golang) already provides for
// exactly this, rather than formatting metric lines by hand.
type Server struct {
	httpServer *http.Server
}

// NewServer constructs a Server listening on addr (e.g. ":9090" or
// "127.0.0.1:9090"). It does not start listening until Run is called.
func NewServer(addr string, r *Registry) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{}))
	return &Server{httpServer: &http.Server{Addr: addr, Handler: mux}}
}

// Run starts serving and blocks until either ctx is canceled (in which
// case it shuts down gracefully — waiting up to shutdownTimeout for
// in-flight scrapes to finish — and returns nil) or the server fails
// to start or serve on its own (e.g. the address is already in use),
// in which case that error is returned immediately without waiting
// for ctx.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		err := s.httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil // the expected outcome of Shutdown below, not a real failure
		}
		serveErr <- err
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-serveErr
	}
}
