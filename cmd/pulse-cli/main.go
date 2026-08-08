// Command pulse-cli is the operator-facing command-line tool for Pulse:
// version reporting and configuration validation today, with more
// operational commands (querying topology, traces, etc.) arriving as the
// corresponding subsystems are built.
package main

import (
	"os"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
