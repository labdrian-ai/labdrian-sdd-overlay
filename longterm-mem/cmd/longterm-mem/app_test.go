package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The app is what the commands build once, from what main composed them with, and ask for what they need to reach:
// the vault of a project, Engram's database, the use cases wired to both. These cases hold the three things every
// command used to spell out for itself and says the same way: what it says when it cannot go on, which vault it is
// given, and what it says when Engram cannot be opened.

func TestApp_RefuseSaysWhatEveryCommandSaysAndAnswersTheCode(t *testing.T) {
	a := commands{}.newApp("promote reconcile")

	var code int
	stderr := captureStderr(t, func() { code = a.refuse(errors.New("boom"), exitRegistrationConflict) })

	if code != exitRegistrationConflict {
		t.Errorf("refuse answered %d, want the code it was given, %d", code, exitRegistrationConflict)
	}
	if want := "longterm-mem: promote reconcile: boom\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestApp_VaultRootIsTheVaultTheCommandWasGiven(t *testing.T) {
	vaultRoot := t.TempDir()
	t.Setenv("LONGTERM_MEM_VAULT", "")

	var got string
	var code int
	stderr := captureStderr(t, func() { got, code = commands{}.newApp("sync").vaultRoot("any-project", vaultRoot) })

	if got != vaultRoot || code != exitOK || stderr != "" {
		t.Errorf("vaultRoot = (%q, %d) with stderr %q, want the vault it was given, %q, and silence", got, code, stderr, vaultRoot)
	}
}

// A project with no row in the registry is a vault that is not configured, a registry that cannot be read is the
// program's own fault, and each is said once, with the name of the command in front and the code of the contract.
func TestApp_VaultRootRefusesWithTheCodeOfTheFailure(t *testing.T) {
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("HOME", t.TempDir())
	registry := filepath.Join(t.TempDir(), "vaults.json")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", registry)

	for name, tc := range map[string]struct {
		registry string
		want     int
	}{
		"a project the registry has no row for": {`{"schema": 1, "vaults": {}}` + "\n", exitVaultNotConfigured},
		"a registry that cannot be read":        {"{ not json", exitInternal},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(registry, []byte(tc.registry), 0o600); err != nil {
				t.Fatal(err)
			}

			var code int
			stderr := captureStderr(t, func() { _, code = commands{}.newApp("status").vaultRoot("no-such-project", "") })

			if code != tc.want {
				t.Errorf("vaultRoot answered %d, want %d", code, tc.want)
			}
			if !strings.HasPrefix(stderr, "longterm-mem: status: ") || strings.Count(stderr, "\n") != 1 {
				t.Errorf("stderr = %q, want one line that begins with the command's name", stderr)
			}
		})
	}
}

func TestApp_MemoryOpensEngramsDatabase(t *testing.T) {
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", newTestEngramDBWithObservation(t, "app-project", "T", "C"))

	var code int
	var closer interface{ Close() error }
	stderr := captureStderr(t, func() {
		store, exit := commands{}.newApp("query").memory()
		code = exit
		if store != nil {
			closer = store
		}
	})

	if code != exitOK || closer == nil || stderr != "" {
		t.Fatalf("memory = (store %v, %d) with stderr %q, want an open store, exit 0 and silence", closer, code, stderr)
	}
	_ = closer.Close()
}

// An Engram database that cannot be opened is engram_unavailable (exit 4), said once with the name of the command
// in front and the adapter's own words after it.
func TestApp_MemoryRefusesWhenEngramCannotBeOpened(t *testing.T) {
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", filepath.Join(t.TempDir(), "no", "such", "directory", "engram.db"))

	var code int
	stderr := captureStderr(t, func() {
		store, exit := commands{}.newApp("stale").memory()
		code = exit
		if store != nil {
			t.Error("memory returned a store for a database that cannot be opened")
		}
	})

	if code != exitEngramUnavailable {
		t.Errorf("memory answered %d, want %d", code, exitEngramUnavailable)
	}
	if !strings.HasPrefix(stderr, "longterm-mem: stale: engram: connect ") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr = %q, want one line, the command's name and then the adapter's words", stderr)
	}
}

// The result of a command that prints JSON is the same two-space-indented document whichever command it is, and
// a value that cannot be encoded is an internal failure said with the command's name.
func TestApp_PrintJSONIndentsWithTwoSpacesAndRefusesWhatCannotBeEncoded(t *testing.T) {
	a := commands{}.newApp("doctor")

	var code int
	stdout := captureStdout(t, func() { code = a.printJSON(map[string]int{"checks": 8}) })
	if code != exitOK || stdout != "{\n  \"checks\": 8\n}\n" {
		t.Errorf("printJSON = %d with stdout %q, want the indented document and exit 0", code, stdout)
	}

	stderr := captureStderr(t, func() { code = a.printJSON(make(chan int)) })
	if code != exitInternal || !strings.HasPrefix(stderr, "longterm-mem: doctor: encode result: ") {
		t.Errorf("printJSON of a channel = %d with stderr %q, want exit %d and the encode failure", code, stderr, exitInternal)
	}
}
