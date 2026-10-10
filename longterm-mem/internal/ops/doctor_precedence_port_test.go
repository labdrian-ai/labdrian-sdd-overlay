package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// fakePrecedence is a PrecedenceReader that answers a store, or an error, without a file behind it.
type fakePrecedence struct {
	store promote.PrecedenceStore
	err   error
	loads int
}

func (f *fakePrecedence) LoadPrecedence() (promote.PrecedenceStore, error) {
	f.loads++
	return f.store, f.err
}

// portDoctorDeps is the deps of a vault with one promoted page, fully registered but with no precedence
// sidecar on disk at all: whatever Doctor knows about the precedence store, the reader told it.
func portDoctorDeps(t *testing.T, reader PrecedenceReader) (DoctorDeps, promote.Page) {
	t.Helper()
	vaultRoot := t.TempDir()
	page := testdata.WritePromotedPage(t, vaultRoot, "c-000042", "Widget Decision")
	testdata.WriteAddressMap(t, vaultRoot, map[string]string{"wiki/memory/c-000042.md": "c-000042"})
	testdata.RegisterPage(t, vaultRoot, "c-000042", "Widget Decision")
	if _, err := os.Stat(filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("the fixture has a sidecar on disk (stat err = %v); the test would not prove the port is the source", err)
	}
	stateDir, liveIDs := newHealthyEmbeddingDeps(t)
	return DoctorDeps{
		VaultRoot:             vaultRoot,
		Precedence:            reader,
		PrerequisitePresent:   func(string) bool { return true },
		StateDir:              stateDir,
		LiveObservationIDs:    func(string) ([]int64, error) { return liveIDs, nil },
		EmbeddingBackendCheck: func(context.Context) error { return nil },
	}, page
}

func precedenceCheck(t *testing.T, deps DoctorDeps) Check {
	t.Helper()
	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	return checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency)
}

// The store Doctor judges the pages against is the one the reader answers: a page the reader's store tracks
// passes although no file of the vault records it, and the reader is asked once.
func TestDoctorReadsThePrecedenceStoreThroughItsPort(t *testing.T) {
	reader := &fakePrecedence{store: promote.PrecedenceStore{"c-000042": {BodyHash: "b", FrontmatterHash: "f", PromotedRevision: 1}}}
	deps, _ := portDoctorDeps(t, reader)

	if got := precedenceCheck(t, deps); got.Status != CheckPassed {
		t.Fatalf("precedence-sidecar-consistency = %+v, want PASS for a page the reader's store tracks", got)
	}
	if reader.loads != 1 {
		t.Errorf("the reader was asked %d times, want once", reader.loads)
	}
}

// A page the reader's store does not track is named, as a page the sidecar does not know always was.
func TestDoctorNamesAPageTheReadersStoreDoesNotTrack(t *testing.T) {
	deps, _ := portDoctorDeps(t, &fakePrecedence{store: promote.PrecedenceStore{}})

	got := precedenceCheck(t, deps)
	if got.Status != CheckFailed {
		t.Fatalf("precedence-sidecar-consistency = %+v, want FAIL", got)
	}
	for _, want := range []string{".raw/.longterm-mem-manifest.json has no entry for c-000042"} {
		if !strings.Contains(got.Detail, want) {
			t.Errorf("detail %q does not contain %q", got.Detail, want)
		}
	}
}

// A reader that fails is reported against every promoted page, with its error: the pages have no provable
// provenance, and the operator is told why.
func TestDoctorReportsAPrecedenceReaderThatFails(t *testing.T) {
	deps, _ := portDoctorDeps(t, &fakePrecedence{err: errors.New("the sidecar is unreadable")})

	got := precedenceCheck(t, deps)
	if got.Status != CheckFailed {
		t.Fatalf("precedence-sidecar-consistency = %+v, want FAIL", got)
	}
	if want := ".raw/.longterm-mem-manifest.json could not be read, so c-000042 has no provable provenance: the sidecar is unreadable"; !strings.Contains(got.Detail, want) {
		t.Errorf("detail %q does not contain %q", got.Detail, want)
	}
}

// A Doctor handed no reader says so, instead of calling a port that is not there: the check fails and the
// other seven still run.
func TestDoctorWithoutAPrecedenceReaderFailsOnlyThatCheck(t *testing.T) {
	deps, _ := portDoctorDeps(t, nil)

	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency)
	if got.Status != CheckFailed || !strings.Contains(got.Detail, "no precedence reader") {
		t.Fatalf("precedence-sidecar-consistency = %+v, want FAIL naming the missing reader", got)
	}
	if len(report.Checks) != 8 {
		t.Errorf("Doctor reported %d checks, want all 8", len(report.Checks))
	}
	if got := checkStatus(t, report.Checks, CheckRuntimePrerequisites); got.Status != CheckPassed {
		t.Errorf("runtime-prerequisites = %+v, want PASS (one missing port must not abort the others)", got)
	}
}
