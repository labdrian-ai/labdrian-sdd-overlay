package main

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"

// openVault is the vault file system adapter at vaultRoot, as the commands use it: the one place it is built.
// A failure of the adapter begins with what failed and names no consumer; the command that reports it puts
// its own name in front, once (the owner's decision of 2026-10-12: the adapter used to be asked for a
// "promote: " or "ops: " prefix, which the output printed doubled).
func openVault(vaultRoot string) *vaultfs.Vault {
	return vaultfs.New(vaultRoot)
}
