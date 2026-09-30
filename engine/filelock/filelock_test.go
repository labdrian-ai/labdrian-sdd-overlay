package filelock

import (
	"bufio"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func skipUnlessSupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("advisory file locks are only implemented on linux and darwin")
	}
}

// fakeClock is a clock whose sleeping only moves its own time forward, so a
// test can wait out a two second bound without spending any of it. onSleep, when
// set, runs after each sleep with the number of sleeps so far, which lets a test
// release a holder at an exact point of the wait.
type fakeClock struct {
	now     time.Time
	sleeps  []time.Duration
	onSleep func(n int)
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Clock() Clock {
	return Clock{
		Now: func() time.Time { return c.now },
		Sleep: func(d time.Duration) {
			c.sleeps = append(c.sleeps, d)
			c.now = c.now.Add(d)
			if c.onSleep != nil {
				c.onSleep(len(c.sleeps))
			}
		},
	}
}

func (c *fakeClock) slept() time.Duration {
	var total time.Duration
	for _, d := range c.sleeps {
		total += d
	}
	return total
}

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".fixture.lock")
}

func mustAcquire(t *testing.T, path string, opts Options) func() {
	t.Helper()
	unlock, err := Acquire(path, opts)
	if err != nil {
		t.Fatalf("Acquire(%q, %+v) = %v, want a held lock", path, opts, err)
	}
	return unlock
}

func TestAcquireCreatesTheLockFileAndUnlockReleasesIt(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)

	unlock := mustAcquire(t, path, Options{})
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file was not created: %v", err)
	}
	unlock()

	// Released: the same path can be taken again at once.
	mustAcquire(t, path, Options{})()
}

// The lock file is the lock's identity: removing it while another process holds
// or is about to take it would give two processes two different locks. It is
// never removed, by unlock or by anything else here.
func TestTheLockFileIsNeverRemoved(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	mustAcquire(t, path, Options{})()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file is gone after unlock: %v", err)
	}
	if _, err := Acquire(path, Options{Mode: Shared}); err != nil {
		t.Fatalf("shared acquire: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file is gone after a shared acquire: %v", err)
	}
}

func TestUnlockIsIdempotent(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	unlock := mustAcquire(t, path, Options{})
	unlock()
	other := mustAcquire(t, path, Options{})
	defer other()
	// A second call must not release a lock somebody else has taken since.
	unlock()
	if _, err := Acquire(path, Options{Wait: time.Nanosecond, Clock: newFakeClock().Clock()}); !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire after a repeated unlock = %v, want ErrBusy: the second unlock released the other holder", err)
	}
}

// A busy lock is reported after the bound, not waited on forever: the wait is
// spent on the injected clock, so the test itself takes no time.
func TestAcquireReportsBusyAfterTheBound(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	defer mustAcquire(t, path, Options{})()

	clock := newFakeClock()
	start := time.Now()
	unlock, err := Acquire(path, Options{Clock: clock.Clock()})
	if unlock != nil {
		t.Fatal("a busy Acquire returned an unlock function")
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire = %v, want ErrBusy", err)
	}
	var busy *BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("Acquire = %T, want a *BusyError", err)
	}
	if busy.Path != path || busy.Waited != DefaultWait {
		t.Errorf("BusyError = %+v, want path %q and waited %v", busy, path, DefaultWait)
	}
	if !busy.Busy() {
		t.Error("BusyError.Busy() = false")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the lock", err)
	}
	if got := clock.slept(); got < DefaultWait {
		t.Errorf("slept %v on the fake clock, want at least the %v bound", got, DefaultWait)
	}
	for _, d := range clock.sleeps {
		if d != pollInterval {
			t.Fatalf("slept %v in one step, want the %v poll interval", d, pollInterval)
		}
	}
	if real := time.Since(start); real > time.Second {
		t.Errorf("the busy path took %v of real time; the bound must be spent on the injected clock", real)
	}
}

// The lock is tried at least once, however short the bound: on the real clock a
// one nanosecond bound is over before the first attempt, and a free lock is still
// taken. A taken one is refused at once, without sleeping.
func TestAcquireTriesOnceEvenWithNoTimeToWait(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	unlock, err := Acquire(path, Options{Wait: time.Nanosecond})
	if err != nil {
		t.Fatalf("Acquire of a free lock with a tiny bound = %v, want it held", err)
	}
	defer unlock()

	// A clock that runs a second ahead each time it is read, so the bound is over
	// by the first look at it.
	clock := newFakeClock()
	spent := Clock{
		Now:   func() time.Time { clock.now = clock.now.Add(time.Second); return clock.now },
		Sleep: clock.Clock().Sleep,
	}
	if _, err := Acquire(path, Options{Wait: time.Nanosecond, Clock: spent}); !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire of a taken lock with no time to wait = %v, want ErrBusy", err)
	}
	if len(clock.sleeps) != 0 {
		t.Errorf("slept %d times with no time left to wait, want none", len(clock.sleeps))
	}
}

func TestTheBoundAndThePollIntervalAreThePhase7Ones(t *testing.T) {
	if DefaultWait != 2*time.Second || pollInterval != 5*time.Millisecond {
		t.Errorf("DefaultWait = %v and pollInterval = %v, want 2s and 5ms (engine/projection's binding store)", DefaultWait, pollInterval)
	}
}

// A holder that lets go during the wait is waited for: the caller gets the lock
// instead of a busy refusal.
func TestAcquireWaitsForAHolderThatLetsGo(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	release := mustAcquire(t, path, Options{})

	clock := newFakeClock()
	clock.onSleep = func(n int) {
		if n == 3 {
			release()
		}
	}
	unlock, err := Acquire(path, Options{Clock: clock.Clock()})
	if err != nil {
		t.Fatalf("Acquire = %v, want the lock once the holder let go", err)
	}
	defer unlock()
	if len(clock.sleeps) != 3 {
		t.Errorf("slept %d times, want 3 (the holder left during the third sleep)", len(clock.sleeps))
	}
}

func TestSharedLocksCoexistAndExcludeAnExclusiveOne(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	mustAcquire(t, path, Options{})() // creates the file

	first := mustAcquire(t, path, Options{Mode: Shared})
	second := mustAcquire(t, path, Options{Mode: Shared})

	busy := func(mode Mode) error {
		_, err := Acquire(path, Options{Mode: mode, Wait: time.Nanosecond, Clock: newFakeClock().Clock()})
		return err
	}
	if err := busy(Exclusive); !errors.Is(err, ErrBusy) {
		t.Errorf("exclusive against two shared holders = %v, want ErrBusy", err)
	}
	first()
	if err := busy(Exclusive); !errors.Is(err, ErrBusy) {
		t.Errorf("exclusive against one remaining shared holder = %v, want ErrBusy", err)
	}
	second()
	mustAcquire(t, path, Options{})() // free again

	exclusive := mustAcquire(t, path, Options{})
	defer exclusive()
	if err := busy(Shared); !errors.Is(err, ErrBusy) {
		t.Errorf("shared against an exclusive holder = %v, want ErrBusy", err)
	}
}

// A reader never creates the lock file: on a tree nobody has ever written to (or
// one it cannot write to) there is no writer to wait for, so it holds nothing.
func TestSharedOnAMissingLockFileHoldsNothingAndCreatesNothing(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	unlock, err := Acquire(path, Options{Mode: Shared})
	if err != nil || unlock == nil {
		t.Fatalf("Acquire(Shared) of a missing lock file = (unlock nil: %v), %v, want a no-op lock", unlock == nil, err)
	}
	unlock()
	unlock()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a shared acquire created the lock file (%v)", err)
	}
}

func TestSharedWorksOnAReadOnlyLockFile(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	mustAcquire(t, path, Options{})()
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, path, Options{Mode: Shared})()
}

func TestAcquireRefusesASymlinkedLockFile(t *testing.T) {
	skipUnlessSupported(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".fixture.lock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []Mode{Exclusive, Shared} {
		unlock, err := Acquire(link, Options{Mode: mode})
		if err == nil || errors.Is(err, ErrBusy) {
			t.Errorf("mode %v: Acquire through a symlink = %v, want a plain refusal", mode, err)
		}
		if unlock != nil {
			t.Errorf("mode %v: a refused Acquire returned an unlock function", mode)
		}
	}
}

// A lock that cannot be opened is an error, not a busy lock: retrying would
// never help, and the message must not send the caller round in a circle.
func TestAcquireReportsAnUnusableLocationAsAnErrorNotAsBusy(t *testing.T) {
	skipUnlessSupported(t)
	path := filepath.Join(t.TempDir(), "no-such-dir", ".fixture.lock")
	_, err := Acquire(path, Options{})
	if err == nil || errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire in a missing directory = %v, want a plain error", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the lock file", err)
	}
}

// ---- across real processes ------------------------------------------------------

const helperEnv = "FILELOCK_TEST_HELPER_PATH"

// TestHelperProcessHoldsTheLock is not a test: it is the child the tests below
// start. It takes the exclusive lock named by the environment, says so on stdout,
// and holds it until its stdin closes.
func TestHelperProcessHoldsTheLock(t *testing.T) {
	path := os.Getenv(helperEnv)
	if path == "" {
		t.Skip("helper process only")
	}
	unlock, err := Acquire(path, Options{})
	if err != nil {
		os.Stdout.WriteString("error: " + err.Error() + "\n")
		os.Exit(3)
	}
	os.Stdout.WriteString("held\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	unlock()
	os.Exit(0)
}

// TestALockHeldByAnotherProcessIsBusyUntilItExits is the point of the package:
// the exclusion is between processes, not between the goroutines of one.
func TestALockHeldByAnotherProcessIsBusyUntilItExits(t *testing.T) {
	skipUnlessSupported(t)
	path := lockPath(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessHoldsTheLock$")
	cmd.Env = []string{helperEnv + "=" + path}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Wait() })

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "held" {
		t.Fatalf("helper said %q (%v), want it to hold the lock", line, err)
	}

	// A real bound, kept short: this is the only test here that waits on real time.
	begin := time.Now()
	_, err = Acquire(path, Options{Wait: 30 * time.Millisecond})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire while another process holds the lock = %v, want ErrBusy", err)
	}
	if waited := time.Since(begin); waited < 25*time.Millisecond || waited > 2*time.Second {
		t.Errorf("waited %v for a 30ms bound", waited)
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	// The holder exited without unlocking anything by hand: the kernel let go.
	mustAcquire(t, path, Options{Wait: time.Second})()
}

// ---- the platform contract -------------------------------------------------------

func TestUnsupportedPlatformsFailClosed(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		t.Skip("this platform has file locks")
	}
	if _, err := Acquire(lockPath(t), Options{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Acquire = %v, want ErrUnsupported", err)
	}
}

// The package is a system-call wrapper and nothing else: no process execution,
// no network, and none of the module's own packages.
func TestProductionFilesImportOnlyTheLockingStdlib(t *testing.T) {
	allowed := map[string]bool{"errors": true, "fmt": true, "os": true, "syscall": true, "time": true}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			files++
			for _, imp := range file.Imports {
				if p := strings.Trim(imp.Path.Value, `"`); !allowed[p] {
					t.Errorf("%s imports %q; only %v are allowed", filepath.Base(name), p, allowed)
				}
			}
		}
	}
	if files == 0 {
		t.Fatal("no production files found; the walk is broken")
	}
}
