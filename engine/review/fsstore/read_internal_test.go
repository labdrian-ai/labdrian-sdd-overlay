//go:build !windows

package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Read works by path on every platform, so it looks again at the directories after it has read the file, as
// the store does where it cannot hold them open: a directory replaced while the file was being read is no
// longer the one the store looked at, and the bytes are not returned.
func TestReadRefusesADirectoryThatChangedWhileTheFileWasRead(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	put(t, filepath.Join(root, "odd", "review-results", "own-rdd", resultName), "ours")
	put(t, filepath.Join(outside, resultName), "not ours")
	store, fired := racing(pointWalked, "", func(dir string) {
		if err := os.Rename(dir, dir+".moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, dir); err != nil {
			t.Fatal(err)
		}
	})

	got, err := store.Read(root, "own-rdd", resultName)

	if *fired != 1 {
		t.Fatalf("the point after the look at the directories was reached %d times, want once", *fired)
	}
	if !errors.Is(err, ErrChanged) || got != nil {
		t.Errorf("Read() = %q, %v; want no bytes and an error that is %v", got, err, ErrChanged)
	}
}

// A file replaced by a link between the look at it and the open is not the file that was looked at: the open
// would follow the link, so the store compares what it opened with what it saw and refuses the difference.
func TestReadRefusesAFileThatWasReplacedByALinkBeforeItWasOpened(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	file := filepath.Join(root, "odd", "review-results", "own-rdd", resultName)
	put(t, file, "ours")
	put(t, filepath.Join(outside, "target.json"), "not ours")
	store, fired := racing(pointLooked, "", func(dir string) {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, "target.json"), file); err != nil {
			t.Fatal(err)
		}
	})

	got, err := store.Read(root, "own-rdd", resultName)

	if *fired != 1 {
		t.Fatalf("the point after the look at the file was reached %d times, want once", *fired)
	}
	if !errors.Is(err, ErrChanged) || got != nil {
		t.Errorf("Read() = %q, %v; want no bytes and an error that is %v", got, err, ErrChanged)
	}
}
