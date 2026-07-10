// Package cli implements the sdev-go subcommands dispatched by the bash router
// bin/sdev for commands that have been ported from bash (strangler migration).
package cli

import (
	"fmt"
	"os"
)

// Run dispatches on the first argument (the subcommand) and returns the process
// exit code. A missing or unknown subcommand is a usage error (exit 2), matching
// how bin/sdev treats unknown input.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sdev-go: missing subcommand")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "version":
		return Version(rest)
	case "status":
		return Status(rest)
	case "ps":
		return PS(rest)
	case "ls", "list":
		return LS(rest)
	case "doctor":
		return Doctor(rest)
	case "alloc":
		return Alloc(rest)
	case "lease":
		return Lease(rest)
	case "release":
		return Release(rest)
	case "hold":
		return Hold(rest)
	case "destroy":
		return Destroy(rest)
	case "prune":
		return Prune(rest)
	case "new":
		return Newtask(rest)
	case "end":
		return End(rest)
	case "start":
		return Start(rest)
	default:
		fmt.Fprintf(os.Stderr, "sdev-go: unknown subcommand %q\n", sub)
		return 2
	}
}
