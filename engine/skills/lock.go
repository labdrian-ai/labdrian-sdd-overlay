package skills

// The overlay lock: what SkillsCoreAt holds around a verb so that the registry,
// the manifest, the approval records, and the skill tree it reads never change
// under it.

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// LockMode is how a lock is held.
type LockMode int

const (
	// LockExclusive is a writer's lock: one holder, and no shared ones.
	LockExclusive LockMode = iota
	// LockShared is a reader's lock: any number of holders, none while an
	// exclusive one holds. A shared lock never creates the lock file, so a reader
	// works on a tree it cannot write to.
	LockShared
)

// Locker takes advisory locks on behalf of SkillsCoreAt. It is an interface, and
// this package does not implement it, because the implementation needs system
// calls and a clock and this package's import allowlist (zero_fetch_test.go)
// admits neither: engine/cmd passes one built on engine/filelock, and tests pass
// fakes.
//
// Lock waits at most a bound of its own choosing. It returns the function that
// releases the lock, or an error: an error that answers Busy() bool with true
// (engine/filelock's BusyError does), directly or through Unwrap, means the lock
// stayed taken past the bound and the call can be retried; any other error means
// the lock could not be taken at all.
type Locker interface {
	// Lock takes the lock on the lock file at path, creating it for an exclusive
	// lock and never for a shared one.
	Lock(path string, mode LockMode) (unlock func(), err error)
	// LockDir takes the lock on the directory dir itself, creating nothing in it:
	// for a directory that belongs to the user, such as a project root. It fails,
	// rather than doing without, when dir cannot be locked.
	LockDir(dir string, mode LockMode) (unlock func(), err error)
}

// ExitBusy is the exit code of a skills verb that could not run because another
// one holds the lock. It is distinct from 1, which means the verb ran, or was
// refused, and did not succeed: a caller that sees 2 changed nothing and can
// retry.
const ExitBusy = 2

// RegistryLockPath is the lock file that serializes every verb working on the
// registry at registryPath: a dot-named sibling of the registry, so it sits beside
// what it protects, is skipped by the skills tree scan (ScanSkillFiles ignores
// dot-names), and is covered by one .gitignore line. It is never removed.
func RegistryLockPath(registryPath string) string {
	return filepath.Join(filepath.Dir(registryPath), "."+filepath.Base(registryPath)+".lock")
}

// lockRequest is one lock a verb needs before it runs.
type lockRequest struct {
	path    string
	dir     bool // path is a directory locked itself (a project root), not a lock file
	mode    LockMode
	subject string // what the lock protects, for the messages
}

// lockRequestsFor lists, in the order they must be taken, the locks a verb needs.
//
// Overlay lock, keyed by the registry the verb names (--registry, or the default
// every verb shares):
//
//   - add, remove, sync-manifest, approve: exclusive. Each reads shared state,
//     decides, and writes it back; two of them interleaved lose an update, and a
//     reader in between sees the registry and the manifest disagree, because the
//     pair is written as two renames. approve is here because a second approval of
//     the same bytes must find the first one's record valid and leave it alone, and
//     because install must never find an approval half written in a skill directory.
//   - validate, install: shared. validate compares the registry with the manifest
//     and must not see the pair between its two renames; install copies skill
//     directories the writers above touch. Neither writes anything in the overlay,
//     so shared is enough, any number of them run together, and a shared lock never
//     creates the lock file, so both work on a read-only overlay.
//   - list, status, lint: one atomic file or none, so none.
//
// Project lock, keyed by the project root directory, taken on the directory itself
// so that no file appears in the user's repository:
//
//   - project-register, project-revise, project-retire, install: exclusive. Each
//     reads the project's lock file (.labdrian/procedural-skills.lock.json), decides,
//     and writes it back together with the skill files it stages, so two of them on
//     one root lose an update, or leave a skill written and unregistered. install is
//     the same read-decide-write on the project's .claude/skills. A verb that finds
//     no usable project root (the flag is missing, has no value, or is relative)
//     takes no lock: it refuses before it reads or writes anything.
//   - project-status only reads and reports; it takes none.
//
// LOCK ORDER. A verb that needs both takes the overlay lock first and the project
// lock second, and lets go in the opposite order. Every verb that holds two locks
// must follow this, including the ones added later (install's ownership work
// inherits it); the requests below are listed in that order, and
// TestLockRequestsAreAlwaysOverlayBeforeProject fails if a verb asks for them the
// other way round.
//
// Every read of shared state happens after the locks are held, in the verb itself,
// so nothing decided before the lock is trusted after it.
func lockRequestsFor(verb string, args []string) []lockRequest {
	var requests []lockRequest
	overlay := func(mode LockMode) {
		registryPath, _, _, _, _, _ := parseFlags(args)
		requests = append(requests, lockRequest{
			path:    RegistryLockPath(registryPath),
			mode:    mode,
			subject: "the registry " + registryPath,
		})
	}
	project := func(root string) {
		requests = append(requests, lockRequest{
			path:    filepath.Clean(root),
			dir:     true,
			mode:    LockExclusive,
			subject: "the project " + filepath.Clean(root),
		})
	}
	switch verb {
	case "add", "remove", "sync-manifest", "approve":
		overlay(LockExclusive)
	case "validate":
		overlay(LockShared)
	case "install":
		overlay(LockShared)
		// install writes into the working directory. If that cannot be resolved the
		// verb reports it itself, before it writes anything.
		if cwd, err := installCwd(); err == nil && filepath.IsAbs(cwd) {
			project(cwd)
		}
	case "project-register", "project-revise", "project-retire":
		if root, ok := projectRootArg(args); ok {
			project(root)
		}
	}
	return requests
}

// projectRootArg is the project root the project verbs will use, read the way they
// read it: the last --project-root before an end-of-options marker, whose value must
// be present, must not look like a flag, and must be an absolute path. When it is
// not, the verb is about to refuse, before it reads or writes anything, so there is
// nothing to lock.
func projectRootArg(args []string) (string, bool) {
	root := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		if args[i] != "--project-root" {
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			return "", false
		}
		root = args[i+1]
		i++
	}
	if root == "" || !filepath.IsAbs(root) {
		return "", false
	}
	return root, true
}

// acquireLocks takes every lock the verb needs, in order, and returns the function
// that releases them in reverse. When one cannot be taken it releases those it
// holds, says why, calls exit (ExitBusy for a lock that stayed taken, 1 for one
// that could not be taken at all), and reports false: the verb must not run.
//
// The lock is held until the release function runs. A production caller exits the
// process from inside the verb, which skips it; the kernel frees the lock when the
// process ends.
func acquireLocks(verb string, args []string, locker Locker, stderr io.Writer, exit func(int)) (release func(), ok bool) {
	requests := lockRequestsFor(verb, args)
	if len(requests) == 0 {
		return func() {}, true
	}
	if locker == nil {
		fmt.Fprintf(stderr, "error: skills %s: no lock is configured, so it will not run unserialized with the other skills commands\n", verb)
		exit(1)
		return nil, false
	}
	var unlocks []func()
	releaseAll := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, req := range requests {
		var unlock func()
		var err error
		what := "lock " + req.path
		if req.dir {
			what = "lock on the directory " + req.path
			unlock, err = locker.LockDir(req.path, req.mode)
		} else {
			unlock, err = locker.Lock(req.path, req.mode)
		}
		if err != nil {
			releaseAll()
			if isBusy(err) {
				fmt.Fprintf(stderr, "error: skills %s: another skills command is in progress for %s (%s); nothing was changed, retry in a moment\n", verb, req.subject, what)
				exit(ExitBusy)
			} else {
				fmt.Fprintf(stderr, "error: skills %s: cannot take the %s: %v\n", verb, what, err)
				exit(1)
			}
			return nil, false
		}
		unlocks = append(unlocks, unlock)
	}
	return releaseAll, true
}

// isBusy reports whether err, or an error it wraps, says the lock stayed taken.
// It asks by method, not by type, because this package cannot import the package
// that defines the locker's errors.
func isBusy(err error) bool {
	for depth := 0; err != nil && depth < 16; depth++ {
		if b, ok := err.(interface{ Busy() bool }); ok && b.Busy() {
			return true
		}
		wrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = wrapped.Unwrap()
	}
	return false
}
