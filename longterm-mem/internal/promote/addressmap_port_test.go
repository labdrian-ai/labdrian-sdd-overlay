package promote

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The fake is a recorder and a reader, as the file system adapter is.
var (
	_ AddressMapRecorder = (*memAddressMap)(nil)
	_ AddressMapReader   = (*memAddressMap)(nil)
)

// addressFunc-style value-receiver recorder: a nil one is a port that is not there although no pointer is nil.
type recorderFunc func(path, address string, createdAt time.Time) error

func (f recorderFunc) RecordAddress(path, address string, createdAt time.Time) error {
	return f(path, address, createdAt)
}

// writePageCarrying leaves the promoted page of Engram observation engramID at address in the vault.
func writePageCarrying(t *testing.T, vaultRoot, address string, engramID int64) {
	t.Helper()
	obs := memory.Observation{ID: engramID, Type: "decision", Title: "Already Promoted", Content: "Body.", Project: "labdrian-sdd-overlay"}
	page, err := EmitPage(obs, address, nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A new page is entered in the address map under its address-derived path, at the instant the caller's
// clock gave: the map is told, and the vault's counter has been spent for it.
func TestAllocateAddress_AFreshAddressIsRecordedUnderItsPath(t *testing.T) {
	addresses := &memAddressMap{}
	at := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	address, err := allocateAddress(t.Context(), t.TempDir(), "labdrian-sdd-overlay", 101, staticAddress(testAddress), addresses, at)
	if err != nil {
		t.Fatalf("allocateAddress: %v", err)
	}
	if address != testAddress {
		t.Fatalf("address = %q, want %q", address, testAddress)
	}
	want := []addressRecord{{path: "wiki/memory/c-000042.md", address: "c-000042", createdAt: at}}
	if len(addresses.records) != 1 || addresses.records[0] != want[0] {
		t.Fatalf("records = %+v, want %+v", addresses.records, want)
	}
}

// A re-promotion reuses the address the page already carries: the allocator is not asked and the address map
// is not written again, however the map is wired.
func TestAllocateAddress_AReusedAddressRecordsNothing(t *testing.T) {
	vaultRoot := t.TempDir()
	writePageCarrying(t, vaultRoot, "c-000099", 101)
	allocator := &countingAddresses{AddressAllocator: staticAddress(testAddress)}
	addresses := &memAddressMap{}

	address, err := allocateAddress(t.Context(), vaultRoot, "labdrian-sdd-overlay", 101, allocator, addresses, testInstant)
	if err != nil {
		t.Fatalf("allocateAddress: %v", err)
	}
	if address != "c-000099" || allocator.asked != 0 {
		t.Fatalf("address = %q after %d allocator calls, want the reused c-000099 and none", address, allocator.asked)
	}
	if len(addresses.records) != 0 {
		t.Errorf("a reused address was recorded again: %+v", addresses.records)
	}

	if address, err := allocateAddress(t.Context(), vaultRoot, "labdrian-sdd-overlay", 101, nil, nil, testInstant); err != nil || address != "c-000099" {
		t.Errorf("allocateAddress with neither port = (%q, %v), want the page's own address", address, err)
	}
}

// An allocator that fails, or answers nothing, has allocated nothing, so nothing is entered in the map.
func TestAllocateAddress_NothingAllocatedRecordsNothing(t *testing.T) {
	for name, allocator := range map[string]AddressAllocator{
		"the allocator fails":     failingAddresses{err: errors.New("the counter is locked")},
		"the allocator says none": failingAddresses{},
	} {
		t.Run(name, func(t *testing.T) {
			addresses := &memAddressMap{}
			if address, err := allocateAddress(t.Context(), t.TempDir(), "labdrian-sdd-overlay", 101, allocator, addresses, testInstant); err == nil {
				t.Fatalf("allocateAddress = %q, nil error, want a failure", address)
			}
			if len(addresses.records) != 0 {
				t.Errorf("records = %+v, want none", addresses.records)
			}
		})
	}
}

// The recorder is a port the caller wires. One that is missing is named, but only when an address is needed,
// and before the allocator is asked: an address spent on a promotion that cannot enter it in the map is spent
// for nothing.
func TestAllocateAddress_AMissingRecorderIsRefusedBeforeAnAddressIsSpent(t *testing.T) {
	var nilPointer *memAddressMap
	var nilFunc recorderFunc
	for name, recorder := range map[string]AddressMapRecorder{"absent": nil, "a nil pointer": nilPointer, "a nil function": nilFunc} {
		t.Run(name, func(t *testing.T) {
			allocator := &countingAddresses{AddressAllocator: staticAddress(testAddress)}

			address, err := allocateAddress(t.Context(), t.TempDir(), "labdrian-sdd-overlay", 101, allocator, recorder, testInstant)
			if !errors.Is(err, errNoAddressMapRecorder) {
				t.Fatalf("allocateAddress = (%q, %v), want errNoAddressMapRecorder", address, err)
			}
			if allocator.asked != 0 {
				t.Errorf("the allocator was asked %d time(s) before the missing recorder was noticed", allocator.asked)
			}
		})
	}
}

// A recorder that fails is promotion's failure, as the recorder reported it.
func TestAllocateAddress_ARecorderFailureReachesTheCaller(t *testing.T) {
	boom := errors.New("the manifest cannot be written")
	addresses := &memAddressMap{recordErr: boom}

	address, err := allocateAddress(t.Context(), t.TempDir(), "labdrian-sdd-overlay", 101, staticAddress(testAddress), addresses, testInstant)
	if !errors.Is(err, boom) || address != "" {
		t.Fatalf("allocateAddress = (%q, %v), want no address and the recorder's own error", address, err)
	}
}

// A promotion whose address cannot be entered in the map writes nothing at all, the Writer's half of the port.
func TestPromote_AnAddressTheMapCannotRecordMeansNothingIsWritten(t *testing.T) {
	vaultRoot := t.TempDir()
	store := PrecedenceStore{}
	repo := &memPrecedence{}
	w := &Writer{VaultRoot: vaultRoot, Store: store, Precedence: repo, Addresses: staticAddress(testAddress), AddressMap: &memAddressMap{recordErr: errors.New("no")}, Clock: &fakeClock{at: testInstant}}
	obs := memory.Observation{ID: 704, Type: "decision", Title: "Unrecordable", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}

	if result, err := w.Promote(t.Context(), obs, false); err == nil {
		t.Fatalf("Promote = %+v, nil error, want the failure to record the address", result)
	}
	if entries, err := os.ReadDir(vaultRoot); err != nil || len(entries) != 0 {
		t.Errorf("the vault holds %d entries (read err = %v), want none", len(entries), err)
	}
	if len(store) != 0 || repo.saves != 0 {
		t.Errorf("the precedence store holds %d entries after %d saves, want neither", len(store), repo.saves)
	}
}

// A Writer enters the page it creates in the map it was wired with, once, under the path the page is written
// to, at the instant its clock gave.
func TestPromote_EntersTheNewPageInTheAddressMap(t *testing.T) {
	addresses := &memAddressMap{}
	w := &Writer{VaultRoot: t.TempDir(), Store: PrecedenceStore{}, Precedence: &memPrecedence{}, Addresses: staticAddress(testAddress), AddressMap: addresses, Clock: &fakeClock{at: testInstant}}
	obs := memory.Observation{ID: 705, Type: "decision", Title: "Recorded", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}

	result, err := w.Promote(t.Context(), obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	want := addressRecord{path: result.Page.Path, address: testAddress, createdAt: testInstant}
	if len(addresses.records) != 1 || addresses.records[0] != want {
		t.Fatalf("records = %+v, want exactly %+v", addresses.records, want)
	}
}

// The page check reads the address map through its port: an entry that names the page passes, an entry that
// names another path, or none, is a finding, and a map that is not one is "not valid JSON" in the words the
// doctor has always used.
func TestLintPage_ChecksTheAddressMapItIsHanded(t *testing.T) {
	obs := memory.Observation{ID: 43, Type: "decision", Title: "Checked", Content: "Body.", Project: "labdrian-sdd-overlay"}
	page, err := EmitPage(obs, "c-000043", nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	addressMapDetails := func(addresses AddressMapReader) []string {
		var details []string
		for _, d := range LintPage(page, t.TempDir(), addresses) {
			if d.Rule == "address-map" {
				details = append(details, d.Detail)
			}
		}
		return details
	}
	expect := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("address-map details = %q, want %q", got, want)
		}
	}

	t.Run("an entry that names the page", func(t *testing.T) {
		expect(t, addressMapDetails(&memAddressMap{entries: AddressMap{page.Path: page.Address}}))
	})
	t.Run("an entry that names another path", func(t *testing.T) {
		expect(t, addressMapDetails(&memAddressMap{entries: AddressMap{"wiki/memory/elsewhere.md": page.Address}}),
			"address c-000043 maps to wiki/memory/elsewhere.md, not wiki/memory/c-000043.md")
	})
	t.Run("no entry", func(t *testing.T) {
		expect(t, addressMapDetails(&memAddressMap{entries: AddressMap{}}), "address c-000043 has no address_map entry")
	})
	t.Run("a map that is not one", func(t *testing.T) {
		expect(t, addressMapDetails(&memAddressMap{loadErr: corruptAddressMap()}), ".raw/.manifest.json is not valid JSON")
	})
	t.Run("a manifest that is not there passes", func(t *testing.T) {
		expect(t, addressMapDetails(&memAddressMap{}))
	})
	t.Run("a manifest that cannot be read passes", func(t *testing.T) {
		expect(t, addressMapDetails(&memAddressMap{loadErr: &fs.PathError{Op: "read", Path: ".raw/.manifest.json", Err: errors.New("is a directory")}}))
	})
	t.Run("a reader that is missing is named", func(t *testing.T) {
		var nilPointer *memAddressMap
		expect(t, addressMapDetails(nil), "promote: no address map reader was wired")
		expect(t, addressMapDetails(nilPointer), "promote: no address map reader was wired")
	})
}
