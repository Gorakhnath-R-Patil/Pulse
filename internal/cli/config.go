package cli

import (
	"fmt"
	"io"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

const configUsage = `Usage:
  pulse-cli config validate -target <agent|collector> -file <path>
`

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "validate" {
		fmt.Fprint(stderr, configUsage)
		return ExitUsage
	}
	return runConfigValidate(args[1:], stdout, stderr)
}

func runConfigValidate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("pulse-cli config validate", stderr)
	target := fs.String("target", "", "component the config file belongs to: agent or collector")
	file := fs.String("file", "", "path to the configuration file")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	if *file == "" {
		fmt.Fprintln(stderr, "pulse-cli: -file is required")
		fmt.Fprint(stderr, configUsage)
		return ExitUsage
	}

	var err error
	switch *target {
	case "agent":
		_, err = config.LoadAgentConfig(*file)
	case "collector":
		_, err = config.LoadCollectorConfig(*file)
	case "":
		fmt.Fprintln(stderr, "pulse-cli: -target is required (agent or collector)")
		fmt.Fprint(stderr, configUsage)
		return ExitUsage
	default:
		fmt.Fprintf(stderr, "pulse-cli: unknown -target %q (want agent or collector)\n", *target)
		return ExitUsage
	}

	if err != nil {
		fmt.Fprintf(stderr, "pulse-cli: %s config %q is invalid: %v\n", *target, *file, err)
		return ExitFailure
	}

	fmt.Fprintf(stdout, "%s config %q is valid\n", *target, *file)
	return ExitSuccess
}
