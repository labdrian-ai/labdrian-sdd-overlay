// Package vaultfs is the file system adapter of a vault: it reads and writes the files longterm-mem keeps
// there, behind the ports the consumers own: the precedence store, the address map, the pages, the catalog, the
// log, the record of the last sync and the question of what is at the vault root. It is the one place the
// promotion and the diagnostics' vault files are read and written from; the vault's own index scripts
// (internal/vault) keep their markers and their sentinel, the files of the scripts it runs, and nothing else.
//
// The vault is the user's own Obsidian vault, shared with tools that are not this module, so what this package
// writes there is written the way a file of someone else is: replaced whole and atomically, its mode kept
// when it already exists, a symlink to it followed. Where the files are is vaultlayout's to say.
//
// Only the composition root names this package.
package vaultfs

import (
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// Vault is the vault at one root. It satisfies the ports its consumers declare, and holds no state of its
// own beyond the root: every call reads or writes the files as they are.
type Vault struct {
	layout      vaultlayout.Layout
	errorPrefix string
}

// New is the Vault whose root is root. It touches no file; a root that is not a vault shows at the first read
// or write.
func New(root string, options ...Option) *Vault {
	v := &Vault{layout: vaultlayout.Layout{Root: root}}
	for _, option := range options {
		option(v)
	}
	return v
}

// Option adjusts a Vault at its construction.
type Option func(*Vault)

// WithErrorPrefix makes every error the Vault returns begin with prefix and ": ", wrapping the failure it
// reports. The adapter names no consumer on its own: the prefix a command's output has always carried is for the
// composition root to ask for, so the text an operator reads is decided where the commands are.
func WithErrorPrefix(prefix string) Option {
	return func(v *Vault) { v.errorPrefix = prefix }
}

// RootIsDirectory reports whether the root of the vault, following a link, is a directory. A root that cannot be
// examined (it is not there, it is a link to nothing, a path of it goes through a file) is the file system's own
// error, naming the root and carrying no prefix, and no answer.
func (v *Vault) RootIsDirectory() (bool, error) {
	info, err := os.Stat(v.layout.Root)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// fail is err with the Vault's error prefix in front, if it has one.
func (v *Vault) fail(err error) error {
	if v.errorPrefix == "" {
		return err
	}
	return fmt.Errorf("%s: %w", v.errorPrefix, err)
}
