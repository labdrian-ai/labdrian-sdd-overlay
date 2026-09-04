package promote

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFileAtomic edits files inside the user's own Obsidian vault, not
// longterm-mem's state directory: PatchStatusFields rewrites an existing
// wiki/memory page in place, and the update path republishes one. Those
// pages are the user's notes — routinely 0o644, routinely inside a git
// repository, and in a synced or dotfiles-style vault routinely reached
// through a symlink. Replacing them with a fresh os.CreateTemp inode
// re-permissions every page it touches to 0o600 and silently swaps a
// symlink for a regular file.
//
// These are the same two identity assertions the register package needed,
// asked of the writer that edits markdown rather than config.

func permOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// TestWriteFileAtomic_PreservesAnExistingPagesMode: a vault page the user
// (or their sync tool, or their git checkout) left at 0o644 must not come
// back 0o600 because longterm-mem patched a frontmatter field.
func TestWriteFileAtomic_PreservesAnExistingPagesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.md")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed page: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod page: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new\n")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	if got := permOf(t, path); got != 0o644 {
		t.Fatalf("page mode after rewrite = %o, want the pre-existing %o", got, 0o644)
	}
}

// TestWriteFileAtomic_CreatesNewPagesOwnerOnly pins the other half: a page
// longterm-mem creates itself carries promoted memory content, so it starts
// owner-only. Preserving an existing mode must not turn into "inherit
// whatever the umask says" for a file nobody has an opinion about yet.
func TestWriteFileAtomic_CreatesNewPagesOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "page.md")

	if err := writeFileAtomic(path, []byte("new\n")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	if got := permOf(t, path); got != 0o600 {
		t.Fatalf("new page mode = %o, want %o", got, 0o600)
	}
}

// TestWriteFileAtomic_WritesThroughASymlink: a vault whose pages are
// symlinks into a synced or version-controlled store. Renaming onto the
// link deletes it and leaves a regular file, so the user's real page never
// receives the edit while promote reports success.
func TestWriteFileAtomic_WritesThroughASymlink(t *testing.T) {
	vault := t.TempDir()
	store := t.TempDir()
	target := filepath.Join(store, "tracked.md")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed store page: %v", err)
	}
	link := filepath.Join(vault, "page.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := writeFileAtomic(link, []byte("new\n")); err != nil {
		t.Fatalf("writeFileAtomic through a symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink (mode %v) — the write replaced the user's link", link, info.Mode())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if string(got) != "new\n" {
		t.Fatalf("symlink target content = %q, want %q", got, "new\n")
	}
}
