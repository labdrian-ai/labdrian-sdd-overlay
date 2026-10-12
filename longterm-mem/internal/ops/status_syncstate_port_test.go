package ops

import (
	"context"
	"errors"
	"testing"
)

// statusOver runs Status with the sync state reader it is given and nothing else of the vault: the reader alone
// says what the last sync was.
func statusOver(t *testing.T, syncState SyncStateReader) (StatusReport, error) {
	t.Helper()
	return Status(context.Background(), StatusDeps{
		VaultRoot:        t.TempDir(),
		SyncState:        syncState,
		StateDir:         t.TempDir(),
		EngramReachable:  func(context.Context) (bool, string) { return true, "" },
		VaultProvisioned: func(string) bool { return true },
	}, "labdrian-sdd-overlay")
}

// Status reports the completion the reader answers, as the reader gives it, and asks once. It does not read the
// instant: an answer that is not an instant is still the record's.
func TestStatusReportsTheLastSyncTheReaderAnswers(t *testing.T) {
	for _, completedAt := range []string{"2026-08-31T12:00:00Z", "2026-10-10T14:00:00+02:00", "yesterday"} {
		reader := &fakeSyncState{completedAt: completedAt}
		report, err := statusOver(t, reader)
		if err != nil || report.LastSyncCompletedAt != completedAt {
			t.Errorf("Status = (%+v, %v), want the last sync %q", report, err, completedAt)
		}
		if reader.reads != 1 {
			t.Errorf("the record was read %d times, want once", reader.reads)
		}
	}
}

// Nothing recorded is "never", never a made-up time.
func TestStatusReportsNeverWhenTheReaderHasNoCompletion(t *testing.T) {
	report, err := statusOver(t, &fakeSyncState{})
	if err != nil || report.LastSyncCompletedAt != "never" {
		t.Fatalf("Status = (%+v, %v), want the last sync \"never\"", report, err)
	}
}

// A reader that cannot read or understand the record fails Status with the reason it gives, untouched, and no
// field of a report carries a value derived from an unreadable record.
func TestStatusFailsWithTheReasonTheReaderGives(t *testing.T) {
	cause := errors.New("parse /vault/.vault-meta/longterm-mem-sync-state.json: unexpected end of JSON input")

	report, err := statusOver(t, &fakeSyncState{err: cause})

	if !errors.Is(err, cause) || err.Error() != cause.Error() {
		t.Fatalf("error = %v, want the reader's own, untouched", err)
	}
	if report != (StatusReport{}) {
		t.Errorf("report = %+v, want the zero value", report)
	}
}

// A reader that was not wired is named, rather than called.
func TestStatusNamesASyncStateReaderThatWasNotWired(t *testing.T) {
	var typedNil *fakeSyncState
	for name, reader := range map[string]SyncStateReader{"no reader": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			report, err := statusOver(t, reader)
			if err == nil || err.Error() != "ops: no sync state reader was wired" {
				t.Fatalf("Status = (%+v, %v), want the missing reader named", report, err)
			}
			if report != (StatusReport{}) {
				t.Errorf("report = %+v, want the zero value", report)
			}
		})
	}
}
