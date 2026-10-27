package fsstore_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

// Read returns the bytes of a file of the directory of the feature as they were stored.
func TestReadReturnsWhatCreateStored(t *testing.T) {
	root := t.TempDir()
	want := []byte("{\"version\": 1}\n")
	if err := (fsstore.Store{}).Create(root, "own-rdd", resultName, want); err != nil {
		t.Fatalf("setup: Create() error = %v", err)
	}

	got, err := (fsstore.Store{}).Read(root, "own-rdd", resultName)

	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("Read() = %q, %v; want %q", got, err, want)
	}
}

// What is not stored is not found, whether the file, the directory of the feature or the root is missing, and
// asking makes nothing.
func TestReadOfWhatIsNotStoredIsNotFoundAndMakesNothing(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")
	empty, noFile := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(featureDir(noFile, "own-rdd"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, root := range map[string]string{"a root that is not there": absent, "no directory of the feature": empty, "no file": noFile} {
		t.Run(name, func(t *testing.T) {
			got, err := (fsstore.Store{}).Read(root, "own-rdd", resultName)

			if !errors.Is(err, app.ErrNotFound) || got != nil {
				t.Errorf("Read() = %q, %v; want no bytes and an error that is %v", got, err, app.ErrNotFound)
			}
		})
	}
	if _, err := os.Lstat(absent); !os.IsNotExist(err) {
		t.Errorf("the root is there after Read (%v), want nothing made", err)
	}
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("the root holds %v after Read, want nothing made", entries)
	}
	if entries, _ := os.ReadDir(featureDir(noFile, "own-rdd")); len(entries) != 0 {
		t.Errorf("the directory of the feature holds %v after Read, want nothing made", entries)
	}
}

// Nothing a command line gave leads out of odd/review-results/ when a file is read, as when one is written:
// not a feature that is more than a plain segment, not a name that is a path, and not a link on the way.
func TestReadRefusesWhatLeadsOutsideTheResultsDirectory(t *testing.T) {
	store := fsstore.Store{}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, resultName), []byte("not ours"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Run("a feature that is not a single safe segment", func(t *testing.T) {
		for _, feature := range []string{"..", "../x", "a/b", "/etc", ""} {
			if _, err := store.Read(t.TempDir(), feature, resultName); !errors.Is(err, review.ErrInvalidFeature) {
				t.Errorf("Read(feature %q) error = %v, want one that is %v", feature, err, review.ErrInvalidFeature)
			}
		}
	})
	t.Run("a name that is a path", func(t *testing.T) {
		for _, name := range []string{"", ".", "..", "../x.json", "a/b.json", ".hidden", "a\x00b", filepath.Join("..", filepath.Base(outside), resultName)} {
			if _, err := store.Read(t.TempDir(), "own-rdd", name); !errors.Is(err, fsstore.ErrInvalidName) {
				t.Errorf("Read(name %q) error = %v, want one that is %v", name, err, fsstore.ErrInvalidName)
			}
		}
	})
	if runtime.GOOS == "windows" {
		return
	}
	for name, dir := range map[string][]string{
		"odd is a link":                          {"odd"},
		"review-results is a link":               {"odd", "review-results"},
		"the directory of the feature is a link": {"odd", "review-results", "own-rdd"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			link(t, outside, filepath.Join(append([]string{root}, dir...)...))

			got, err := store.Read(root, "own-rdd", resultName)

			if !errors.Is(err, fsstore.ErrNotADirectory) || got != nil {
				t.Errorf("Read() = %q, %v; want no bytes and an error that is %v: the file the link leads to is not ours", got, err, fsstore.ErrNotADirectory)
			}
		})
	}
}

// Only a regular file is read: a link at the name, even one to a file of the same directory, and anything
// else that is not a file are refused and followed nowhere.
func TestReadRefusesWhatIsNotARegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	dir := featureDir(root, "own-rdd")
	mustWrite(t, filepath.Join(outside, "target.json"), "not ours")
	mustWrite(t, filepath.Join(dir, "333333333333-1.json"), "ours")
	link(t, filepath.Join(outside, "target.json"), filepath.Join(dir, "222222222222-1.json"))
	link(t, "333333333333-1.json", filepath.Join(dir, "222222222222-2.json"))
	if err := os.Mkdir(filepath.Join(dir, "222222222222-3.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"222222222222-1.json", "222222222222-2.json", "222222222222-3.json"} {
		got, err := (fsstore.Store{}).Read(root, "own-rdd", name)

		if !errors.Is(err, fsstore.ErrNotAFile) || got != nil {
			t.Errorf("Read(%s) = %q, %v; want no bytes and an error that is %v", name, got, err, fsstore.ErrNotAFile)
		}
	}
}

// A file is read whole up to review.MaxStoredBytes and no further: one byte more is refused, so that a file
// that is not one of ours is not read into memory.
func TestReadStopsAtTheLimitOfAStoredFile(t *testing.T) {
	root := t.TempDir()
	dir := featureDir(root, "own-rdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int64{"222222222222-1.json": review.MaxStoredBytes, "222222222222-2.json": review.MaxStoredBytes + 1} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil || f.Truncate(size) != nil || f.Close() != nil {
			t.Fatalf("setup: %s: %v", name, err)
		}
	}

	at, err := (fsstore.Store{}).Read(root, "own-rdd", "222222222222-1.json")
	if err != nil || len(at) != review.MaxStoredBytes {
		t.Errorf("Read() of a file at the limit = %d bytes, %v; want %d bytes", len(at), err, review.MaxStoredBytes)
	}
	above, err := (fsstore.Store{}).Read(root, "own-rdd", "222222222222-2.json")
	if !errors.Is(err, fsstore.ErrTooLarge) || above != nil {
		t.Errorf("Read() of a file above the limit = %d bytes, %v; want no bytes and an error that is %v", len(above), err, fsstore.ErrTooLarge)
	}
}
