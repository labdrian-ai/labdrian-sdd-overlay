package promote

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The catalog, wiki/index.md, the log, wiki/log.md, and the record of the last sync are reached only through the
// ports the package owns (IndexRepository, LogRepository, SyncStateRecorder): the adapter keeps the files where
// the layout says and replaces them atomically; these tests hold the package to what it asks of the ports, with
// fakes that tell what they were asked and fail on demand.

var (
	errCatalogBoom = errors.New("boom")
	registerAt     = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
)

// registration returns the catalog, the log and the sync state recorder pagesWriter wired the Writer with.
func registration(w *Writer) (index, log *memFile, syncState *memSyncState) {
	return w.Index.(*memFile), w.Log.(*memFile), w.SyncState.(*memSyncState)
}

func TestRegisterIndexReadsAndReplacesTheCatalogThroughItsPort(t *testing.T) {
	t.Run("a catalog the vault does not have yet is an empty one", func(t *testing.T) {
		index := newAbsentMemFile()
		if err := RegisterIndex(index, testAddress, "Widget Decision"); err != nil {
			t.Fatalf("RegisterIndex: %v", err)
		}
		want := "<!-- longterm-mem:begin -->\n- [[c-000042|Widget Decision]]\n<!-- longterm-mem:end -->\n"
		if index.reads != 1 || len(index.writes) != 1 || index.writes[0] != want {
			t.Fatalf("reads = %d, writes = %q, want one read and exactly %q written", index.reads, index.writes, want)
		}
	})

	t.Run("what is there is kept around the block", func(t *testing.T) {
		index := newMemFile("# My index\n\nNotes by hand.\n")
		if err := RegisterIndex(index, testAddress, "Widget Decision"); err != nil {
			t.Fatalf("RegisterIndex: %v", err)
		}
		want := "# My index\n\nNotes by hand.\n\n<!-- longterm-mem:begin -->\n- [[c-000042|Widget Decision]]\n<!-- longterm-mem:end -->\n"
		if len(index.writes) != 1 || index.writes[0] != want {
			t.Fatalf("writes = %q, want exactly %q", index.writes, want)
		}
	})

	t.Run("a catalog that is not there is told by the error and not by the text", func(t *testing.T) {
		index := newMemFile("content that must not matter")
		index.readErr = fmt.Errorf("open wiki/index.md: %w", fs.ErrNotExist)
		if err := RegisterIndex(index, testAddress, "Widget Decision"); err != nil {
			t.Fatalf("RegisterIndex: %v", err)
		}
		if len(index.writes) != 1 || strings.Contains(index.writes[0], "must not matter") {
			t.Fatalf("writes = %q, want a catalog made from nothing", index.writes)
		}
	})
}

// Any other failure to read the catalog is the file system's own, worded around the file, and nothing is written.
func TestRegisterIndexNamesACatalogItCannotRead(t *testing.T) {
	index := newMemFile("")
	index.readErr = errCatalogBoom

	err := RegisterIndex(index, testAddress, "Widget Decision")

	if want := "promote: read " + index.location + ": boom"; err == nil || err.Error() != want {
		t.Fatalf("RegisterIndex error = %v, want %q", err, want)
	}
	if !errors.Is(err, errCatalogBoom) || len(index.writes) != 0 {
		t.Errorf("the cause is lost or the catalog was written: %v, %q", err, index.writes)
	}
}

// A marker block that was left open is refused and the catalog is not replaced.
func TestRegisterIndexRefusesAnOpenBlockWithoutWriting(t *testing.T) {
	index := newMemFile("<!-- longterm-mem:begin -->\n- [[c-000001|Old]]\n")

	err := RegisterIndex(index, testAddress, "Widget Decision")

	if err == nil || !strings.Contains(err.Error(), "malformed longterm-mem marker block") || len(index.writes) != 0 {
		t.Fatalf("RegisterIndex = %v after writes %q, want the refusal and no write", err, index.writes)
	}
}

// The failure to replace the catalog is the port's, untouched.
func TestRegisterIndexReturnsTheFailureToReplaceTheCatalogUntouched(t *testing.T) {
	index := newAbsentMemFile()
	index.writeErr = errCatalogBoom

	if err := RegisterIndex(index, testAddress, "Widget Decision"); err != errCatalogBoom {
		t.Fatalf("RegisterIndex error = %v, want the port's own", err)
	}
}

// A repository that was not wired is named, and nothing is called.
func TestRegisterIndexAndLogNameARepositoryThatWasNotWired(t *testing.T) {
	var typedNilIndex, typedNilLog *memFile
	for name, index := range map[string]IndexRepository{"absent": nil, "a nil pointer": typedNilIndex} {
		if err := RegisterIndex(index, testAddress, "x"); !errors.Is(err, errNoIndexRepository) {
			t.Errorf("RegisterIndex with %s = %v, want errNoIndexRepository", name, err)
		}
	}
	for name, log := range map[string]LogRepository{"absent": nil, "a nil pointer": typedNilLog} {
		if err := RegisterLog(log, testAddress, "x", registerAt); !errors.Is(err, errNoLogRepository) {
			t.Errorf("RegisterLog with %s = %v, want errNoLogRepository", name, err)
		}
	}
}

func TestRegisterLogReadsAndReplacesTheLogThroughItsPort(t *testing.T) {
	t.Run("a log the vault does not have yet is an empty one", func(t *testing.T) {
		log := newAbsentMemFile()
		if err := RegisterLog(log, testAddress, "Widget Decision", registerAt); err != nil {
			t.Fatalf("RegisterLog: %v", err)
		}
		want := "## [2026-10-10] promote | Widget Decision\n\n[[c-000042|Widget Decision]]\n\n"
		if log.reads != 1 || len(log.writes) != 1 || log.writes[0] != want {
			t.Fatalf("reads = %d, writes = %q, want one read and exactly %q written", log.reads, log.writes, want)
		}
	})

	t.Run("the entry goes before the first header on or before its date", func(t *testing.T) {
		log := newMemFile("## [2027-01-01] ingest | Later\n\nl\n\n## [2025-01-01] ingest | Earlier\n\ne\n")
		if err := RegisterLog(log, testAddress, "Widget Decision", registerAt); err != nil {
			t.Fatalf("RegisterLog: %v", err)
		}
		want := "## [2027-01-01] ingest | Later\n\nl\n\n## [2026-10-10] promote | Widget Decision\n\n[[c-000042|Widget Decision]]\n\n## [2025-01-01] ingest | Earlier\n\ne\n"
		if len(log.writes) != 1 || log.writes[0] != want {
			t.Fatalf("writes = %q, want exactly %q", log.writes, want)
		}
	})

	t.Run("a log that cannot be read is named and not written", func(t *testing.T) {
		log := newMemFile("")
		log.readErr = errCatalogBoom
		err := RegisterLog(log, testAddress, "Widget Decision", registerAt)
		if want := "promote: read " + log.location + ": boom"; err == nil || err.Error() != want || len(log.writes) != 0 {
			t.Fatalf("RegisterLog = %v after writes %q, want %q and no write", err, log.writes, want)
		}
	})

	t.Run("the failure to replace the log is the port's, untouched", func(t *testing.T) {
		log := newAbsentMemFile()
		log.writeErr = errCatalogBoom
		if err := RegisterLog(log, testAddress, "Widget Decision", registerAt); err != errCatalogBoom {
			t.Fatalf("RegisterLog error = %v, want the port's own", err)
		}
	})
}

// A promotion registers its page in the catalog and then in the log, through the ports, dated with its clock.
func TestPromoteRegistersThePageInTheCatalogAndTheLogThroughTheirPorts(t *testing.T) {
	w, _, _ := pagesWriter(t, newMemPages(nil))
	index, log, _ := registration(w)

	if _, err := w.Promote(t.Context(), pageObservation(701), false); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if len(index.writes) != 1 || !strings.Contains(index.writes[0], "[[c-000042|Through The Port]]") {
		t.Errorf("the catalog was written %q, want one write naming the page", index.writes)
	}
	if len(log.writes) != 1 || !strings.Contains(log.writes[0], "## [2026-08-01] promote | Through The Port") {
		t.Errorf("the log was written %q, want one write dated with the clock", log.writes)
	}
}

// The log is dated with the day in UTC, whatever zone the clock tells its instant in: an instant late in the
// evening west of Greenwich is already the next day there.
func TestPromoteDatesTheLogEntryWithTheDayInUTC(t *testing.T) {
	w, _, _ := pagesWriter(t, newMemPages(nil))
	_, log, _ := registration(w)
	w.Clock = &fakeClock{at: time.Date(2026, 10, 10, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60))}

	if _, err := w.Promote(t.Context(), pageObservation(709), false); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if len(log.writes) != 1 || !strings.HasPrefix(log.writes[0], "## [2026-10-11] promote | ") {
		t.Errorf("the log was written %q, want an entry dated 2026-10-11, the day in UTC", log.writes)
	}
}

// A page is registered in the catalog before the log; a catalog that cannot be written stops the registration,
// and the page, which is written and durable by then, stays: the doctor names it unregistered.
func TestPromoteReportsACatalogItCannotWriteAndLeavesThePage(t *testing.T) {
	pages := newMemPages(nil)
	w, _, _ := pagesWriter(t, pages)
	index, log, _ := registration(w)
	index.writeErr = errCatalogBoom

	_, err := w.Promote(t.Context(), pageObservation(702), false)

	if err != errCatalogBoom {
		t.Fatalf("Promote error = %v, want the port's own", err)
	}
	if len(log.writes) != 0 {
		t.Errorf("the log was written %q although the catalog was not", log.writes)
	}
	if len(pages.writes) != 1 {
		t.Errorf("the page was written %d times, want it kept", len(pages.writes))
	}
}

func TestPromoteReportsALogItCannotWrite(t *testing.T) {
	w, _, _ := pagesWriter(t, newMemPages(nil))
	_, log, _ := registration(w)
	log.writeErr = errCatalogBoom

	if _, err := w.Promote(t.Context(), pageObservation(703), false); err != errCatalogBoom {
		t.Fatalf("Promote error = %v, want the port's own", err)
	}
}

// A page that an update skipped because of a local edit is registered nowhere: nothing was written for it.
func TestPromoteRegistersNothingForAnIneligibleObservation(t *testing.T) {
	w, _, _ := pagesWriter(t, newMemPages(nil))
	index, log, _ := registration(w)
	process := pageObservation(704)
	process.TopicKey = "sdd/some-change/progress"

	if _, err := w.Promote(t.Context(), process, false); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if index.reads+len(index.writes)+log.reads+len(log.writes) != 0 {
		t.Errorf("the catalog and the log were used (%d reads, %d writes, %d reads, %d writes) for an observation nothing is written for", index.reads, len(index.writes), log.reads, len(log.writes))
	}
}

// A Writer built without its catalog or its log is refused before anything is written and before an address is
// spent, and says which port it lacks; the ports are asked for in the order the Writer lists them, after the
// ones a patch needs. A patch alone (Propagate) registers nothing and asks for neither.
func TestWriterWithoutACatalogOrALogIsRefusedBeforeAnythingIsDone(t *testing.T) {
	for _, tc := range []struct {
		name      string
		unwire    func(w *Writer)
		want      error
		wantNames string
	}{
		{"no catalog", func(w *Writer) { w.Index = nil }, errNoIndexRepository, "no index repository"},
		{"a nil catalog", func(w *Writer) { var nilIndex *memFile; w.Index = nilIndex }, errNoIndexRepository, "no index repository"},
		{"no log", func(w *Writer) { w.Log = nil }, errNoLogRepository, "no log repository"},
		{"neither: the catalog first", func(w *Writer) { w.Index, w.Log = nil, nil }, errNoIndexRepository, "no index repository"},
		{"no pages: before the catalog", func(w *Writer) { w.Index, w.Pages = nil, nil }, errNoPageRepository, "no page repository"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, repo, allocator := pagesWriter(t, newMemPages(nil))
			tc.unwire(w)

			_, err := w.Promote(t.Context(), pageObservation(705), false)
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.wantNames) {
				t.Errorf("Promote error = %v, want %v", err, tc.want)
			}
			if allocator.asked != 0 || repo.saves != 0 {
				t.Errorf("allocator asked %d times, precedence saved %d times, want neither", allocator.asked, repo.saves)
			}
			mem := &fakeMemory{}
			if _, err := Sync(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject); !errors.Is(err, tc.want) {
				t.Errorf("Sync error = %v, want %v", err, tc.want)
			}
			if len(mem.listed) != 0 {
				t.Errorf("Sync listed the observations of %v before it was refused", mem.listed)
			}
		})
	}

	t.Run("a patch asks for neither", func(t *testing.T) {
		w, _, _ := pagesWriter(t, newMemPages(nil))
		w.Index, w.Log, w.SyncState = nil, nil, nil
		if _, err := Propagate(t.Context(), Deps{Memory: &fakeMemory{}, Writer: w}, pagesProject); err != nil {
			t.Errorf("Propagate = %v, want it to run without a catalog, a log or a sync state", err)
		}
	})
}

// Sync records its completion through the port, in UTC, after the index is rebuilt and not before.
func TestSyncRecordsItsCompletionThroughThePort(t *testing.T) {
	t.Run("in UTC, whatever zone the clock is in", func(t *testing.T) {
		w, _, _ := pagesWriter(t, newMemPages(nil))
		_, _, syncState := registration(w)
		w.Clock = &fakeClock{at: time.Date(2026, 10, 10, 14, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))}

		if _, err := Sync(t.Context(), Deps{Memory: &fakeMemory{}, Writer: w}, pagesProject); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if want := []time.Time{registerAt}; len(syncState.recorded) != 1 || !syncState.recorded[0].Equal(want[0]) || syncState.recorded[0].Location() != time.UTC {
			t.Errorf("recorded = %v, want exactly %v, in UTC", syncState.recorded, want)
		}
	})

	t.Run("after the rebuild", func(t *testing.T) {
		w, _, _ := pagesWriter(t, newMemPages(nil))
		_, _, syncState := registration(w)
		var seenAtRebuild int
		deps := Deps{Memory: &fakeMemory{}, Writer: w, Index: IndexRebuilderFunc(func(context.Context) error { seenAtRebuild = len(syncState.recorded); return nil })}

		if _, err := Sync(t.Context(), deps, pagesProject); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if seenAtRebuild != 0 || len(syncState.recorded) != 1 {
			t.Errorf("%d records at the rebuild and %d after, want none and one", seenAtRebuild, len(syncState.recorded))
		}
	})

	t.Run("not when the rebuild failed", func(t *testing.T) {
		w, _, _ := pagesWriter(t, newMemPages(nil))
		_, _, syncState := registration(w)
		deps := Deps{Memory: &fakeMemory{}, Writer: w, Index: IndexRebuilderFunc(func(context.Context) error { return errCatalogBoom })}

		_, err := Sync(t.Context(), deps, pagesProject)

		if !errors.Is(err, errCatalogBoom) || len(syncState.recorded) != 0 {
			t.Errorf("Sync = %v after records %v, want the rebuild's failure and no record", err, syncState.recorded)
		}
	})

	t.Run("a record that cannot be made is the run's failure, in the words it has always had", func(t *testing.T) {
		w, _, _ := pagesWriter(t, newMemPages(nil))
		_, _, syncState := registration(w)
		syncState.err = errCatalogBoom

		report, err := Sync(t.Context(), Deps{Memory: &fakeMemory{live: []memory.Observation{pageObservation(706)}}, Writer: w}, pagesProject)

		if err == nil || err.Error() != "promote: sync: boom" || !errors.Is(err, errCatalogBoom) {
			t.Errorf("Sync error = %v, want %q", err, "promote: sync: boom")
		}
		if len(report.Promoted) != 1 {
			t.Errorf("report = %+v, want what was promoted reported beside the failure", report)
		}
	})
}

// A Writer without the recorder is refused by Sync before it lists a single observation; a promotion does not
// ask for it, and a plan, which writes nothing, never records.
func TestSyncWithoutARecorderIsRefusedBeforeAnythingIsDone(t *testing.T) {
	var typedNil *memSyncState
	for name, recorder := range map[string]SyncStateRecorder{"absent": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			w, repo, _ := pagesWriter(t, newMemPages(nil))
			w.SyncState = recorder
			mem := &fakeMemory{live: []memory.Observation{pageObservation(707)}}

			_, err := Sync(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)

			if !errors.Is(err, errNoSyncStateRecorder) || err.Error() != "promote: sync: promote: the writer has no sync state recorder" {
				t.Errorf("Sync error = %v, want errNoSyncStateRecorder behind the sync prefix", err)
			}
			if len(mem.listed) != 0 || repo.saves != 0 {
				t.Errorf("listed %v, saved %d times, want nothing done", mem.listed, repo.saves)
			}
			if _, err := w.Promote(t.Context(), pageObservation(707), false); err != nil {
				t.Errorf("Promote = %v, want a promotion not to ask for the recorder", err)
			}
		})
	}
}

func TestAPlanNeverRecordsASync(t *testing.T) {
	w, _, _ := pagesWriter(t, newMemPages(nil))
	index, log, syncState := registration(w)

	if _, err := Plan(t.Context(), Deps{Memory: &fakeMemory{live: []memory.Observation{pageObservation(708)}}, Writer: w}, pagesProject); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(syncState.recorded)+len(index.writes)+len(log.writes) != 0 {
		t.Errorf("the plan recorded %v and wrote %q, %q, want nothing", syncState.recorded, index.writes, log.writes)
	}
}
