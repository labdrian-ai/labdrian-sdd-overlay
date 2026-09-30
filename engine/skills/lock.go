package skills

// The overlay lock: what SkillsCoreAt holds around a verb so that the registry,
// the manifest, the approval records, and the skill tree it reads never change
// under it.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LockMode is how a lock is held.
type LockMode int

const (
	// LockExclusive is a writer's lock: one holder, and no shared ones.
	LockExclusive LockMode = iota
	// LockShared is a reader's lock: any number of holders, none while an
	// exclusive one holds. A shared lock never creates the lock file, so a reader
	// works on a tree it cannot write to; on a lock file that does not exist it holds
	// nothing, which is why validate checks afterwards that the file is still absent.
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

// defaultRegistryPath is the registry a verb works on when it is given no
// --registry: relative, so it means the working directory of the process.
const defaultRegistryPath = "skills.registry.yaml"

// maxRereadAttempts bounds how many times a reader that raced the first writer
// reads: the first read and the reads that follow it. See
// rereadsWhenTheLockFileAppears.
const maxRereadAttempts = 3

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
	// registry, when set, is a registry file that must exist before this lock is
	// asked for, because asking creates the lock file beside it. The exception is a
	// verb that never reads the registry (approve) and was told which registry's
	// lock to take: there the path is only the name of the lock.
	registry         string
	registryOptional bool
	// rereads marks a shared lock on a lock file whose holder reads several files
	// that must agree. When that file does not exist, the lock holds nothing (see
	// filelock.Shared), so the verb's read is provisional: see
	// rereadsWhenTheLockFileAppears.
	rereads bool
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
//     creates the lock file, so both work on a read-only overlay. A shared lock on a
//     lock file that does not exist yet holds nothing, and validate, which reads
//     files that must agree, is marked to read again when the file appears during its
//     read (rereadsWhenTheLockFileAppears). install is not: it reads one atomic
//     file, the registry, and a tree that no locked verb writes into except the
//     approval record and the temporary file behind it, which it never copies.
//   - list, status, lint: one atomic file or none, so none.
//
// Project lock, keyed by the project root directory, taken on the directory itself
// so that no file appears in the user's repository:
//
//   - project-register, project-revise, project-retire, install, adopt: exclusive.
//     Each reads the project's lock file (.labdrian/procedural-skills.lock.json),
//     decides, and writes it back together with the skill files it stages, so two of
//     them on one root lose an update, or leave a skill written and unregistered.
//     install is the same read-decide-write on the project's .claude/skills and
//     .agents/skills, and adopt records what is there. A verb that finds no usable
//     project root (the flag is missing, has no value, or is relative) takes no
//     lock: it refuses before it reads or writes anything.
//   - project-status: shared. It only reads, so any number run together, but it
//     reads the lock file and then the skill files it lists, and between a revision's
//     renames those disagree. Waiting for the writer is what keeps it from reporting
//     a skill as human-owned for the moment it takes the writer to finish.
//
// The project root is read once, by the same code the verb reads it with:
// parseProjectArgs for the project verbs, and for install the working directory,
// which SkillsCoreAt resolves before it asks for any lock and hands to the verb as
// installRoot, so that what is locked is what is written. A verb that cannot name a
// usable root takes no lock.
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
func lockRequestsFor(verb string, args []string, installRoot string) []lockRequest {
	var requests []lockRequest
	overlay := func(mode LockMode, rereads bool) {
		registryPath, _, _, _, _, _ := parseFlags(args)
		req := lockRequest{
			path:    RegistryLockPath(registryPath),
			mode:    mode,
			subject: "the registry " + registryPath,
			rereads: rereads,
		}
		if mode == LockExclusive {
			// An exclusive lock creates the lock file, and a lock file created beside a
			// registry that does not exist (a raw call made from any directory, with the
			// default registry path) is litter in a directory that is not an overlay.
			// add, remove, and sync-manifest read the registry and would fail anyway.
			// approve does not read it, so an explicit path is taken as the name of the
			// lock; the default path, which is only the working directory's, is not.
			req.registry = registryPath
			req.registryOptional = verb == "approve" && registryPath != defaultRegistryPath
		}
		requests = append(requests, req)
	}
	project := func(root string, mode LockMode) {
		requests = append(requests, lockRequest{
			path:    filepath.Clean(root),
			dir:     true,
			mode:    mode,
			subject: "the project " + filepath.Clean(root),
		})
	}
	switch verb {
	case "add", "remove", "sync-manifest", "approve":
		overlay(LockExclusive, false)
	case "validate":
		overlay(LockShared, true)
	case "install", "adopt":
		overlay(LockShared, false)
		if installRoot != "" {
			project(installRoot, LockExclusive)
		}
	case "project-register", "project-revise", "project-retire":
		if root, ok := projectRootArg(verb, args); ok {
			project(root, LockExclusive)
		}
	case "project-status":
		if root, ok := projectRootArg(verb, args); ok {
			project(root, LockShared)
		}
	}
	return requests
}

// projectRootArg is the project root a project verb will use: what parseProjectArgs,
// the parser the verb itself uses, reads from the arguments that follow the verb's
// name. When the command line does not parse, or the root is missing or not
// absolute, the verb is about to refuse before it reads or writes anything, so there
// is nothing to lock.
func projectRootArg(verb string, args []string) (string, bool) {
	spec, ok := projectArgSpecs[verb]
	if !ok {
		return "", false
	}
	parsed, err := parseProjectArgs(spec, stripVerb(args, verb))
	if err != nil || parsed.Root == "" || !filepath.IsAbs(parsed.Root) {
		return "", false
	}
	return parsed.Root, true
}

// acquireLocks takes every lock the verb needs, in order, and returns the function
// that releases them in reverse. When one cannot be taken it releases those it
// holds, says why, calls exit (ExitBusy for a lock that stayed taken, 1 for one
// that could not be taken at all), and reports false: the verb must not run.
//
// The lock is held until the release function runs. A production caller exits the
// process from inside the verb, which skips it; the kernel frees the lock when the
// process ends.
//
// provisional lists the lock files of the requests marked rereads that did not exist
// when they were asked for, whose shared locks therefore hold nothing:
// rereadsWhenTheLockFileAppears says what the caller does about them.
func acquireLocks(verb string, args []string, installRoot string, locker Locker, stderr io.Writer, exit func(int)) (release func(), provisional []string, ok bool) {
	requests := lockRequestsFor(verb, args, installRoot)
	if len(requests) == 0 {
		return func() {}, nil, true
	}
	if locker == nil {
		fmt.Fprintf(stderr, "error: skills %s: no lock is configured, so it will not run unserialized with the other skills commands\n", verb)
		exit(1)
		return nil, nil, false
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
		if req.registry != "" && !req.registryOptional {
			if _, statErr := os.Stat(req.registry); statErr != nil {
				releaseAll()
				fmt.Fprintf(stderr, "error: skills %s: reading registry %q: %v; nothing was locked and nothing was changed\n", verb, req.registry, statErr)
				exit(1)
				return nil, nil, false
			}
		}
		if req.rereads && !req.dir && req.mode == LockShared {
			// Looked at before the locker is asked: a file that is absent now may be
			// created by the time the locker opens it, which only costs one more read.
			if _, statErr := os.Stat(req.path); os.IsNotExist(statErr) {
				provisional = append(provisional, req.path)
			}
		}
		if req.dir {
			unlock, err = locker.LockDir(req.path, req.mode)
		} else {
			unlock, err = locker.Lock(req.path, req.mode)
		}
		if err != nil {
			releaseAll()
			// The lock that failed is named as the locker tried it, which can differ
			// from the path asked for: a directory reached through a symlink is locked
			// where it really is, and that is the one another process holds.
			locked := lockedPath(err, req.path)
			what := "lock " + locked
			if req.dir {
				what = "lock on the directory " + locked
			}
			if isBusy(err) {
				fmt.Fprintf(stderr, "error: skills %s: another skills command is in progress for %s (%s); nothing was changed, retry in a moment\n", verb, req.subject, what)
				exit(ExitBusy)
			} else {
				fmt.Fprintf(stderr, "error: skills %s: cannot take the %s: %v\n", verb, what, err)
				exit(1)
			}
			return nil, nil, false
		}
		unlocks = append(unlocks, unlock)
	}
	return releaseAll, provisional, true
}

// rereadsWhenTheLockFileAppears is the answer to the one case a shared lock cannot
// cover. A shared lock on a lock file that does not exist holds nothing, so the
// first writer ever can create the file, take the lock, and change the registry and
// the manifest (two renames) while the reader reads them. A writer creates the file
// before it writes anything, so if the file is still absent when the read is done,
// no write began during it and the read stands; if it exists now, the read may have
// seen the pair between its renames, and is discarded. The caller then reads again,
// and this time the file exists, so the lock it takes is real and waits for the
// writer to finish. The bound is for a file that is removed again between attempts,
// which nothing in this program does.
func rereadsWhenTheLockFileAppears(provisional []string) bool {
	for _, path := range provisional {
		// Anything but "still absent" counts as appeared: a stat that fails for
		// another reason cannot prove that no writer began.
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// maxErrorChain bounds a walk along wrapped errors. A real chain is a few links
// long. The bound is for one that never ends: an error type can unwrap to itself or
// to a cycle, nothing in the language forbids it, and this walk runs while a verb is
// reporting a failed lock, where hanging would be the worst answer.
const maxErrorChain = 16

// isBusy reports whether err, or an error it wraps, says the lock stayed taken.
// It asks by method, not by type, because this package cannot import the package
// that defines the locker's errors.
func isBusy(err error) bool {
	for depth := 0; err != nil && depth < maxErrorChain; depth++ {
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

// lockedPath is the path a failed lock was tried on: the one the error names with a
// LockPath method (engine/filelock's BusyError has one), looked for the way isBusy
// looks for Busy, or else the path that was asked for.
func lockedPath(err error, asked string) string {
	for depth := 0; err != nil && depth < maxErrorChain; depth++ {
		if n, ok := err.(interface{ LockPath() string }); ok && n.LockPath() != "" {
			return n.LockPath()
		}
		wrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = wrapped.Unwrap()
	}
	return asked
}
