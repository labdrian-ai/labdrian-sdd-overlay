package durable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func permOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func mustWriteFile(t *testing.T, path string, data string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s to %o: %v", path, mode, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestWriteFile_CreatesAbsentFileWithFallbackPerm: with nothing to preserve,
// the caller's fallback perm is what the new file gets — not os.CreateTemp's
// 0o600 default, which would silently override a caller that asked for
// something else.
func TestWriteFile_CreatesAbsentFileWithFallbackPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")

	if err := WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got := mustReadFile(t, path); got != "{}\n" {
		t.Fatalf("content = %q, want %q", got, "{}\n")
	}
	if got := permOf(t, path); got != 0o644 {
		t.Fatalf("mode = %o, want %o (the fallback perm, not os.CreateTemp's default)", got, 0o644)
	}
}

// TestWriteFile_PreservesAnExistingFilesMode: the file already on disk owns
// its permissions. The fallback perm applies to creation only, so a caller
// passing 0o600 must not re-permission a 0o644 file the user set.
func TestWriteFile_PreservesAnExistingFilesMode(t *testing.T) {
	for _, mode := range []os.FileMode{0o644, 0o640, 0o664, 0o400, 0o444} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "existing.json")
			mustWriteFile(t, path, "old\n", mode)

			if err := WriteFile(path, []byte("new\n"), 0o600); err != nil {
				t.Fatalf("WriteFile over a %o file: %v", mode, err)
			}

			if got := mustReadFile(t, path); got != "new\n" {
				t.Fatalf("content = %q, want %q", got, "new\n")
			}
			if got := permOf(t, path); got != mode {
				t.Fatalf("mode = %o, want the pre-existing %o", got, mode)
			}
		})
	}
}

// TestWriteFile_ReplacesAReadOnlyFile is the atomicity assertion: a
// rename-based replace needs write permission on the DIRECTORY, a
// truncate-and-write needs it on the FILE. A 0o444 file therefore can only
// be replaced, never rewritten in place.
func TestWriteFile_ReplacesAReadOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly.json")
	mustWriteFile(t, path, "old\n", 0o444)

	if err := WriteFile(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile over a read-only file: %v — a durable write replaces the file, it does not rewrite it in place", err)
	}
	if got := mustReadFile(t, path); got != "new\n" {
		t.Fatalf("content = %q, want %q", got, "new\n")
	}
	if got := permOf(t, path); got != 0o444 {
		t.Fatalf("mode = %o, want %o", got, 0o444)
	}
}

// TestWriteFile_WritesThroughASymlink: a dotfiles layout points the config
// path at a file inside a tracked repository. Renaming onto the LINK
// deletes it and leaves a regular file behind, so the user's real file
// never receives the write. The link must survive and its target must be
// what changed.
func TestWriteFile_WritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	store := t.TempDir()
	target := filepath.Join(store, "tracked.json")
	mustWriteFile(t, target, "old\n", 0o644)
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := WriteFile(link, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile through a symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink (mode %v) — the write replaced the user's link", link, info.Mode())
	}
	if got := mustReadFile(t, target); got != "new\n" {
		t.Fatalf("symlink target content = %q, want %q", got, "new\n")
	}
	if got := permOf(t, target); got != 0o644 {
		t.Fatalf("symlink target mode = %o, want %o", got, 0o644)
	}
	// The temp file must be created next to the RESOLVED file, never next
	// to the link: rename cannot cross filesystems, and a dotfiles store is
	// routinely on a different mount than $HOME.
	assertNoLeftovers(t, dir, "config.json")
	assertNoLeftovers(t, store, "tracked.json")
}

// TestWriteFile_WritesThroughASymlinkedDirectory: the link need not be the
// file itself. A whole config directory symlinked into a dotfiles store has
// the same failure mode for the temp file's placement.
func TestWriteFile_WritesThroughASymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(realDir, "config.json"), "old\n", 0o640)
	linkDir := filepath.Join(root, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}

	if err := WriteFile(filepath.Join(linkDir, "config.json"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile through a symlinked directory: %v", err)
	}

	info, err := os.Lstat(linkDir)
	if err != nil {
		t.Fatalf("lstat link dir: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink", linkDir)
	}
	if got := mustReadFile(t, filepath.Join(realDir, "config.json")); got != "new\n" {
		t.Fatalf("content = %q, want %q", got, "new\n")
	}
	if got := permOf(t, filepath.Join(realDir, "config.json")); got != 0o640 {
		t.Fatalf("mode = %o, want %o", got, 0o640)
	}
}

// TestWriteFile_CreatesThroughADanglingSymlink: a dotfiles link whose
// target has not been created yet still names where the user wants the file
// to live. Replacing the link with a regular file would silently break the
// layout on the very first write.
func TestWriteFile_CreatesThroughADanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	store := t.TempDir()
	target := filepath.Join(store, "not-yet.json")
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := WriteFile(link, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile through a dangling symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink", link)
	}
	if got := mustReadFile(t, target); got != "new\n" {
		t.Fatalf("target content = %q, want %q", got, "new\n")
	}
	if got := permOf(t, target); got != 0o644 {
		t.Fatalf("target mode = %o, want the fallback %o", got, 0o644)
	}
}

// TestWriteFile_RefusesASymlinkCycle: resolving links by hand needs a hop
// budget, or a cycle spins forever instead of returning an error.
func TestWriteFile_RefusesASymlinkCycle(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Fatalf("symlink a->b: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatalf("symlink b->a: %v", err)
	}

	err := WriteFile(a, []byte("new\n"), 0o600)
	if err == nil {
		t.Fatal("WriteFile onto a symlink cycle returned nil, want an error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v, want it to name the symlink problem", err)
	}
}

// TestWriteFile_LeavesNoTempFileBehindOnSuccess: a temp file that outlives
// its write is litter in a directory the user reads.
func TestWriteFile_LeavesNoTempFileBehindOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	mustWriteFile(t, path, "old\n", 0o644)

	if err := WriteFile(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertNoLeftovers(t, dir, "config.json")
}

// TestWriteFile_LeavesNoTempFileBehindOnFailure: the failure path has to
// clean up too, or a full disk leaves a growing pile of half-written temps
// next to the user's config.
//
// The failure is forced at the RENAME, not earlier: a destination that is
// an existing directory lets the temp file be created, written, synced and
// chmod'd before anything goes wrong, so the test genuinely exercises the
// cleanup rather than a path where no temp file was ever made.
func TestWriteFile_LeavesNoTempFileBehindOnFailure(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "config.json")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("seed a directory where the file should go: %v", err)
	}

	if err := WriteFile(blocked, []byte("new\n"), 0o600); err == nil {
		t.Fatal("WriteFile onto an existing directory returned nil, want an error")
	}
	assertNoLeftovers(t, dir, "config.json")
}

// TestWriteFile_DoesNotWidenAMoreRestrictiveFile: os.CreateTemp always
// creates at 0o600, so a destination MORE restrictive than that (0o400)
// needs the chmod to narrow the temp file, not only to widen it — and the
// caller's createPerm must not leak in as an upgrade. Nothing is exposed to
// another user in the meantime (0o600 is already owner-only), but the
// visible file must never carry a mode its owner did not choose.
func TestWriteFile_DoesNotWidenAMoreRestrictiveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")
	mustWriteFile(t, path, "old\n", 0o400)

	if err := WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := permOf(t, path); got != 0o400 {
		t.Fatalf("mode = %o, want the pre-existing %o — a more restrictive file must not be widened", got, 0o400)
	}
}

// assertNoLeftovers fails if dir holds anything besides the named entries.
func assertNoLeftovers(t *testing.T, dir string, allowed ...string) {
	t.Helper()
	keep := map[string]bool{}
	for _, name := range allowed {
		keep[name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if keep[e.Name()] {
			continue
		}
		t.Fatalf("unexpected leftover %s in %s", e.Name(), dir)
	}
}
