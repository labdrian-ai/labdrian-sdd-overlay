// Command longterm-mem is the packaged mid/long-term memory layer binary:
// an MCP stdio server plus CLI subcommands for querying, indexing, syncing,
// and promoting project memory (see longterm-mem/README.md).
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches the requested subcommand and returns the process exit code.
// Exit codes follow the contract in design.md: 0 ok, 2 usage.
func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "index":
		return cmdIndex(args[1:])
	case "query":
		return cmdQuery(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "longterm-mem: unknown subcommand %q\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: longterm-mem <subcommand> [flags]")
}
