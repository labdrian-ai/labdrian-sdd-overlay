package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/identityledger"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// projectFlagUsage is the shared --project flag description. The flag is
// optional now: omitted, the project is resolved from the working
// directory (internal/projectid).
const projectFlagUsage = "project name (default: resolved from the working directory)"

// resolveProjectFlag turns a possibly-empty --project into the project a
// command should act on, and returns exitOK or the exit code the caller
// must return.
//
// Two behaviours meet here.
//
// When --project is omitted, the project is resolved from the working
// directory through projectid's chain (declared file, normalized origin
// remote, realpath of the git common dir). When that resolution fails, the
// command refuses exactly as it did before -- an unresolved project must
// never become an empty one, because Engram is queried
// `WHERE project = ?` with whatever it is handed -- but the refusal now
// says why resolution failed instead of only restating the flag's name.
//
// When --project IS given and the working directory is inside a git
// repository whose canonical identity does not correspond to it, the
// mismatch is reported and the command proceeds. That is the moment a
// fragmenting call is detectable, and the argument for warning rather than
// refusing is written out in projectid.Correspondence.Warning: the failure
// being prevented is a silent one, and an operator naming another project
// on purpose is legitimate and must stay possible.
func resolveProjectFlag(cmd, given string) (string, int) {
	if given == "" {
		adoption, notes, err := adoptFromWorkingDirectory()
		if err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project is required: it could not be resolved from the working directory: %v\n", cmd, err)
			return "", exitUsage
		}
		reportAdoption(cmd, adoption, notes)
		return adoption.Identity.Project, exitOK
	}

	wd, err := os.Getwd()
	if err != nil {
		// The correspondence check is a diagnostic, never a gate: an
		// unreadable working directory must not turn an explicit
		// --project into a failure.
		return given, exitOK
	}
	c, err := projectid.CheckCorrespondence(wd, given)
	if err != nil {
		if !errors.Is(err, projectid.ErrNotARepository) {
			fmt.Fprintf(os.Stderr, "WARN %s: could not check --project against the working directory: %v\n", cmd, err)
		}
		return given, exitOK
	}
	if w := c.Warning(); w != "" {
		fmt.Fprintf(os.Stderr, "WARN %s: %s\n", cmd, w)
	}
	return given, exitOK
}

// reportAdoption tells the operator which project the command is acting on
// and, when it applies, what integrating that identity still owes.
func reportAdoption(cmd string, a projectid.Adoption, notes []string) {
	if a.Adopted {
		fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project not given, adopted %q from the working directory (the memory already lives under this name; derived via the %s rule)\n", cmd, a.Identity.Project, a.Identity.Rule)
	} else {
		fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project not given, resolved %q from the working directory (via the %s rule)\n", cmd, a.Identity.Project, a.Identity.Rule)
	}

	if len(a.PendingIntegration) > 0 {
		// Provable, not guessed: every name here came out of THIS
		// repository's own metadata in a single read, so they are the same
		// repository beyond argument. What is not ours is the merging --
		// R-002 keeps this module's Engram connection read-only -- so the
		// operator gets the finding and the remedy rather than a silent
		// half-measure or a write we are not entitled to make.
		fmt.Fprintf(os.Stderr, "WARN %s: %s also hold memory and are the same repository as %q; they are owed an integration into it. longterm-mem reads Engram read-only and must not merge it: run `engram projects consolidate` to fold them in.\n",
			cmd, quoteAll(a.PendingIntegration), a.Identity.Project)
	}

	for _, note := range notes {
		// An unconsulted store is not an empty one. Saying so is the whole
		// point: silence here would be indistinguishable from "this really
		// is a fresh repository", which is the answer that mints the second
		// pile.
		fmt.Fprintf(os.Stderr, "WARN %s: %s, so an identity the memory already uses may have been missed\n", cmd, note)
	}
}

func quoteAll(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("%q", n))
	}
	return strings.Join(quoted, ", ")
}

// adoptFromWorkingDirectory resolves the working directory's identity and
// lets what is already STORED decide which of the derivable names is used.
//
// Adoption runs only on this path, where longterm-mem is choosing the name
// itself. An explicit --project is the operator choosing it, and there the
// existing correspondence warning already says when that choice disagrees
// with the directory.
//
// This seam exists for the CLI and only for the CLI. Do NOT reach for it
// from internal/mcpserver to default or validate an MCP tool's project
// field: the MCP server is launched by a runtime (an editor, an agent host)
// whose working directory has no relationship to the project a given call
// is asking about, so adopting from its cwd would bind observations to
// wherever the host happened to start -- the exact misattribution this
// whole mechanism exists to prevent.
func adoptFromWorkingDirectory() (projectid.Adoption, []string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return projectid.Adoption{}, nil, fmt.Errorf("reading the working directory: %w", err)
	}

	stores, notes := openEstablishedStores()
	defer stores.close()

	// The ledger is consulted BEFORE resolving and written AFTER, because
	// the two answer different halves of one question: what this repository
	// was called before, and what it is called now.
	remembered, commonDir, ledgerNotes := rememberedNames(wd)
	notes = append(notes, ledgerNotes...)

	a, err := projectid.AdoptWith(wd, projectid.AdoptOptions{
		Established: stores.lookup,
		Remembered:  remembered,
	})
	if err != nil {
		return projectid.Adoption{}, notes, err
	}

	notes = append(notes, recordDerivedNames(commonDir, a.Derived)...)
	return a, notes, nil
}

// rememberedNames reads the names this repository was known by from its
// ledger, and returns the common directory the ledger lives under so the
// caller can write back to the same place.
//
// Only the STRICT names are offered for adoption. A ledger's bare directory
// name can equally name somebody else's repository, and adopting it would
// bind this one to another project's memory -- see projectid.DerivedName's
// Strict field for why the visible failure is the better trade.
func rememberedNames(wd string) ([]string, string, []string) {
	commonDir, err := projectid.CommonDir(wd)
	if err != nil {
		// Not a repository, or unreadable metadata. AdoptWith is about to
		// report that far more precisely than this path could.
		return nil, "", nil
	}

	entries, err := identityledger.Names(commonDir)
	if err != nil {
		return nil, commonDir, []string{fmt.Sprintf("this repository's identity ledger could not be read (%v)", err)}
	}

	var remembered []string
	for _, e := range entries {
		if e.Adoptable {
			remembered = append(remembered, e.Name)
		}
	}
	return remembered, commonDir, nil
}

// recordDerivedNames writes today's derived names into the repository's
// ledger, so a name stays findable after the repository stops deriving it.
//
// A ledger that cannot be written is a note, never a failure: the command
// the operator actually asked for has nothing to do with it, and refusing
// to run because a record could not be kept would be its own kind of
// memory loss.
func recordDerivedNames(commonDir string, derived []projectid.DerivedName) []string {
	if commonDir == "" || len(derived) == 0 {
		return nil
	}
	names := make([]identityledger.Name, 0, len(derived))
	for _, d := range derived {
		names = append(names, identityledger.Name{
			Name:      d.Name,
			Rule:      string(d.Rule),
			Adoptable: d.Strict,
		})
	}
	if err := identityledger.Record(commonDir, names, time.Now().UTC()); err != nil {
		return []string{fmt.Sprintf("this repository's identity ledger could not be written (%v)", err)}
	}
	return nil
}

// establishedStores answers "does memory already live under this name?"
// from the two stores longterm-mem is allowed to consult: its own vault
// registry, and Engram's database, read-only.
type establishedStores struct {
	vaults map[string]bool
	store  *engram.Store
}

// openEstablishedStores opens both, returning a note for each one that
// could not be consulted rather than an error. A store that cannot be read
// must not stop the command -- most subcommands never touch Engram -- but
// it must not read as "nothing is established" in silence either, so the
// caller reports every note it gets back.
func openEstablishedStores() (*establishedStores, []string) {
	s := &establishedStores{vaults: map[string]bool{}}
	var notes []string

	// Load, never Seed: consulting the registry must not create it. A
	// missing registry is simply a machine where nothing is configured yet.
	if reg, err := vaultreg.Load(defaultVaultsPath()); err == nil {
		for name := range reg.Vaults {
			s.vaults[name] = true
		}
	} else if !os.IsNotExist(errors.Unwrap(err)) {
		notes = append(notes, fmt.Sprintf("the vault registry could not be read (%v)", err))
	}

	if store, err := engram.Open(os.Getenv(engramDBEnvVar)); err == nil {
		s.store = store
	} else {
		notes = append(notes, fmt.Sprintf("Engram's database could not be opened (%v)", err))
	}
	return s, notes
}

func (s *establishedStores) lookup(name string) (bool, error) {
	if s.vaults[name] {
		return true, nil
	}
	if s.store == nil {
		return false, nil
	}
	return s.store.HasMemory(name)
}

func (s *establishedStores) close() {
	if s.store != nil {
		s.store.Close()
	}
}
