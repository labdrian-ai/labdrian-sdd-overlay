package fsstore_test

// Where the sweep keeps its two copies and its lock (own-rdd T6b3, S6 b): <state home>/labdrian/review-sweep/<repo key>/,
// with the directories base and head, emptied at each run, and one lock for the repository. The tests make their own
// state home in a temporary directory and never read the environment.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

func sweepCopies(t *testing.T) (fsstore.Copies, string) {
	t.Helper()
	home := t.TempDir()
	copies, err := fsstore.NewCopies(home)
	if err != nil {
		t.Fatalf("NewCopies error = %v", err)
	}
	return copies, home
}

func repoAt(main string) app.Repository {
	return app.Repository{Root: main, Main: main, CommonDir: filepath.Join(main, ".git")}
}

// keyOf is the key of a repository: the first 16 hex characters of the sha256 of the root of its main working tree.
func keyOf(main string) string {
	sum := sha256.Sum256([]byte(main))
	return hex.EncodeToString(sum[:])[:16]
}

func TestCopiesAreBuiltOverAnAbsoluteStateHomeOnly(t *testing.T) {
	if _, err := fsstore.NewCopies("relative/state"); err == nil {
		t.Error("NewCopies of a relative state home gave no error")
	}
}

func TestResetGivesTheStablePathOfTheCopyAndEmptiesItAtEachRun(t *testing.T) {
	copies, home := sweepCopies(t)
	repo := repoAt("/work/main")

	path, err := copies.Reset(repo, app.CopyBase)
	if err != nil {
		t.Fatalf("Reset error = %v", err)
	}
	want := filepath.Join(home, "labdrian", "review-sweep", keyOf("/work/main"), "base")
	if path != want {
		t.Errorf("Reset path = %q, want %q", path, want)
	}
	if err := os.MkdirAll(filepath.Join(path, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sub", "stale.go"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	again, err := copies.Reset(repo, app.CopyBase)

	entries, readErr := os.ReadDir(again)
	if err != nil || again != path || readErr != nil || len(entries) != 0 {
		t.Errorf("second Reset = %q, %v, entries %d (%v), want the same path, emptied", again, err, len(entries), readErr)
	}
	head, err := copies.Reset(repo, app.CopyHead)
	if err != nil || head != filepath.Join(filepath.Dir(want), "head") {
		t.Errorf("Reset head = %q, %v, want the sibling directory head", head, err)
	}
}

func TestEachRepositoryHasItsOwnCopies(t *testing.T) {
	copies, _ := sweepCopies(t)

	one, errOne := copies.Reset(repoAt("/work/one"), app.CopyHead)
	two, errTwo := copies.Reset(repoAt("/work/two"), app.CopyHead)

	if errOne != nil || errTwo != nil || one == two {
		t.Errorf("Reset = %q, %v and %q, %v, want two paths", one, errOne, two, errTwo)
	}
}

func TestACopyIsKeyedByTheMainWorkingTreeNotByTheLinkedOne(t *testing.T) {
	copies, _ := sweepCopies(t)
	main := repoAt("/work/main")
	linked := app.Repository{Root: "/work/wt/feature", Main: "/work/main", CommonDir: "/work/main/.git"}

	a, errA := copies.Reset(main, app.CopyBase)
	b, errB := copies.Reset(linked, app.CopyBase)

	if errA != nil || errB != nil || a != b {
		t.Errorf("Reset = %q, %v and %q, %v, want one path for the worktrees of one repository", a, errA, b, errB)
	}
}

func TestResetRefusesASideThatIsNotBaseOrHeadAndALinkWhereACopyGoes(t *testing.T) {
	copies, _ := sweepCopies(t)
	repo := repoAt("/work/main")
	if _, err := copies.Reset(repo, "../escape"); err == nil {
		t.Error("Reset of a side named ../escape gave no error")
	}
	path, err := copies.Reset(repo, app.CopyBase)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = copies.Reset(repo, app.CopyBase)

	if err == nil {
		t.Error("Reset where a link stands gave no error")
	}
	if data, readErr := os.ReadFile(filepath.Join(outside, "keep.txt")); readErr != nil || string(data) != "mine" {
		t.Errorf("the file the link leads to = %q, %v, want it untouched", data, readErr)
	}
}

func TestOnlyOneSweepOfARepositoryHoldsTheLock(t *testing.T) {
	copies, _ := sweepCopies(t)
	repo := repoAt("/work/main")

	unlock, err := copies.Lock(repo)
	if err != nil {
		t.Fatalf("Lock error = %v", err)
	}
	_, busy := copies.Lock(repo)
	if !errors.Is(busy, app.ErrSweepBusy) {
		t.Errorf("second Lock error = %v, want ErrSweepBusy: it does not wait", busy)
	}
	if other, err := copies.Lock(repoAt("/work/other")); err != nil {
		t.Errorf("Lock of another repository error = %v, want it free", err)
	} else {
		other()
	}

	unlock()
	unlock() // giving it back twice changes nothing
	again, err := copies.Lock(repo)

	if err != nil {
		t.Fatalf("Lock after the release error = %v, want the lock free", err)
	}
	again()
}

func TestTheLockOfASweepThatFailedIsFreeForTheNextOne(t *testing.T) {
	copies, home := sweepCopies(t)
	repo := repoAt("/work/main")
	unlock, err := copies.Lock(repo)
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	file := filepath.Join(home, "labdrian", "review-sweep", keyOf("/work/main"), "lock")
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("lock file = %v, %v, want a private file that stays after the release", info, err)
	}
	if next, err := copies.Lock(repo); err != nil {
		t.Errorf("Lock error = %v, want a lock that a released one does not hold", err)
	} else {
		next()
	}
}

func TestTheMessageOfABusyLockIsTheOneTheCommandPrints(t *testing.T) {
	if !strings.Contains(app.ErrSweepBusy.Error(), "another review sweep of this repository is running") {
		t.Errorf("ErrSweepBusy = %q", app.ErrSweepBusy)
	}
}
