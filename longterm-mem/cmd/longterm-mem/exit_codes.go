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
	// exitInternal: longterm-mem itself could not complete the work --
	// an unreadable or unparseable state file, an encode failure, an
	// observation it could not process.
	//
	// It is the code for "something on longterm-mem's own side went
	// wrong", NOT for "nothing you can do about it". This comment used to
	// say the operator could not fix it by changing configuration, which
	// contradicted the very next function in this file: vaultExitCode
	// deliberately routes a corrupt ~/.labdrian-overlay/vaults.json here,
	// and says so, and that file is configuration the operator owns and
	// repairs by hand. What actually distinguishes exit 1 is that it is
	// the residue: every failure that is not one of the individually
	// named, individually actionable conditions below (usage, no vault
	// configured, Engram unavailable, a vault script that failed, a
	// registration conflict, a missing observation, an unresolvable path)
	// lands here, and the operator has to read stderr to learn which.
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
	// exitPathUnresolvable: `register`/`unregister` could not resolve a
	// precondition from the environment at all -- no resolvable HOME, so
	// no install-state directory, no default binary path, or no config
	// root for the named runtime.
	//
	// It is deliberately NOT exitInternal. Exit 1 is "a target was
	// attempted and failed" -- a config that would not parse, a file that
	// could not be written. This is the opposite: nothing was attempted
	// and nothing was touched, and the fix is the caller's environment
	// (set HOME, or pass --state-dir/--binary/--config-root) rather than
	// the runtime's configuration. Sharing one code left a script unable
	// to tell "you forgot a flag" from "your ~/.claude.json is broken",
	// which are not the same event and do not have the same remedy.
	//
	// It lived in register_paths.go until this round, next to the rules
	// that raise it, with a doc comment that re-derived the whole contract
	// above from scratch to justify picking 8. That is precisely the
	// arrangement this file's own opening paragraph rules out: a reader
	// choosing the next code scans THIS list, and a code the list does not
	// show is a collision waiting to be written. Proximity to the raising
	// code is not worth a contract that is only complete in one file and
	// only true in another (exit_codes_source_test.go).
	exitPathUnresolvable = 8
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
//
// It probes BOTH tables longterm-mem reads, because the callers it serves
// read both: promote.Sync goes through observations, promote.Propagate
// goes through memory_relations (engram.Store.RelatedEdges), and the two
// report into one joined error value. Probing observations alone answered
// "readable" for a database whose relations table had been dropped or
// renamed out from under us -- exactly as unavailable, and classified exit
// 1 (internal, "longterm-mem's own side went wrong") instead of exit 4.
// One probe cannot stand for a corpus read through two tables.
//
// The relations probe passes an observation id no row can carry, so the
// query matches nothing and costs nothing; what is under test is whether
// the statement can be prepared and run at all.
func engramReadable(store *engram.Store, project string) bool {
	if _, err := store.ListObservations(project); err != nil {
		return false
	}
	if _, err := store.RelatedEdges(0); err != nil {
		return false
	}
	return true
}
