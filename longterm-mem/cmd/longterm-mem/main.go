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
// Exit codes follow the contract in design.md, named in exit_codes.go; run
// itself only ever produces exitUsage.
func run(args []string) int {
	if len(args) == 0 {
		usage()
		return exitUsage
	}

	switch args[0] {
	case "index":
		return cmdIndex(args[1:])
	case "query":
		return cmdQuery(args[1:])
	case "sync":
		return cmdSync(args[1:])
	case "status":
		return cmdStatus(args[1:])
	case "stale":
		return cmdStale(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	case "promote":
		return cmdPromote(args[1:])
	case "mcp":
		return cmdMCP(args[1:])
	case "register":
		return cmdRegister(args[1:])
	case "unregister":
		return cmdUnregister(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "longterm-mem: unknown subcommand %q\n", args[0])
		usage()
		return exitUsage
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: longterm-mem <subcommand> [flags]")
}
