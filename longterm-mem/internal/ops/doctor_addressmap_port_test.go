package ops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"
)

// fakeAddressMap is an AddressMapReader that answers a map, or an error, without a file behind it.
type fakeAddressMap struct {
	entries promote.AddressMap
	err     error
}

func (f *fakeAddressMap) LoadAddressMap() (promote.AddressMap, error) { return f.entries, f.err }

// addressMapDoctorDeps is the deps of a vault with one promoted page, fully registered and tracked by its
// precedence sidecar, but with no address map on disk at all: whatever Doctor knows about the address map,
// the reader told it.
func addressMapDoctorDeps(t *testing.T, reader AddressMapReader) DoctorDeps {
	t.Helper()
	vaultRoot := t.TempDir()
	page := testdata.WritePromotedPage(t, vaultRoot, "c-000042", "Widget Decision")
	testdata.WritePrecedenceEntry(t, vaultRoot, page)
	testdata.RegisterPage(t, vaultRoot, "c-000042", "Widget Decision")
	if _, err := os.Stat(filepath.Join(vaultRoot, ".raw", ".manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("the fixture has an address map on disk (stat err = %v); the test would not prove the port is the source", err)
	}
	stateDir, liveIDs := newHealthyEmbeddingDeps(t)
	return DoctorDeps{
		VaultRoot:             vaultRoot,
		Precedence:            vaultfs.New(vaultRoot),
		AddressMap:            reader,
		PrerequisitePresent:   func(string) bool { return true },
		StateDir:              stateDir,
		LiveObservationIDs:    func(string) ([]int64, error) { return liveIDs, nil },
		EmbeddingBackendCheck: func(context.Context) error { return nil },
	}
}

func addressMapCheck(t *testing.T, deps DoctorDeps) Check {
	t.Helper()
	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	return checkStatus(t, report.Checks, CheckAddressMapIntegrity)
}

// The map Doctor judges the pages against is the one the reader answers: a page the reader's map names passes
// although no file of the vault records it.
func TestDoctorReadsTheAddressMapThroughItsPort(t *testing.T) {
	deps := addressMapDoctorDeps(t, &fakeAddressMap{entries: promote.AddressMap{"wiki/memory/c-000042.md": "c-000042"}})

	if got := addressMapCheck(t, deps); got.Status != CheckPassed {
		t.Fatalf("address-map-integrity = %+v, want PASS for a page the reader's map names", got)
	}
}

// A page the reader's map names under another path, or does not name, is a finding in the words the doctor
// has always used.
func TestDoctorNamesAPageTheReadersAddressMapGetsWrong(t *testing.T) {
	for name, tc := range map[string]struct {
		entries promote.AddressMap
		want    string
	}{
		"another path": {promote.AddressMap{"wiki/memory/elsewhere.md": "c-000042"}, "address c-000042 maps to wiki/memory/elsewhere.md, not wiki/memory/c-000042.md"},
		"no entry":     {promote.AddressMap{}, "address c-000042 has no address_map entry"},
	} {
		t.Run(name, func(t *testing.T) {
			got := addressMapCheck(t, addressMapDoctorDeps(t, &fakeAddressMap{entries: tc.entries}))
			if got.Status != CheckFailed || got.Detail != tc.want {
				t.Fatalf("address-map-integrity = %+v, want FAIL with %q", got, tc.want)
			}
		})
	}
}

// A reader that says the map is corrupt is a finding; one that cannot read it at all is not, as a manifest
// the vault does not have has never been the page's fault.
func TestDoctorTellsACorruptAddressMapFromOneThatCannotBeRead(t *testing.T) {
	t.Run("corrupt", func(t *testing.T) {
		got := addressMapCheck(t, addressMapDoctorDeps(t, &fakeAddressMap{err: fmt.Errorf("parse: %w", promote.ErrAddressMapCorrupt)}))
		if got.Status != CheckFailed || got.Detail != ".raw/.manifest.json is not valid JSON" {
			t.Fatalf("address-map-integrity = %+v, want FAIL: .raw/.manifest.json is not valid JSON", got)
		}
	})
	t.Run("missing or unreadable", func(t *testing.T) {
		for name, err := range map[string]error{"missing": fmt.Errorf("read: %w", fs.ErrNotExist), "unreadable": errors.New("is a directory")} {
			if got := addressMapCheck(t, addressMapDoctorDeps(t, &fakeAddressMap{err: err})); got.Status != CheckPassed {
				t.Errorf("%s: address-map-integrity = %+v, want PASS", name, got)
			}
		}
	})
}

// A Doctor handed no reader says so, instead of calling a port that is not there: the check fails, once and in
// ops's words, and the other seven still run -- the registration check among them, which reads no address map
// to report what it reports.
func TestDoctorWithoutAnAddressMapReaderFailsOnlyThatCheck(t *testing.T) {
	var typedNil *fakeAddressMap
	for name, reader := range map[string]AddressMapReader{"absent": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			deps := addressMapDoctorDeps(t, reader)

			report, err := Doctor(context.Background(), deps, doctorTestProject)
			if err != nil {
				t.Fatalf("Doctor: %v", err)
			}
			got := checkStatus(t, report.Checks, CheckAddressMapIntegrity)
			if got.Status != CheckFailed || got.Detail != "ops: no address map reader was wired" {
				t.Fatalf("address-map-integrity = %+v, want FAIL naming the missing reader once", got)
			}
			if len(report.Checks) != 8 {
				t.Errorf("Doctor reported %d checks, want all 8", len(report.Checks))
			}
			for _, name := range []string{CheckWikiRegistrationConsistency, CheckPrecedenceSidecarConsistency, CheckRuntimePrerequisites} {
				if got := checkStatus(t, report.Checks, name); got.Status != CheckPassed {
					t.Errorf("%s = %+v, want PASS (one missing port must not abort the others)", name, got)
				}
			}
		})
	}
}

// With no promoted page there is no address to check, so a Doctor with no reader has nothing to complain of.
func TestDoctorWithoutAnAddressMapReaderAndWithoutPagesPasses(t *testing.T) {
	deps := addressMapDoctorDeps(t, nil)
	deps.VaultRoot = t.TempDir()
	deps.Precedence = vaultfs.New(deps.VaultRoot)

	if got := addressMapCheck(t, deps); got.Status != CheckPassed || strings.TrimSpace(got.Detail) != "" {
		t.Fatalf("address-map-integrity = %+v, want PASS", got)
	}
}
