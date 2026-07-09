// Command sdev-go is the Go implementation of ported sdev subcommands. It is a
// multicall binary: the first argument selects the subcommand. The bash router
// bin/sdev execs it for commands that have been migrated.
package main

import (
	"os"

	"github.com/santhosh2011/sdev/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
