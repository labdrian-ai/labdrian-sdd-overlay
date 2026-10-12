package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

const promotePackage = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"

// The Writer every command that promotes uses is built in one place, with its vault, its address allocator, its
// clock and the repository its precedence store is paired with. Promote and sync used to each spell the
// literal out.
func TestNewPromoteWriterWiresTheWriterOfAVault(t *testing.T) {
	vaultRoot := t.TempDir()

	clock := fixedClock{at: fixtureInstant}

	writer, err := newPromoteWriter(vaultRoot, clock)
	if err != nil {
		t.Fatalf("newPromoteWriter: %v", err)
	}
	if got, want := writer.Pages.PagesLocation(), filepath.Join(vaultRoot, "wiki", "memory"); got != want {
		t.Errorf("Pages are kept at %q, want the vault's, %q", got, want)
	}
	if got, want := writer.Index.IndexLocation(), filepath.Join(vaultRoot, "wiki", "index.md"); got != want {
		t.Errorf("Index is kept at %q, want the vault's, %q", got, want)
	}
	if got, want := writer.Log.LogLocation(), filepath.Join(vaultRoot, "wiki", "log.md"); got != want {
		t.Errorf("Log is kept at %q, want the vault's, %q", got, want)
	}
	if writer.SyncState == nil {
		t.Error("SyncState is not wired: the first sync this Writer runs would be refused")
	}
	if got, want := writer.Addresses, (promote.AddressAllocator)(vault.AddressAllocator{Root: vaultRoot}); got != want {
		t.Errorf("Addresses = %#v, want the vault's allocator %#v", got, want)
	}
	if writer.Clock != promote.Clock(clock) {
		t.Errorf("Clock = %#v, want the clock the Writer was given, %#v", writer.Clock, clock)
	}
	if writer.Precedence == nil || writer.Store == nil {
		t.Errorf("Precedence = %v, Store = %v, want the repository and the store it holds", writer.Precedence, writer.Store)
	}
	if writer.AddressMap == nil {
		t.Error("AddressMap is not wired: the first page this Writer creates would be refused")
	}
}

// A vault whose precedence sidecar cannot be parsed gives no Writer, and the failure reaches the caller as the
// repository reported it, with no consumer's name in front.
func TestNewPromoteWriterFailsWithTheSidecarsOwnError(t *testing.T) {
	vaultRoot := t.TempDir()
	writeCorruptSidecar(t, vaultRoot)

	writer, err := newPromoteWriter(vaultRoot, fixedClock{at: fixtureInstant})
	if err == nil || writer != nil {
		t.Fatalf("newPromoteWriter = (%v, %v), want no writer and the parse failure", writer, err)
	}
	want := "parse " + filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json") + ": "
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
}

// No command builds a promote.Writer itself: a Writer built twice is a Writer that drifts, and a Writer with
// a port forgotten is refused only when it is first used.
func TestCommandsBuildTheirWriterThroughNewPromoteWriter(t *testing.T) {
	program, _ := sourceFiles(t)
	for _, name := range program {
		if name == "rundeps.go" {
			continue
		}
		for _, at := range usesOf(t, name, promotePackage, "Writer") {
			t.Errorf("%s names promote.Writer; commands get their Writer from newPromoteWriter", at)
		}
	}
	if len(usesOf(t, "rundeps.go", promotePackage, "Writer")) == 0 {
		t.Error("rundeps.go no longer builds the promote.Writer: the guard looks for the wrong type")
	}
}
