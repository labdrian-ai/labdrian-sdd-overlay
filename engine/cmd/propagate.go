package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/fsstore"
)

// This file is the command line of `engine propagate`. What the command does, the pass that scopes
// the row of a contract into a registry and the loop that reads every write back, is the use case in
// engine/propagator/app; where the registry lives is the adapter in engine/propagator/fsstore.
// What is left here is what only a command knows: the arguments, the words a person reads, the exit
// code, and the lock that keeps two runs of the command from writing the registry at once.

// registryAbsentNote is what the command says when there is no registry and none was required: the
// project does not use the overlay, so there is nothing to scope.
const registryAbsentNote = "project does not use the overlay (no-op)"

// propagateOptions are the arguments of the command. A flag whose value is missing at the end of
// the line is ignored, and the last occurrence of a flag wins.
type propagateOptions struct {
	registry             string
	contractFile         string
	contractPath         string
	embeddedName         string
	contractPathExplicit bool
	requireRegistry      bool
}

func parsePropagateOptions(args []string) propagateOptions {
	opts := propagateOptions{contractPath: defaultContractPath}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			i++
			if i < len(args) {
				opts.registry = args[i]
			}
		case "--contract-file":
			i++
			if i < len(args) {
				opts.contractFile = args[i]
			}
		case "--contract-path":
			i++
			if i < len(args) {
				opts.contractPath = args[i]
				opts.contractPathExplicit = true
			}
		case "--embedded-contract":
			i++
			if i < len(args) {
				opts.embeddedName = args[i]
			}
		case "--require-registry":
			opts.requireRegistry = true
		}
	}
	return opts
}

// registryPathFromArgs extracts the cleaned --registry value, empty if absent. It names the
// sidecar lock before the arguments are judged, so the lock is taken even for a run that is about
// to be refused for a missing contract: a held lock is reported before an argument error, as it
// always was.
func registryPathFromArgs(args []string) string {
	// Keep the LAST occurrence to match parsePropagateOptions, so the sidecar lock always guards
	// the same file the use case reads and writes.
	path := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--registry" && i+1 < len(args) {
			path = filepath.Clean(args[i+1])
		}
	}
	return path
}

// acquireRegistryLock takes an exclusive advisory lock on lockPath (a sidecar file next to the
// registry) through engine/filelock, waiting up to filelock.DefaultWait for a holder to let go and
// then failing with a *filelock.BusyError. It serializes the read-modify-write cycle across
// concurrent propagate processes (both contract hooks fire on every UserPromptSubmit), and the
// bound means a hung holder costs the next prompt two seconds and an error, not its whole session.
// Returns a release func.
//
// The kernel releases the lock on process exit, so an os.Exit inside the command (which skips
// defers) can never leave the lock held.
func acquireRegistryLock(lockPath string) (release func(), err error) {
	return filelock.Acquire(lockPath, filelock.Options{})
}

// runPropagate implements the 'propagate' subcommand. Fails LOUD on any error (exits 1).
//
// Concurrency safety (two layers, plus the use case's refusal of an empty registry):
//  1. an exclusive flock on <registry>.lock serializes the read-modify-write against the sibling
//     propagate process spawned by the other contract hook; it waits up to filelock.DefaultWait
//     for a holder, then exits 1 naming the lock instead of waiting for as long as the holder
//     lives;
//  2. the registry write itself is atomic (temp file + rename), so even a reader outside the lock
//     can never observe a truncated/empty registry.
func runPropagate(args []string) {
	if registryPath := registryPathFromArgs(args); registryPath != "" {
		release, err := acquireRegistryLock(registryPath + ".lock")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: acquiring registry lock: %v\n", err)
			os.Exit(1)
		}
		defer release()
	}
	propagateCommand(args, os.Stdout, os.Stderr, fsstore.Registry{}, os.ReadFile, os.Exit)
}

// propagateCommand is the testable core of the command: the registry store, the reader of the
// contract file and the exit are handed in, so a test can run every branch without a file system.
// It does not take the lock; runPropagate does, before this runs.
func propagateCommand(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	store app.RegistryStore,
	readFile readFileFn,
	exit func(int),
) {
	opts := parsePropagateOptions(args)
	if opts.registry == "" {
		fmt.Fprintln(stderr, "error: --registry is required")
		exit(1)
		return
	}
	// Clean the registry path to prevent path-traversal via "../" segments.
	registry := filepath.Clean(opts.registry)

	// Resolve the contract and its block scope. An embedded contract (engine-owned managed text)
	// takes precedence and overrides marker and label so it writes a DISTINCT block that never
	// collides with minimalism-contract.
	cfg := propagator.Config{ContractPath: opts.contractPath}
	rowLabel := propagator.DefaultRowLabel
	var source app.ContractSource
	if opts.embeddedName != "" {
		spec, ok := embeddedContract(opts.embeddedName)
		if !ok {
			fmt.Fprintf(stderr, "error: unknown embedded contract %q\n", opts.embeddedName)
			exit(1)
			return
		}
		source = textContract(spec.content)
		cfg.BeginMarker = spec.beginMarker
		cfg.EndMarker = spec.endMarker
		cfg.RowLabel = spec.rowLabel
		rowLabel = spec.rowLabel
		// Use the embedded contract's own path in the registry row unless the caller explicitly
		// overrode --contract-path.
		if !opts.contractPathExplicit {
			cfg.ContractPath = spec.defaultPath
		}
	} else {
		if opts.contractFile == "" {
			fmt.Fprintln(stderr, "error: --contract-file is required")
			exit(1)
			return
		}
		source = fileContract{path: opts.contractFile, read: readFile}
	}

	outcome, err := app.New(store).PropagateVerified(app.Request{
		Registry:        registry,
		Contract:        source,
		Config:          cfg,
		RequireRegistry: opts.requireRegistry,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", propagateMessage(err))
		exit(1)
		return
	}
	switch outcome {
	case app.Absent:
		fmt.Fprintf(stdout, "propagate: registry not found at %s — %s\n", registry, registryAbsentNote)
	case app.Written:
		fmt.Fprintf(stdout, "registry: %s scoped row inserted/updated\n", rowLabel)
	case app.Unchanged:
		// A correct scoped row is the steady state on every prompt; stay silent so the hook only
		// speaks when it changed something.
	}
}

// propagateMessage is what the command tells a person about an error of the use case. The words
// are the command's, and they are what the command has always said; an error the use case does not
// know to be one of its own is said as it came.
func propagateMessage(err error) string {
	var (
		contractRead *app.ContractReadError
		registryRead *app.RegistryReadError
		required     *app.RegistryRequiredError
		registryWrit *app.RegistryWriteError
		empty        *app.EmptyRegistryError
		inconsistent *app.InconsistentRegistryError
		unverified   *app.UnverifiedWriteError
		lost         *app.LostWriteError
		contractFail *app.ContractParseError
		rewrite      *app.RewriteError
	)
	switch {
	case errors.As(err, &contractRead):
		return fmt.Sprintf("reading contract file: %v", contractRead.Err)
	case errors.As(err, &contractFail):
		return contractFail.Err.Error()
	case errors.As(err, &registryRead):
		return fmt.Sprintf("reading registry file: %v", registryRead.Err)
	case errors.As(err, &required):
		return fmt.Sprintf("registry required but not found at %s (--require-registry)", required.Path)
	case errors.As(err, &rewrite):
		return rewrite.Err.Error()
	case errors.As(err, &registryWrit):
		return fmt.Sprintf("writing registry: %v", registryWrit.Err)
	case errors.As(err, &empty):
		return fmt.Sprintf("the registry at %s read empty on all %d attempts — refusing to propagate", empty.Path, empty.Attempts)
	case errors.As(err, &inconsistent):
		return fmt.Sprintf("the registry at %s was in an inconsistent state across %d attempts — refusing to propagate",
			inconsistent.Path, inconsistent.Attempts)
	case errors.As(err, &unverified):
		return fmt.Sprintf("registry write to %s could not be verified after %d attempts — the verification read itself failed: %v",
			unverified.Path, unverified.Attempts, unverified.Err)
	case errors.As(err, &lost):
		return fmt.Sprintf("registry write to %s did not persist after %d attempts — "+
			"a concurrent external writer (unrelated to engine propagate) appears to be clobbering it",
			lost.Path, lost.Attempts)
	}
	return err.Error()
}

// fileContract is the contract in a file the person named. It is read every time it is asked for.
type fileContract struct {
	path string
	read readFileFn
}

func (c fileContract) Text() (string, error) {
	b, err := c.read(c.path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// textContract is a contract the engine carries.
type textContract string

func (c textContract) Text() (string, error) { return string(c), nil }
