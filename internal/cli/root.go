// Package cli implements the pulse-cli command tree.
//
// pulse-cli is deliberately built on the standard library's flag package
// with a small manual subcommand dispatcher rather than a framework like
// Cobra. With three commands total today, a framework would add a
// dependency and indirection without paying for itself; see
// docs/design/decisions.md for the full reasoning. Execute is factored out
// of main() so it can be unit tested without spawning a subprocess.
package cli

import (
	"flag"
	"fmt"
	"io"
)

// Exit codes follow the common Unix convention: 0 success, 1 runtime
// failure, 2 usage error.
const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

const usage = `pulse-cli is the operator command-line tool for Pulse.

Usage:
  pulse-cli <command> [arguments]

Commands:
  version          Print version information and exit.
  config validate  Validate a pulse-agent or pulse-collector config file.

Run 'pulse-cli <command> -h' for details on a specific command.
`

// Execute runs the pulse-cli command tree against args (typically
// os.Args[1:]), writing normal output to stdout and errors/usage to
// stderr. It returns a process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version":
		return runVersion(rest, stdout, stderr)
	case "config":
		return runConfig(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return ExitSuccess
	default:
		fmt.Fprintf(stderr, "pulse-cli: unknown command %q\n\n%s", cmd, usage)
		return ExitUsage
	}
}

// newFlagSet returns a FlagSet configured to write its own usage/error
// output to stderr instead of the package-level default of os.Stderr,
// which would bypass the writer Execute was given and break tests.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}
