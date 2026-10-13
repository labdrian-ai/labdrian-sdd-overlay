package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// treeWith makes a directory holding a .git directory, a file and a subdirectory, and says where it is.
func treeWith(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{".git", "skills"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func touch(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

// git's own metadata changes its modification time when a git process only reads, so the time of .git is
// not part of the listing; every other entry still shows when it is touched.
func TestTreeListing_IgnoresTheTimeOfGitAndNothingElse(t *testing.T) {
	dir := treeWith(t)
	before := treeListing(t, dir)

	touch(t, filepath.Join(dir, ".git"))
	if after := treeListing(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("the time of .git changed the listing:\nbefore %v\nafter  %v", before, after)
	}

	touch(t, filepath.Join(dir, "README"))
	if after := treeListing(t, dir); reflect.DeepEqual(before, after) {
		t.Errorf("the time of a file did not change the listing: %v", after)
	}
}

// What is left out is the time of .git, not the fact that it is there and what kind of thing it is: a .git
// that is removed, or replaced by a file (a worktree's pointer), changes the listing.
func TestTreeListing_NoticesGitBeingRemovedOrReplaced(t *testing.T) {
	removed, replaced := treeWith(t), treeWith(t)
	beforeRemoved, beforeReplaced := treeListing(t, removed), treeListing(t, replaced)

	if err := os.Remove(filepath.Join(removed, ".git")); err != nil {
		t.Fatal(err)
	}
	if after := treeListing(t, removed); reflect.DeepEqual(beforeRemoved, after) {
		t.Errorf("removing .git did not change the listing: %v", after)
	}

	if err := os.Remove(filepath.Join(replaced, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replaced, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if after := treeListing(t, replaced); reflect.DeepEqual(beforeReplaced, after) {
		t.Errorf("replacing .git by a file did not change the listing: %v", after)
	}
}
