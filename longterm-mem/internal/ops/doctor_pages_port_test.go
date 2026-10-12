package ops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"
)

// The port the doctor owns is a subset of the one promotion owns, and the file system adapter satisfies it: a
// method of either that drifts from the other stops this file building.
var (
	_ PageReader = promote.PageReader(nil)
	_ PageReader = (*vaultfs.Vault)(nil)
)

// fakePages is a PageReader that answers the pages it was given, in the order it was given them, without a
// file behind them; it can also fail on demand.
type fakePages struct {
	order    []string
	contents map[string]string
	// listErr, when set, is what ListPages answers; readErrs makes the read of an address fail.
	listErr  error
	readErrs map[string]error
	// location is what PagesLocation answers.
	location string
}

func newFakePages(pages ...[2]string) *fakePages {
	f := &fakePages{contents: map[string]string{}, location: "/fake/vault/wiki/memory"}
	for _, page := range pages {
		f.order = append(f.order, page[0])
		f.contents[page[0]] = page[1]
	}
	return f
}

func (f *fakePages) ListPages() ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]string(nil), f.order...), nil
}

func (f *fakePages) ReadPage(address string) (string, error) {
	if err := f.readErrs[address]; err != nil {
		return "", err
	}
	content, ok := f.contents[address]
	if !ok {
		return "", fmt.Errorf("read %s: %w", address, fs.ErrNotExist)
	}
	return content, nil
}

func (f *fakePages) PageExists(address string) (bool, error) {
	_, ok := f.contents[address]
	return ok, nil
}

func (f *fakePages) PagesLocation() string { return f.location }

// pagesDoctorDeps is the deps of a vault with one promoted page on disk, c-000042, fully registered and
// tracked, whose pages the reader alone says: whatever Doctor knows about the pages, the reader told it.
// The address map answers an empty map, so a page the reader lists is a page the map does not name.
func pagesDoctorDeps(t *testing.T, pages PageReader) DoctorDeps {
	t.Helper()
	vaultRoot := t.TempDir()
	page := testdata.WritePromotedPage(t, vaultRoot, "c-000042", "Widget Decision")
	testdata.WritePrecedenceEntry(t, vaultRoot, page)
	testdata.RegisterPage(t, vaultRoot, "c-000042", "Widget Decision")
	embeddingIndex, liveIDs := newHealthyEmbeddingDeps(t)
	return DoctorDeps{
		VaultRoot:             vaultRoot,
		Root:                  vaultfs.New(vaultRoot),
		Index:                 vaultfs.New(vaultRoot),
		Log:                   vaultfs.New(vaultRoot),
		Pages:                 pages,
		Precedence:            vaultfs.New(vaultRoot),
		AddressMap:            &fakeAddressMap{entries: promote.AddressMap{}},
		PrerequisitePresent:   func(string) bool { return true },
		EmbeddingIndex:        embeddingIndex,
		LiveObservationIDs:    func(string) ([]int64, error) { return liveIDs, nil },
		EmbeddingBackendCheck: func(context.Context) error { return nil },
	}
}

func pageChecks(t *testing.T, deps DoctorDeps) map[string]Check {
	t.Helper()
	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if len(report.Checks) != 8 {
		t.Fatalf("Doctor reported %d checks, want all 8", len(report.Checks))
	}
	checks := map[string]Check{}
	for _, check := range report.Checks {
		checks[check.Name] = check
	}
	return checks
}

// pageCheckNames are the three checks that judge the promoted pages.
var pageCheckNames = []string{CheckAddressMapIntegrity, CheckWikiRegistrationConsistency, CheckPrecedenceSidecarConsistency}

// The pages Doctor judges are the ones the reader answers: a page it lists is judged by all three checks,
// and the page that is on the disk of the vault, which the reader does not list, is not.
func TestDoctorJudgesThePagesTheReaderListsAndNoOthers(t *testing.T) {
	checks := pageChecks(t, pagesDoctorDeps(t, newFakePages([2]string{"c-000099", "---\naddress: c-000099\n---\nbody"})))

	for name, want := range map[string]string{
		CheckAddressMapIntegrity:          "address c-000099 has no address_map entry",
		CheckWikiRegistrationConsistency:  "wiki/index.md has no link to c-000099; wiki/log.md has no entry for c-000099",
		CheckPrecedenceSidecarConsistency: ".raw/.longterm-mem-manifest.json has no entry for c-000099, so longterm-mem cannot prove it wrote that page",
	} {
		if got := checks[name]; got.Status != CheckFailed || got.Detail != want {
			t.Errorf("%s = %+v, want FAIL with exactly %q (the page on the disk, c-000042, is not the reader's)", name, got, want)
		}
	}
}

// A reader with no page leaves nothing to judge: the three checks pass although the vault's disk holds a
// page that no map names.
func TestDoctorWithAReaderWithoutPagesPasses(t *testing.T) {
	checks := pageChecks(t, pagesDoctorDeps(t, newFakePages()))

	for _, name := range pageCheckNames {
		if got := checks[name]; got.Status != CheckPassed {
			t.Errorf("%s = %+v, want PASS", name, got)
		}
	}
}

// A page the reader lists and cannot read is a finding of its own, in the words the doctor has always used,
// and the pages after it are still judged.
func TestDoctorNamesAPageTheReaderCannotReadAndGoesOn(t *testing.T) {
	pages := newFakePages([2]string{"c-000050", ""}, [2]string{"c-000051", "---\naddress: c-000051\n---\n"})
	pages.readErrs = map[string]error{"c-000050": errors.New("read /fake/vault/wiki/memory/c-000050.md: is a directory")}
	checks := pageChecks(t, pagesDoctorDeps(t, pages))

	const unreadable = "promoted page c-000050.md could not be read: read /fake/vault/wiki/memory/c-000050.md: is a directory"
	for _, name := range pageCheckNames {
		got := checks[name]
		if got.Status != CheckFailed || !strings.HasPrefix(got.Detail, unreadable+"; ") || !strings.Contains(got.Detail, "c-000051") {
			t.Errorf("%s = %+v, want FAIL naming the unreadable page first and then judging c-000051", name, got)
		}
	}
}

// A reader that cannot list the pages fails the three checks that need them, with the cause and the place it
// was listed, and the five others run.
func TestDoctorWhoseReaderCannotListFailsOnlyThePageChecks(t *testing.T) {
	pages := newFakePages()
	pages.listErr = errors.New("open /fake/vault/wiki/memory: not a directory")
	checks := pageChecks(t, pagesDoctorDeps(t, pages))

	const want = "ops: list promoted pages in /fake/vault/wiki/memory: open /fake/vault/wiki/memory: not a directory"
	for _, name := range pageCheckNames {
		if got := checks[name]; got.Status != CheckFailed || got.Detail != want {
			t.Errorf("%s = %+v, want FAIL with %q", name, got, want)
		}
	}
	for _, name := range []string{CheckVaultConfigResolvable, CheckRuntimePrerequisites, CheckEmbeddingIndexPresent, CheckEmbeddingIndexFresh, CheckEmbeddingBackendReachable} {
		if got := checks[name]; got.Status != CheckPassed {
			t.Errorf("%s = %+v, want PASS (a pages directory that cannot be listed must not abort the others)", name, got)
		}
	}
}

// A Doctor handed no reader says so, instead of calling a port that is not there: the three page checks fail,
// each once and in ops's words, and the others still run.
func TestDoctorWithoutAPageReaderFailsOnlyThePageChecks(t *testing.T) {
	var typedNil *fakePages
	for name, reader := range map[string]PageReader{"absent": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			checks := pageChecks(t, pagesDoctorDeps(t, reader))

			for _, check := range pageCheckNames {
				if got := checks[check]; got.Status != CheckFailed || got.Detail != "ops: no page reader was wired" {
					t.Errorf("%s = %+v, want FAIL naming the missing reader once", check, got)
				}
			}
			for _, check := range []string{CheckVaultConfigResolvable, CheckRuntimePrerequisites} {
				if got := checks[check]; got.Status != CheckPassed {
					t.Errorf("%s = %+v, want PASS (one missing port must not abort the others)", check, got)
				}
			}
		})
	}
}

// The location a listing failure names is the one the reader gives, not one the doctor builds from the root.
func TestDoctorNamesTheLocationTheReaderGivesForTheDirectoryItCouldNotList(t *testing.T) {
	pages := newFakePages()
	pages.listErr = errors.New("boom")
	pages.location = filepath.Join("elsewhere", "pages")

	got := pageChecks(t, pagesDoctorDeps(t, pages))[CheckAddressMapIntegrity]

	if want := "ops: list promoted pages in " + filepath.Join("elsewhere", "pages") + ": boom"; got.Detail != want {
		t.Fatalf("address-map-integrity detail = %q, want %q", got.Detail, want)
	}
}
