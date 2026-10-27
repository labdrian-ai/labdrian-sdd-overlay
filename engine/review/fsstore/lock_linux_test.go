package fsstore_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

// A lock of the directory of a feature is held until it is given back: a second one, in this process or in
// another, waits for it. Each Lock opens the directory again, and the kernel ties the lock to what was opened,
// so two calls of one process wait for each other as two processes do.
func TestLockHoldsTheDirectoryOfTheFeatureUntilItIsGivenBack(t *testing.T) {
	root := t.TempDir()
	if err := (fsstore.Store{}).Create(root, "own-rdd", resultName, []byte("{}\n")); err != nil {
		t.Fatalf("setup: Create() error = %v", err)
	}
	unlock, err := (fsstore.Store{}).Lock(root, "own-rdd")
	if err != nil {
		t.Fatalf("Lock() error = %v, want nil", err)
	}
	type taken struct {
		unlock func()
		err    error
	}
	second := make(chan taken, 1)
	go func() {
		u, err := (fsstore.Store{}).Lock(root, "own-rdd")
		second <- taken{u, err}
	}()

	select {
	case got := <-second:
		t.Fatalf("a second Lock() returned (error %v) while the first was held", got.err)
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	select {
	case got := <-second:
		if got.err != nil {
			t.Fatalf("the second Lock() error = %v, want nil once the first was given back", got.err)
		}
		got.unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("the second Lock() did not return after the first was given back")
	}
}

// A lock is of the directory the store walks to: one that is not there is not found and is not made, a link in
// its place is refused, and a feature that is not one safe segment is refused before anything is opened.
func TestLockRefusesWhatIsNotTheDirectoryOfTheFeature(t *testing.T) {
	absent := t.TempDir()
	if _, err := (fsstore.Store{}).Lock(absent, "own-rdd"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("Lock() of a feature with no directory error = %v, want %v", err, app.ErrNotFound)
	}
	if _, err := os.Stat(filepath.Join(absent, "odd")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Lock() made %s, want nothing made", filepath.Join(absent, "odd"))
	}

	linked := t.TempDir()
	if err := os.MkdirAll(filepath.Join(linked, review.ResultsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(linked, review.ResultsDir, "own-rdd")); err != nil {
		t.Fatal(err)
	}
	if _, err := (fsstore.Store{}).Lock(linked, "own-rdd"); !errors.Is(err, fsstore.ErrNotADirectory) {
		t.Errorf("Lock() of a feature directory that is a link error = %v, want %v", err, fsstore.ErrNotADirectory)
	}

	if _, err := (fsstore.Store{}).Lock(t.TempDir(), "../own-rdd"); !errors.Is(err, review.ErrInvalidFeature) {
		t.Errorf("Lock() of an unsafe feature error = %v, want %v", err, review.ErrInvalidFeature)
	}
}
