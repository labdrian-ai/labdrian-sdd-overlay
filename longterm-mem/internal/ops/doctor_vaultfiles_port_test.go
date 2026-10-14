package ops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/opstest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"
)

// The ports the doctor and status own for the catalog, the log, the vault root and the sync state are what the
// file system adapter satisfies, and the catalog's is a subset of the one promotion owns: a method of either that
// drifts from the other stops this file building.
var (
	_ IndexReader     = promote.IndexReader(nil)
	_ IndexReader     = promote.IndexRepository(nil)
	_ IndexReader     = (*vaultfs.Vault)(nil)
	_ LogReader       = promote.LogRepository(nil)
	_ LogReader       = (*vaultfs.Vault)(nil)
	_ RootInspector   = (*vaultfs.Vault)(nil)
	_ SyncStateReader = (*vaultfs.Vault)(nil)
)

// fakeFile is an IndexReader and a LogReader that answer a content and an error, and count the reads.
type fakeFile struct {
	content string
	err     error
	reads   int
}

func (f *fakeFile) ReadIndex() (string, error) { f.reads++; return f.content, f.err }
func (f *fakeFile) ReadLog() (string, error)   { f.reads++; return f.content, f.err }

// fakeRoot is a RootInspector that answers what it was told, and counts the questions.
type fakeRoot struct {
	isDir bool
	err   error
	asked int
}

func (f *fakeRoot) RootIsDirectory() (bool, error) { f.asked++; return f.isDir, f.err }

// fakeSyncState is a SyncStateReader that answers what it was told, and counts the reads.
type fakeSyncState struct {
	completedAt string
	err         error
	reads       int
}

func (f *fakeSyncState) LastSyncCompletedAt() (string, error) { f.reads++; return f.completedAt, f.err }

// registeredDeps is the deps of a vault with one promoted page on disk, c-000042, whose catalog and log are not
// registered anywhere on disk: whatever Doctor knows about its registration, the readers told it. A test gives the
// readers the content it wants the check to judge.
func registeredDeps(t *testing.T, index IndexReader, log LogReader) DoctorDeps {
	t.Helper()
	vaultRoot := t.TempDir()
	page := opstest.WritePromotedPage(t, vaultRoot, "c-000042", "Widget Decision")
	opstest.WritePrecedenceEntry(t, vaultRoot, page)
	embeddingIndex, liveIDs := newHealthyEmbeddingDeps(t)
	return DoctorDeps{
		VaultRoot:             vaultRoot,
		Root:                  vaultfs.New(vaultRoot),
		Index:                 index,
		Log:                   log,
		Pages:                 vaultfs.New(vaultRoot),
		Precedence:            vaultfs.New(vaultRoot),
		AddressMap:            &fakeAddressMap{entries: promote.AddressMap{"wiki/memory/c-000042.md": "c-000042"}},
		PrerequisitePresent:   func(string) bool { return true },
		EmbeddingIndex:        embeddingIndex,
		LiveObservationIDs:    func(string) ([]int64, error) { return liveIDs, nil },
		EmbeddingBackendCheck: func(context.Context) error { return nil },
	}
}

func checkOf(t *testing.T, deps DoctorDeps, name string) Check {
	t.Helper()
	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	return checkStatus(t, report.Checks, name)
}

// The registration Doctor judges is the one the readers answer: a page the catalog and the log they give name is
// registered although no file of the vault says so.
func TestDoctorReadsTheCatalogAndTheLogThroughTheirPorts(t *testing.T) {
	index := &fakeFile{content: "- [[c-000042|Widget Decision]]\n"}
	log := &fakeFile{content: "## [2026-08-31] promote | Widget Decision\n\n[[c-000042|Widget Decision]]\n"}

	got := checkOf(t, registeredDeps(t, index, log), CheckWikiRegistrationConsistency)

	if got.Status != CheckPassed {
		t.Fatalf("wiki-registration-consistency = %+v, want PASS for a page the readers' catalog and log name", got)
	}
	// Doctor runs every check: the catalog is read by two of them for the one page (the address-map check runs
	// the page check's inbound-link rule among its others, and the registration check runs it alone), the log by
	// the registration check only.
	if index.reads != 2 || log.reads != 1 {
		t.Errorf("the catalog was read %d times and the log %d, want twice and once for one page", index.reads, log.reads)
	}
}

// A page the catalog or the log the readers give does not name is a finding in the words the doctor has always
// used, each file on its own.
func TestDoctorNamesThePageTheReadersCatalogAndLogDoNotName(t *testing.T) {
	named := "[[c-000042|Widget Decision]]\n"
	for name, tc := range map[string]struct {
		index, log string
		want       string
	}{
		"the catalog does not name it": {"- [[c-000099|Other]]\n", named, "wiki/index.md has no link to c-000042"},
		"the log does not name it":     {named, "# Log\n", "wiki/log.md has no entry for c-000042"},
		"neither names it":             {"", "", "wiki/index.md has no link to c-000042; wiki/log.md has no entry for c-000042"},
		"only its address in prose":    {"c-000042 is mentioned\n", "c-000042 is mentioned\n", "wiki/index.md has no link to c-000042; wiki/log.md has no entry for c-000042"},
	} {
		t.Run(name, func(t *testing.T) {
			got := checkOf(t, registeredDeps(t, &fakeFile{content: tc.index}, &fakeFile{content: tc.log}), CheckWikiRegistrationConsistency)
			if got.Status != CheckFailed || got.Detail != tc.want {
				t.Fatalf("wiki-registration-consistency = %+v, want FAIL with %q", got, tc.want)
			}
		})
	}
}

// A catalog or a log the reader cannot read, for whatever reason, is one the page is not in.
func TestDoctorTakesACatalogOrALogItCannotReadForOneThePageIsNotIn(t *testing.T) {
	healthy := &fakeFile{content: "[[c-000042|Widget Decision]]\n"}
	cause := fmt.Errorf("open: %w", fs.ErrNotExist)

	t.Run("the catalog", func(t *testing.T) {
		got := checkOf(t, registeredDeps(t, &fakeFile{content: healthy.content, err: cause}, healthy), CheckWikiRegistrationConsistency)
		if got.Status != CheckFailed || got.Detail != "wiki/index.md is missing" {
			t.Fatalf("wiki-registration-consistency = %+v, want FAIL: wiki/index.md is missing", got)
		}
	})
	t.Run("the log", func(t *testing.T) {
		got := checkOf(t, registeredDeps(t, healthy, &fakeFile{content: healthy.content, err: errors.New("read: is a directory")}), CheckWikiRegistrationConsistency)
		if got.Status != CheckFailed || got.Detail != "wiki/log.md has no entry for c-000042" {
			t.Fatalf("wiki-registration-consistency = %+v, want FAIL: wiki/log.md has no entry for c-000042", got)
		}
	})
}

// With no page to look for the files are not read at all, and an unreadable one is not a finding.
func TestDoctorReadsNeitherTheCatalogNorTheLogWhenThereIsNoPage(t *testing.T) {
	index := &fakeFile{err: errors.New("never read")}
	log := &fakeFile{err: errors.New("never read")}
	deps := registeredDeps(t, index, log)
	deps.Pages = newFakePages()

	got := checkOf(t, deps, CheckWikiRegistrationConsistency)

	if got.Status != CheckPassed {
		t.Fatalf("wiki-registration-consistency = %+v, want PASS with no page to register", got)
	}
	if index.reads != 0 || log.reads != 0 {
		t.Errorf("the catalog was read %d times and the log %d, want neither read", index.reads, log.reads)
	}
}

// A reader that was not wired is named once, and only when there is a page to look for in the file; the check
// does not call it, and does not say once per page what it says once.
func TestDoctorNamesACatalogOrALogReaderThatWasNotWired(t *testing.T) {
	var typedNil *fakeFile
	healthy := &fakeFile{content: "[[c-000042|Widget Decision]]\n"}

	t.Run("no catalog reader", func(t *testing.T) {
		got := checkOf(t, registeredDeps(t, nil, healthy), CheckWikiRegistrationConsistency)
		if got.Status != CheckFailed || got.Detail != "ops: no index reader was wired" {
			t.Fatalf("wiki-registration-consistency = %+v, want FAIL: ops: no index reader was wired", got)
		}
	})
	t.Run("no log reader", func(t *testing.T) {
		got := checkOf(t, registeredDeps(t, healthy, nil), CheckWikiRegistrationConsistency)
		if got.Status != CheckFailed || got.Detail != "ops: no log reader was wired" {
			t.Fatalf("wiki-registration-consistency = %+v, want FAIL: ops: no log reader was wired", got)
		}
	})
	t.Run("a nil pointer is no reader", func(t *testing.T) {
		got := checkOf(t, registeredDeps(t, typedNil, typedNil), CheckWikiRegistrationConsistency)
		if got.Status != CheckFailed || got.Detail != "ops: no index reader was wired; ops: no log reader was wired" {
			t.Fatalf("wiki-registration-consistency = %+v, want both readers named once", got)
		}
	})
	t.Run("with no page nothing is named", func(t *testing.T) {
		deps := registeredDeps(t, nil, nil)
		deps.Pages = newFakePages()
		if got := checkOf(t, deps, CheckWikiRegistrationConsistency); got.Status != CheckPassed {
			t.Fatalf("wiki-registration-consistency = %+v, want PASS with no page to register", got)
		}
	})
}

// The address-map check runs the page check's other rules too, and reads the catalog through the same port; it
// reports only its own rule, so a catalog that does not name the page is not its finding.
func TestDoctorsAddressMapCheckIsNotAboutTheCatalog(t *testing.T) {
	for name, index := range map[string]IndexReader{"a catalog without the page": &fakeFile{}, "an unreadable one": &fakeFile{err: errors.New("x")}, "none": nil} {
		t.Run(name, func(t *testing.T) {
			got := checkOf(t, registeredDeps(t, index, &fakeFile{}), CheckAddressMapIntegrity)
			if got.Status != CheckPassed {
				t.Fatalf("address-map-integrity = %+v, want PASS: the page is in the address map, and the catalog is another check's", got)
			}
		})
	}
}

// The root Doctor reports on is the one the inspector answers for: the inspector is asked once, and its answer is
// the check's, in the words the doctor has always used.
func TestDoctorAsksTheInspectorWhatIsAtTheVaultRoot(t *testing.T) {
	deps := registeredDeps(t, &fakeFile{}, &fakeFile{})
	deps.VaultRoot = "/the/vault"

	t.Run("a directory", func(t *testing.T) {
		root := &fakeRoot{isDir: true}
		deps.Root = root
		if got := checkOf(t, deps, CheckVaultConfigResolvable); got.Status != CheckPassed || root.asked != 1 {
			t.Fatalf("vault-config-resolvable = %+v after %d questions, want PASS after one", got, root.asked)
		}
	})
	t.Run("not a directory", func(t *testing.T) {
		deps.Root = &fakeRoot{}
		got := checkOf(t, deps, CheckVaultConfigResolvable)
		if got.Status != CheckFailed || got.Detail != "/the/vault is not a directory" {
			t.Fatalf("vault-config-resolvable = %+v, want FAIL: /the/vault is not a directory", got)
		}
	})
	t.Run("a root that cannot be examined", func(t *testing.T) {
		deps.Root = &fakeRoot{err: errors.New("stat /the/vault: no such file or directory")}
		got := checkOf(t, deps, CheckVaultConfigResolvable)
		want := "/the/vault does not resolve to an existing directory: stat /the/vault: no such file or directory"
		if got.Status != CheckFailed || got.Detail != want {
			t.Fatalf("vault-config-resolvable = %+v, want FAIL with %q", got, want)
		}
	})
}

// A vault root that is not configured is named without asking anyone, and an inspector that was not wired is
// named, rather than called.
func TestDoctorDoesNotAskForARootItWasNotGivenAndNamesAnInspectorThatWasNotWired(t *testing.T) {
	t.Run("no root configured", func(t *testing.T) {
		deps := registeredDeps(t, &fakeFile{}, &fakeFile{})
		root := &fakeRoot{isDir: true}
		deps.VaultRoot, deps.Root = "", root
		got := checkOf(t, deps, CheckVaultConfigResolvable)
		if got.Status != CheckFailed || got.Detail != "vault root is not configured" || root.asked != 0 {
			t.Fatalf("vault-config-resolvable = %+v after %d questions, want FAIL: not configured, and nobody asked", got, root.asked)
		}
	})
	t.Run("no inspector", func(t *testing.T) {
		deps := registeredDeps(t, &fakeFile{}, &fakeFile{})
		deps.Root = nil
		got := checkOf(t, deps, CheckVaultConfigResolvable)
		if got.Status != CheckFailed || got.Detail != "ops: no root inspector was wired" {
			t.Fatalf("vault-config-resolvable = %+v, want FAIL: ops: no root inspector was wired", got)
		}
	})
	t.Run("a nil pointer is no inspector", func(t *testing.T) {
		deps := registeredDeps(t, &fakeFile{}, &fakeFile{})
		var typedNil *fakeRoot
		deps.Root = typedNil
		if got := checkOf(t, deps, CheckVaultConfigResolvable); !strings.Contains(got.Detail, "no root inspector was wired") {
			t.Fatalf("vault-config-resolvable = %+v, want the missing inspector named", got)
		}
	})
}
