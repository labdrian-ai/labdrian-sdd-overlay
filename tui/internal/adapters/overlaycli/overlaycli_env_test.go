package overlaycli

// The environment the backend runs under. In the program the adapter is built
// with New, whose env is nil: every child process inherits the environment of
// the TUI (STATE_DIR and HOME must reach the backend unchanged, or the TUI would
// act on another state than the shell it was started from). Running an action
// and reading the launch probe took no environment of their own before the
// layers, and still take none; only the read-only questions (the catalog and the
// backups) can be given one, which is how the contract tests keep them off the
// real state. These tests pin both halves.

import (
	"os"
	"strings"
	"testing"
)

// pinned is the variable the scripts below print: a stand-in for STATE_DIR.
const pinned = "TUI_PINNED_VARIABLE"

// echoPinned is a backend that prints the variable as it sees it.
const echoPinned = `printf '%s' "$` + pinned + `"`

// withParentEnvironment sets the variable in the environment of the test
// process, as an operator's shell would have set STATE_DIR.
func withParentEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(pinned, "from-the-parent")
}

// cliWithAnEnvironment is an adapter whose env field is not nil: the test seam
// the contract tests use.
func cliWithAnEnvironment(root string) CLI {
	return CLI{root: root, env: []string{pinned + "=from-the-field"}}
}

func TestOverlayCLI_Run_RunsUnderTheEnvironmentOfTheCaller(t *testing.T) {
	withParentEnvironment(t)
	root := t.TempDir()
	writeScriptBackend(t, root, echoPinned)

	if got := New(root).Run([]string{"apply"}).Output; got != "from-the-parent" {
		t.Errorf("an action ran with %s = %q, want the caller's value: it must inherit the environment", pinned, got)
	}
}

func TestOverlayCLI_Report_RunsUnderTheEnvironmentOfTheCaller(t *testing.T) {
	withParentEnvironment(t)
	root := t.TempDir()
	writeScriptBackend(t, root, echoPinned)

	if got := New(root).Report(); got != "from-the-parent" {
		t.Errorf("the probe ran with %s = %q, want the caller's value: it must inherit the environment", pinned, got)
	}
}

// An adapter that carries an environment of its own still runs actions and the
// probe under the caller's: no seam can change what an action runs under.
func TestOverlayCLI_ActionsAndTheProbeNeverTakeTheEnvOfTheAdapter(t *testing.T) {
	withParentEnvironment(t)
	root := t.TempDir()
	writeScriptBackend(t, root, echoPinned)
	cli := cliWithAnEnvironment(root)

	if got := cli.Run([]string{"apply"}).Output; got != "from-the-parent" {
		t.Errorf("an action ran with %s = %q, want the caller's value, not the adapter's", pinned, got)
	}
	if got := cli.Report(); got != "from-the-parent" {
		t.Errorf("the probe ran with %s = %q, want the caller's value, not the adapter's", pinned, got)
	}
}

// The read-only questions are the one place the environment of the adapter is
// used, and with none they inherit the caller's.
func TestOverlayCLI_TheQuestionsTakeTheEnvOfTheAdapterWhenItHasOne(t *testing.T) {
	withParentEnvironment(t)
	catalog := `printf '%s\tcopy\n' "$` + pinned + `"`
	listing := `printf 'Backups for claude (newest last):\n  %s (v1)\n' "$` + pinned + `"`

	tests := []struct {
		name   string
		cli    func(root string) CLI
		script string
		answer func(t *testing.T, cli CLI) string
		want   string
	}{
		{"the catalog without an env", New, catalog, targetNameOf, "from-the-parent"},
		{"the catalog with an env", cliWithAnEnvironment, catalog, targetNameOf, "from-the-field"},
		{"the backups without an env", New, listing, backupTimestampOf, "from-the-parent"},
		{"the backups with an env", cliWithAnEnvironment, listing, backupTimestampOf, "from-the-field"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeScriptBackend(t, root, tc.script)
			if got := tc.answer(t, tc.cli(root)); got != tc.want {
				t.Errorf("the question ran with %s = %q, want %q", pinned, got, tc.want)
			}
		})
	}
}

func targetNameOf(t *testing.T, cli CLI) string {
	t.Helper()
	targets, err := cli.Targets()
	if err != nil || len(targets) != 1 {
		t.Fatalf("Targets = %+v, %v, want one target", targets, err)
	}
	return targets[0].Name
}

func backupTimestampOf(t *testing.T, cli CLI) string {
	t.Helper()
	backup, ok, err := cli.LatestBackup("claude")
	if err != nil || !ok {
		t.Fatalf("LatestBackup = %+v, %t, %v, want a backup", backup, ok, err)
	}
	return strings.TrimSpace(backup.Timestamp)
}

// The test process itself must not carry the variable the tests set: t.Setenv
// restores it, and this fails the run if something sets it for good.
func TestPinnedVariableIsNotSetOutsideTheTests(t *testing.T) {
	if value, set := os.LookupEnv(pinned); set {
		t.Errorf("%s = %q outside a test that sets it", pinned, value)
	}
}
