package execrunner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeBinary writes an executable shell script called name into a fresh directory, names that
// directory as the PATH of the test, and returns the script's path. The scripts are POSIX shell.
func fakeBinary(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake binaries are shell scripts")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func TestLookPathFindsTheProgramOnThePath(t *testing.T) {
	want := fakeBinary(t, "pi", "exit 0")
	got, err := New().LookPath("pi")
	if err != nil {
		t.Fatalf("LookPath: %v", err)
	}
	if got != want {
		t.Errorf("LookPath = %q, want %q", got, want)
	}
}

func TestLookPathReportsAMissingProgram(t *testing.T) {
	if _, err := New().LookPath("pi"); err == nil {
		t.Fatal("LookPath found a pi on an empty PATH")
	}
}

func TestRunReturnsTheCombinedOutput(t *testing.T) {
	bin := fakeBinary(t, "pi", `printf 'out\n'; printf 'err\n' >&2`)
	out, err := New().Run(context.Background(), bin)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "out\nerr\n" {
		t.Errorf("output = %q, want both streams", out)
	}
}

// The arguments reach the program one by one and as they are: nothing is interpreted by a shell.
func TestRunPassesTheArgumentsUninterpreted(t *testing.T) {
	bin := fakeBinary(t, "pi", `for a in "$@"; do printf '[%s]' "$a"; done`)
	hostile := `dir; $(touch injected); ` + "`id`" + ` \`
	out, err := New().Run(context.Background(), bin, "install", hostile)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := "[install][" + hostile + "]"; string(out) != want {
		t.Errorf("the program saw %q, want %q", out, want)
	}
}

// A failing program is an error that still carries what it printed, so the adapter can tell the
// person why.
func TestRunKeepsTheOutputOfAFailingProgram(t *testing.T) {
	bin := fakeBinary(t, "pi", `printf 'no such package\n'; exit 3`)
	out, err := New().Run(context.Background(), bin, "install", "x")
	if err == nil {
		t.Fatal("Run reported success for a program that exited 3")
	}
	if strings.TrimSpace(string(out)) != "no such package" {
		t.Errorf("output = %q, want what the program printed", out)
	}
	if !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("error = %q, want it to name the exit status", err)
	}
}

func TestRunReportsAProgramThatCannotStart(t *testing.T) {
	if _, err := New().Run(context.Background(), filepath.Join(t.TempDir(), "nothing-here")); err == nil {
		t.Fatal("Run reported success for a program that does not exist")
	}
}

// The deadline stops a program that never ends, and the error says it was the deadline.
func TestRunStopsAProgramAtItsDeadline(t *testing.T) {
	bin := fakeBinary(t, "pi", `exec /bin/sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := New().Run(ctx, bin, "install", "x")
	if err == nil {
		t.Fatal("Run reported success for a program stopped at its deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to be the deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("Run took %v to give up on a 200ms deadline", elapsed)
	}
}

// A deadline that has passed before the program starts starts nothing.
func TestRunStartsNothingAfterTheDeadline(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	bin := fakeBinary(t, "pi", `: > '`+marker+`'`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New().Run(ctx, bin); err == nil {
		t.Fatal("Run reported success on a cancelled context")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the program ran although its context was already over")
	}
}
