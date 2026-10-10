package vaultfs

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// Vault is the adapter behind the port promote owns for the precedence store.
var _ promote.PrecedenceRepository = (*Vault)(nil)

// sidecarPath is where the precedence sidecar of the vault at root lives: written out as the literal a vault
// written by an earlier build holds, not derived from the layout under test.
func sidecarPath(root string) string {
	return filepath.Join(root, ".raw", ".longterm-mem-manifest.json")
}

// A vault nothing was promoted in has no sidecar: loading it answers an empty store, not an error, and
// creates nothing.
func TestLoadPrecedenceOfAFreshVaultIsEmptyAndCreatesNothing(t *testing.T) {
	root := t.TempDir()

	store, err := New(root).LoadPrecedence()
	if err != nil {
		t.Fatalf("LoadPrecedence: %v", err)
	}
	if store == nil || len(store) != 0 {
		t.Fatalf("store = %#v, want an empty, non-nil store", store)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("loading created %d entries in the vault, want none", len(entries))
	}
}

// What is saved is read back, entry for entry, by a new adapter on the same vault -- the revision included,
// which a hashes-only entry written before the field existed lacks.
func TestSavedPrecedenceIsReadBack(t *testing.T) {
	root := t.TempDir()
	want := promote.PrecedenceStore{
		"c-000042": {BodyHash: "body-1", FrontmatterHash: "fm-1", PromotedRevision: 7},
		"c-000043": {BodyHash: "body-2", FrontmatterHash: "fm-2"},
	}
	if err := New(root).SavePrecedence(want); err != nil {
		t.Fatalf("SavePrecedence: %v", err)
	}

	got, err := New(root).LoadPrecedence()
	if err != nil {
		t.Fatalf("LoadPrecedence: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("store = %+v, want %+v", got, want)
	}
	for address, entry := range want {
		if got[address] != entry {
			t.Errorf("entry %s = %+v, want %+v", address, got[address], entry)
		}
	}
}

// The file is the store as JSON, indented by two spaces and ended by a newline, entries in the order of their
// addresses, the revision left out when it is not recorded: other tools and the people reading the vault see
// these bytes, so they are pinned as the bytes themselves.
func TestSavePrecedenceWritesTheBytesTheVaultHasAlwaysHeld(t *testing.T) {
	root := t.TempDir()
	store := promote.PrecedenceStore{
		"c-000043": {BodyHash: "b2", FrontmatterHash: "f2"},
		"c-000042": {BodyHash: "b1", FrontmatterHash: "f1", PromotedRevision: 3},
	}
	if err := New(root).SavePrecedence(store); err != nil {
		t.Fatalf("SavePrecedence: %v", err)
	}

	got, err := os.ReadFile(sidecarPath(root))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "c-000042": {
    "body_hash": "b1",
    "frontmatter_hash": "f1",
    "promoted_revision": 3
  },
  "c-000043": {
    "body_hash": "b2",
    "frontmatter_hash": "f2"
  }
}
`
	if string(got) != want {
		t.Fatalf("the sidecar holds\n%s\nwant\n%s", got, want)
	}
}

// A sidecar this module creates is owner-only, in a directory it makes when the vault has none yet: it
// holds fingerprints of memory content, and a file brought into existence here starts private.
func TestSavePrecedenceCreatesTheSidecarOwnerOnly(t *testing.T) {
	root := t.TempDir()
	if err := New(root).SavePrecedence(promote.PrecedenceStore{}); err != nil {
		t.Fatalf("SavePrecedence: %v", err)
	}

	info, err := os.Stat(sidecarPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("a created sidecar has mode %o, want 600", mode)
	}
	if dir, err := os.Stat(filepath.Dir(sidecarPath(root))); err != nil || !dir.IsDir() {
		t.Errorf("the sidecar's directory was not created (stat err = %v)", err)
	}
}

// A sidecar that exists keeps the mode its owner gave it: saving replaces the content, not the file's
// identity, so a vault kept at 0644 and tracked by git does not turn 0600 on the first promotion.
func TestSavePrecedenceKeepsTheModeOfAnExistingSidecar(t *testing.T) {
	root := t.TempDir()
	path := sidecarPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New(root).SavePrecedence(promote.PrecedenceStore{"c-000001": {BodyHash: "b", FrontmatterHash: "f"}}); err != nil {
		t.Fatalf("SavePrecedence: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Errorf("the sidecar has mode %o after a save, want the 644 it had", mode)
	}
}

// A sidecar reached through a symlink (a synced or dotfiles-style vault) is edited where it really is, and
// the link stays a link.
func TestSavePrecedenceFollowsASymlinkedSidecar(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target-sidecar.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := sidecarPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if err := New(root).SavePrecedence(promote.PrecedenceStore{"c-000001": {BodyHash: "b", FrontmatterHash: "f"}}); err != nil {
		t.Fatalf("SavePrecedence: %v", err)
	}

	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the sidecar is no longer a symlink (lstat err = %v)", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "c-000001") {
		t.Errorf("the file behind the link holds %q, want the saved store", data)
	}
}

// Saving replaces the file whole and leaves nothing beside it: no temporary file survives a save.
func TestSavePrecedenceLeavesNoTemporaryFile(t *testing.T) {
	root := t.TempDir()
	for range 3 {
		if err := New(root).SavePrecedence(promote.PrecedenceStore{"c-000001": {BodyHash: "b", FrontmatterHash: "f"}}); err != nil {
			t.Fatalf("SavePrecedence: %v", err)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(sidecarPath(root)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != ".longterm-mem-manifest.json" {
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		t.Fatalf("the sidecar's directory holds %v, want only the sidecar", names)
	}
}

// Saving replaces the file rather than rewriting it in place: a reader that had the old file open keeps
// reading the old content whole, and never sees the truncated half-file an in-place write passes through.
// That is what makes a save atomic for the tools reading the vault at the same time.
func TestSavePrecedenceReplacesTheFileInsteadOfRewritingIt(t *testing.T) {
	root := t.TempDir()
	path := sidecarPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const old = "{\"c-000001\":{\"body_hash\":\"old\",\"frontmatter_hash\":\"old\"}}\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := New(root).SavePrecedence(promote.PrecedenceStore{"c-000002": {BodyHash: "new", FrontmatterHash: "new"}}); err != nil {
		t.Fatalf("SavePrecedence: %v", err)
	}

	seen, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(seen) != old {
		t.Errorf("a reader of the file as it was saw %q after the save, want the old content whole", seen)
	}
	now, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(now), "c-000002") {
		t.Errorf("the sidecar holds %q after the save, want the new store", now)
	}
}

// A sidecar that cannot be read is an error that names the file: a directory where the file belongs is
// neither "nothing promoted yet" nor a store.
func TestLoadPrecedenceReportsASidecarThatCannotBeRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(sidecarPath(root), 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := New(root).LoadPrecedence()
	if err == nil {
		t.Fatalf("LoadPrecedence = %+v, nil error, want the read failure", store)
	}
	if store != nil {
		t.Errorf("store = %+v alongside an error, want none", store)
	}
	if want := "promote: read " + sidecarPath(root) + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
}

// A sidecar that is not a store is an error that names the file, never an empty store that the next save
// would write over the fingerprints it could not read.
func TestLoadPrecedenceReportsASidecarThatIsNotJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(sidecarPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecarPath(root), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := New(root).LoadPrecedence()
	if err == nil {
		t.Fatalf("LoadPrecedence = %+v, nil error, want the parse failure", store)
	}
	if store != nil {
		t.Errorf("store = %+v alongside an error, want none", store)
	}
	if want := "promote: parse " + sidecarPath(root) + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
}

// A save that cannot land is an error, and leaves what was there: a directory where the file belongs cannot
// be replaced.
func TestSavePrecedenceReportsASidecarItCannotReplace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(sidecarPath(root), 0o755); err != nil {
		t.Fatal(err)
	}

	err := New(root).SavePrecedence(promote.PrecedenceStore{"c-000001": {BodyHash: "b", FrontmatterHash: "f"}})
	if err == nil {
		t.Fatal("SavePrecedence = nil error, want the failure to replace a directory")
	}
	if !strings.HasPrefix(err.Error(), "promote: ") {
		t.Errorf("error %q does not start with the prefix promotion has always given it", err)
	}
	if info, statErr := os.Stat(sidecarPath(root)); statErr != nil || !info.IsDir() {
		t.Errorf("what was at the sidecar's path is gone (stat err = %v)", statErr)
	}
}
