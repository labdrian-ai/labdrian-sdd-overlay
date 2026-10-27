//go:build !windows

package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The tests of this file are inside the package because they change the directories under a Store at the exact
// moment it is using them, which only its race hook allows: another process doing it for real is a race no test
// can win on purpose. Symbolic links and renames over directories need no privilege here, which is why the file
// leaves out windows.

const resultName = "222222222222-1.json"

// impl is one way the Store does its two jobs. Every platform has the one that works by path; linux has the one
// that holds the directories open as well, and that is the one Names and Create are there. A property both must
// have is tried on all of them (impls); one that belongs to only one is tried on it by name.
type impl struct {
	name   string
	names  func(s Store, root, feature string) ([]string, error)
	create func(s Store, root, feature, name string, data []byte) error
}

func byPath() impl { return impl{"by path", Store.namesByPath, Store.createByPath} }

// racing is a Store that calls do when its work reaches point, for the directory the point is about, and counts
// the calls. level, when it is not empty, narrows the calls to the directory whose path ends with it
// (odd, odd/review-results or odd/review-results/<feature>).
func racing(point, level string, do func(dir string)) (Store, *int) {
	fired := 0
	return Store{race: func(p, dir string) {
		if p != point || (level != "" && !strings.HasSuffix(filepath.ToSlash(dir), "/"+level)) {
			return
		}
		fired++
		do(dir)
	}}, &fired
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func namesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// A directory that another process makes between the store's look that found it missing and its own mkdir is
// not an error: the store looks at what is there like at any directory it found, and goes on when it is a real
// directory. A link or a file that was put there instead is refused and followed nowhere.
func TestCreateLooksAtADirectoryThatAppearsWhileItIsBeingMade(t *testing.T) {
	for _, im := range impls() {
		for _, level := range []string{"odd", "odd/review-results", "odd/review-results/own-rdd"} {
			for _, tc := range []struct {
				name    string
				appear  func(t *testing.T, dir, outside string)
				stored  bool
				mention string
			}{
				{"a directory", func(t *testing.T, dir, outside string) {
					// Another mode than the store would give, to see that the store leaves the mode of a directory it did
					// not make as it is.
					if err := os.Mkdir(dir, 0o700); err != nil {
						t.Fatal(err)
					}
				}, true, ""},
				{"a symbolic link to another directory", func(t *testing.T, dir, outside string) {
					if err := os.Symlink(outside, dir); err != nil {
						t.Fatal(err)
					}
				}, false, "is a symbolic link"},
				{"a file", func(t *testing.T, dir, outside string) {
					put(t, dir, "not a directory")
				}, false, "is not a directory"},
			} {
				t.Run(im.name+": "+tc.name+" at "+level, func(t *testing.T) {
					root, outside := t.TempDir(), t.TempDir()
					store, fired := racing(pointMissing, level, func(dir string) { tc.appear(t, dir, outside) })

					err := im.create(store, root, "own-rdd", resultName, []byte("x"))

					if *fired != 1 {
						t.Fatalf("%s was found missing %d times, want once: the branch under test was not reached", level, *fired)
					}
					file := filepath.Join(root, "odd", "review-results", "own-rdd", resultName)
					if tc.stored {
						if content, readErr := os.ReadFile(file); err != nil || readErr != nil || string(content) != "x" {
							t.Errorf("Create() = %v, stored %q (%v); want the file stored whole in the directory that appeared", err, content, readErr)
						}
						if info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(level))); statErr != nil {
							t.Errorf("stat %s: %v", level, statErr)
						} else if info.Mode().Perm() != 0o700 {
							t.Errorf("%s has mode %v after Create, want the 0700 of the writer that made it left as it is", level, info.Mode().Perm())
						}
						return
					}
					if !errors.Is(err, ErrNotADirectory) || !strings.Contains(err.Error(), tc.mention) {
						t.Errorf("Create() error = %v, want one that is %v and says %q", err, ErrNotADirectory, tc.mention)
					}
					if names := namesIn(t, outside); len(names) != 0 {
						t.Errorf("the directory the link leads to holds %q, want nothing written through it", names)
					}
					if exists(file) {
						t.Errorf("%s exists, want nothing stored", file)
					}
				})
			}
		}
	}
}

// A directory of the path that is replaced, while the store uses it, by a link or by another directory is no
// longer the one the store looked at, and nothing it did there is trusted: the listing is not returned, the
// file is not published, and one that was is reported. The standard library has no create or link relative to
// a directory that is held open, so the store looks again instead.
func TestByPathRefusesADirectoryThatChangedWhileItWasInUse(t *testing.T) {
	for _, change := range []struct {
		name string
		do   func(t *testing.T, dir, outside string)
	}{
		{"a link to another directory", func(t *testing.T, dir, outside string) {
			if err := os.Rename(dir, dir+".moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, dir); err != nil {
				t.Fatal(err)
			}
		}},
		{"another directory", func(t *testing.T, dir, outside string) {
			if err := os.Rename(dir, dir+".moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run("the listing is not returned after "+change.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			put(t, filepath.Join(root, "odd", "review-results", "own-rdd", "222222222222-1.json"), "a")
			put(t, filepath.Join(outside, "999999999999-1.json"), "not ours")
			store, fired := racing(pointWalked, "", func(dir string) { change.do(t, dir, outside) })

			names, err := store.namesByPath(root, "own-rdd")

			if *fired != 1 {
				t.Fatalf("the point after the look at the directories was reached %d times, want once", *fired)
			}
			if !errors.Is(err, ErrChanged) || names != nil {
				t.Errorf("Names() = %q, %v; want no names and an error that is %v", names, err, ErrChanged)
			}
		})
		t.Run("the file is not published after "+change.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			store, fired := racing(pointWalked, "", func(dir string) { change.do(t, dir, outside) })

			err := store.createByPath(root, "own-rdd", resultName, []byte("x"))

			if *fired != 1 {
				t.Fatalf("the point after the look at the directories was reached %d times, want once", *fired)
			}
			if !errors.Is(err, ErrChanged) {
				t.Errorf("Create() error = %v, want one that is %v", err, ErrChanged)
			}
			for _, dir := range []string{outside, filepath.Join(root, "odd", "review-results", "own-rdd"), filepath.Join(root, "odd", "review-results", "own-rdd.moved")} {
				if names := namesIn(t, dir); len(names) != 0 {
					t.Errorf("%s holds %q, want nothing published and no temporary file left", dir, names)
				}
			}
		})
		t.Run("a file that was linked is reported after "+change.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			store, fired := racing(pointLinked, "", func(dir string) { change.do(t, dir, outside) })

			err := store.createByPath(root, "own-rdd", resultName, []byte("x"))

			if *fired != 1 {
				t.Fatalf("the point after the link was reached %d times, want once", *fired)
			}
			if !errors.Is(err, ErrChanged) || !strings.Contains(err.Error(), resultName+" was linked") {
				t.Errorf("Create() error = %v, want one that is %v and says that %s was linked", err, ErrChanged, resultName)
			}
			if names := namesIn(t, outside); len(names) != 0 {
				t.Errorf("the directory outside holds %q, want nothing", names)
			}
		})
	}
}

// The look again covers every directory of the path, not only the last. A directory above the one of the feature
// that is moved aside and replaced by a link back to it leads to the very directory the store looked at, so no
// comparison of the last directory can tell; only the look at the directory above it does, and a link in the
// path is what the store refuses. A listing is not returned, a file is not published, and one that was is
// reported.
func TestByPathRefusesALinkPutInPlaceOfADirectoryAboveTheFeature(t *testing.T) {
	for _, level := range []string{"odd", "odd/review-results"} {
		for _, use := range []struct {
			name  string
			point string
			run   func(s Store, root string) ([]string, error)
			// held is what the directory of the feature holds afterwards, for the uses that write.
			held []string
			// said is what the error has to say besides being ErrChanged.
			said string
		}{
			{"a listing", pointWalked, func(s Store, root string) ([]string, error) {
				return s.namesByPath(root, "own-rdd")
			}, nil, ""},
			{"a file about to be linked", pointWalked, func(s Store, root string) ([]string, error) {
				return nil, s.createByPath(root, "own-rdd", resultName, []byte("x"))
			}, []string{"111111111111-1.json"}, ""},
			{"a file that was linked", pointLinked, func(s Store, root string) ([]string, error) {
				return nil, s.createByPath(root, "own-rdd", resultName, []byte("x"))
			}, []string{"111111111111-1.json", resultName}, resultName + " was linked"},
		} {
			t.Run(use.name+" under a link in place of "+level, func(t *testing.T) {
				root := t.TempDir()
				above := filepath.Join(root, filepath.FromSlash(level))
				feature := filepath.Join(root, "odd", "review-results", "own-rdd")
				put(t, filepath.Join(feature, "111111111111-1.json"), "a")
				store, fired := racing(use.point, "", func(string) {
					if err := os.Rename(above, above+".moved"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(above+".moved", above); err != nil {
						t.Fatal(err)
					}
				})

				names, err := use.run(store, root)

				if *fired != 1 {
					t.Fatalf("the point of the race was reached %d times, want once", *fired)
				}
				if !errors.Is(err, ErrChanged) || names != nil || !strings.Contains(err.Error(), use.said) {
					t.Errorf("got %q, %v; want no names and an error that is %v and says %q", names, err, ErrChanged, use.said)
				}
				if use.held != nil {
					if got := namesIn(t, feature); !reflect.DeepEqual(got, use.held) {
						t.Errorf("the directory of the feature holds %q, want %q: nothing more published and no temporary file left", got, use.held)
					}
				}
			})
		}
	}
}

// What changes in a directory is not a change of the directory: a writer that adds a file while another lists
// or publishes is the race the store exists for, and it is not refused.
func TestAChangeOfWhatIsInADirectoryIsNotAChangeOfTheDirectory(t *testing.T) {
	for _, im := range impls() {
		t.Run(im.name+": Names", func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "odd", "review-results", "own-rdd")
			put(t, filepath.Join(dir, "222222222222-1.json"), "a")
			store, fired := racing(pointWalked, "", func(dir string) { put(t, filepath.Join(dir, "222222222222-2.json"), "b") })

			names, err := im.names(store, root, "own-rdd")

			if *fired != 1 || err != nil || !reflect.DeepEqual(names, []string{"222222222222-1.json", "222222222222-2.json"}) {
				t.Errorf("Names() = %q, %v after %d writes in between; want both files and no error", names, err, *fired)
			}
		})
		t.Run(im.name+": Create", func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "odd", "review-results", "own-rdd")
			written := 0
			store := Store{race: func(point, dir string) {
				written++
				put(t, filepath.Join(dir, "other-"+point+".json"), "other writer")
			}}

			err := im.create(store, root, "own-rdd", resultName, []byte("x"))

			if err != nil || written == 0 {
				t.Fatalf("Create() = %v after %d writes in between; want nil: the directories are the same", err, written)
			}
			if content, readErr := os.ReadFile(filepath.Join(dir, resultName)); readErr != nil || string(content) != "x" {
				t.Errorf("stored file = %q, %v; want the content", content, readErr)
			}
		})
	}
}

// The temporary file that a Create stages is the one Names hides: at the moment the file is linked into place
// the directory holds both names, and only the result is listed. The temporary one is gone when Create returns.
func TestNamesDoesNotListTheTemporaryFileACreateHasStaged(t *testing.T) {
	for _, im := range impls() {
		t.Run(im.name, func(t *testing.T) {
			root := t.TempDir()
			var onDisk, listed []string
			store := Store{race: func(point, dir string) {
				if point != pointLinked {
					return
				}
				onDisk = namesIn(t, dir)
				names, err := im.names(Store{}, root, "own-rdd")
				if err != nil {
					t.Errorf("Names() error = %v, want nil", err)
				}
				listed = names
			}}

			if err := im.create(store, root, "own-rdd", resultName, []byte("x")); err != nil {
				t.Fatalf("Create() error = %v, want nil", err)
			}

			temporary := 0
			for _, name := range onDisk {
				if strings.HasPrefix(name, ".review-") && strings.HasSuffix(name, ".tmp") {
					temporary++
				}
			}
			if len(onDisk) != 2 || temporary != 1 {
				t.Fatalf("the directory held %q when the file was linked, want the result and the one temporary file of the write", onDisk)
			}
			if !reflect.DeepEqual(listed, []string{resultName}) {
				t.Errorf("Names() = %q while the write was in flight, want only %q", listed, resultName)
			}
			if got := namesIn(t, filepath.Join(root, "odd", "review-results", "own-rdd")); !reflect.DeepEqual(got, []string{resultName}) {
				t.Errorf("the directory holds %q after Create, want only %q", got, resultName)
			}
		})
	}
}
