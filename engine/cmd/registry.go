package main

// The 'registry' subcommand: what the overlay does to a project's .atl/skill-registry.md besides
// what `propagate` does. Its verb repair-sdd puts back the rows of the SDD executor skills
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
//
// Its other verb, refresh, writes the table of skills that `gentle-ai skill-registry refresh` used to
// write, and every line around it stays as it was (engine/skillregistry, its use case
// engine/skillregistry/app, its adapter engine/skillregistry/fsscan). It takes the same lock. It says
// what gentle-ai said, in its words, because the overlay script and the hook that ask for it read
// them; a failure is "error: ...", as in the other verbs of the engine.

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry"
	sddapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry/app"
	sddfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry/fsstore"
	skillapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillregistry/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillregistry/fsscan"
)

// runRegistry implements the 'registry' subcommand over the files of the machine.
func runRegistry(p process, d deps, args []string) {
	if len(args) > 0 && args[0] == "refresh" {
		runRegistryRefresh(p, d, args[1:])
		return
	}
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
		fmt.Fprintln(stderr, "error: registry requires a verb: repair-sdd or refresh")
		exit(1)
		return
	}
	if args[0] != "repair-sdd" {
		fmt.Fprintf(stderr, "error: unknown registry verb %q; expected repair-sdd or refresh\n", args[0])
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

// refreshArgs is the command line of 'registry refresh', read.
type refreshArgs struct {
	cwd          string
	force, quiet bool
	// skipIfBusy leaves a registry whose lock is taken for the holder, at once and silently, instead
	// of waiting for it and failing: the prompt hook runs with it.
	skipIfBusy bool
}

// parseRefreshArgs reads the arguments after the verb. A flag nobody named, a --cwd that has no
// value and a --cwd that is not given are refused, saying which.
func parseRefreshArgs(args []string) (refreshArgs, error) {
	var out refreshArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force":
			out.force = true
		case "--quiet":
			out.quiet = true
		case "--skip-if-busy":
			out.skipIfBusy = true
		case flagCwd:
			if i+1 >= len(args) {
				return refreshArgs{}, fmt.Errorf("registry refresh: flag %q requires a value", args[i])
			}
			i++
			out.cwd = args[i]
		default:
			return refreshArgs{}, fmt.Errorf("registry refresh: unknown argument %q", args[i])
		}
	}
	if out.cwd == "" {
		return refreshArgs{}, fmt.Errorf("registry refresh requires %s", flagCwd)
	}
	return out, nil
}

// registryLocker is the lock of the registry as the use case reaches it: the sidecar file beside
// it that `propagate` and a repair take too (registryLockPath). The directory of the registry is
// made first, because the lock is a file in it and a project that has only a .git has no .atl yet.
type registryLocker struct {
	store registryStore
	// noWait tries the lock once and reports a taken one at once (filelock.ErrBusy), where the
	// default waits for it up to the bound the registry lock has.
	noWait bool
}

func (l registryLocker) Lock(registry string) (func(), error) {
	if err := l.store.Home(registry); err != nil {
		return nil, err
	}
	if l.noWait {
		return filelock.Acquire(registryLockPath(registry), filelock.Options{NoWait: true})
	}
	return acquireRegistryLock(registryLockPath(registry))
}

// runRegistryRefresh implements 'registry refresh' over the files of the machine, with the home and
// the clock of the deps.
func runRegistryRefresh(p process, d deps, args []string) {
	home, err := d.homeDir()
	if err != nil {
		// No home: the project's skills only.
		home = ""
	}
	store := sddfs.Registry{}
	parsed, _ := parseRefreshArgs(args) // a command line that is refused is said by the core below
	service := skillapp.New(store, fsscan.Scanner{}, registryLocker{store: store, noWait: parsed.skipIfBusy}, d.clock())
	runRegistryRefreshCore(service, d.workingDir, home, args, p.stdout, p.stderr, p.exit)
}

// runRegistryRefreshCore is the testable core of 'registry refresh': it reads the command line,
// runs the use case and tells the outcome in the words of gentle-ai, unless --quiet. A failure is
// said on stderr and ends with exit 1, with the registry as it was.
func runRegistryRefreshCore(service *skillapp.Service, workingDir func() (string, error), home string, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := parseRefreshArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	cwd := parsed.cwd
	if !filepath.IsAbs(cwd) {
		wd, err := workingDir()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot resolve %s: %v\n", cwd, err)
			exit(1)
			return
		}
		cwd = filepath.Join(wd, cwd)
	}
	cwd = filepath.Clean(cwd)

	result, err := service.Refresh(skillapp.Request{Cwd: cwd, Home: home, Force: parsed.force})
	if err != nil && parsed.skipIfBusy && errors.Is(err, filelock.ErrBusy) {
		return
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	if parsed.quiet {
		return
	}
	switch result.Outcome {
	case skillapp.Skipped:
		fmt.Fprintf(stdout, "Skill registry refresh skipped (no-project-marker): %s is not a project root; run it from a project directory (one containing .git or .atl), or create the project first.\n", cwd)
	case skillapp.UpToDate:
		fmt.Fprintf(stdout, "Skill registry up to date (cache-hit): %s\n", result.Registry)
	case skillapp.Refreshed:
		fmt.Fprintf(stdout, "Skill registry refreshed (%d skills): %s\n", result.Skills, result.Registry)
	}
}
