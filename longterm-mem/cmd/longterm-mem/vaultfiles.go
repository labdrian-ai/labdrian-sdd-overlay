package main

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"

// vaultErrorPrefix is what an operator has always read in front of a failure to read or write one of the vault's
// files: the commands' output is part of the interface, and it began with the name of the promotion that first
// touched those files.
const vaultErrorPrefix = "promote"

// statusErrorPrefix is what `status` has always put in front of a failure to read the record of the last sync:
// the status report is the operability surface's, not the promotion's.
const statusErrorPrefix = "ops"

// openVault is the vault file system adapter at vaultRoot, as the commands use it. The adapter names no
// consumer; the prefix the output carries is decided here, once, for promote, sync, reconcile and the doctor.
func openVault(vaultRoot string) *vaultfs.Vault {
	return vaultfs.New(vaultRoot, vaultfs.WithErrorPrefix(vaultErrorPrefix))
}

// openVaultForStatus is the same adapter, as `status` uses it: the prefix of its failures is the report's own.
func openVaultForStatus(vaultRoot string) *vaultfs.Vault {
	return vaultfs.New(vaultRoot, vaultfs.WithErrorPrefix(statusErrorPrefix))
}
