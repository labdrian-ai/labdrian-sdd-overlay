package vaultregfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

var _ vaultreg.Store = Store{}

func storeIn(t *testing.T) Store {
	t.Helper()
	return Store{Path: filepath.Join(t.TempDir(), "state", "vaults.json")}
}

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readRaw(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestExists_SaysWhetherAFileIsThere(t *testing.T) {
	store := storeIn(t)

	if exists, err := store.Exists(); exists || err != nil {
		t.Errorf("Exists before the registry is written = (%v, %v), want (false, nil)", exists, err)
	}
	writeRaw(t, store.Path, "{}")
	if exists, err := store.Exists(); !exists || err != nil {
		t.Errorf("Exists after it is written = (%v, %v), want (true, nil)", exists, err)
	}
}

// A path that cannot be looked at for a reason other than that nothing is there (here a directory component that is a
// file) is a failure that names the file, not "no registry": taking it for an absence would seed over it.
func TestExists_AFailureToLookIsNotAnAbsence(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeRaw(t, blocker, "a file where a directory should be")
	store := Store{Path: filepath.Join(blocker, "vaults.json")}

	exists, err := store.Exists()

	if exists || err == nil {
		t.Fatalf("Exists = (%v, %v), want (false, an error)", exists, err)
	}
	if want := "vaultreg: stat " + store.Path + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to begin %q", err, want)
	}
}

func TestLoad_ReadsTheRowsAndSaysWhereTheyCameFrom(t *testing.T) {
	store := storeIn(t)
	writeRaw(t, store.Path, `{"schema": 1, "vaults": {"a": {"path": "~/a", "seeded": true}, "b": {"path": "/b"}}}`)

	reg, err := store.Load()

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Origin != store.Path {
		t.Errorf("Origin = %q, want the path it was read from, %q", reg.Origin, store.Path)
	}
	if len(reg.Vaults) != 2 || reg.Vaults["a"] != (vaultreg.Entry{Path: "~/a", Seeded: true}) || reg.Vaults["b"] != (vaultreg.Entry{Path: "/b"}) {
		t.Errorf("Vaults = %+v, want the two rows as written", reg.Vaults)
	}
}

// A registry may carry fields this program does not know, and a schema it does not read: the rows are what it
// takes from it. One with no rows at all is an empty registry, not a missing one.
func TestLoad_IsTolerantOfWhatItDoesNotKnowAndTakesNoRowsForAnEmptyRegistry(t *testing.T) {
	store := storeIn(t)
	writeRaw(t, store.Path, `{"schema": 99, "note": "by hand", "vaults": {"a": {"path": "/a", "colour": "green"}}}`)
	if reg, err := store.Load(); err != nil || reg.Vaults["a"].Path != "/a" {
		t.Errorf("Load of a registry with fields it does not know = (%+v, %v), want its row", reg, err)
	}

	writeRaw(t, store.Path, `{"schema": 1}`)
	reg, err := store.Load()
	if err != nil || reg.Vaults == nil || len(reg.Vaults) != 0 {
		t.Errorf("Load of a registry with no rows = (%+v, %v), want an empty, usable one", reg, err)
	}
}

func TestLoad_AnAbsentRegistryIsNotExistAndSaysWhatItReadAndWhere(t *testing.T) {
	store := storeIn(t)

	_, err := store.Load()

	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load = %v, want an error that wraps fs.ErrNotExist", err)
	}
	if want := "vaultreg: read " + store.Path + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to begin %q", err, want)
	}
}

func TestLoad_AFileThatIsNotJSONIsRefusedWithItsName(t *testing.T) {
	store := storeIn(t)
	writeRaw(t, store.Path, "{ not json")

	_, err := store.Load()

	if err == nil {
		t.Fatal("Load = nil, want an error")
	}
	if want := "vaultreg: parse " + store.Path + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to begin %q", err, want)
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, which is taken for an absent registry", err)
	}
}

// The bytes of a registry are the ones the program has always written: two spaces of indentation, the schema
// first, the rows by name, a seeded row marked and any other not, and a newline at the end.
func TestSave_WritesTheRegistryAsItHasAlwaysBeenWritten(t *testing.T) {
	store := storeIn(t)

	err := store.Save(vaultreg.Registry{
		Vaults: map[string]vaultreg.Entry{"b-person": {Path: "/b"}, "a-seeded": {Path: "~/a", Seeded: true}},
		Origin: "somewhere else, which is not written",
	})

	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := "{\n  \"schema\": 1,\n  \"vaults\": {\n    \"a-seeded\": {\n      \"path\": \"~/a\",\n      \"seeded\": true\n    },\n    \"b-person\": {\n      \"path\": \"/b\"\n    }\n  }\n}\n"
	if got := readRaw(t, store.Path); got != want {
		t.Errorf("registry =\n%s\nwant\n%s", got, want)
	}
}

// The registry is per-user state: the file is owner-only and so is the directory made for it, which a fresh
// install does not have yet.
func TestSave_MakesTheRegistryAndItsDirectoryOwnerOnly(t *testing.T) {
	store := storeIn(t)

	if err := store.Save(vaultreg.Registry{Vaults: map[string]vaultreg.Entry{"p": {Path: "/p"}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	for path, want := range map[string]os.FileMode{store.Path: 0o600, filepath.Dir(store.Path): 0o700} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode of %s = %o, want %o", path, got, want)
		}
	}
}

func TestSave_ThenLoad_GivesBackTheRows(t *testing.T) {
	store := storeIn(t)
	rows := map[string]vaultreg.Entry{"a": {Path: "~/a", Seeded: true}, "b": {Path: "/b"}}

	if err := store.Save(vaultreg.Registry{Vaults: rows}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reg, err := store.Load()

	if err != nil || len(reg.Vaults) != 2 || reg.Vaults["a"] != rows["a"] || reg.Vaults["b"] != rows["b"] {
		t.Errorf("Load after Save = (%+v, %v), want the rows saved", reg, err)
	}
}

// A registry that cannot be written says so with the name of the file and of the package it belongs to; the words
// are those of durable.WriteJSON after the name of the package, as they were of the sequence it replaced.
func TestSave_NamesTheFileWhenItCannotBeWritten(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeRaw(t, blocker, "a file where a directory should be")
	store := Store{Path: filepath.Join(blocker, "vaults.json")}

	err := store.Save(vaultreg.Registry{})

	if err == nil {
		t.Fatal("Save under a file = nil, want an error")
	}
	if want := "vaultreg: create directory for " + store.Path + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to begin %q", err, want)
	}
}
