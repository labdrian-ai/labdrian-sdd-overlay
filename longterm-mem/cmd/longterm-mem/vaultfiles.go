package main

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"

// vaultErrorPrefix is what an operator has always read in front of a failure to read or write one of the vault's
// files: the commands' output is part of the interface, and it began with the name of the promotion that first
// touched those files.
const vaultErrorPrefix = "promote"

// openVault is the vault file system adapter at vaultRoot, as the commands use it. The adapter names no
// consumer; the prefix the output carries is decided here, once, for promote, sync, reconcile and the doctor.
func openVault(vaultRoot string) *vaultfs.Vault {
	return vaultfs.New(vaultRoot, vaultfs.WithErrorPrefix(vaultErrorPrefix))
}
