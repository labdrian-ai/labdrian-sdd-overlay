package main

// The 'registry' subcommand: what the overlay does to a project's .atl/skill-registry.md besides
// what `propagate` does. Its one verb, repair-sdd, puts back the rows of the SDD executor skills
// that gentle-ai leaves out of the registry it generates. The overlay script used to edit the
// markdown itself, with awk and grep and no lock (Phase 9, B2), so a repair could race the
// `propagate` hook that writes the same file on every prompt; the rule is the domain's
// (engine/sddregistry), the use case reads and writes through ports (engine/sddregistry/app), the
// adapter is the file system (engine/sddregistry/fsstore), and this file is the command line, the
// lock that keeps a repair and a `propagate` from writing the registry at once, and the words.
//
// The words are the overlay script's, kept as they were because the script's output is pinned and
// read by people: a refusal is "ERROR: ..." and a missing skill "WARN: ...", where the commands of
// the engine otherwise say "error: ...".

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry"
	sddapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry/app"
	sddfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry/fsstore"
)

// runRegistry implements the 'registry' subcommand over the files of the machine.
func runRegistry(p process, args []string) {
	runRegistryCore(sddfs.Registry{}, sddfs.Skills{}, args, p.stdout, p.stderr, p.exit)
}

// registryStore is the registry as the command reaches it: the port of the use case, and the
// directory of the registry, which the lock needs before the use case runs.
type registryStore interface {
	sddapp.RegistryStore
	// Home makes the directory the registry at path lives in, when there is none.
	Home(path string) error
}

// repairSDDArgs is the command line of 'registry repair-sdd', read.
type repairSDDArgs struct {
	registry   string
	target     string
	skillsRoot string
}

// parseRepairSDDArgs reads the arguments after the verb. A flag nobody named, a flag that has no
// value, and a flag that is not given are refused, saying which.
func parseRepairSDDArgs(args []string) (repairSDDArgs, error) {
	var out repairSDDArgs
	values := map[string]*string{flagRegistry: &out.registry, flagTarget: &out.target, flagSkillsRoot: &out.skillsRoot}
	for i := 0; i < len(args); i++ {
		value, ok := values[args[i]]
		if !ok {
			return repairSDDArgs{}, fmt.Errorf("registry repair-sdd: unknown argument %q", args[i])
		}
		if i+1 >= len(args) {
			return repairSDDArgs{}, fmt.Errorf("registry repair-sdd: flag %q requires a value", args[i])
		}
		i++
		*value = args[i]
	}
	if out.registry == "" || out.target == "" || out.skillsRoot == "" {
		return repairSDDArgs{}, fmt.Errorf("registry repair-sdd requires %s, %s, and %s", flagRegistry, flagTarget, flagSkillsRoot)
	}
	return out, nil
}

// registryLockPath is the lock every writer of the registry at path takes: a sidecar file beside it
// (see acquireRegistryLock). `propagate` takes the same one, so a repair and a propagation of the
// same registry take turns.
func registryLockPath(registry string) string { return filepath.Clean(registry) + ".lock" }

// runRegistryCore is the testable core of the registry subcommand: it names the verb, reads the
// command line, takes the lock of the registry, runs the use case and tells the outcome. A repair
// that finds executor skills not installed says each on stderr, then sums up on stdout; a refusal
// or a failure is said on stderr and ends with exit 1, with the registry as it was.
func runRegistryCore(store registryStore, skills sddapp.SkillSource, args []string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: registry requires a verb: repair-sdd")
		exit(1)
		return
	}
	if args[0] != "repair-sdd" {
		fmt.Fprintf(stderr, "error: unknown registry verb %q; expected repair-sdd\n", args[0])
		exit(1)
		return
	}
	parsed, err := parseRepairSDDArgs(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	// The lock is a file beside the registry, and the registry of a project's first repair has no
	// directory yet.
	if err := store.Home(parsed.registry); err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot make the directory of the registry %s: %v\n", parsed.registry, err)
		exit(1)
		return
	}
	release, err := acquireRegistryLock(registryLockPath(parsed.registry))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: acquiring registry lock: %v\n", err)
		exit(1)
		return
	}
	defer release()

	result, err := sddapp.New(store, skills).Repair(sddapp.Request{Registry: parsed.registry, SkillsRoot: parsed.skillsRoot})
	if err != nil {
		refuseRepairSDD(err, parsed.registry, stderr)
		exit(1)
		return
	}
	for _, path := range result.Missing {
		fmt.Fprintf(stderr, "WARN: missing %s SDD executor skill: %s\n", parsed.target, path)
	}
	fmt.Fprintf(stdout, "repair-sdd-registry: registry=%s target=%s sdd_executors=%d missing=%d\n",
		parsed.registry, parsed.target, result.Rows, len(result.Missing))
}

// refuseRepairSDD says why the registry could not be repaired. A registry without a heading is said
// in the words of the overlay script, which names the registry after the heading.
func refuseRepairSDD(err error, registry string, stderr io.Writer) {
	var missing *sddregistry.MissingSectionError
	if errors.As(err, &missing) {
		fmt.Fprintf(stderr, "ERROR: %v: %s\n", missing, registry)
		return
	}
	fmt.Fprintf(stderr, "ERROR: %v\n", err)
}
