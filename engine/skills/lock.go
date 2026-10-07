package skills

// The overlay lock: what SkillsCoreAt holds around a verb so that the registry,
// the manifest, the approval records, and the skill tree it reads never change
// under it.

import (
	"errors"
	"fmt"
	"io"
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

// Locker takes advisory locks on behalf of SkillsCoreAt, and tells whether a file is there. It
// is an interface, and this package does not implement it, because the implementation needs
// system calls and a clock and this package's import allowlist (zero_fetch_test.go) admits
// neither: engine/cmd passes one built on engine/filelock, and tests pass fakes.
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
	// Exists says whether path is there: nil when it is, and otherwise the failure that says why
	// it is not, with the words of the system. An error that is(fs.ErrNotExist) means the path
	// is absent, which is an answer; any other (a permission, a loop of links) means the
	// question could not be answered. The domain asks it of the lock files and the registry
	// it is about to lock, because the locker is what knows where a lock lives.
	Exists(path string) error
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

// LockRequest is one lock a verb needs before it runs.
type LockRequest struct {
	// Path is the lock file, or, for Dir, the directory locked itself.
	Path string
	// Dir says Path is a directory locked itself (a project root), not a lock file.
	Dir  bool
	Mode LockMode
	// Subject is what the lock protects, for the messages.
	Subject string
	// Registry, when set, is a registry file that must exist before this lock is
	// asked for, because asking creates the lock file beside it. The exception is a
	// verb that never reads the registry (approve) and was told which registry's
	// lock to take: there the path is only the name of the lock, and SkipRegistryCheck
	// is set, which leaves out the check that the registry exists before the lock is
	// taken.
	Registry          string
	SkipRegistryCheck bool
	// Rereads marks a shared lock on a lock file whose holder reads several files
	// that must agree. When that file does not exist, the lock holds nothing (see
	// filelock.Shared), so the verb's read is provisional: see
	// rereadsWhenTheLockFileAppears.
	Rereads bool
}

// overlayLockRequest is the lock on the overlay of the registry at registryPath that verb takes
// in mode: the lock file beside the registry, which serializes every verb working on it.
func overlayLockRequest(verb, registryPath string, mode LockMode, rereads bool) LockRequest {
	req := LockRequest{
		Path:    RegistryLockPath(registryPath),
		Mode:    mode,
		Subject: "the registry " + registryPath,
		Rereads: rereads,
	}
	if mode == LockExclusive {
		// An exclusive lock creates the lock file, and a lock file created beside a
		// registry that does not exist (a raw call made from any directory, with the
		// default registry path) is litter in a directory that is not an overlay.
		// add, remove, and sync-manifest read the registry and would fail anyway.
		// approve does not read it, so an explicit path is taken as the name of the
		// lock; the default path, which is only the working directory's, is not.
		req.Registry = registryPath
		req.SkipRegistryCheck = verb == "approve" && registryPath != defaultRegistryPath
	}
	return req
}

// OverlayLocks is the overlay lock verb takes on the registry at registryPath, as the verbs that
// work on an overlay take it: exclusive for add, remove, sync-manifest and approve, shared for
// validate, which reads again when the lock file appears during its read (ReadConsistently), and
// shared for install and adopt. A verb that takes none (list, status, lint) gets no request.
// Which verb needs which lock, and why, is explained at lockRequestsFor.
func OverlayLocks(verb, registryPath string) []LockRequest {
	switch verb {
	case "add", "remove", "sync-manifest", "approve":
		return []LockRequest{overlayLockRequest(verb, registryPath, LockExclusive, false)}
	case "validate":
		return []LockRequest{overlayLockRequest(verb, registryPath, LockShared, true)}
	case "install", "adopt":
		return []LockRequest{overlayLockRequest(verb, registryPath, LockShared, false)}
	}
	return nil
}

// ProjectLocks is the lock verb takes on the project whose root directory is root, as the verbs
// that install into a project or keep its lock take it: exclusive for install, adopt,
// project-register, project-revise and project-retire, shared for project-status. It is taken on
// the directory itself, so that no file appears in the user's repository. A verb that works on no
// project gets no request.
//
// A verb that needs both locks takes the overlay lock (OverlayLocks) first and the project lock
// second, and lets go in the opposite order; the caller puts the requests in that order.
func ProjectLocks(verb, root string) []LockRequest {
	mode := LockExclusive
	switch verb {
	case "install", "adopt", "project-register", "project-revise", "project-retire":
	case "project-status":
		mode = LockShared
	default:
		return nil
	}
	clean := filepath.Clean(root)
	return []LockRequest{{Path: clean, Dir: true, Mode: mode, Subject: "the project " + clean}}
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
//     read (rereadsWhenTheLockFileAppears; validate runs behind a use case, which takes
//     this lock through OverlayLocks and ReadConsistently, and this function no longer
//     lists it). install is not: it reads one atomic
//     file, the registry, and a tree that no locked verb writes into except the
//     approval record and the temporary file behind it, which it never copies.
//   - list, status, lint: one atomic file or none, so none. They run behind use cases in
//     skills/app and are not dispatched here.
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
func lockRequestsFor(verb string, args []string, installRoot string) []LockRequest {
	var requests []LockRequest
	project := func(root string, mode LockMode) {
		requests = append(requests, LockRequest{
			Path:    filepath.Clean(root),
			Dir:     true,
			Mode:    mode,
			Subject: "the project " + filepath.Clean(root),
		})
	}
	switch verb {
	case "install", "adopt":
		requests = append(requests, overlayLockRequest(verb, registryOfArgs(args), LockShared, false))
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

// registryOfArgs is the registry install and adopt lock: the last --registry of their arguments, or
// the default every verb shares. The flags that the wrapper appends take their value, so that the
// value of one is not read as a flag, as it always was; install and adopt read their own flags
// later, after the lock is held. It goes when these two verbs move behind a use case, which locks
// through OverlayLocks.
func registryOfArgs(args []string) string {
	registryPath := defaultRegistryPath
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			if i+1 < len(args) {
				registryPath = args[i+1]
				i++
			}
		case "--manifest", "--source-root", "--repo", "--ref":
			if i+1 < len(args) {
				i++
			}
		}
	}
	return registryPath
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

// LockFailure says in which way the locks of a verb were refused.
type LockFailure int

const (
	// LockNotConfigured: the verb needs a lock and no Locker was wired, so it will not run
	// unserialized with the other skills commands.
	LockNotConfigured LockFailure = iota + 1
	// LockRegistryUnreadable: the registry the lock is keyed by cannot be inspected, so the lock
	// file beside it is not asked for (asking would create it).
	LockRegistryUnreadable
	// LockBusy: the lock stayed taken past the bound of the locker; the verb changed nothing and
	// can be run again.
	LockBusy
	// LockUnavailable: the lock could not be taken at all.
	LockUnavailable
)

// LockError is why a verb did not get the locks it needs. It says it in the words a person is
// told ("skills <verb>: ..."), and which exit code answers it: ExitBusy for a lock that stayed
// taken, 1 for any other.
type LockError struct {
	Failure LockFailure
	// Verb names the verb in the words of the refusal.
	Verb string
	// Request is the lock that was being asked for; it is the zero request for LockNotConfigured.
	Request LockRequest
	// Locked is the path the locker tried, which can differ from Request.Path: a directory reached
	// through a symlink is locked where it really is.
	Locked string
	// Err is the cause: the failure to inspect the registry, or the locker's error.
	Err error
}

func (e *LockError) Unwrap() error { return e.Err }

// ExitCode is the code the verb exits with: ExitBusy when the lock stayed taken, 1 otherwise.
func (e *LockError) ExitCode() int {
	if e.Failure == LockBusy {
		return ExitBusy
	}
	return 1
}

func (e *LockError) Error() string {
	what := "lock " + e.Locked
	if e.Request.Dir {
		what = "lock on the directory " + e.Locked
	}
	switch e.Failure {
	case LockNotConfigured:
		return fmt.Sprintf("skills %s: no lock is configured, so it will not run unserialized with the other skills commands", e.Verb)
	case LockRegistryUnreadable:
		return fmt.Sprintf("skills %s: reading registry %q: %v; nothing was locked and nothing was changed", e.Verb, e.Request.Registry, e.Err)
	case LockBusy:
		return fmt.Sprintf("skills %s: another skills command is in progress for %s (%s); nothing was changed, retry in a moment", e.Verb, e.Request.Subject, what)
	default:
		return fmt.Sprintf("skills %s: cannot take the %s: %v", e.Verb, what, e.Err)
	}
}

// HeldLocks is what AcquireLocks took: Release lets go of it in the reverse of the order it was
// taken. Provisional lists the lock files of the requests marked Rereads that did not exist when
// they were asked for, whose shared locks therefore hold nothing: RereadsWhenTheLockFileAppears
// says what the caller does about them.
type HeldLocks struct {
	release     func()
	Provisional []string
}

// Release lets go of every lock. It is safe to call on the zero value and more than once.
func (h HeldLocks) Release() {
	if h.release != nil {
		h.release()
	}
}

// AcquireLocks takes every lock of requests, in order, for verb. When one cannot be taken it
// releases those it holds and returns a *LockError saying why: the verb must not run. No request
// needs no Locker.
//
// The lock is held until Release runs. A production caller exits the process from inside the verb,
// which skips it; the kernel frees the lock when the process ends.
func AcquireLocks(verb string, locker Locker, requests []LockRequest) (HeldLocks, error) {
	if len(requests) == 0 {
		return HeldLocks{}, nil
	}
	if locker == nil {
		return HeldLocks{}, &LockError{Failure: LockNotConfigured, Verb: verb}
	}
	var unlocks []func()
	releaseAll := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	var provisional []string
	for _, req := range requests {
		var unlock func()
		var err error
		if req.Registry != "" && !req.SkipRegistryCheck {
			if statErr := locker.Exists(req.Registry); statErr != nil {
				releaseAll()
				return HeldLocks{}, &LockError{Failure: LockRegistryUnreadable, Verb: verb, Request: req, Err: statErr}
			}
		}
		if req.Rereads && !req.Dir && req.Mode == LockShared {
			// Looked at before the locker is asked: a file that is absent now may be
			// created by the time the locker opens it, which only costs one more read.
			if isAbsent(locker.Exists(req.Path)) {
				provisional = append(provisional, req.Path)
			}
		}
		if req.Dir {
			unlock, err = locker.LockDir(req.Path, req.Mode)
		} else {
			unlock, err = locker.Lock(req.Path, req.Mode)
		}
		if err != nil {
			releaseAll()
			// The lock that failed is named as the locker tried it, which can differ
			// from the path asked for: a directory reached through a symlink is locked
			// where it really is, and that is the one another process holds.
			failure := LockUnavailable
			if isBusy(err) {
				failure = LockBusy
			}
			return HeldLocks{}, &LockError{Failure: failure, Verb: verb, Request: req, Locked: lockedPath(err, req.Path), Err: err}
		}
		unlocks = append(unlocks, unlock)
	}
	return HeldLocks{release: releaseAll, Provisional: provisional}, nil
}

// acquireLocks takes every lock the verb needs, in order, and returns the function
// that releases them in reverse. When one cannot be taken it releases those it
// holds, says why, calls exit (ExitBusy for a lock that stayed taken, 1 for one
// that could not be taken at all), and reports false: the verb must not run.
func acquireLocks(verb string, args []string, installRoot string, locker Locker, stderr io.Writer, exit func(int)) (release func(), ok bool) {
	held, err := AcquireLocks(verb, locker, lockRequestsFor(verb, args, installRoot))
	if err != nil {
		var refusal *LockError
		if errors.As(err, &refusal) {
			fmt.Fprintf(stderr, "error: %v\n", refusal)
			exit(refusal.ExitCode())
		}
		return nil, false
	}
	return held.Release, true
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
//
// There are three answers, not two. Absent: no write began, the read stands. Present:
// a writer may have begun, read again. Anything else (a stat that fails for another
// reason: a permission, a loop of links): the check cannot say either way, and that is
// neither a writer arriving nor a reason to read again, because the same failure will
// come back, and the bound would turn it into a busy exit that tells the caller to
// retry. It is returned, with the path it names, and the caller refuses with it.
//
// Every path is checked and the two signals are independent: appeared is true when any
// path appeared, and err is the first inspection failure, whatever order the paths came
// in and whichever signal was seen first. Neither overwrites the other. A caller that
// is handed both discards the read either way; it refuses on the error, because
// reading again cannot clear it, and the appeared flag is still there for a caller
// that needs to tell the two situations apart. (validate asks about one path, so the
// verbs cannot produce both; the function's answer is pinned by its own test.)
func rereadsWhenTheLockFileAppears(locker Locker, provisional []string) (appeared bool, err error) {
	for _, path := range provisional {
		statErr := locker.Exists(path)
		switch {
		case statErr == nil:
			appeared = true
		case !isAbsent(statErr) && err == nil:
			err = statErr
		}
	}
	return appeared, err
}

// ReadRaceError is why a read made under shared locks was not trusted and was not made again:
// either the lock file could not be inspected (Inspection), which reading again would meet
// again, or the registry kept changing while it was read for MaxRereadAttempts attempts.
type ReadRaceError struct {
	Verb string
	// Inspection is the failure to tell whether a writer began while the read was made; nil when
	// the read gave up because the lock file kept appearing.
	Inspection error
	// Attempts is how many reads were made.
	Attempts int
}

func (e *ReadRaceError) Unwrap() error { return e.Inspection }

// ExitCode is 1 for a lock file that could not be inspected, and ExitBusy for one that kept
// appearing, because the caller can retry the second and not the first.
func (e *ReadRaceError) ExitCode() int {
	if e.Inspection != nil {
		return 1
	}
	return ExitBusy
}

func (e *ReadRaceError) Error() string {
	if e.Inspection != nil {
		return fmt.Sprintf("skills %s: cannot tell whether a writer began while it read: %v; nothing was changed", e.Verb, e.Inspection)
	}
	return fmt.Sprintf("skills %s: the registry kept changing while it was being read (%d attempts); nothing was changed, retry in a moment", e.Verb, e.Attempts)
}

// ReadConsistently makes read under the locks of requests and returns what it read once that
// read is known to stand: a read made under a shared lock that held nothing (the lock file did
// not exist) is made again when the file appeared while it read, as rereadsWhenTheLockFileAppears
// explains. An error is a *LockError, when the locks could not be taken, or a *ReadRaceError,
// when the read cannot be trusted; in both nothing was returned and nothing was changed.
func ReadConsistently[T any](verb string, locker Locker, requests []LockRequest, read func() T) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		held, err := AcquireLocks(verb, locker, requests)
		if err != nil {
			return zero, err
		}
		result := read()
		raced, statErr := rereadsWhenTheLockFileAppears(locker, held.Provisional)
		held.Release()
		switch {
		case statErr != nil:
			// Whether a writer began cannot be known, so the read cannot be trusted. It wins over
			// raced: a read that may be torn is discarded either way, and reading again would meet
			// the same failure and end in a busy exit.
			return zero, &ReadRaceError{Verb: verb, Inspection: statErr, Attempts: attempt}
		case !raced:
			return result, nil
		case attempt >= maxRereadAttempts:
			return zero, &ReadRaceError{Verb: verb, Attempts: attempt}
		}
	}
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
