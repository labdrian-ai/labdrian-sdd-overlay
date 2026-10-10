package vaultlayout

import (
	"path/filepath"
	"testing"
)

// The paths are the vault's contract with the tools that share it (the wiki-ingest scripts, Obsidian,
// the people who read the files): a vault written by an earlier build must keep reading, so each one is
// pinned here as the literal it has always been, not derived from the constant under test.
func TestThePathsOfTheVaultAreTheOnesItAlwaysHad(t *testing.T) {
	for _, tc := range []struct {
		name, got, want string
	}{
		{"the directory of the promoted pages", PagesDir, "wiki/memory"},
		{"the catalog", IndexFile, "wiki/index.md"},
		{"the promotion log", LogFile, "wiki/log.md"},
		{"the address manifest", AddressManifestFile, ".raw/.manifest.json"},
		{"the precedence sidecar", PrecedenceFile, ".raw/.longterm-mem-manifest.json"},
		{"the sync state", SyncStateFile, ".vault-meta/longterm-mem-sync-state.json"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// A page lives under the pages directory, named by its address.
func TestAPageIsNamedByItsAddressUnderThePagesDirectory(t *testing.T) {
	if got, want := PageFile("c-000042"), "wiki/memory/c-000042.md"; got != want {
		t.Errorf("PageFile = %q, want %q", got, want)
	}
}

// Path joins a vault-relative path onto the root of the vault the way filepath.Join does, whichever
// separator the platform uses; a path the vault has not got stays a path inside the root.
func TestPathPlacesAVaultRelativePathUnderTheRoot(t *testing.T) {
	layout := Layout{Root: filepath.Join("vaults", "brain")}

	for _, tc := range []struct{ rel, want string }{
		{PrecedenceFile, filepath.Join("vaults", "brain", ".raw", ".longterm-mem-manifest.json")},
		{PageFile("c-000001"), filepath.Join("vaults", "brain", "wiki", "memory", "c-000001.md")},
		{PagesDir, filepath.Join("vaults", "brain", "wiki", "memory")},
	} {
		if got := layout.Path(tc.rel); got != tc.want {
			t.Errorf("Path(%q) = %q, want %q", tc.rel, got, tc.want)
		}
	}
}
