package fsstore_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

// The store is the Results port of the use cases.
var _ app.Results = fsstore.Store{}

const resultName = "222222222222-1.json"

// featureDir is odd/review-results/<feature> under root.
func featureDir(root, feature string) string {
	return filepath.Join(root, "odd", "review-results", feature)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// Create makes the results directory of the feature when it is not there, and writes the file whole,
// readable by everyone, in it. The directory and the file are the modes of any file of the repository,
// whatever the umask of the process says.
func TestCreateMakesTheDirectoryAndWritesTheFile(t *testing.T) {
	root := t.TempDir()
	want := []byte("{\"version\": 1}\n")

	if err := (fsstore.Store{}).Create(root, "own-rdd", resultName, want); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(featureDir(root, "own-rdd"), resultName))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("stored file = %q, %v; want %q", got, err, want)
	}
	info, err := os.Stat(filepath.Join(featureDir(root, "own-rdd"), resultName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Errorf("file mode = %v, want 0644", info.Mode().Perm())
	}
	for _, dir := range []string{"odd", "odd/review-results", "odd/review-results/own-rdd"} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s: %v, want a directory", dir, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
			t.Errorf("%s mode = %v, want 0755", dir, info.Mode().Perm())
		}
	}
}

// A name that is taken stays as it is: nothing replaces it, and the refusal is the one the use cases
// retry on. Nothing else is left in the directory, the temporary file of the write included.
func TestCreateNeverReplacesAFileAndLeavesNothingBehind(t *testing.T) {
	root := t.TempDir()
	store := fsstore.Store{}
	if err := store.Create(root, "own-rdd", resultName, []byte("first")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := store.Create(root, "own-rdd", resultName, []byte("second"))

	if !errors.Is(err, app.ErrExists) {
		t.Fatalf("Create() over a file error = %v, want one that is %v", err, app.ErrExists)
	}
	got, readErr := os.ReadFile(filepath.Join(featureDir(root, "own-rdd"), resultName))
	if readErr != nil || string(got) != "first" {
		t.Errorf("the file holds %q, %v; want the first content, unchanged", got, readErr)
	}
	entries, _ := os.ReadDir(featureDir(root, "own-rdd"))
	if len(entries) != 1 || entries[0].Name() != resultName {
		t.Errorf("the directory holds %d entries %v, want only %s", len(entries), entries, resultName)
	}
}

// Even a dangling link at the name is a name that is taken: the content is never written through it.
func TestCreateDoesNotWriteThroughASymlinkAtTheName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	dir := featureDir(root, "own-rdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "target.json")
	if err := os.Symlink(target, filepath.Join(dir, resultName)); err != nil {
		t.Fatal(err)
	}

	err := (fsstore.Store{}).Create(root, "own-rdd", resultName, []byte("x"))

	if !errors.Is(err, app.ErrExists) {
		t.Errorf("Create() error = %v, want one that is %v", err, app.ErrExists)
	}
	if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the target of the link exists (%v), want it untouched", statErr)
	}
}

// Exactly one of the writers that race for one name wins; the others are told the name is taken.
func TestCreateHasOneWinnerWhenWritersRaceForOneName(t *testing.T) {
	root := t.TempDir()
	store := fsstore.Store{}
	const writers = 8
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = store.Create(root, "own-rdd", resultName, []byte{byte('a' + i)})
		}(i)
	}
	wg.Wait()

	wins, taken := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, app.ErrExists):
			taken++
		default:
			t.Errorf("a writer failed with %v, want success or %v", err, app.ErrExists)
		}
	}
	if wins != 1 || taken != writers-1 {
		t.Errorf("%d writers won and %d found the name taken, want 1 and %d", wins, taken, writers-1)
	}
	entries, _ := os.ReadDir(featureDir(root, "own-rdd"))
	if len(entries) != 1 {
		t.Errorf("the directory holds %v, want only the file of the winner", entries)
	}
}

func TestNamesListsTheEntriesOfTheDirectoryOfTheFeatureInOrder(t *testing.T) {
	root := t.TempDir()
	dir := featureDir(root, "own-rdd")
	mustWrite(t, filepath.Join(dir, "222222222222-2.json"), "b")
	mustWrite(t, filepath.Join(dir, "222222222222-1.json"), "a")
	mustWrite(t, filepath.Join(dir, "222222222222-1.verifier.md"), "v")
	mustWrite(t, filepath.Join(featureDir(root, "other"), "333333333333-1.json"), "o")

	got, err := (fsstore.Store{}).Names(root, "own-rdd")
	if err != nil {
		t.Fatalf("Names() error = %v, want nil", err)
	}
	want := []string{"222222222222-1.json", "222222222222-1.verifier.md", "222222222222-2.json"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %q, want %q", got, want)
	}
}

// The order is the store's, not the order the file system lists in, which depends on the file system and on
// the order the files were made in: enough names, made in no order, leave no chance of an order that is sorted
// by luck.
func TestNamesListsManyEntriesSortedWhateverOrderTheyWereMadeIn(t *testing.T) {
	root := t.TempDir()
	dir := featureDir(root, "own-rdd")
	var want []string
	for n := 1; n <= 12; n++ {
		want = append(want, fmt.Sprintf("222222222222-%02d.json", n))
	}
	for _, n := range []int{7, 3, 11, 1, 9, 5, 12, 2, 10, 4, 8, 6} {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("222222222222-%02d.json", n)), "x")
	}

	got, err := (fsstore.Store{}).Names(root, "own-rdd")

	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %q, %v; want %q sorted", got, err, want)
	}
}

// A write in flight stages its content in a hidden temporary file of the directory (.review-*.tmp), and that
// file is not a result: it is not listed, so no caller takes it for one. A name that only looks like it is a
// name like any other.
func TestNamesDoesNotListTheTemporaryFilesOfAWriteInFlight(t *testing.T) {
	root := t.TempDir()
	dir := featureDir(root, "own-rdd")
	mustWrite(t, filepath.Join(dir, "222222222222-1.json"), "a")
	mustWrite(t, filepath.Join(dir, ".review-1234567890.tmp"), "half of a result")
	mustWrite(t, filepath.Join(dir, ".review-7.tmp"), "half of another")
	mustWrite(t, filepath.Join(dir, ".gitkeep"), "")
	mustWrite(t, filepath.Join(dir, "review-1.tmp"), "no dot in front")
	mustWrite(t, filepath.Join(dir, ".review-1.tmp.bak"), "not ending as one")

	got, err := (fsstore.Store{}).Names(root, "own-rdd")
	if err != nil {
		t.Fatalf("Names() error = %v, want nil", err)
	}

	want := []string{".gitkeep", ".review-1.tmp.bak", "222222222222-1.json", "review-1.tmp"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %q, want %q: the two temporary files left out and the rest listed", got, want)
	}
}

// A feature that has no directory yet has no result, and asking makes nothing.
func TestNamesOfAFeatureWithoutADirectoryIsEmptyAndMakesNothing(t *testing.T) {
	root := t.TempDir()

	got, err := (fsstore.Store{}).Names(root, "own-rdd")

	if err != nil || len(got) != 0 {
		t.Errorf("Names() = %q, %v; want no name and no error", got, err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("the root holds %v after Names, want nothing made", entries)
	}
}

// A root that is not there has no results either: asking for the names says none, with no error, and makes
// nothing.
func TestNamesUnderARootThatIsNotThereIsEmptyAndMakesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")

	got, err := (fsstore.Store{}).Names(root, "own-rdd")

	if err != nil || len(got) != 0 {
		t.Errorf("Names() = %q, %v; want no name and no error", got, err)
	}
	if _, statErr := os.Lstat(root); !os.IsNotExist(statErr) {
		t.Errorf("the root is there after Names (%v), want nothing made", statErr)
	}
}

// Nothing a command line gave leads out of odd/review-results/: not a feature that is more than a plain
// segment, not a name that is a path, and not a directory of the path that has been made a link.
func TestTheStoreRefusesWhatLeadsOutsideTheResultsDirectory(t *testing.T) {
	store := fsstore.Store{}
	t.Run("a feature that is not a single safe segment", func(t *testing.T) {
		root := t.TempDir()
		for _, feature := range []string{"..", "../x", "a/b", "/etc", "", "a\x00b"} {
			if err := store.Create(root, feature, resultName, []byte("x")); !errors.Is(err, review.ErrInvalidFeature) {
				t.Errorf("Create(feature %q) error = %v, want one that is %v", feature, err, review.ErrInvalidFeature)
			}
			if _, err := store.Names(root, feature); !errors.Is(err, review.ErrInvalidFeature) {
				t.Errorf("Names(feature %q) error = %v, want one that is %v", feature, err, review.ErrInvalidFeature)
			}
		}
		if entries, _ := os.ReadDir(root); len(entries) != 0 {
			t.Errorf("the root holds %v after the refusals, want nothing made", entries)
		}
	})
	t.Run("a name that is a path", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		for _, name := range []string{"", ".", "..", "../x.json", "a/b.json", `a\b.json`, "/abs.json", ".hidden", "a\x00b", filepath.Join("..", filepath.Base(outside), "x.json")} {
			if err := store.Create(root, "own-rdd", name, []byte("x")); !errors.Is(err, fsstore.ErrInvalidName) {
				t.Errorf("Create(name %q) error = %v, want one that is %v", name, err, fsstore.ErrInvalidName)
			}
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("the directory outside holds %v, want nothing written there", entries)
		}
		if _, err := os.Stat(featureDir(root, "own-rdd")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the directory of the feature exists (%v) after names were refused, want nothing made", err)
		}
	})
	t.Run("a name longer than a file name can be", func(t *testing.T) {
		root := t.TempDir()

		err := store.Create(root, "own-rdd", strings.Repeat("a", 256), []byte("x"))

		if !errors.Is(err, fsstore.ErrInvalidName) || !strings.Contains(err.Error(), "longer than 255 bytes") {
			t.Errorf("Create() error = %v, want one that is %v and says the name is longer than 255 bytes", err, fsstore.ErrInvalidName)
		}
		if _, err := os.Stat(featureDir(root, "own-rdd")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the directory of the feature exists (%v) after the name was refused, want nothing made", err)
		}
	})
}

// A link anywhere on the way from the root to the directory of the feature, or a file where a directory
// should be, is refused and followed nowhere: what is stored never goes where the link leads.
func TestTheStoreFollowsNoLinkOnTheWayToTheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on windows")
	}
	// The refusal says which of the two it is, a link or something that is not a directory, so that a person
	// knows what to look at.
	for _, tc := range []struct {
		name    string
		setup   func(t *testing.T, root, outside string)
		mention string
	}{
		{"odd is a link", func(t *testing.T, root, outside string) {
			link(t, outside, filepath.Join(root, "odd"))
		}, "is a symbolic link"},
		{"review-results is a link", func(t *testing.T, root, outside string) {
			link(t, outside, filepath.Join(root, "odd", "review-results"))
		}, "is a symbolic link"},
		{"the directory of the feature is a link", func(t *testing.T, root, outside string) {
			link(t, outside, filepath.Join(root, "odd", "review-results", "own-rdd"))
		}, "is a symbolic link"},
		{"odd is a file", func(t *testing.T, root, outside string) {
			mustWrite(t, filepath.Join(root, "odd"), "not a directory")
		}, "is not a directory"},
		{"the directory of the feature is a file", func(t *testing.T, root, outside string) {
			mustWrite(t, filepath.Join(root, "odd", "review-results", "own-rdd"), "not a directory")
		}, "is not a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			tc.setup(t, root, outside)
			store := fsstore.Store{}

			if err := store.Create(root, "own-rdd", resultName, []byte("x")); !errors.Is(err, fsstore.ErrNotADirectory) || !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("Create() error = %v, want one that is %v and says %q", err, fsstore.ErrNotADirectory, tc.mention)
			}
			if _, err := store.Names(root, "own-rdd"); !errors.Is(err, fsstore.ErrNotADirectory) || !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("Names() error = %v, want one that is %v and says %q", err, fsstore.ErrNotADirectory, tc.mention)
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Errorf("the directory the link leads to holds %v, want nothing written there", entries)
			}
		})
	}
}

func link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, name); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// A directory that cannot be written is an error, and no file or temporary file is left in it.
func TestCreateFailsWhenTheDirectoryCannotBeWritten(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory stops nobody who is root, and windows has other modes")
	}
	root := t.TempDir()
	dir := featureDir(root, "own-rdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := (fsstore.Store{}).Create(root, "own-rdd", resultName, []byte("x"))

	if err == nil || errors.Is(err, app.ErrExists) {
		t.Fatalf("Create() error = %v, want a failure that is not %v", err, app.ErrExists)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("Create() error = %v, want it to name the directory that cannot be written: %s", err, dir)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the directory holds %v, want nothing left", entries)
	}
}
