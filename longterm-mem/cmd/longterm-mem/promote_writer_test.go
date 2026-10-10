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

	writer, err := newPromoteWriter(vaultRoot)
	if err != nil {
		t.Fatalf("newPromoteWriter: %v", err)
	}
	if writer.VaultRoot != vaultRoot {
		t.Errorf("VaultRoot = %q, want %q", writer.VaultRoot, vaultRoot)
	}
	if got, want := writer.Addresses, (promote.AddressAllocator)(vault.AddressAllocator{Root: vaultRoot}); got != want {
		t.Errorf("Addresses = %#v, want the vault's allocator %#v", got, want)
	}
	if _, ok := writer.Clock.(utcClock); !ok {
		t.Errorf("Clock = %T, want the UTC wall clock", writer.Clock)
	}
	if writer.Precedence == nil || writer.Store == nil {
		t.Errorf("Precedence = %v, Store = %v, want the repository and the store it holds", writer.Precedence, writer.Store)
	}
}

// A vault whose precedence sidecar cannot be parsed gives no Writer, and the failure reaches the caller as the
// repository reported it, under the prefix the commands' output carries.
func TestNewPromoteWriterFailsWithTheSidecarsOwnError(t *testing.T) {
	vaultRoot := t.TempDir()
	writeCorruptSidecar(t, vaultRoot)

	writer, err := newPromoteWriter(vaultRoot)
	if err == nil || writer != nil {
		t.Fatalf("newPromoteWriter = (%v, %v), want no writer and the parse failure", writer, err)
	}
	want := "promote: parse " + filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json") + ": "
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
