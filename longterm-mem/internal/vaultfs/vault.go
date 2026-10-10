// Package vaultfs is the file system adapter of a vault: it reads and writes the files longterm-mem keeps
// there, behind the ports the consumers own (promote.PrecedenceRepository today; the address map, the pages
// and the sync state, log and index as the L3 slices that follow move behind theirs).
//
// The vault is the user's own Obsidian vault, shared with tools that are not this module, so what this package
// writes there is written the way a file of someone else is: replaced whole and atomically, its mode kept
// when it already exists, a symlink to it followed. Where the files are is vaultlayout's to say.
//
// Only the composition root names this package.
package vaultfs

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"

// Vault is the vault at one root. It satisfies the ports its consumers declare, and holds no state of its
// own beyond the root: every call reads or writes the files as they are.
type Vault struct {
	layout vaultlayout.Layout
}

// New is the Vault whose root is root. It touches no file; a root that is not a vault shows at the first read
// or write.
func New(root string) *Vault {
	return &Vault{layout: vaultlayout.Layout{Root: root}}
}
