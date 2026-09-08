package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/topology"
)

const topologyUsage = `Usage:
  pulse-cli topology -clickhouse-addr <addr>[,<addr>...] [-database pulse] [-table events] [-format text|dot]

Queries a ClickHouse table pulse-collector has been storing events in
(see docs/design/clickhouse-storage.md) and prints the resulting
service dependency graph — see docs/design/service-topology.md for
what this graph shows and what it honestly doesn't.
`

func runTopology(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("pulse-cli topology", stderr)
	addr := fs.String("clickhouse-addr", "", "comma-separated ClickHouse native-protocol addresses, e.g. localhost:9000")
	database := fs.String("database", "pulse", "ClickHouse database to query")
	table := fs.String("table", "events", "table to query")
	format := fs.String("format", "text", "output format: text or dot")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	if *addr == "" {
		fmt.Fprintln(stderr, "pulse-cli: -clickhouse-addr is required")
		fmt.Fprint(stderr, topologyUsage)
		return ExitUsage
	}
	if *format != "text" && *format != "dot" {
		fmt.Fprintf(stderr, "pulse-cli: unknown -format %q (want text or dot)\n", *format)
		return ExitUsage
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: strings.Split(*addr, ","),
		Auth: clickhouse.Auth{Database: *database},
	})
	if err != nil {
		fmt.Fprintf(stderr, "pulse-cli: connecting to ClickHouse: %v\n", err)
		return ExitFailure
	}
	defer conn.Close()

	g, err := topology.Query(ctx, conn, *table)
	if err != nil {
		fmt.Fprintf(stderr, "pulse-cli: querying topology: %v\n", err)
		return ExitFailure
	}

	if *format == "dot" {
		err = topology.WriteDOT(stdout, g)
	} else {
		err = topology.WriteText(stdout, g)
	}
	if err != nil {
		fmt.Fprintf(stderr, "pulse-cli: writing output: %v\n", err)
		return ExitFailure
	}
	return ExitSuccess
}
