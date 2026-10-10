// Package vaultlayout is the one place that says where longterm-mem finds things inside a vault: the
// pages it promotes, the catalog and log it registers them in, and the files that keep its own state.
//
// The vault is shared with tools that are not this module (the wiki-ingest scripts, Obsidian, the
// people who read it), so these paths are a contract, not a choice of this module's: a vault written by
// an earlier build must keep reading. Promotion, the diagnostics and the file system adapter used to
// each carry their own copy of them; they read them from here.
//
// The package holds names and the joining of a name to a root, and does no I/O. Reading and writing the
// files is the vault file system adapter's.
package vaultlayout

import "path/filepath"

// The vault-relative paths, slash-separated whatever the platform: Layout.Path turns one into a path of
// the platform.
const (
	// PagesDir is the directory the promoted pages live in, one file per page named by its address.
	PagesDir = "wiki/memory"
	// IndexFile is the vault's master catalog, in which every promoted page is registered.
	IndexFile = "wiki/index.md"
	// LogFile is the vault's append-only promotion log.
	LogFile = "wiki/log.md"
	// AddressManifestFile is the wiki-ingest-owned manifest whose address_map this module extends.
	AddressManifestFile = ".raw/.manifest.json"
	// PrecedenceFile is longterm-mem's own sidecar: the fingerprint of what it last wrote for each page.
	PrecedenceFile = ".raw/.longterm-mem-manifest.json"
	// SyncStateFile records when the last sync completed.
	SyncStateFile = ".vault-meta/longterm-mem-sync-state.json"
)

// PageFile is the vault-relative path of the page promoted under address.
func PageFile(address string) string { return PagesDir + "/" + address + ".md" }

// Layout is one vault: the root its vault-relative paths are under.
type Layout struct {
	// Root is the directory of the vault.
	Root string
}

// Path is the path of rel, a vault-relative path of this package, in the vault Layout describes.
func (l Layout) Path(rel string) string { return filepath.Join(l.Root, rel) }
