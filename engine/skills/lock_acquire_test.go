package skills

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// The typed side of the locks: what AcquireLocks and ReadConsistently answer, without a verb, a
// stream or an exit, so a caller that is not the old dispatcher says it in its own way.

// scriptedLocker is a Locker whose Exists answers from a script, so that a lock file can
// appear, or fail to be inspected, at the moment a test chooses. Taking a lock records it.
type scriptedLocker struct {
	recordingLocker
	exists func(path string) error
}

func (l *scriptedLocker) Exists(path string) error { return l.exists(path) }

func absentEverywhere(string) error { return fs.ErrNotExist }

func TestAcquireLocksWithNothingToLockNeedsNoLocker(t *testing.T) {
	held, err := AcquireLocks("lint", nil, nil)
	if err != nil || len(held.Provisional) != 0 {
		t.Fatalf("AcquireLocks = %+v, %v, want no error and nothing held", held, err)
	}
	held.Release() // the zero value releases nothing, and does not panic
	held.Release()
}

func TestAcquireLocksRefusesToRunUnserialized(t *testing.T) {
	_, err := AcquireLocks("add", nil, []LockRequest{{Path: "/o/.r.lock", Mode: LockExclusive}})
	var refusal *LockError
	if !errors.As(err, &refusal) || refusal.Failure != LockNotConfigured || refusal.ExitCode() != 1 {
		t.Fatalf("err = %v, want LockNotConfigured with exit 1", err)
	}
	if want := "skills add: no lock is configured, so it will not run unserialized with the other skills commands"; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
}

func TestAcquireLocksTakesThemInOrderAndReleasesThemInReverse(t *testing.T) {
	locker := &recordingLocker{}
	held, err := AcquireLocks("install", locker, []LockRequest{
		{Path: "/o/.r.lock", Mode: LockShared},
		{Path: "/p", Dir: true, Mode: LockExclusive},
	})
	if err != nil {
		t.Fatal(err)
	}
	held.Release()
	want := []string{"lock shared /o/.r.lock", "lockdir exclusive /p", "unlock /p", "unlock /o/.r.lock"}
	if got := locker.log(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestAcquireLocksDoesNotAskForALockWhoseRegistryCannotBeInspected(t *testing.T) {
	locker := &scriptedLocker{exists: func(string) error { return errors.New("permission denied") }}
	_, err := AcquireLocks("remove", locker, []LockRequest{{Path: "/o/.r.lock", Mode: LockExclusive, Registry: "/o/r.yaml"}})
	var refusal *LockError
	if !errors.As(err, &refusal) || refusal.Failure != LockRegistryUnreadable || refusal.ExitCode() != 1 {
		t.Fatalf("err = %v, want LockRegistryUnreadable with exit 1", err)
	}
	if want := `skills remove: reading registry "/o/r.yaml": permission denied; nothing was locked and nothing was changed`; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
	if got := locker.log(); len(got) != 0 {
		t.Errorf("a lock was asked for: %v", got)
	}
}

func TestAcquireLocksLeavesTheRegistryCheckOutWhenTheRequestSaysSo(t *testing.T) {
	locker := &scriptedLocker{exists: func(string) error { return errors.New("not asked") }}
	held, err := AcquireLocks("approve", locker, []LockRequest{{Path: "/o/.r.lock", Mode: LockExclusive, Registry: "/o/r.yaml", SkipRegistryCheck: true}})
	if err != nil {
		t.Fatalf("err = %v, want the registry not to be inspected", err)
	}
	held.Release()
}

func TestAcquireLocksSaysABusyLockAndLetsGoOfTheOnesItHolds(t *testing.T) {
	locker := &recordingLocker{failOn: map[string]error{"/p": busyErr{"/real/p"}}}
	_, err := AcquireLocks("install", locker, []LockRequest{
		{Path: "/o/.r.lock", Mode: LockShared, Subject: "the registry /o/r.yaml"},
		{Path: "/p", Dir: true, Mode: LockExclusive, Subject: "the project /p"},
	})
	var refusal *LockError
	if !errors.As(err, &refusal) || refusal.Failure != LockBusy || refusal.ExitCode() != ExitBusy {
		t.Fatalf("err = %v, want LockBusy with exit %d", err, ExitBusy)
	}
	want := "skills install: another skills command is in progress for the project /p (lock on the directory /p); nothing was changed, retry in a moment"
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
	if got := locker.log(); got[len(got)-1] != "unlock /o/.r.lock" {
		t.Errorf("events = %v, want the lock it held let go of", got)
	}
}

type lockPathErr struct{ path string }

func (e lockPathErr) Error() string    { return "no" }
func (e lockPathErr) Busy() bool       { return true }
func (e lockPathErr) LockPath() string { return e.path }

func TestAcquireLocksNamesTheLockAsTheLockerTriedIt(t *testing.T) {
	locker := &recordingLocker{fail: lockPathErr{"/resolved/p"}}
	_, err := AcquireLocks("adopt", locker, []LockRequest{{Path: "/link/p", Dir: true, Mode: LockExclusive, Subject: "the project /link/p"}})
	if err == nil || !strings.Contains(err.Error(), "lock on the directory /resolved/p") {
		t.Errorf("err = %v, want the directory the locker really locked", err)
	}
}

func TestAcquireLocksSaysALockThatCannotBeTakenAtAllWithExitOne(t *testing.T) {
	locker := &recordingLocker{fail: errors.New("read-only file system")}
	_, err := AcquireLocks("sync-manifest", locker, []LockRequest{{Path: "/o/.r.lock", Mode: LockExclusive}})
	var refusal *LockError
	if !errors.As(err, &refusal) || refusal.Failure != LockUnavailable || refusal.ExitCode() != 1 {
		t.Fatalf("err = %v, want LockUnavailable with exit 1", err)
	}
	if want := "skills sync-manifest: cannot take the lock /o/.r.lock: read-only file system"; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
	if !errors.Is(err, refusal.Err) {
		t.Error("the cause is not reachable with errors.Is")
	}
}

func TestAcquireLocksRemembersTheSharedLockFilesThatDidNotExist(t *testing.T) {
	locker := &scriptedLocker{exists: absentEverywhere}
	held, err := AcquireLocks("validate", locker, []LockRequest{{Path: "/o/.r.lock", Mode: LockShared, Rereads: true}})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if len(held.Provisional) != 1 || held.Provisional[0] != "/o/.r.lock" {
		t.Errorf("Provisional = %v, want the lock file that held nothing", held.Provisional)
	}

	present := &scriptedLocker{exists: func(string) error { return nil }}
	held2, _ := AcquireLocks("validate", present, []LockRequest{{Path: "/o/.r.lock", Mode: LockShared, Rereads: true}})
	if len(held2.Provisional) != 0 {
		t.Errorf("Provisional = %v for a lock file that exists, want none", held2.Provisional)
	}
	held2.Release()
}

func validateRequest() []LockRequest {
	return []LockRequest{{Path: "/o/.r.lock", Mode: LockShared, Rereads: true, Subject: "the registry /o/r.yaml"}}
}

func TestReadConsistentlyReadsOnceWhenTheLockFileExists(t *testing.T) {
	locker := &scriptedLocker{exists: func(string) error { return nil }}
	reads := 0
	got, err := ReadConsistently("validate", locker, validateRequest(), func() string { reads++; return "read" })
	if err != nil || got != "read" || reads != 1 {
		t.Errorf("ReadConsistently = %q, %v after %d reads, want one read", got, err, reads)
	}
	if ev := locker.log(); len(ev) != 2 || ev[1] != "unlock /o/.r.lock" {
		t.Errorf("events = %v, want the lock taken and let go of", ev)
	}
}

func TestReadConsistentlyReadsAgainWhenTheFirstWriterAppearedDuringTheRead(t *testing.T) {
	created := false
	locker := &scriptedLocker{exists: func(string) error {
		if created {
			return nil
		}
		return fs.ErrNotExist
	}}
	reads := 0
	got, err := ReadConsistently("validate", locker, validateRequest(), func() int {
		reads++
		if reads == 1 {
			created = true // the first writer takes its lock while this read is made
		}
		return reads
	})
	if err != nil || got != 2 || reads != 2 {
		t.Errorf("ReadConsistently = %d, %v after %d reads, want the second read, which is the one that stands", got, err, reads)
	}
}

func TestReadConsistentlyKeepsTheReadWhenNoWriterBegan(t *testing.T) {
	locker := &scriptedLocker{exists: absentEverywhere}
	reads := 0
	got, err := ReadConsistently("validate", locker, validateRequest(), func() int { reads++; return reads })
	if err != nil || got != 1 || reads != 1 {
		t.Errorf("ReadConsistently = %d, %v after %d reads, want the read to stand: the lock file never appeared", got, err, reads)
	}
}

func TestReadConsistentlyGivesUpWhenTheRegistryKeepsChanging(t *testing.T) {
	locker := &scriptedLocker{}
	calls := 0
	locker.exists = func(string) error {
		calls++
		if calls%2 == 1 { // asked before the lock is taken: absent; asked after the read: there
			return fs.ErrNotExist
		}
		return nil
	}
	reads := 0
	_, err := ReadConsistently("validate", locker, validateRequest(), func() int { reads++; return reads })
	var race *ReadRaceError
	if !errors.As(err, &race) || race.ExitCode() != ExitBusy || race.Attempts != maxRereadAttempts || reads != maxRereadAttempts {
		t.Fatalf("err = %v after %d reads, want a ReadRaceError with exit %d after %d attempts", err, reads, ExitBusy, maxRereadAttempts)
	}
	if want := "skills validate: the registry kept changing while it was being read (3 attempts); nothing was changed, retry in a moment"; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
}

func TestReadConsistentlyRefusesAReadItCannotTellWasNotTorn(t *testing.T) {
	calls := 0
	locker := &scriptedLocker{}
	locker.exists = func(string) error {
		calls++
		if calls == 1 {
			return fs.ErrNotExist
		}
		return errors.New("permission denied")
	}
	reads := 0
	_, err := ReadConsistently("validate", locker, validateRequest(), func() int { reads++; return reads })
	var race *ReadRaceError
	if !errors.As(err, &race) || race.ExitCode() != 1 || reads != 1 {
		t.Fatalf("err = %v after %d reads, want a ReadRaceError with exit 1 and no second read", err, reads)
	}
	if want := "skills validate: cannot tell whether a writer began while it read: permission denied; nothing was changed"; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
}

func TestReadConsistentlyDoesNotReadWhenTheLocksCannotBeTaken(t *testing.T) {
	locker := &recordingLocker{fail: busyErr{"/o/.r.lock"}}
	reads := 0
	_, err := ReadConsistently("validate", locker, validateRequest(), func() int { reads++; return reads })
	var refusal *LockError
	if !errors.As(err, &refusal) || refusal.Failure != LockBusy || reads != 0 {
		t.Errorf("err = %v after %d reads, want the lock refusal and no read", err, reads)
	}
}

// The overlay lock a use case takes is the one the dispatcher takes for the same verb and the
// same registry: one policy, said once.
func TestOverlayLocksAreTheLocksTheDispatcherTakes(t *testing.T) {
	const registry = "/o/skills.registry.yaml"
	for _, verb := range []string{"approve", "install", "adopt"} {
		want := lockRequestsFor(verb, []string{verb, "--registry", registry}, "")
		if got := OverlayLocks(verb, registry); len(got) != 1 || len(want) != 1 || got[0] != want[0] {
			t.Errorf("%s: OverlayLocks = %+v, the dispatcher takes %+v", verb, got, want)
		}
	}
	// The verbs that run behind a use case take their lock in their adapter (engine/cmd), which asks
	// OverlayLocks, so the dispatcher takes none for them.
	for _, verb := range []string{"add", "remove", "sync-manifest"} {
		if got := lockRequestsFor(verb, []string{verb, "--registry", registry}, ""); len(got) != 0 {
			t.Errorf("%s: the dispatcher takes %+v, want none: the adapter of the use case takes the lock", verb, got)
		}
		if got := OverlayLocks(verb, registry); len(got) != 1 || got[0].Mode != LockExclusive || got[0].Path != "/o/.skills.registry.yaml.lock" {
			t.Errorf("%s: OverlayLocks = %+v, want an exclusive lock on the lock file beside the registry", verb, got)
		}
	}
	for _, verb := range []string{"list", "status", "lint", "project-status", "nuke", ""} {
		if got := OverlayLocks(verb, registry); len(got) != 0 {
			t.Errorf("%s: OverlayLocks = %+v, want no overlay lock", verb, got)
		}
	}
	// validate is run by a use case, not by the dispatcher, so the policy is only said here.
	if got := OverlayLocks("validate", registry); got[0].Mode != LockShared || !got[0].Rereads || got[0].Path != "/o/.skills.registry.yaml.lock" {
		t.Errorf("validate: %+v, want a shared lock on the lock file beside the registry that is read again if the file appears", got[0])
	}
	if got := OverlayLocks("approve", "/o/team.registry.yaml"); !got[0].SkipRegistryCheck {
		t.Error("approve with an explicit registry path takes it as the name of the lock, and does not need the registry to exist")
	}
}
