package statestore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Which platforms have a store is decided where the code is built: Supported is true
// on linux and darwin and false everywhere else. The expectation is computed from the
// running platform and the input is what a store passes, so neither is a value the
// test chooses to make itself pass.
func TestRequirePlatformAcceptsExactlyThePlatformsThatHaveAStore(t *testing.T) {
	want := runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	if Supported != want {
		t.Fatalf("Supported = %v on %s, want %v: only linux and darwin have a no-follow read", Supported, runtime.GOOS, want)
	}
	sentinel := errors.New("some store: unsupported platform")
	err := RequirePlatform(sentinel)
	if want != (err == nil) {
		t.Fatalf("RequirePlatform on %s = %v, want it to accept exactly the platforms that have a store", runtime.GOOS, err)
	}
}

// A platform without a store is refused with the caller's own sentinel, so each store
// keeps the message and the errors.Is target it always had, and the platform is named.
func TestCheckPlatformRefusesAnUnsupportedPlatformWithTheCallersSentinel(t *testing.T) {
	sentinel := errors.New("some store: unsupported platform")
	for _, goos := range []string{"windows", "freebsd", "plan9"} {
		err := checkPlatform(false, goos, sentinel)
		want := "some store: unsupported platform: " + goos + " (supported: linux, darwin)"
		if !errors.Is(err, sentinel) || err.Error() != want {
			t.Errorf("checkPlatform(false, %q) = %v, want the sentinel wrapped as %q", goos, err, want)
		}
	}
	if err := checkPlatform(true, "linux", sentinel); err != nil {
		t.Errorf("checkPlatform(true, linux) = %v, want nil", err)
	}
}

// A store is built over a state home the composition root resolved; one that is not an
// absolute path would shape paths relative to whatever directory the process runs in.
func TestCheckHomeAcceptsOnlyAnAbsolutePath(t *testing.T) {
	if err := CheckHome(filepath.Join(string(filepath.Separator), "srv", "state")); err != nil {
		t.Errorf("CheckHome(absolute) = %v, want nil", err)
	}
	for _, home := range []string{"", "state", "./state", "../state"} {
		err := CheckHome(home)
		want := `state home "` + home + `" is not an absolute path`
		if err == nil || err.Error() != want {
			t.Errorf("CheckHome(%q) = %v, want %q (no store name: each caller adds its own)", home, err, want)
		}
	}
}

// A store makes the two checks together, in this order, and keeps what it printed when it
// made them by hand: the platform refusal is the caller's own sentinel wrapped with the
// platform, bare, and the state home refusal carries the store's name. The platform is
// judged first, so on a platform with no store even a relative state home is reported as
// the platform, never as the home.
func TestCheckStoreRefusesThePlatformBeforeTheStateHome(t *testing.T) {
	sentinel := errors.New("some store: unsupported platform")
	absolute := filepath.Join(string(filepath.Separator), "srv", "state")

	err := checkStore(false, "windows", "some store", sentinel, "relative/state")
	want := "some store: unsupported platform: windows (supported: linux, darwin)"
	if !errors.Is(err, sentinel) || err.Error() != want {
		t.Errorf("an unsupported platform with a relative home = %v, want the sentinel wrapped as %q", err, want)
	}

	for _, home := range []string{"", "state", "./state", "../state"} {
		err := checkStore(true, "linux", "some store", sentinel, home)
		want := `some store: state home "` + home + `" is not an absolute path`
		if err == nil || err.Error() != want || errors.Is(err, sentinel) {
			t.Errorf("checkStore(supported, %q) = %v, want %q and not the platform sentinel", home, err, want)
		}
	}

	if err := checkStore(true, "linux", "some store", sentinel, absolute); err != nil {
		t.Errorf("checkStore(supported, absolute home) = %v, want nil", err)
	}
}

// RequireStore is checkStore for the running platform: a store built on a supported
// platform over an absolute home is accepted, and a relative home is refused with the
// store's name.
func TestRequireStoreChecksTheRunningPlatformAndTheHome(t *testing.T) {
	skipUnlessSupported(t)
	sentinel := errors.New("some store: unsupported platform")
	if err := RequireStore("some store", sentinel, filepath.Join(string(filepath.Separator), "srv", "state")); err != nil {
		t.Errorf("RequireStore(absolute home) = %v, want nil", err)
	}
	err := RequireStore("some store", sentinel, "state")
	if want := `some store: state home "state" is not an absolute path`; err == nil || err.Error() != want {
		t.Errorf("RequireStore(relative home) = %v, want %q", err, want)
	}
}

// ReadFileSized reports the size of the file it opened, the same descriptor the bytes
// came from, so a caller that stopped at its limit can say how large the file is
// without a second look at a path that may have changed.
func TestReadFileSizedReportsTheSizeOfTheOpenedFileBeyondTheLimit(t *testing.T) {
	skipUnlessSupported(t)
	path := filepath.Join(t.TempDir(), "record")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	data, size, err := ReadFileSized(path, 11)
	if err != nil || len(data) != 11 || size != 100 {
		t.Fatalf("ReadFileSized(limit 11) = %d bytes, size %d, %v, want 11 bytes, size 100", len(data), size, err)
	}
	data, size, err = ReadFileSized(path, 0)
	if err != nil || len(data) != 100 || size != 100 {
		t.Fatalf("ReadFileSized(no limit) = %d bytes, size %d, %v, want 100 bytes, size 100", len(data), size, err)
	}
}

func TestReadFileSizedKeepsTheRefusalsOfReadFile(t *testing.T) {
	skipUnlessSupported(t)
	dir := t.TempDir()
	if _, _, err := ReadFileSized(filepath.Join(dir, "missing"), 0); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: error = %v, want fs.ErrNotExist", err)
	}
	if _, _, err := ReadFileSized(dir, 0); !errors.Is(err, ErrNotRegular) {
		t.Errorf("a directory: error = %v, want ErrNotRegular", err)
	}
}
