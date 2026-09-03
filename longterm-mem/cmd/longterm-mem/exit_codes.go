package main

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// The exit codes below are this binary's published contract (design.md
// "Contracts"). They live in one place because their whole value is that a
// caller can act on them without reading stderr: the moment two unrelated
// failures share a code, or one failure is spelled as a bare literal in
// six subcommands, the code stops being a contract and becomes a number
// that happens to be non-zero.
const (
	// exitOK: the command did what it was asked to do.
	exitOK = 0
	// exitInternal: longterm-mem itself failed -- an unreadable state
	// file, an encode failure, an observation it could not process. Not
	// something the operator can fix by changing configuration.
	exitInternal = 1
	// exitUsage: the invocation is wrong (bad flag, missing --project).
	exitUsage = 2
	// exitVaultNotConfigured: the named project has no vault. Reserved
	// for vaultreg.ErrVaultNotConfigured -- see vaultExitCode.
	exitVaultNotConfigured = 3
	// exitEngramUnavailable: Engram's database could not be opened or
	// read.
	exitEngramUnavailable = 4
	// exitVaultSubprocessFailed: one of the vault's own scripts failed.
	exitVaultSubprocessFailed = 5
	// exitRegistrationConflict: an artifact already occupies the target
	// location and longterm-mem cannot prove it wrote it, so it refused
	// and changed nothing.
	exitRegistrationConflict = 6
	// exitNotFound: the named observation does not exist.
	exitNotFound = 7
)

// vaultExitCode maps a vaultreg.Resolve failure onto that contract.
//
// Resolve fails for two unrelated reasons, and only one of them is exit
// 3's. A project with no registry row is genuinely vault_not_configured:
// the operator fixes it by adding a row. A registry file that cannot be
// seeded, read, parsed, or expanded is a longterm-mem-side failure -- the
// row may well be sitting right there in the file -- and reporting it as
// vault_not_configured sends the operator to add an entry that already
// exists while the real cause (a corrupt ~/.labdrian-overlay/vaults.json)
// stays invisible. Every one of those is exitInternal.
func vaultExitCode(err error) int {
	if errors.Is(err, vaultreg.ErrVaultNotConfigured) {
		return exitVaultNotConfigured
	}
	return exitInternal
}

// engramReadable reports whether store can still answer a project-scoped
// listing.
//
// It exists for one job: telling exitEngramUnavailable from exitInternal
// on a failure path, when the failing call is buried inside a package that
// reports its per-item and its fatal errors through one joined error value
// (promote.Sync/promote.Propagate). Matching on error text would bind the
// exit code to a message string; re-probing asks the question the exit
// code actually means -- "is Engram unavailable?" -- and answers it from
// the database itself. It runs only after a failure, so a healthy run pays
// nothing for it.
func engramReadable(store *engram.Store, project string) bool {
	_, err := store.ListObservations(project)
	return err == nil
}
