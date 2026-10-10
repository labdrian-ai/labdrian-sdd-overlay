package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAllocator materializes a fixture scripts/allocate-address.sh under root: a real shell
// entrypoint (shebang and exec bit), as the vault's own script is.
func writeAllocator(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "allocate-address.sh"), []byte(body), 0o755); err != nil {
		t.Fatalf("write the allocator fixture: %v", err)
	}
}

// The adapter runs the vault's allocator script from the vault root and returns what it printed, trimmed:
// the script ends its line, and the address does not carry the newline.
func TestAddressAllocatorReturnsTheAddressTheScriptPrints(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\necho c-000042\n")

	address, err := AddressAllocator{Root: root}.NextAddress()
	if err != nil {
		t.Fatalf("NextAddress: %v", err)
	}
	if address != "c-000042" {
		t.Fatalf("address = %q, want c-000042", address)
	}
}

// The script runs with the vault root as its working directory, which is how the real one finds its
// counter: two calls against a script that keeps a counter there give two different addresses.
func TestAddressAllocatorRunsTheScriptInTheVaultRoot(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\nn=0\n[ -f .counter ] && read n < .counter\nn=$((n+1))\necho \"$n\" > .counter\nprintf 'c-%06d\\n' \"$n\"\n")
	allocator := AddressAllocator{Root: root}

	first, err := allocator.NextAddress()
	if err != nil {
		t.Fatalf("NextAddress (first): %v", err)
	}
	second, err := allocator.NextAddress()
	if err != nil {
		t.Fatalf("NextAddress (second): %v", err)
	}
	if first != "c-000001" || second != "c-000002" {
		t.Fatalf("addresses = %q, %q, want c-000001 then c-000002", first, second)
	}
}

// A script that exits non-zero is a failure that names the script, the exit code and what it said on
// stderr; no address is returned with it.
func TestAddressAllocatorReportsAScriptThatFails(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\necho 'the counter is locked' >&2\nexit 3\n")

	address, err := AddressAllocator{Root: root}.NextAddress()
	if err == nil {
		t.Fatalf("NextAddress = %q, nil error, want the failure of the script", address)
	}
	if address != "" {
		t.Errorf("address = %q alongside an error, want none", address)
	}
	for _, want := range []string{"scripts/allocate-address.sh", "exited 3", "the counter is locked"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A script that succeeds and prints nothing allocated nothing: an empty address must not pass for one.
func TestAddressAllocatorReportsAScriptThatPrintsNoAddress(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\nprintf '  \\n'\n")

	address, err := AddressAllocator{Root: root}.NextAddress()
	if err == nil {
		t.Fatalf("NextAddress = %q, nil error, want an error for a script that printed nothing", address)
	}
	if !strings.Contains(err.Error(), "produced no address") {
		t.Errorf("error %q does not say the script produced no address", err)
	}
}

// A vault with no allocator script is not a vault this adapter can allocate in, and the runner's refusal
// is carried up, wrapped in what was being done.
func TestAddressAllocatorReportsAVaultWithoutTheScript(t *testing.T) {
	_, err := AddressAllocator{Root: t.TempDir()}.NextAddress()
	if err == nil {
		t.Fatal("NextAddress = nil error, want an error for a vault with no allocator script")
	}
	if !strings.Contains(err.Error(), "allocate address") {
		t.Errorf("error %q does not say what was being done", err)
	}
}
