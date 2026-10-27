package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// On linux the store holds each directory of the path open from the moment it has walked to it, and does what
// it does relative to the one it holds. These tests change a path at the exact moment the store is about to use
// it, which is the only way to try a race on purpose: whatever happens to the paths, the store has worked in
// the directories it walked.

// swapped says where the directory of the feature is after a path above it, or it, has been moved aside and
// something else put in its place.
type swapped struct {
	root string
	// original is the directory the store walked, at the place it was moved to.
	original string
	// instead is the directory the path of the feature leads to now.
	instead string
}

func TestTheDirectoriesTheStoreWalkedAreTheOnesItWorksInWhateverHappensToTheirPaths(t *testing.T) {
	const decoy = "999999999999-1.json"
	feature := filepath.Join("odd", "review-results", "own-rdd")
	for _, level := range []string{feature, filepath.Join("odd", "review-results"), "odd"} {
		for _, change := range []struct {
			name string
			do   func(t *testing.T, above, rel, outside string)
		}{
			{"a link to another directory", func(t *testing.T, above, rel, outside string) {
				if err := os.Rename(above, above+".moved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, above); err != nil {
					t.Fatal(err)
				}
			}},
			{"another directory", func(t *testing.T, above, rel, outside string) {
				if err := os.Rename(above, above+".moved"); err != nil {
					t.Fatal(err)
				}
				put(t, filepath.Join(above, rel, decoy), "not ours")
			}},
		} {
			for _, use := range []struct {
				name  string
				point string
				run   func(t *testing.T, store Store, w swapped)
			}{
				{"a listing", pointWalked, func(t *testing.T, store Store, w swapped) {
					names, err := store.Names(w.root, "own-rdd")
					if err != nil || !reflect.DeepEqual(names, []string{"111111111111-1.json"}) {
						t.Errorf("Names() = %q, %v; want the files of the directory that was walked and no error", names, err)
					}
				}},
				{"a file about to be linked", pointWalked, publishes},
				{"a file that was linked", pointLinked, publishes},
			} {
				t.Run(use.name+" after "+change.name+" of "+level, func(t *testing.T) {
					root, outside := t.TempDir(), t.TempDir()
					above := filepath.Join(root, level)
					rel, err := filepath.Rel(above, filepath.Join(root, feature))
					if err != nil {
						t.Fatal(err)
					}
					put(t, filepath.Join(root, feature, "111111111111-1.json"), "a")
					put(t, filepath.Join(outside, rel, decoy), "not ours")
					store, fired := racing(use.point, "", func(string) { change.do(t, above, rel, outside) })

					use.run(t, store, swapped{root: root, original: filepath.Join(above+".moved", rel), instead: filepath.Join(above, rel)})

					if *fired != 1 {
						t.Fatalf("the point of the race was reached %d times, want once: the change was not made", *fired)
					}
				})
			}
		}
	}
}

// publishes is the use of the store that writes: the result is stored whole in the directory that was walked,
// and the directory the path leads to after the change gets nothing, not even a temporary file.
func publishes(t *testing.T, store Store, w swapped) {
	err := store.Create(w.root, "own-rdd", resultName, []byte("x"))

	if err != nil {
		t.Errorf("Create() error = %v, want nil: the directory that was walked is the one that is used", err)
	}
	if got, readErr := os.ReadFile(filepath.Join(w.original, resultName)); readErr != nil || string(got) != "x" {
		t.Errorf("the directory that was walked holds %q (%v), want the result stored whole in it", got, readErr)
	}
	if got, want := namesIn(t, w.original), []string{"111111111111-1.json", resultName}; !reflect.DeepEqual(got, want) {
		t.Errorf("the directory that was walked holds %q, want %q: the result and no temporary file", got, want)
	}
	if got, want := namesIn(t, w.instead), []string{"999999999999-1.json"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the directory the path leads to now holds %q, want only %q: nothing published into it", got, want)
	}
}

// openDescriptors is how many descriptors the process has open.
func openDescriptors(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("the descriptors of the process cannot be listed here: %v", err)
	}
	return len(entries)
}

// The store keeps directories open while it works, so every way out of a call has to close them: the result
// stored, the name taken, a refusal on the way down, a directory that is not there, a directory that cannot be
// written.
func TestTheStoreHoldsNoDescriptorOnceACallHasReturned(t *testing.T) {
	root, linked, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(linked, "odd")); err != nil {
		t.Fatal(err)
	}
	readOnly := t.TempDir()
	put(t, filepath.Join(readOnly, "odd", "review-results", "own-rdd", "111111111111-1.json"), "a")
	if err := os.Chmod(filepath.Join(readOnly, "odd", "review-results", "own-rdd"), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(readOnly, "odd", "review-results", "own-rdd"), 0o755) })
	store := Store{}
	calls := []struct {
		name string
		do   func()
	}{
		{"a result stored", func() { _ = store.Create(root, "own-rdd", resultName, []byte("x")) }},
		{"a name that is taken", func() { _ = store.Create(root, "own-rdd", resultName, []byte("y")) }},
		{"a listing", func() { _, _ = store.Names(root, "own-rdd") }},
		{"a listing of a feature that has no directory", func() { _, _ = store.Names(root, "other") }},
		{"a listing under a root that is not there", func() { _, _ = store.Names(filepath.Join(root, "absent"), "own-rdd") }},
		{"a result under a root that is not there", func() { _ = store.Create(filepath.Join(root, "absent"), "own-rdd", resultName, nil) }},
		{"a feature that is not one safe segment", func() { _ = store.Create(root, "../own-rdd", resultName, nil) }},
		{"a name that is not one plain name", func() { _ = store.Create(root, "own-rdd", "a/b", nil) }},
		{"a link on the way down", func() { _ = store.Create(linked, "own-rdd", resultName, nil) }},
		{"a listing with a link on the way down", func() { _, _ = store.Names(linked, "own-rdd") }},
		{"a directory that cannot be written", func() { _ = store.Create(readOnly, "own-rdd", resultName, nil) }},
	}
	// The first call of each may open a descriptor the runtime keeps for good; the count starts after them.
	for _, c := range calls {
		c.do()
	}

	for _, c := range calls {
		before := openDescriptors(t)
		c.do()
		if after := openDescriptors(t); after != before {
			t.Errorf("%s: %d descriptors open before the call and %d after it, want the same", c.name, before, after)
		}
	}
}

// A directory that cannot be flushed after the file was linked leaves the result stored: the error says so
// (app.ErrNotFlushed), and the file is there under its name with its content.
func TestAHeldCreateWhoseDirectoryCannotBeFlushedSaysTheResultIsStored(t *testing.T) {
	root := t.TempDir()
	s := Store{syncDir: func(*os.File) error { return errors.New("input/output error") }}

	err := s.createHeld(root, "own-rdd", "222222222222-1.json", []byte("x"))

	if !errors.Is(err, app.ErrNotFlushed) {
		t.Fatalf("createHeld() error = %v, want an error wrapping app.ErrNotFlushed", err)
	}
	data, readErr := os.ReadFile(filepath.Join(root, "odd", "review-results", "own-rdd", "222222222222-1.json"))
	if readErr != nil || string(data) != "x" {
		t.Errorf("stored file = %q, %v; want \"x\"", data, readErr)
	}
}

// The temporary files a held write stages are the ones Names hides: the name is built from tempPattern.
func TestTheTemporaryNameOfAWriteIsOneNamesHides(t *testing.T) {
	for _, token := range []string{"0", "18446744073709551615"} {
		if name := temporaryName(token); !isTemporary(name) {
			t.Errorf("isTemporary(temporaryName(%q) = %q) = false, want true", token, name)
		}
	}
}
