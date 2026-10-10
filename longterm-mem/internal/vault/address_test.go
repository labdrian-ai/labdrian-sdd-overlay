package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	address, err := AddressAllocator{Root: root}.NextAddress(t.Context())
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

	first, err := allocator.NextAddress(t.Context())
	if err != nil {
		t.Fatalf("NextAddress (first): %v", err)
	}
	second, err := allocator.NextAddress(t.Context())
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

	address, err := AddressAllocator{Root: root}.NextAddress(t.Context())
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

	address, err := AddressAllocator{Root: root}.NextAddress(t.Context())
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
	_, err := AddressAllocator{Root: t.TempDir()}.NextAddress(t.Context())
	if err == nil {
		t.Fatal("NextAddress = nil error, want an error for a vault with no allocator script")
	}
	if !strings.Contains(err.Error(), "allocate address") {
		t.Errorf("error %q does not say what was being done", err)
	}
}

// The address becomes a file name and a manifest key, so stdout that is more than one line (a warning
// ahead of the address, say) is a script that did not answer as the contract says, and is refused
// instead of being handed on as the address. A trailing newline, or surrounding blanks, are not lines.
func TestAddressAllocatorRefusesOutputOfMoreThanOneLine(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\necho 'warning: the counter file was recreated'\necho c-000001\n")

	address, err := AddressAllocator{Root: root}.NextAddress(t.Context())
	if err == nil {
		t.Fatalf("NextAddress = %q, nil error, want a script that printed two lines refused", address)
	}
	if address != "" {
		t.Errorf("address = %q alongside an error, want none", address)
	}
	if !strings.Contains(err.Error(), "more than one line") {
		t.Errorf("error %q does not say the script printed more than one line", err)
	}
}

// A script that hangs is stopped by the adapter's own timeout when the caller set no deadline: the call
// returns, with the script killed, and the error says the deadline passed. The timeout is the field the
// test shortens; the production bound is the same code path with allocateTimeout in it.
func TestAddressAllocatorStopsAHungScriptAtItsOwnTimeout(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\nexec sleep 30\n")

	started := time.Now()
	address, err := AddressAllocator{Root: root, Timeout: 200 * time.Millisecond}.NextAddress(t.Context())
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("NextAddress took %v: the script was not killed at the timeout", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("NextAddress = (%q, %v), want an error that is context.DeadlineExceeded", address, err)
	}
}

// A caller that cancels, or whose deadline passes, ends the allocation: the script is killed with its
// children (the exec'd sleep would otherwise outlive the test by half a minute) and the error says the
// context ended, not that the script exited 124.
func TestAddressAllocatorStopsWhenTheCallersContextEnds(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\nexec sleep 30\n")
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	address, err := AddressAllocator{Root: root}.NextAddress(ctx)
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("NextAddress took %v: the script was not killed when the context ended", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("NextAddress = (%q, %v), want an error that is context.DeadlineExceeded", address, err)
	}
	if address != "" {
		t.Errorf("address = %q alongside an error, want none", address)
	}
	if !strings.Contains(err.Error(), "allocate address") {
		t.Errorf("error %q does not say what was being done", err)
	}
}

// An explicit cancellation is not a deadline, and it ends the allocation the same way: the script is
// killed and the error is context.Canceled, not "exited 124".
func TestAddressAllocatorStopsWhenTheCallerCancels(t *testing.T) {
	root := t.TempDir()
	// The script says it is running before it hangs, and the caller cancels only once it has said so:
	// the cancellation reaches a live script, not one that has yet to start, whatever the machine's
	// speed. The outer deadline keeps a script that never starts from hanging the test, and it would
	// surface as DeadlineExceeded, not as the Canceled this test expects.
	marker := filepath.Join(root, "running")
	writeAllocator(t, root, "#!/bin/sh\n: > "+marker+"\nexec sleep 30\n")
	bounded, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	ctx, cancel := context.WithCancel(bounded)
	defer cancel()
	go func() {
		for bounded.Err() == nil {
			if _, err := os.Stat(marker); err == nil {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	started := time.Now()
	address, err := AddressAllocator{Root: root}.NextAddress(ctx)
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("NextAddress took %v: the script was not killed when the caller cancelled", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NextAddress = (%q, %v), want an error that is context.Canceled", address, err)
	}
	if address != "" {
		t.Errorf("address = %q alongside an error, want none", address)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("the script never ran (%v): the cancellation was not delivered to a live script", statErr)
	}
}

// A script that exits 124 by itself, with its context still live, is an ordinary failure: 124 is the code
// the runner reports for a killed script, but only the context says that one was.
func TestAddressAllocatorTreatsAnExit124WithALiveContextAsAnOrdinaryFailure(t *testing.T) {
	root := t.TempDir()
	writeAllocator(t, root, "#!/bin/sh\necho 'gave up by itself' >&2\nexit 124\n")

	address, err := AddressAllocator{Root: root}.NextAddress(t.Context())
	if err == nil {
		t.Fatalf("NextAddress = %q, nil error, want the failure of the script", address)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Errorf("error %v claims the context ended; it did not", err)
	}
	for _, want := range []string{"exited 124", "gave up by itself"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
