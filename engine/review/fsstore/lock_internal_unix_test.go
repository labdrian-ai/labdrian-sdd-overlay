//go:build unix

package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// featureDir makes the directory of the feature own-rdd under a new root, with one result in it.
func featureDir(t *testing.T) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "odd", "review-results", "own-rdd")
	put(t, filepath.Join(dir, resultName), "{}\n")
	return root, dir
}

// lockers are the two ways the store takes a lock, which these tests hold to the same rules: the one Lock is on
// linux, and the one by path that it is on the other systems that have flock(2).
var lockers = map[string]func(s Store, root, feature string) (func(), error){
	"Lock":       Store.Lock,
	"lockByPath": Store.lockByPath,
}

// A lock is held until it is given back: another, in this process or in another, waits for it.
func TestEveryLockWaitsForTheOneBeforeUntilItIsGivenBack(t *testing.T) {
	for name, lock := range lockers {
		t.Run(name, func(t *testing.T) {
			root, _ := featureDir(t)
			unlock, err := lock(Store{}, root, "own-rdd")
			if err != nil {
				t.Fatalf("%s() error = %v, want nil", name, err)
			}
			second := make(chan error, 1)
			go func() {
				u, err := lock(Store{}, root, "own-rdd")
				if err == nil {
					u()
				}
				second <- err
			}()
			select {
			case err := <-second:
				t.Fatalf("a second %s() returned (error %v) while the first was held", name, err)
			case <-time.After(200 * time.Millisecond):
			}
			unlock()
			select {
			case err := <-second:
				if err != nil {
					t.Errorf("the second %s() error = %v, want nil once the first was given back", name, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("the second %s() did not return after the first was given back", name)
			}
		})
	}
}

// A holder that does not give the lock back does not keep every other caller waiting for ever: after the wait
// of the store, Lock gives up with ErrLockHeld, naming the directory.
func TestALockThatIsNotGivenBackInTimeIsErrLockHeld(t *testing.T) {
	for name, lock := range lockers {
		t.Run(name, func(t *testing.T) {
			root, dir := featureDir(t)
			unlock, err := lock(Store{}, root, "own-rdd")
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			defer unlock()
			started := time.Now()

			_, err = lock(Store{lockWait: 150 * time.Millisecond}, root, "own-rdd")

			if !errors.Is(err, ErrLockHeld) || !strings.Contains(err.Error(), dir) {
				t.Errorf("%s() error = %v, want one that is %v and names %s", name, err, ErrLockHeld, dir)
			}
			if waited := time.Since(started); waited < 150*time.Millisecond || waited > 5*time.Second {
				t.Errorf("%s() gave up after %s, want after the wait of 150ms", name, waited)
			}
		})
	}
}

// A file system that cannot lock is an error, never a lock that is taken: the verdict that blocks is not stored
// where the round cannot be guarded.
func TestALockTheFileSystemCannotTakeIsAnError(t *testing.T) {
	for name, lock := range lockers {
		t.Run(name, func(t *testing.T) {
			root, _ := featureDir(t)
			store := Store{flock: func(int, int) error { return syscall.ENOLCK }}

			if _, err := lock(store, root, "own-rdd"); !errors.Is(err, syscall.ENOLCK) {
				t.Errorf("%s() error = %v, want one that wraps %v", name, err, syscall.ENOLCK)
			}
		})
	}
}

// A lock is of the directory the store walks to: one that is not there is not found and is not made, a link in
// its place is refused, and so is a directory replaced between the look and the open.
func TestALockIsOfTheDirectoryTheStoreWalkedTo(t *testing.T) {
	for name, lock := range lockers {
		t.Run(name, func(t *testing.T) {
			absent := t.TempDir()
			if _, err := lock(Store{}, absent, "own-rdd"); !errors.Is(err, app.ErrNotFound) || exists(filepath.Join(absent, "odd")) {
				t.Errorf("%s() of a feature with no directory error = %v, want %v and nothing made", name, err, app.ErrNotFound)
			}
			linked := t.TempDir()
			if err := os.MkdirAll(filepath.Join(linked, review.ResultsDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), filepath.Join(linked, review.ResultsDir, "own-rdd")); err != nil {
				t.Fatal(err)
			}
			if _, err := lock(Store{}, linked, "own-rdd"); !errors.Is(err, ErrNotADirectory) {
				t.Errorf("%s() of a feature directory that is a link error = %v, want %v", name, err, ErrNotADirectory)
			}
			unsafe := t.TempDir()
			if _, err := lock(Store{}, unsafe, "../own-rdd"); !errors.Is(err, review.ErrInvalidFeature) || exists(filepath.Join(unsafe, "odd")) {
				t.Errorf("%s() of an unsafe feature error = %v, want %v and nothing made", name, err, review.ErrInvalidFeature)
			}
		})
	}

	root, dir := featureDir(t)
	store, fired := racing(pointWalked, "", func(string) {
		if err := os.Rename(dir, dir+".moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := store.lockByPath(root, "own-rdd"); *fired != 1 || !errors.Is(err, ErrChanged) {
		t.Errorf("lockByPath() of a directory replaced before it was opened error = %v (point reached %d times), want %v", err, *fired, ErrChanged)
	}
}
