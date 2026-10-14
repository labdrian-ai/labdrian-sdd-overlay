package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// cmdPromote implements `longterm-mem promote --project P --id <engram_id>
// [--vault DIR]` (R-032): explicitly promote one Engram observation
// through the same page-emission, addressing, and registration path any
// other eligible observation uses (promote.ExplicitPromote ->
// Writer.Promote's explicit=true override, R-007), regardless of its
// automatic eligibility. An invalid or nonexistent id is rejected with
// exit 7 (not_found), never a silent no-op, and a promotion the local-edit
// precedence rule refuses (R-030) exits 6 (registration_conflict) rather
// than reporting a page it deliberately did not write as promoted.
func (c commands) cmdPromote(args []string) int {
	// `promote reconcile <address>` is its own verb, dispatched before the
	// flag set below ever sees the arguments: it takes a page address, not
	// an --id, and its refusals are its own.
	if len(args) > 0 && args[0] == "reconcile" {
		return c.cmdPromoteReconcile(args[1:])
	}

	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", projectFlagUsage)
	id := fs.Int64("id", 0, "Engram observation id (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	a := c.newApp("promote")
	resolvedProject, exit := a.project(*project, false)
	if exit != exitOK {
		return exit
	}
	if *id <= 0 {
		fmt.Fprintln(os.Stderr, "longterm-mem: promote: --id is required")
		return exitUsage
	}
	vaultRoot, exit := a.vaultRoot(resolvedProject, *vaultDir)
	if exit != exitOK {
		return exit
	}
	store, exit := a.memoryToPromoteFrom()
	if exit != exitOK {
		return exit
	}
	defer store.Close()

	result, err := runPromote(context.Background(), store, vaultRoot, c.clock, *id)
	if err != nil {
		if errors.Is(err, promote.ErrObservationNotFound) {
			return a.refuse(err, exitNotFound)
		}
		return a.refuse(err, exitInternal)
	}
	return reportPromotion(result, *id)
}

// reportPromotion says what a promotion did and answers the exit code of it.
//
// A skip wrote nothing: it is a refusal, not a promotion, and must not
// wear either the word or the exit code of one. Exit 6
// (registration_conflict) is this binary's existing code for exactly
// this shape -- an artifact already occupies the target location,
// longterm-mem cannot prove it owns it, so it refuses and leaves the
// file byte-identical (cmd_register.go/cmd_unregister.go's own
// untagged-entry refusal). The diagnostic goes to stderr, where a
// caller reading stdout for the promoted address finds nothing to
// mistake for one.
//
// The same holds for an observation left alone because a page of the
// vault could not be read and no readable page is its own: it may
// already have a page, the unreadable one, so promotion made no second
// page for it. There is no page to name, so the observation is.
func reportPromotion(result promote.Result, observationID int64) int {
	if line, refused := refusalLine(result, observationID); refused {
		fmt.Println(line)
		if result.Action.Diagnostic != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: promote: %s\n", result.Action.Diagnostic.Detail)
		}
		return exitRegistrationConflict
	}

	fmt.Printf("longterm-mem: promoted %s (%s)\n", result.Page.Address, result.Action.Kind)
	return exitOK
}

// refusalLine is the line a promotion that wrote nothing prints on stdout, and whether it was a refusal at all.
// A refused edit names the page it left alone; an observation left alone for a page that could not be read has
// no page, so it names the observation.
func refusalLine(result promote.Result, observationID int64) (string, bool) {
	switch result.Action.Kind {
	case promote.ActionSkippedLocalEdit:
		return fmt.Sprintf("longterm-mem: refused %s (%s)", result.Page.Address, result.Action.Kind), true
	case promote.ActionSkippedUnreadablePages:
		return fmt.Sprintf("longterm-mem: refused observation %d (%s)", observationID, result.Action.Kind), true
	default:
		return "", false
	}
}

// cmdPromoteReconcile implements `longterm-mem promote reconcile --project P
// <address>`: adopt ONE named, already-promoted page into the precedence
// store, so future promotions of it proceed normally.
//
// It is the operator's exit from the state promotion deliberately refuses
// and `doctor` names: a page whose sidecar entry carries no evidence
// separating longterm-mem's own unrecorded write from a human's edit.
// Promotion's refusal there is a skip, and a skip suppresses the store
// write that would have repaired the entry, so without this command the
// page is refused identically on every run forever.
//
// THE ABSENCE OF A BULK FORM IS THE DESIGN. A human naming one address is
// precisely the consent the automatic path lacks -- which is why the
// automatic path refuses in the first place. `--all`, or any invocation
// naming more addresses than the operator wrote out, would reintroduce
// behind a flag the silent mass-adoption that ambiguity rules out, with the
// consent reduced to one keystroke covering pages nobody looked at. --all
// is therefore DECLARED here rather than left undefined, so reaching for it
// answers with that reason instead of "flag provided but not defined".
func (c commands) cmdPromoteReconcile(args []string) int {
	fs := flag.NewFlagSet("promote reconcile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", projectFlagUsage)
	vaultDir := fs.String("vault", "", "vault path override")
	all := fs.Bool("all", false, "refused: reconcile adopts exactly one explicitly named address")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *all {
		fmt.Fprintln(os.Stderr, "longterm-mem: promote reconcile: --all is refused: reconcile adopts exactly one address, named explicitly, because that naming is the consent the automatic promotion path lacks; adopting in bulk would overwrite pages nobody looked at")
		return exitUsage
	}
	if rest := fs.Args(); len(rest) != 1 {
		return refuseAddresses(rest)
	}
	a := c.newApp("promote reconcile")
	resolvedProject, exit := a.project(*project, false)
	if exit != exitOK {
		return exit
	}
	address := fs.Args()[0]

	vaultRoot, exit := a.vaultRoot(resolvedProject, *vaultDir)
	if exit != exitOK {
		return exit
	}

	vaultFiles := openVault(vaultRoot)
	outcome, err := promote.Reconcile(vaultFiles, resolvedProject, address, vaultFiles)
	if err != nil {
		return a.refuse(err, reconcileExitCode(err))
	}

	if !outcome.Adopted {
		// Deliberately exit 0. Reconcile is run off a doctor report, and a
		// page repaired between the report and the repair (by an ordinary
		// promotion, which is how the population of unrecorded entries
		// shrinks) must not fail a command whose work is already done.
		fmt.Printf("longterm-mem: %s already recorded at revision %d; nothing to reconcile\n", outcome.Address, outcome.PromotedRevision)
		return exitOK
	}
	fmt.Printf("longterm-mem: reconciled %s at revision %d\n", outcome.Address, outcome.PromotedRevision)
	return exitOK
}

// refuseAddresses refuses an invocation that did not name exactly one address.
func refuseAddresses(rest []string) int {
	// Go's flag package stops parsing at the first positional, so
	// `reconcile c-000701 --project P` leaves three positionals and
	// looks exactly like an attempted bulk adoption. Diagnosing it as
	// one tells the operator the invocation was refused on purpose and
	// never mentions the thing actually wrong, so the same command gets
	// retyped. The two mistakes are distinguishable: only the ordering
	// one leaves a flag-looking token among the positionals.
	for _, arg := range rest {
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "longterm-mem: promote reconcile: %s was read as an address, not a flag: flag parsing stops at the first positional, so every flag must come before the address (promote reconcile --project P <address>)\n", arg)
			return exitUsage
		}
	}
	fmt.Fprintf(os.Stderr, "longterm-mem: promote reconcile: expected exactly one address, got %d: reconcile adopts one page at a time on purpose\n", len(rest))
	return exitUsage
}

// reconcileExitCode is the exit code of a reconcile that was refused.
func reconcileExitCode(err error) int {
	switch {
	case errors.Is(err, promote.ErrPageNotFound):
		return exitNotFound
	case errors.Is(err, promote.ErrInvalidAddress):
		// A malformed address is a mistyped invocation, not a broken
		// vault: the operator has to fix what they typed.
		return exitUsage
	case errors.Is(err, promote.ErrNotThatPage):
		// Something occupies wiki/memory/<address>.md that is not that
		// page, and longterm-mem changed nothing -- the same shape the
		// promote path answers with when an artifact holds its target.
		return exitRegistrationConflict
	case errors.Is(err, promote.ErrUnusablePage):
		// Exit 6, not exit 1, and the choice is deliberate. Exit 1 is
		// this binary's residue for "longterm-mem's own side went
		// wrong" (exit_codes.go), and nothing went wrong on our side:
		// the command ran exactly as designed and found a file at the
		// target path it cannot read as a promoted page. That is the
		// shape exit 6 names -- an artifact occupies the target
		// location, longterm-mem cannot prove it owns it, so it
		// refuses and leaves the file byte-identical -- and it is the
		// code the neighbouring ErrNotThatPage refusal already
		// answers with for the same condition one field over.
		//
		// The alternatives were considered and rejected: exit 7
		// (not_found) is false, because the page IS there; exit 2
		// (usage) blames an invocation that was correct; exit 9 is
		// doctor's verdict code and belongs to doctor. The
		// distinction that matters to the operator is "your vault
		// holds an unusable page, which doctor already named" versus
		// "report a bug", and exit 1 said the second.
		return exitRegistrationConflict
	case errors.Is(err, promote.ErrLocalEditPreserved):
		// The same code the explicit promote path answers with when it
		// refuses a page it cannot prove it wrote: an artifact occupies
		// the target and longterm-mem changed nothing.
		return exitRegistrationConflict
	default:
		return exitInternal
	}
}
