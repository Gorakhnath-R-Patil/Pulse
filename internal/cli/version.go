package cli

import (
	"fmt"
	"io"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

func runVersion(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("pulse-cli version", stderr)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	fmt.Fprintln(stdout, version.String())
	return ExitSuccess
}
