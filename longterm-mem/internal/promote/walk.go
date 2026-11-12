package promote

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// walkOutcome is a memory a walk ran a promotion for: the page it wrote, or the hand-edited page it left alone
// (Result.Action.Kind is ActionSkippedLocalEdit, and the Result carries the page that was not written).
type walkOutcome struct {
	ID     int64
	Result Result
}

// walkFailure is a memory a walk could not process. deciding says the failure is about the pages: the vault's pages
// could not be listed, or the page of the memory could not be read as one, as opposed to a page that could not be
// written for it or a memory that cannot be promoted at all (an id that is not a number, a store whose pages have no
// format), which say so in their own words. Only the first kind is worded "check promoted state" by the passes that
// have always worded it so.
type walkFailure struct {
	SyncFailure
	deciding bool
}

// walkReport is what one walk found: the memories it wrote pages for, and what it did with the others.
type walkReport struct {
	// written lists the memories a promotion ran for, in the walk's order.
	written []walkOutcome
	// skipped counts ineligible and already-current memories, neither of which costs a write; bridged the memories
	// that are the same piece of memory as one of another store that has a page.
	skipped, bridged int
	failed           []walkFailure
	// withheld lists the memories left alone because a page that could not be read may be theirs.
	withheld []int64
	// titles names the memories a dry run would write a page for, in the walk's order.
	titles []string
	// unreadable names every page of the vault the walk could not read and skipped.
	unreadable []UnreadablePage
}

// walk is the one walk of every promotion pass of every store: it reads the vault's pages once and decides each
// memory of memories with decide, the decision the explicit promote makes, so that a sync, its dry run and an
// explicit promote share one decision and one write path (Writer.apply).
//
// They differ in one input only, whether a page that is at its memory's revision is rewritten. A sync never
// rewrites it: walk calls decide with rewriteCurrent false. An explicit promote does for the stores whose
// pageStyle says so (pageStyle.rewriteCurrent, which Writer.Promote reads), as the explicit promote of an Engram
// observation always has.
//
// With apply the walk writes each memory the decision says needs a page (Writer.apply); without, it names the
// memory by title and writes nothing, so a dry run predicts the run. One memory that cannot be processed does not
// abort the walk: it is recorded in failed and the walk goes on, since one persistently broken memory would
// otherwise wedge the project's sync forever.
func (w *Writer) walk(ctx context.Context, memories []memory.Memory, apply bool) walkReport {
	var report walkReport
	index, indexErr := readPages(w.Pages)
	if indexErr == nil {
		report.unreadable = index.unreadable
	}
	for _, m := range memories {
		if !Eligible(m, false) {
			report.skipped++
			continue
		}
		id, err := numericID(m)
		style, styleErr := styleOf(m.Identity.Source)
		switch {
		case err != nil:
			// Not about the state of its page: the memory itself cannot be promoted, and says why in its own words.
			report.fail(id, err, false)
			continue
		case styleErr != nil:
			report.fail(id, styleErr, false)
			continue
		case indexErr != nil:
			report.fail(id, indexErr, true)
			continue
		}
		scan := index.scanFor(m)
		chosen, err := decide(m, scan, style, false)
		if err != nil {
			report.fail(id, err, true)
			continue
		}
		switch chosen {
		case decideBridged:
			report.bridged++
			continue
		case decideCurrent:
			report.skipped++
			continue
		case decideWithheld:
			report.withheld = append(report.withheld, id)
			continue
		}
		if !apply {
			report.titles = append(report.titles, m.Title)
			continue
		}
		result, err := w.apply(ctx, m, style, scan, chosen)
		if err != nil {
			report.fail(id, err, false)
			continue
		}
		if result.Action.Kind != ActionSkippedLocalEdit {
			// The memories after this one see the page this one has: a successor link names it.
			index.noteAddress(m.Project, m.Identity, result.Page.Address)
		}
		report.written = append(report.written, walkOutcome{ID: id, Result: result})
	}
	return report
}

func (r *walkReport) fail(id int64, err error, deciding bool) {
	r.failed = append(r.failed, walkFailure{SyncFailure: SyncFailure{ObservationID: id, Err: err}, deciding: deciding})
}
