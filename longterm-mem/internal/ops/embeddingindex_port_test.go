package ops

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"
)

// The embedding index is read through the port the package owns, for the project of the call, and what the
// diagnostics print of it is what the port says: its location, its words, the instant it was built at.

// statusDepsWith are the dependencies of a Status over an empty vault and the given index reader.
func statusDepsWith(t *testing.T, index EmbeddingIndexReader) StatusDeps {
	t.Helper()
	vaultRoot := t.TempDir()
	return StatusDeps{
		VaultRoot:        vaultRoot,
		SyncState:        vaultfs.New(vaultRoot),
		EmbeddingIndex:   index,
		EngramReachable:  func(context.Context) (bool, string) { return true, "" },
		VaultProvisioned: func(string) bool { return true },
	}
}

// doctorDepsWith are the dependencies of a Doctor over a healthy vault, the given index reader and live rows.
func doctorDepsWith(t *testing.T, index EmbeddingIndexReader, liveIDs ...int64) DoctorDeps {
	t.Helper()
	vaultRoot := t.TempDir()
	const address, title = "c-000042", "Widget Decision"
	page := testdata.WritePromotedPage(t, vaultRoot, address, title)
	testdata.WriteAddressMap(t, vaultRoot, map[string]string{"wiki/memory/" + address + ".md": address})
	testdata.WritePrecedenceEntry(t, vaultRoot, page)
	testdata.RegisterPage(t, vaultRoot, address, title)
	return DoctorDeps{
		VaultRoot:             vaultRoot,
		Root:                  vaultfs.New(vaultRoot),
		Index:                 vaultfs.New(vaultRoot),
		Log:                   vaultfs.New(vaultRoot),
		Precedence:            vaultfs.New(vaultRoot),
		Pages:                 vaultfs.New(vaultRoot),
		AddressMap:            vaultfs.New(vaultRoot),
		PrerequisitePresent:   func(string) bool { return true },
		EmbeddingIndex:        index,
		LiveObservationIDs:    func(string) ([]int64, error) { return liveIDs, nil },
		EmbeddingBackendCheck: func(context.Context) error { return nil },
	}
}

func TestStatus_ReportsTheBuildTimeTheIndexPortGives(t *testing.T) {
	index := &fakeEmbeddingIndex{location: fakeIndexLocation, builtAt: "2026-03-14T09:00:00Z"}

	report, err := Status(context.Background(), statusDepsWith(t, index), "proj")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if report.EmbeddingIndexBuiltAt != "2026-03-14T09:00:00Z" {
		t.Errorf("EmbeddingIndexBuiltAt = %q, want the instant the port gave", report.EmbeddingIndexBuiltAt)
	}
	if want := []string{"proj"}; !reflect.DeepEqual(index.asked, want) {
		t.Errorf("the port was asked for %v, want %v: the project of the call, and nothing else", index.asked, want)
	}
}

// An index nobody built, one that cannot be read and any other failure of the port are all "never": Status never
// invents a timestamp it cannot stand behind, and none of them fails it.
func TestStatus_NeverReportsAnInstantTheIndexPortCouldNotGive(t *testing.T) {
	for name, err := range map[string]error{
		"nobody built an index":           nothingBuilt{"fake: nothing was built"},
		"an index that cannot be trusted": errors.New("fake: index is corrupted"),
	} {
		t.Run(name, func(t *testing.T) {
			index := &fakeEmbeddingIndex{location: fakeIndexLocation, builtAt: "2026-03-14T09:00:00Z", err: err}

			report, statusErr := Status(context.Background(), statusDepsWith(t, index), "proj")

			if statusErr != nil || report.EmbeddingIndexBuiltAt != "never" {
				t.Errorf("Status = (%q, %v), want the literal \"never\" and no error", report.EmbeddingIndexBuiltAt, statusErr)
			}
		})
	}
}

// Status built without a reader of the index says so, as it does for a sync state it cannot read: it is a
// wiring mistake, not a project that has no index.
func TestStatus_WithoutAnIndexPortSaysSo(t *testing.T) {
	var typedNil *fakeEmbeddingIndex
	for name, index := range map[string]EmbeddingIndexReader{"no reader": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			report, err := Status(context.Background(), statusDepsWith(t, index), "proj")

			if !errors.Is(err, errNoEmbeddingIndexReader) || report != (StatusReport{}) {
				t.Errorf("Status = (%+v, %v), want the zero report and %v", report, err, errNoEmbeddingIndexReader)
			}
		})
	}
}

func TestDoctor_NamesWhereTheIndexPortLookedWhenNobodyBuiltAnIndex(t *testing.T) {
	// The wrapped error is the same fact: the diagnostics find what an error says it is, however it is wrapped.
	wrapped := fmt.Errorf("while reading: %w", nothingBuilt{"fake: nothing was built"})
	for name, err := range map[string]error{"as the port gives it": nothingBuilt{"fake: nothing was built"}, "wrapped": wrapped} {
		t.Run(name, func(t *testing.T) {
			index := &fakeEmbeddingIndex{location: fakeIndexLocation, err: err}

			report, doctorErr := Doctor(context.Background(), doctorDepsWith(t, index), "proj")
			if doctorErr != nil {
				t.Fatalf("Doctor: %v", doctorErr)
			}

			got := checkStatus(t, report.Checks, CheckEmbeddingIndexPresent)
			want := Check{Name: CheckEmbeddingIndexPresent, Status: CheckFailed, Detail: "no embedding index has been built at /state/index/proj; run `longterm-mem index --embeddings`"}
			if got != want {
				t.Errorf("embedding-index-present = %+v, want %+v", got, want)
			}
		})
	}
}

// A state directory nobody can name is not an index nobody built: the two checks that read the index say that
// the directory could not be resolved and what sets it, as the error says it is (StateDirUnresolved), however it
// is wrapped, and the checks that do not read the index run as they do.
func TestDoctor_SaysWhenTheStateDirectoryCouldNotBeResolved(t *testing.T) {
	unresolved := unresolvedState{"fake words that are never read"}
	for name, err := range map[string]error{"as the port gives it": unresolved, "wrapped": fmt.Errorf("while reading: %w", unresolved)} {
		t.Run(name, func(t *testing.T) {
			index := &fakeEmbeddingIndex{location: fakeIndexLocation, err: err}

			report, doctorErr := Doctor(context.Background(), doctorDepsWith(t, index, 1), "proj")
			if doctorErr != nil {
				t.Fatalf("Doctor: %v", doctorErr)
			}

			const hint = StateDirHint
			wantPresent := Check{Name: CheckEmbeddingIndexPresent, Status: CheckFailed, Detail: "the state directory could not be resolved, so no embedding index can be looked for; " + hint}
			wantFresh := Check{Name: CheckEmbeddingIndexFresh, Status: CheckFailed, Detail: "freshness cannot be determined: the state directory could not be resolved; " + hint}
			if got := checkStatus(t, report.Checks, CheckEmbeddingIndexPresent); got != wantPresent {
				t.Errorf("embedding-index-present = %+v, want %+v", got, wantPresent)
			}
			if got := checkStatus(t, report.Checks, CheckEmbeddingIndexFresh); got != wantFresh {
				t.Errorf("embedding-index-fresh = %+v, want %+v", got, wantFresh)
			}
			if got := checkStatus(t, report.Checks, CheckEmbeddingBackendReachable); got.Status != CheckPassed {
				t.Errorf("embedding-backend-reachable = %+v, want PASSed: the backend does not need the state directory", got)
			}
		})
	}
}

// What an error says it is decides, and its words do not: an error whose text sounds like a missing index is
// told as an index that could not be loaded, with those words, the way any other failure is.
func TestDoctor_TellsAnIndexNobodyBuiltByWhatTheErrorSaysItIs(t *testing.T) {
	index := &fakeEmbeddingIndex{location: fakeIndexLocation, err: errors.New("no embedding index has been built")}

	report, err := Doctor(context.Background(), doctorDepsWith(t, index), "proj")
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	got := checkStatus(t, report.Checks, CheckEmbeddingIndexPresent)
	want := Check{Name: CheckEmbeddingIndexPresent, Status: CheckFailed, Detail: "embedding index at /state/index/proj could not be loaded: no embedding index has been built"}
	if got != want {
		t.Errorf("embedding-index-present = %+v, want %+v", got, want)
	}
}

// Freshness cannot be judged of an index that cannot be read, whatever the reason, and says so with the port's
// own words and the location it gave.
func TestDoctor_SaysFreshnessCannotBeDeterminedWhenTheIndexPortFails(t *testing.T) {
	for name, err := range map[string]error{
		"nobody built an index":           nothingBuilt{"fake: nothing was built"},
		"an index that cannot be trusted": errors.New("fake: index is corrupted"),
	} {
		t.Run(name, func(t *testing.T) {
			index := &fakeEmbeddingIndex{location: fakeIndexLocation, err: err}

			report, doctorErr := Doctor(context.Background(), doctorDepsWith(t, index, 1), "proj")
			if doctorErr != nil {
				t.Fatalf("Doctor: %v", doctorErr)
			}

			got := checkStatus(t, report.Checks, CheckEmbeddingIndexFresh)
			want := Check{Name: CheckEmbeddingIndexFresh, Status: CheckFailed, Detail: "freshness cannot be determined: embedding index at /state/index/proj could not be loaded: " + err.Error()}
			if got != want {
				t.Errorf("embedding-index-fresh = %+v, want %+v", got, want)
			}
		})
	}
}

// The index is compared with the live rows by the ids it vouches for, as sets: an index as long as the corpus
// with other rows in it is behind by the rows it lacks, and rows it holds that are no longer live do not hide
// that.
func TestDoctor_ComparesTheIdsTheIndexPortGivesWithTheLiveRowsAsSets(t *testing.T) {
	for name, tc := range map[string]struct {
		indexed []int64
		live    []int64
		want    Check
	}{
		"the same rows": {[]int64{1, 2, 3}, []int64{3, 2, 1}, Check{Name: CheckEmbeddingIndexFresh, Status: CheckPassed}},
		"as many rows, not the same ones": {[]int64{1, 2, 9}, []int64{1, 2, 3}, Check{
			Name: CheckEmbeddingIndexFresh, Status: CheckFailed,
			Detail: "embedding index is missing 1 live row(s) for proj; run `longterm-mem index --embeddings`"}},
		"rows no longer live are no harm": {[]int64{1, 2, 3, 9}, []int64{1, 2, 3}, Check{Name: CheckEmbeddingIndexFresh, Status: CheckPassed}},
		"an index of none": {nil, []int64{1, 2}, Check{
			Name: CheckEmbeddingIndexFresh, Status: CheckFailed,
			Detail: "embedding index is missing 2 live row(s) for proj; run `longterm-mem index --embeddings`"}},
	} {
		t.Run(name, func(t *testing.T) {
			index := &fakeEmbeddingIndex{location: fakeIndexLocation, builtAt: "2026-01-01T00:00:00Z", ids: tc.indexed}

			report, err := Doctor(context.Background(), doctorDepsWith(t, index, tc.live...), "proj")
			if err != nil {
				t.Fatalf("Doctor: %v", err)
			}

			if got := checkStatus(t, report.Checks, CheckEmbeddingIndexFresh); got != tc.want {
				t.Errorf("embedding-index-fresh = %+v, want %+v", got, tc.want)
			}
			if got := checkStatus(t, report.Checks, CheckEmbeddingIndexPresent); got.Status != CheckPassed {
				t.Errorf("embedding-index-present = %+v, want PASSed: the port gave an index", got)
			}
		})
	}
}

// Doctor built without a reader of the index fails the two checks that read it, naming the wiring, and runs
// every other check.
func TestDoctor_WithoutAnIndexPortTheTwoChecksThatReadItSaySo(t *testing.T) {
	var typedNil *fakeEmbeddingIndex
	for name, index := range map[string]EmbeddingIndexReader{"no reader": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			report, err := Doctor(context.Background(), doctorDepsWith(t, index, 1), "proj")
			if err != nil {
				t.Fatalf("Doctor: %v", err)
			}

			for _, check := range []string{CheckEmbeddingIndexPresent, CheckEmbeddingIndexFresh} {
				got := checkStatus(t, report.Checks, check)
				if got.Status != CheckFailed || got.Detail != "ops: no embedding index reader was wired" {
					t.Errorf("%s = %+v, want FAILed with the wiring named", check, got)
				}
			}
			if got := checkStatus(t, report.Checks, CheckEmbeddingBackendReachable); got.Status != CheckPassed {
				t.Errorf("embedding-backend-reachable = %+v, want PASSed: one missing port must not stop the other checks", got)
			}
		})
	}
}

// The backend's two failures are told apart by what the error says it is, however it is wrapped, and not by the
// package that makes it or by its words: an error that says neither is an unreachable backend, with its words.
func TestDoctor_TellsTheBackendsFailuresByWhatTheErrorSaysItIs(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"a model that is not pulled", &modelNotPulled{"m"}, "backend answered but required model is missing: the model m is not pulled"},
		{"the same, wrapped", fmt.Errorf("probe: %w", &modelNotPulled{"m"}), "backend answered but required model is missing: probe: the model m is not pulled"},
		{"a backend that is down", &backendDown{errors.New("refused")}, "backend is unreachable: the backend is down: refused"},
		{"an error that says neither", errors.New("redirect refused"), "backend is unreachable: redirect refused"},
		{"words that look like a missing model", errors.New("the model is missing"), "backend is unreachable: the model is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, live := newHealthyEmbeddingDeps(t)
			deps := doctorDepsWith(t, index, live...)
			deps.EmbeddingBackendCheck = func(context.Context) error { return tc.err }

			report, err := Doctor(context.Background(), deps, "proj")
			if err != nil {
				t.Fatalf("Doctor: %v", err)
			}

			got := checkStatus(t, report.Checks, CheckEmbeddingBackendReachable)
			if got.Status != CheckFailed || got.Detail != tc.want {
				t.Errorf("embedding-backend-reachable = %+v, want FAILed with %q", got, tc.want)
			}
		})
	}
}
