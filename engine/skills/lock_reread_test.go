package skills

// A shared lock on a lock file that does not exist yet holds nothing: no writer has
// ever taken the lock there, so nobody has been mid-write, and a reader must work
// on a tree it cannot write to. The first writer ever creates the file and writes
// while that reader reads, and the reader can then see the registry and the manifest
// disagree. validate is the reader that reads several files which must agree, so it
// checks, after its read, that the lock file is still absent, and reads again under a
// lock that is real when it is not. These tests force that interleaving with the
// read seam: the "first writer" runs inside validate's read of the registry.

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// firstWriterDuringTheRead returns a readFile that, the first time the registry is
// read, lets a complete `add` of id happen and the lock file appear, as they would if
// a writer took its lock at that moment. The reader that asked for the registry holds
// the bytes from before the add; the manifest it reads next is the one after it.
func firstWriterDuringTheRead(t *testing.T, f lockFixture, id string) (readFileFn, *int) {
	t.Helper()
	var once sync.Once
	reads := 0
	return func(name string) ([]byte, error) {
		data, err := os.ReadFile(name)
		if filepath.Clean(name) == filepath.Clean(f.reg) {
			reads++
			once.Do(func() {
				writeTestFile(t, f.lockPath, "")
				if r := runWriterToCompletion("add", append([]string{id}, f.flags()...)); r.code != 0 {
					t.Errorf("the writer's add: exit %d, stderr %q", r.code, r.stderr)
				}
			})
		}
		return data, err
	}, &reads
}

// runWriterToCompletion runs a verb the way the first writer's process would, to
// completion, without going through the lock under test.
func runWriterToCompletion(verb string, args []string) coreRun {
	return runAt(verb, args, os.ReadFile, nil, noopLocker{})
}

func TestValidateThatRacedTheFirstWriterReadsAgainUnderTheRealLock(t *testing.T) {
	f := newLockFixture(t).withSkills(t, "newbie")
	if _, err := os.Stat(f.lockPath); err == nil {
		t.Fatal("the fixture already has a lock file, so it is not a first-write scenario")
	}
	readFile, _ := firstWriterDuringTheRead(t, f, "newbie")
	locker := &recordingLocker{}

	r := runAt("validate", f.flags(), readFile, nil, locker)

	if r.code != 0 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q: validate reported the torn state it read while the first writer ran", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "aligned (2 skills)") {
		t.Errorf("stdout %q does not report the settled registry of 2 skills", r.stdout)
	}
	want := []string{
		"lock shared " + f.lockPath, "unlock " + f.lockPath, // the read that held nothing
		"lock shared " + f.lockPath, "unlock " + f.lockPath, // the read under the lock that is real
	}
	if got := locker.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("lock events = %v, want %v", got, want)
	}
}

// With no writer in sight the first read is the only one, and what it printed is
// printed once, in order, with its exit code.
func TestValidateWithNoLockFileAndNoWriterReadsOnce(t *testing.T) {
	f := newLockFixture(t)
	locker := &recordingLocker{}
	reads := 0
	readFile := func(name string) ([]byte, error) {
		if filepath.Clean(name) == filepath.Clean(f.reg) {
			reads++
		}
		return os.ReadFile(name)
	}

	r := runAt("validate", f.flags(), readFile, nil, locker)

	if r.code != 0 || !strings.Contains(r.stdout, "aligned (1 skills)") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if reads != 1 {
		t.Errorf("the registry was read %d times, want 1", reads)
	}
	if got := locker.log(); len(got) != 2 {
		t.Errorf("lock events = %v, want one lock and one unlock", got)
	}
}

// A validate that finds real divergences and no writer reports them, once, and
// exits 1: buffering the first read must not swallow or reorder a failure.
func TestValidateReportsARealDivergenceFromTheBufferedReadOnce(t *testing.T) {
	f := newLockFixture(t).withSkills(t, "unregistered")
	locker := &recordingLocker{}

	r := runAt("validate", f.flags(), os.ReadFile, nil, locker)

	if r.code != 1 || !strings.Contains(r.stderr, "unregistered") {
		t.Fatalf("exit %d, stderr %q, want exit 1 naming the unregistered skill", r.code, r.stderr)
	}
	if n := strings.Count(r.stderr, "[UNREGISTERED_ON_DISK]"); n != 1 {
		t.Errorf("the divergence was printed %d times, want once: %q", n, r.stderr)
	}
	if got := locker.log(); len(got) != 2 {
		t.Errorf("lock events = %v, want one lock and one unlock", got)
	}
}

// When the lock file exists the lock is real, the read is not repeated, and
// nothing is buffered: output appears as the verb prints it.
func TestValidateUnderARealLockIsNotRepeated(t *testing.T) {
	f := newLockFixture(t)
	writeTestFile(t, f.lockPath, "")
	locker := &recordingLocker{}
	reads := 0
	readFile := func(name string) ([]byte, error) {
		if filepath.Clean(name) == filepath.Clean(f.reg) {
			reads++
		}
		return os.ReadFile(name)
	}

	if r := runAt("validate", f.flags(), readFile, nil, locker); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if reads != 1 {
		t.Errorf("the registry was read %d times under a real lock, want 1", reads)
	}
}

// The retry is bounded. A lock file that keeps vanishing between attempts (here, a
// locker that removes it) cannot keep validate reading forever: it stops, says the
// registry kept changing, changes nothing, and exits with the busy code.
func TestValidateGivesUpWhenTheLockFileKeepsAppearingAndVanishing(t *testing.T) {
	f := newLockFixture(t)
	attempts := 0
	locker := &vanishingLocker{lockPath: f.lockPath, attempts: &attempts}
	readFile := func(name string) ([]byte, error) {
		if filepath.Clean(name) == filepath.Clean(f.reg) {
			writeTestFile(t, f.lockPath, "") // a writer is always just arriving
		}
		return os.ReadFile(name)
	}

	r := runAt("validate", f.flags(), readFile, nil, locker)

	if r.code != ExitBusy || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit %d and nothing on stdout", r.code, r.stdout, r.stderr, ExitBusy)
	}
	for _, want := range []string{"skills validate", "kept changing", "retry"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if attempts != maxRereadAttempts {
		t.Errorf("%d attempts, want the bound of %d", attempts, maxRereadAttempts)
	}
}

// Only "the file is still absent" proves that no writer began. A stat that fails for
// any other reason proves nothing either way, and is not evidence that a writer
// arrived: reading again until the bound and then calling it "busy" would tell the
// caller to retry a failure that will not clear. validate refuses, with exit 1 and the
// reason the system gave, after the first read.
func TestValidateRefusesWithTheRealReasonWhenTheLockFileCannotBeInspected(t *testing.T) {
	f := newLockFixture(t)
	loop := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skipf("cannot make a symbolic link that points at itself: %v", err)
	}
	_, statErr := os.Stat(loop)
	pathErr, ok := statErr.(*fs.PathError)
	if !ok || os.IsNotExist(statErr) {
		t.Fatalf("a symbolic link that points at itself gave %v, want a stat failure that is not 'does not exist'", statErr)
	}
	attempts := 0
	locker := &vanishingLocker{lockPath: f.lockPath, attempts: &attempts}
	readFile := func(name string) ([]byte, error) {
		if filepath.Clean(name) == filepath.Clean(f.reg) {
			// Something unusable appears where the lock file would be, during the
			// read; the locker removes it again when the lock is released.
			_ = os.Remove(f.lockPath)
			if err := os.Symlink(f.lockPath, f.lockPath); err != nil {
				t.Errorf("cannot stage the unusable lock path: %v", err)
			}
		}
		return os.ReadFile(name)
	}

	r := runAt("validate", f.flags(), readFile, nil, locker)

	if r.code != 1 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 1 and nothing on stdout", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"skills validate", f.lockPath, pathErr.Err.Error(), "nothing was changed"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	for _, unwanted := range []string{"kept changing", "retry"} {
		if strings.Contains(r.stderr, unwanted) {
			t.Errorf("stderr %q contains %q: a failure that will not clear was reported as a busy one", r.stderr, unwanted)
		}
	}
	if attempts != 1 {
		t.Errorf("%d attempts, want 1: the verb read again after a failure that is not a writer arriving", attempts)
	}
}

// The check answers for several lock files, and each answer is independent of the
// others: a file that appeared is reported as appeared even when another file in the
// list cannot be inspected, and an inspection failure is reported even when another
// file appeared. The two signals used to overwrite each other, in an order that
// depended on where the paths sat in the list. Only the first kind of signal says the
// read is torn, only the second says the check cannot tell, and a caller that is
// handed both must be able to see both. validate asks about one file, so the verbs
// cannot reach a list of two; the contract is pinned on the function itself.
func TestRereadsWhenTheLockFileAppearsReportsEachSignalWhateverTheOrder(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent.lock")
	appeared := filepath.Join(dir, "appeared.lock")
	writeTestFile(t, appeared, "")
	loop := filepath.Join(dir, "loop.lock")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skipf("cannot make a symbolic link that points at itself: %v", err)
	}
	if _, err := os.Stat(loop); err == nil || os.IsNotExist(err) {
		t.Fatalf("a symbolic link that points at itself gave %v, want a stat failure that is not 'does not exist'", err)
	}

	for _, tc := range []struct {
		name         string
		paths        []string
		wantAppeared bool
		wantErrAbout string // a path the error must name; empty for no error
	}{
		{"nothing to check", nil, false, ""},
		{"every file still absent", []string{absent}, false, ""},
		{"one file appeared", []string{absent, appeared}, true, ""},
		{"one file cannot be inspected", []string{absent, loop}, false, loop},
		{"one appeared, then one cannot be inspected", []string{appeared, loop}, true, loop},
		{"one cannot be inspected, then one appeared", []string{loop, appeared}, true, loop},
		{"one appeared between two that cannot be inspected", []string{loop, appeared, loop}, true, loop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotAppeared, err := rereadsWhenTheLockFileAppears(tc.paths)

			if gotAppeared != tc.wantAppeared {
				t.Errorf("appeared = %v, want %v: a signal was dropped", gotAppeared, tc.wantAppeared)
			}
			switch {
			case tc.wantErrAbout == "" && err != nil:
				t.Errorf("error = %v, want none", err)
			case tc.wantErrAbout != "" && err == nil:
				t.Errorf("no error, want one that names %s: an inspection failure was dropped", tc.wantErrAbout)
			case tc.wantErrAbout != "" && !strings.Contains(err.Error(), tc.wantErrAbout):
				t.Errorf("error = %v, want it to name %s", err, tc.wantErrAbout)
			}
		})
	}
}

// vanishingLocker grants every lock and removes the lock file when the lock is
// released, so the next attempt finds it absent again.
type vanishingLocker struct {
	lockPath string
	attempts *int
}

func (l *vanishingLocker) Lock(path string, mode LockMode) (func(), error) {
	*l.attempts++
	return func() { _ = os.Remove(l.lockPath) }, nil
}

func (l *vanishingLocker) LockDir(string, LockMode) (func(), error) { return func() {}, nil }
