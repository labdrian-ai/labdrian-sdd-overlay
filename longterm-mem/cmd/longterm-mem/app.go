package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// app is the program's use cases and the adapters under them, composed for one run of a command. newApp builds it
// from what main composed the commands with (the clock and the embedding client factory) and the name the command
// goes by in what it says; the commands parse their arguments, ask the app for what they need and render what
// comes back. Where Engram's database is, how the vault of a project is found, how a Writer and the deps of the
// use cases are wired to the vault's files and scripts: that is said here and in the files that build the deps
// (rundeps.go, identity_wiring.go, vaultfiles.go), once, and the commands do not repeat it.
type app struct {
	commands
	// command is what the command is called in its messages: "sync", "promote reconcile", "index".
	command string
}

// newApp composes the app of the command called command in its messages.
func (c commands) newApp(command string) app {
	return app{commands: c, command: command}
}

// refuse says that the command cannot go on, as every command says it -- `longterm-mem: <command>: <why>` on
// stderr -- and answers the exit code to return.
func (a app) refuse(err error, code int) int {
	fmt.Fprintf(os.Stderr, "longterm-mem: %s: %v\n", a.command, err)
	return code
}

// project is the project the command acts on: the one it was given, or, when it was given none, the one adopted
// from the working directory. A command that promises not to change the repository it inspects (readOnly) does
// not record what the repository derives. exitOK, or the exit code to return.
func (a app) project(given string, readOnly bool) (string, int) {
	if readOnly {
		return a.resolveProjectFlagReadOnly(a.command, given)
	}
	return a.resolveProjectFlag(a.command, given)
}

// vaultRoot is the root of the vault of project: the override the command was given, or the registry's row. A
// command that promises not to write (readOnly) leaves the registry as it found it; any other seeds the registry
// of a machine that has none, the first time it is asked (the owner's decision of 2026-10-14: only a command that
// writes does). exitOK, or the exit code to return after saying why: no vault configured for the project is exit
// 3, a registry that cannot be read is the program's own fault.
func (a app) vaultRoot(project, override string, readOnly bool) (string, int) {
	vaults := machineVaults()
	find := vaults.ToWrite
	if readOnly {
		find = vaults.ToRead
	}
	root, err := find(project, override)
	if err != nil {
		return "", a.refuse(err, vaultExitCode(err))
	}
	return root, exitOK
}

// memory opens Engram's database, read-only, at the path of LONGTERM_MEM_ENGRAM_DB or its default. The caller
// closes the store. exitOK, or the exit code to return after saying why: a database that cannot be opened is
// engram_unavailable.
func (a app) memory() (*engram.Store, int) {
	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		return nil, a.refuse(err, exitEngramUnavailable)
	}
	return store, exitOK
}

// memoryToPromoteFrom is memory for a command that writes pages from what it reads: when Engram could only be
// opened through its immutable=1 fallback, so that the run works from a snapshot and not the live database, the
// command says so on stderr (declareDegradedEngram).
func (a app) memoryToPromoteFrom() (*engram.Store, int) {
	store, exit := a.memory()
	if exit != exitOK {
		return nil, exit
	}
	declareDegradedEngram(store, a.command)
	return store, exitOK
}

// syncDeps is what a sync of the vault at vaultRoot runs over: the memory it reads, a Writer of the vault, and the
// vault's own rebuild of its index. The error is the Writer's, as its precedence repository reported it.
func (a app) syncDeps(store *engram.Store, vaultRoot string) (promote.Deps, error) {
	writer, err := newPromoteWriter(vaultRoot, a.clock)
	if err != nil {
		return promote.Deps{}, err
	}
	return promote.Deps{Memory: store, Writer: writer, Index: vault.IndexRebuilder{Root: vaultRoot, Clock: a.clock}}, nil
}

// printJSON prints v on stdout as the result of a command that was asked for JSON: indented by two spaces. A
// value that cannot be encoded is the program's own fault.
func (a app) printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return a.refuse(fmt.Errorf("encode result: %w", err), exitInternal)
	}
	return exitOK
}
