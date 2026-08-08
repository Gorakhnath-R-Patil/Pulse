// Command pulse-collector receives telemetry forwarded by pulse-agent
// instances. Starting Day 14 it will consume from Kafka and persist to
// ClickHouse (Day 15); today it only establishes configuration, logging,
// and lifecycle plumbing.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/collector"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/logging"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags and drives startup; it is separated from main so tests
// can exercise argument handling without calling os.Exit.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pulse-collector", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to collector configuration file (optional; defaults are used if omitted)")
	showVersion := fs.Bool("version", false, "print version information and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, version.String())
		return 0
	}

	cfg, err := config.LoadCollectorConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "pulse-collector: configuration error: %v\n", err)
		return 1
	}

	logger, err := logging.New(cfg.Logging, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "pulse-collector: logging error: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := collector.New(cfg, logger)
	if err := app.Run(ctx); err != nil {
		logger.Error("pulse-collector exited with error", "error", err)
		return 1
	}
	return 0
}
