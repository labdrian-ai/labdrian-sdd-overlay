package vaultfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// Vault is the adapter behind both ports promote owns for the address map: the one a promotion records
// through, and the one the page check reads through.
var (
	_ promote.AddressMapRecorder = (*Vault)(nil)
	_ promote.AddressMapReader   = (*Vault)(nil)
)

// manifestPath is where the address map of the vault at root lives: written out as the literal a vault
// written by wiki-ingest holds, not derived from the layout under test.
func manifestPath(root string) string {
	return filepath.Join(root, ".raw", ".manifest.json")
}

// writeManifest leaves content, at mode, as the vault's manifest.
func writeManifest(t *testing.T, root, content string, mode os.FileMode) {
	t.Helper()
	path := manifestPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func readManifest(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(manifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

var recordedAt = time.Date(2026, 8, 5, 12, 30, 0, 0, time.UTC)

// A vault with no manifest gets one with the fields a manifest of wiki-ingest's has when it is new, dated at
// the day the caller gave: the bytes are other tools' to read, so they are pinned as the bytes themselves.
func TestRecordAddressSeedsAManifestTheVaultDoesNotHaveYet(t *testing.T) {
	root := t.TempDir()

	if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
		t.Fatalf("RecordAddress: %v", err)
	}

	want := `{
  "address_map": {
    "wiki/memory/c-000042.md": "c-000042"
  },
  "created": "2026-08-05",
  "sources": {},
  "version": 1
}
`
	if got := readManifest(t, root); got != want {
		t.Fatalf("the manifest holds\n%s\nwant\n%s", got, want)
	}
}

// A manifest this module creates is owner-only, in a directory it makes when the vault has none yet.
func TestRecordAddressCreatesTheManifestOwnerOnly(t *testing.T) {
	root := t.TempDir()
	if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
		t.Fatalf("RecordAddress: %v", err)
	}

	info, err := os.Stat(manifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("a created manifest has mode %o, want 600", mode)
	}
	if dir, err := os.Stat(filepath.Dir(manifestPath(root))); err != nil || !dir.IsDir() {
		t.Errorf("the manifest's directory was not created (stat err = %v)", err)
	}
}

// The manifest is wiki-ingest's: a field this module does not know survives a new entry, none is fabricated
// into a manifest that did not carry it, and the entries already there stay.
func TestRecordAddressChangesNothingButTheEntry(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `{"version": 7, "ingest_options": {"dedupe": true}, "address_map": {"wiki/memory/c-000001.md": "c-000001"}}`, 0o644)

	if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
		t.Fatalf("RecordAddress: %v", err)
	}

	want := `{
  "address_map": {
    "wiki/memory/c-000001.md": "c-000001",
    "wiki/memory/c-000042.md": "c-000042"
  },
  "ingest_options": {
    "dedupe": true
  },
  "version": 7
}
`
	if got := readManifest(t, root); got != want {
		t.Fatalf("the manifest holds\n%s\nwant\n%s", got, want)
	}
}

// An entry already there for the page is replaced, and a manifest with no address_map gets one.
func TestRecordAddressReplacesAnEntryAndAddsAMissingMap(t *testing.T) {
	t.Run("a stale entry", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, `{"address_map":{"wiki/memory/c-000042.md":"c-000555"}}`, 0o644)
		if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
			t.Fatalf("RecordAddress: %v", err)
		}
		if got, want := readManifest(t, root), "{\n  \"address_map\": {\n    \"wiki/memory/c-000042.md\": \"c-000042\"\n  }\n}\n"; got != want {
			t.Fatalf("the manifest holds\n%s\nwant\n%s", got, want)
		}
	})
	t.Run("no address_map", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, `{"version":1}`, 0o644)
		if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
			t.Fatalf("RecordAddress: %v", err)
		}
		if got, want := readManifest(t, root), "{\n  \"address_map\": {\n    \"wiki/memory/c-000042.md\": \"c-000042\"\n  },\n  \"version\": 1\n}\n"; got != want {
			t.Fatalf("the manifest holds\n%s\nwant\n%s", got, want)
		}
	})
}

// A manifest that exists keeps the mode its owner gave it: recording replaces the content, not the file's
// identity.
func TestRecordAddressKeepsTheModeOfAnExistingManifest(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `{"address_map":{}}`, 0o640)

	if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
		t.Fatalf("RecordAddress: %v", err)
	}

	info, err := os.Stat(manifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o640 {
		t.Errorf("the manifest has mode %o after a record, want the 640 it had", mode)
	}
}

// A manifest reached through a symlink is edited where it really is, and the link stays a link.
func TestRecordAddressFollowsASymlinkedManifest(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target-manifest.json")
	if err := os.WriteFile(target, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, manifestPath(root)); err != nil {
		t.Fatal(err)
	}

	if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
		t.Fatalf("RecordAddress: %v", err)
	}

	if info, err := os.Lstat(manifestPath(root)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the manifest is no longer a symlink (lstat err = %v)", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "c-000042") {
		t.Errorf("the file behind the link holds %q, want the entry", data)
	}
}

// Recording replaces the file rather than rewriting it in place: a reader that had the old file open keeps
// reading the old content whole, and no temporary file survives.
func TestRecordAddressReplacesTheFileInsteadOfRewritingIt(t *testing.T) {
	root := t.TempDir()
	const old = `{"address_map":{"wiki/memory/c-000001.md":"c-000001"}}`
	writeManifest(t, root, old, 0o644)
	reader, err := os.Open(manifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := New(root).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt); err != nil {
		t.Fatalf("RecordAddress: %v", err)
	}

	seen, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(seen) != old {
		t.Errorf("a reader of the file as it was saw %q after the record, want the old content whole", seen)
	}
	entries, err := os.ReadDir(filepath.Dir(manifestPath(root)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the manifest's directory holds %d entries, want only the manifest", len(entries))
	}
}

// A manifest that cannot be understood is an error that names the file and leaves the file as it was: the
// entry is not recorded over what could not be read.
func TestRecordAddressRefusesAManifestItCannotUnderstand(t *testing.T) {
	for name, tc := range map[string]struct{ content, want string }{
		"not json":                        {"{not json", "parse %s: "},
		"empty":                           {"", "parse %s: "},
		"an array":                        {"[]", "parse %s: "},
		"JSON null":                       {"null", "parse %s: "},
		"JSON null with white space":      {" null\n", "parse %s: "},
		"an address_map that is a string": {`{"address_map":"nope"}`, "parse %s address_map: "},
		"an address_map of numbers":       {`{"address_map":{"a":1}}`, "parse %s address_map: "},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeManifest(t, root, tc.content, 0o644)

			err := New(root, WithErrorPrefix("promote")).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt)
			if err == nil {
				t.Fatal("RecordAddress = nil error, want the parse failure")
			}
			if want := "promote: " + strings.Replace(tc.want, "%s", manifestPath(root), 1); !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
			if got := readManifest(t, root); got != tc.content {
				t.Errorf("the manifest holds %q after the refusal, want it untouched (%q)", got, tc.content)
			}
		})
	}
}

// A directory where the manifest belongs is neither a manifest that is not there yet nor one that can be
// replaced: the record fails, naming the file, and the directory stays.
func TestRecordAddressReportsAManifestItCannotRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(manifestPath(root), 0o755); err != nil {
		t.Fatal(err)
	}

	err := New(root, WithErrorPrefix("promote")).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt)
	if err == nil {
		t.Fatal("RecordAddress = nil error, want the read failure")
	}
	if want := "promote: read " + manifestPath(root) + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
	if info, statErr := os.Stat(manifestPath(root)); statErr != nil || !info.IsDir() {
		t.Errorf("what was at the manifest's path is gone (stat err = %v)", statErr)
	}
}

// A manifest that is a link to a file that does not exist reads as no manifest, as it always did, and the
// write that follows cannot land: the failure names what could not be created, and the link stays.
func TestRecordAddressOverALinkToNothingFailsAndKeepsTheLink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(manifestPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../notes/absent.json", manifestPath(root)); err != nil {
		t.Fatal(err)
	}

	err := New(root, WithErrorPrefix("promote")).RecordAddress("wiki/memory/c-000042.md", "c-000042", recordedAt)
	if err == nil {
		t.Fatal("RecordAddress = nil error, want the failure to create the file behind the link")
	}
	if !strings.HasPrefix(err.Error(), "promote: durable: create temp file for "+filepath.Join(root, "notes", "absent.json")) {
		t.Errorf("error %q does not say what could not be created", err)
	}
	if target, linkErr := os.Readlink(manifestPath(root)); linkErr != nil || target != "../notes/absent.json" {
		t.Errorf("the link is now %q (readlink err = %v), want it kept", target, linkErr)
	}
}

// The map the manifest holds is what a reader gets, as it stands, entry for entry.
func TestLoadAddressMapReturnsTheEntriesOfTheManifest(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `{"version":3,"address_map":{"wiki/memory/c-000001.md":"c-000001","wiki/memory/c-000042.md":"c-000042"},"extra":[1]}`, 0o644)

	got, err := New(root).LoadAddressMap()
	if err != nil {
		t.Fatalf("LoadAddressMap: %v", err)
	}
	want := promote.AddressMap{"wiki/memory/c-000001.md": "c-000001", "wiki/memory/c-000042.md": "c-000042"}
	if len(got) != len(want) {
		t.Fatalf("map = %v, want %v", got, want)
	}
	for path, address := range want {
		if got[path] != address {
			t.Errorf("map[%s] = %q, want %q", path, got[path], address)
		}
	}
}

// A manifest that holds no address map, or an address_map of JSON null, is a manifest with nothing in its
// map: not an error, and not a failure to read.
func TestLoadAddressMapOfAManifestWithNoEntriesIsEmpty(t *testing.T) {
	for name, content := range map[string]string{"no address_map": `{"version":1}`, "an address_map of null": `{"address_map":null}`, "an empty map": `{"address_map":{}}`} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeManifest(t, root, content, 0o644)

			got, err := New(root).LoadAddressMap()
			if err != nil {
				t.Fatalf("LoadAddressMap: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("map = %v, want none", got)
			}
		})
	}
}

// A vault with no manifest, and one whose manifest is a directory, cannot answer a map: the error is not the
// one for a manifest that is corrupt, so a page check can tell nothing to check against from a damaged map,
// and a missing file is still a missing file to whoever asks.
func TestLoadAddressMapOfAManifestThatCannotBeReadIsNotCorruption(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		got, err := New(t.TempDir()).LoadAddressMap()
		if err == nil || got != nil {
			t.Fatalf("LoadAddressMap = (%v, %v), want no map and an error", got, err)
		}
		if errors.Is(err, promote.ErrAddressMapCorrupt) {
			t.Errorf("error %q says the manifest is corrupt", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error %q does not wrap that the file is not there", err)
		}
	})
	t.Run("a directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(manifestPath(root), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := New(root, WithErrorPrefix("promote")).LoadAddressMap()
		if err == nil || got != nil {
			t.Fatalf("LoadAddressMap = (%v, %v), want no map and an error", got, err)
		}
		if errors.Is(err, promote.ErrAddressMapCorrupt) {
			t.Errorf("error %q says the manifest is corrupt", err)
		}
		if want := "promote: read " + manifestPath(root) + ": "; !strings.HasPrefix(err.Error(), want) {
			t.Errorf("error %q does not start with %q", err, want)
		}
	})
}

// A manifest that is there and is not an address map is corrupt, for every way it can fail to be one, and
// the error says so without changing a word of what it says about the file.
func TestLoadAddressMapOfACorruptManifestIsErrAddressMapCorrupt(t *testing.T) {
	for name, content := range map[string]string{
		"not json":                        "{not json",
		"empty":                           "",
		"an array":                        "[]",
		"JSON null":                       "null",
		"JSON null with white space":      " null\n",
		"an address_map that is a string": `{"address_map":"nope"}`,
		"a value that is not a string":    `{"address_map":{"wiki/memory/c-000042.md":42}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeManifest(t, root, content, 0o644)

			got, err := New(root, WithErrorPrefix("promote")).LoadAddressMap()
			if err == nil || got != nil {
				t.Fatalf("LoadAddressMap = (%v, %v), want no map and an error", got, err)
			}
			if !errors.Is(err, promote.ErrAddressMapCorrupt) {
				t.Errorf("error %q is not promote.ErrAddressMapCorrupt", err)
			}
			if want := "promote: parse " + manifestPath(root) + ": "; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
			if strings.Contains(err.Error(), promote.ErrAddressMapCorrupt.Error()) {
				t.Errorf("error %q repeats the sentinel's own text: the sentinel must not change what the error says", err)
			}
		})
	}
}

// A manifest reached through a link is read where it really is.
func TestLoadAddressMapFollowsASymlinkedManifest(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target-manifest.json")
	if err := os.WriteFile(target, []byte(`{"address_map":{"wiki/memory/c-000042.md":"c-000042"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, manifestPath(root)); err != nil {
		t.Fatal(err)
	}

	got, err := New(root).LoadAddressMap()
	if err != nil {
		t.Fatalf("LoadAddressMap: %v", err)
	}
	if got["wiki/memory/c-000042.md"] != "c-000042" {
		t.Errorf("map = %v, want the entry of the file behind the link", got)
	}
}
