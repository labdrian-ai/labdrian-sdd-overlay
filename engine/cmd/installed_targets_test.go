package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	ownershipapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
	ownershipfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/fsstore"
)

// Tests of the command line of 'installed-targets', of the exit codes it ends with and of what it
// says. What is decided about the record is tested where it is decided (engine/runtime/ownership);
// here is what the command takes, how it takes turns with another writer, and its words, which are the
// overlay script's, kept as they were because the script's output is pinned and read by people.

// targetsRun is one run of the command: what it wrote on each stream and the exits it asked for.
type targetsRun struct {
	stdout, stderr string
	exits          []int
}

// quickWiring is the production wiring with a short wait for a lock that is taken.
func quickWiring() ownershipWiring {
	return func(stateDir string) (*ownershipapp.Service, ownershipfs.Layout) {
		store := ownershipfs.New(stateDir)
		store.LockOptions = filelock.Options{Wait: 40 * time.Millisecond}
		return ownershipapp.New(store, ownershipfs.Binaries{}, capability.IsTarget), store.Layout
	}
}

func runTargetsWith(wire ownershipWiring, args ...string) targetsRun {
	var stdout, stderr bytes.Buffer
	var exits []int
	runInstalledTargetsCore(wire, args, &stdout, &stderr, func(code int) { exits = append(exits, code) })
	return targetsRun{stdout.String(), stderr.String(), exits}
}

func runTargets(args ...string) targetsRun { return runTargetsWith(quickWiring(), args...) }

// targetsWorld is a state directory that has nothing in it.
type targetsWorld struct {
	state  string
	layout ownershipfs.Layout
	binary string
}

func newTargetsWorld(t *testing.T) targetsWorld {
	t.Helper()
	state := filepath.Join(t.TempDir(), "state")
	return targetsWorld{state: state, layout: ownershipfs.Layout{StateDir: state}, binary: filepath.Join(state, "bin", "longterm-mem")}
}

// args is the command line of a verb with the state directory of the world.
func (w targetsWorld) args(verb string, more ...string) []string {
	return append([]string{verb, "--state-dir", w.state}, more...)
}

func (w targetsWorld) seed(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(w.layout.RecordDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.layout.Record(), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w targetsWorld) placeBinary(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (w targetsWorld) record(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(w.layout.Record())
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func (w targetsWorld) has(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// unreadable makes the record unreadable, or skips the test for a user who can read anything.
func (w targetsWorld) unreadable(t *testing.T) {
	t.Helper()
	if err := os.Chmod(w.layout.Record(), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(w.layout.Record(), 0o644) })
	if f, err := os.Open(w.layout.Record()); err == nil {
		f.Close()
		t.Skip("this user can read a file whose mode is 0")
	}
}

func TestInstalledTargetsNeedsAVerbAndKnowsOnlyFive(t *testing.T) {
	run := runTargets()
	if want := "error: installed-targets requires a verb: record, release, list, assess, retire\n"; run.stderr != want || run.stdout != "" || !equalInts(run.exits, []int{1}) {
		t.Errorf("no verb = %q, %q, exits %v; want %q on stderr and exit 1", run.stdout, run.stderr, run.exits, want)
	}
	run = runTargets("frobnicate")
	if want := "error: unknown installed-targets verb \"frobnicate\"; expected record, release, list, assess or retire\n"; run.stderr != want || !equalInts(run.exits, []int{1}) {
		t.Errorf("an unknown verb = %q, exits %v; want %q", run.stderr, run.exits, want)
	}
}

func TestInstalledTargetsReadsItsArgumentsStrictlyAndTouchesNothingWhenItRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"record without a state directory", []string{"record", "--target", "claude"}, "error: installed-targets record requires --state-dir and --target\n"},
		{"record without a target", []string{"record", "--state-dir", "S"}, "error: installed-targets record requires --state-dir and --target\n"},
		{"release without a target", []string{"release", "--state-dir", "S"}, "error: installed-targets release requires --state-dir and --target\n"},
		{"list without a state directory", []string{"list"}, "error: installed-targets list requires --state-dir\n"},
		{"assess without a state directory", []string{"assess", "--purge"}, "error: installed-targets assess requires --state-dir\n"},
		{"retire without a binary", []string{"retire", "--state-dir", "S"}, "error: installed-targets retire requires --state-dir and --binary\n"},
		{"a flag nobody named", []string{"record", "--state-dir", "S", "--target", "claude", "--frob"}, "error: installed-targets record: unknown argument \"--frob\"\n"},
		{"a word that is no flag", []string{"list", "--state-dir", "S", "claude"}, "error: installed-targets list: unknown argument \"claude\"\n"},
		{"a flag of another verb: record takes no --purge", []string{"record", "--state-dir", "S", "--target", "claude", "--purge"}, "error: installed-targets record: unknown argument \"--purge\"\n"},
		{"a flag of another verb: assess takes no --binary", []string{"assess", "--state-dir", "S", "--binary", "b"}, "error: installed-targets assess: unknown argument \"--binary\"\n"},
		{"a flag of another verb: list takes no --target", []string{"list", "--state-dir", "S", "--target", "claude"}, "error: installed-targets list: unknown argument \"--target\"\n"},
		{"a flag with no value", []string{"record", "--state-dir", "S", "--target"}, "error: installed-targets record: flag \"--target\" requires a value\n"},
		{"a flag with an empty value", []string{"record", "--state-dir", "S", "--target", ""}, "error: installed-targets record: flag \"--target\" requires a value\n"},
		{"a flag in the place of a value", []string{"record", "--state-dir", "--target", "claude"}, "error: installed-targets record: flag \"--state-dir\" requires a value\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTargetsWorld(t)
			args := make([]string, len(tc.args))
			for i, a := range tc.args {
				if a == "S" {
					a = w.state
				}
				args[i] = a
			}
			run := runTargets(args...)
			if run.stderr != tc.want || run.stdout != "" || !equalInts(run.exits, []int{1}) {
				t.Errorf("stdout %q, stderr %q, exits %v; want only %q and exit 1", run.stdout, run.stderr, run.exits, tc.want)
			}
			if w.has(w.state) {
				t.Error("the state directory was made for a command line that was refused")
			}
		})
	}
}

func TestInstalledTargetsRecordAddsTheTargetOnceAndSaysNothing(t *testing.T) {
	w := newTargetsWorld(t)
	for _, target := range []string{"claude", "codex", "claude"} {
		run := runTargets(w.args("record", "--target", target)...)
		if run.stdout != "" || run.stderr != "" || len(run.exits) != 0 {
			t.Fatalf("record %s = %q, %q, exits %v; want it silent", target, run.stdout, run.stderr, run.exits)
		}
	}
	if got := w.record(t); got != "claude\ncodex\n" {
		t.Errorf("record = %q, want each target once, in the order they were recorded", got)
	}
}

func TestInstalledTargetsRecordRefusesATargetThisBuildCannotInstallForBeforeMakingAnything(t *testing.T) {
	w := newTargetsWorld(t)
	run := runTargets(w.args("record", "--target", "zed")...)
	if want := "ERROR: longterm-mem: refusing to record an unknown install target: zed\n"; run.stderr != want || !equalInts(run.exits, []int{1}) {
		t.Errorf("stderr %q, exits %v; want %q and exit 1", run.stderr, run.exits, want)
	}
	if w.has(w.state) {
		t.Error("the state directory was made for a target that was refused")
	}
}

func TestInstalledTargetsRecordAppendsToWhatTheRecordHeldAsTheScriptDid(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "\x00garbage\nopenc")
	if run := runTargets(w.args("record", "--target", "opencode")...); run.stderr != "" || len(run.exits) != 0 {
		t.Fatalf("stderr %q, exits %v", run.stderr, run.exits)
	}
	if got := w.record(t); got != "\x00garbage\nopencopencode\n" {
		t.Errorf("record = %q: the line is added after the bytes that were there", got)
	}
}

func TestInstalledTargetsReleaseTakesTheTargetOutAndRewritesTheRecordSorted(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "opencode\nclaude\nclaude\ncodex\n\x00junk\n")
	run := runTargets(w.args("release", "--target", "claude")...)
	if run.stdout != "" || run.stderr != "" || len(run.exits) != 0 {
		t.Fatalf("release = %q, %q, exits %v; want it silent", run.stdout, run.stderr, run.exits)
	}
	if got := w.record(t); got != "codex\nopencode\n" {
		t.Errorf("record = %q", got)
	}
	info, err := os.Stat(w.layout.Record())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v (%v), want 0600: the rewrite goes through a temporary file, as the script's did", info.Mode().Perm(), err)
	}
}

func TestInstalledTargetsReleaseWithNoRecordMakesNothing(t *testing.T) {
	w := newTargetsWorld(t)
	run := runTargets(w.args("release", "--target", "claude")...)
	if run.stdout != "" || run.stderr != "" || len(run.exits) != 0 {
		t.Errorf("release = %q, %q, exits %v; want it silent", run.stdout, run.stderr, run.exits)
	}
	if w.has(w.state) {
		t.Error("the state directory was made for a record that is not there")
	}
}

func TestInstalledTargetsListPrintsTheNamesTheRecordVouchesFor(t *testing.T) {
	w := newTargetsWorld(t)
	if run := runTargets(w.args("list")...); run.stdout != "" || run.stderr != "" || len(run.exits) != 0 {
		t.Errorf("list of no record = %q, %q, exits %v; want nothing", run.stdout, run.stderr, run.exits)
	}
	w.seed(t, "opencode\nzed\nclaude\nclaude\n\x00junk\nclau")
	run := runTargets(w.args("list")...)
	if run.stdout != "claude\nopencode\nzed\n" || run.stderr != "" || len(run.exits) != 0 {
		t.Errorf("list = %q, %q, exits %v; want the names, sorted, once each", run.stdout, run.stderr, run.exits)
	}
}

// readerWarning is what the script's reader said of a record it could not read, before anything else.
func readerWarning(w targetsWorld) string {
	return "WARN: longterm-mem: " + w.layout.Record() + " exists but cannot be read; refusing to read it as empty install state\n"
}

func TestInstalledTargetsSayWhatTheyDoWithARecordNobodyCanRead(t *testing.T) {
	t.Run("list fails", func(t *testing.T) {
		w := newTargetsWorld(t)
		w.seed(t, "claude\n")
		w.unreadable(t)
		run := runTargets(w.args("list")...)
		if run.stdout != "" || run.stderr != readerWarning(w) || !equalInts(run.exits, []int{1}) {
			t.Errorf("list = %q, %q, exits %v; want the warning, no name and exit 1", run.stdout, run.stderr, run.exits)
		}
	})
	t.Run("record refuses to append", func(t *testing.T) {
		w := newTargetsWorld(t)
		w.seed(t, "claude\n")
		w.unreadable(t)
		run := runTargets(w.args("record", "--target", "opencode")...)
		want := readerWarning(w) + "ERROR: longterm-mem: could not read " + w.layout.Record() + "; refusing to record opencode against install state that cannot be inspected\n"
		if run.stderr != want || !equalInts(run.exits, []int{1}) {
			t.Errorf("stderr %q, exits %v; want %q", run.stderr, run.exits, want)
		}
	})
	t.Run("release refuses to rewrite", func(t *testing.T) {
		w := newTargetsWorld(t)
		w.seed(t, "claude\nopencode\n")
		w.unreadable(t)
		run := runTargets(w.args("release", "--target", "claude")...)
		want := readerWarning(w) + "ERROR: longterm-mem: could not read " + w.layout.Record() + "; refusing to rewrite it from a file that could not be inspected\n"
		if run.stderr != want || !equalInts(run.exits, []int{1}) {
			t.Errorf("stderr %q, exits %v; want %q", run.stderr, run.exits, want)
		}
	})
}

// stillInstalled is the sentence the script said when targets remain.
func stillInstalled(names string) string {
	return "  longterm-mem: unregistered the requested target(s); still installed for: " + names + " (shared binary and engine-owned tracking left untouched -- re-run uninstall for the remaining target(s), or pass --purge, to remove them)\n"
}

func TestInstalledTargetsAssessSaysWhatAnUninstallComesTo(t *testing.T) {
	for _, tc := range []struct {
		name       string
		record     string // "-" for no record
		flags      []string
		wantStdout func(w targetsWorld) string
		wantStderr func(w targetsWorld) string
		wantExits  []int
	}{
		{"nothing is recorded: the registration record and the binary may go", "", nil, nil, nil, nil},
		{"a target is recorded", "claude\n", nil,
			func(targetsWorld) string { return stillInstalled("claude") }, nil, []int{3}},
		{"two targets are recorded, one to a line", "opencode\ncodex\n", nil,
			func(targetsWorld) string { return stillInstalled("codex\nopencode") }, nil, []int{3}},
		{"a name only a newer build knows holds the binary", "zed\n", nil,
			func(targetsWorld) string { return stillInstalled("zed") }, nil, []int{3}},
		{"garbage alone holds nothing", "\x00zed\nclau", nil, nil, nil, nil},
		{"there is no record", "-", nil, nil, func(w targetsWorld) string {
			return "WARN: longterm-mem: no install tracking found at " + w.layout.Record() + "; leaving the registration record and the shared binary untouched. Re-run with --purge to remove them anyway.\n"
		}, []int{3}},
		{"there is no record and purge forces", "-", []string{"--purge"}, nil, nil, nil},
		{"a target recorded and purge forces", "claude\n", []string{"--purge"}, nil, nil, nil},
		{"an unregister failed", "", []string{"--unregister-failed"}, nil, func(targetsWorld) string {
			return "WARN: longterm-mem: at least one target failed to unregister; leaving the registration record and the shared binary untouched (re-run uninstall for the failed target(s) once resolved, or pass --purge to force removal)\n"
		}, []int{3}},
		{"an unregister failed and purge forces", "claude\n", []string{"--unregister-failed", "--purge"}, nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTargetsWorld(t)
			if tc.record != "-" {
				w.seed(t, tc.record)
			}
			run := runTargets(w.args("assess", tc.flags...)...)
			wantOut, wantErr := "", ""
			if tc.wantStdout != nil {
				wantOut = tc.wantStdout(w)
			}
			if tc.wantStderr != nil {
				wantErr = tc.wantStderr(w)
			}
			if run.stdout != wantOut || run.stderr != wantErr || !equalInts(run.exits, tc.wantExits) {
				t.Errorf("assess = stdout %q, stderr %q, exits %v\nwant stdout %q, stderr %q, exits %v", run.stdout, run.stderr, run.exits, wantOut, wantErr, tc.wantExits)
			}
		})
	}
}

func TestInstalledTargetsAssessOfARecordNobodyCanRead(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "claude\n")
	w.unreadable(t)
	run := runTargets(w.args("assess")...)
	want := readerWarning(w) + "WARN: longterm-mem: the install tracking file at " + w.layout.Record() + " exists but could not be read, so whether any runtime still depends on the shared binary is unknown; leaving the registration record and the shared binary untouched. Fix its permissions and re-run uninstall, or pass --purge to remove them anyway.\n"
	if run.stderr != want || run.stdout != "" || !equalInts(run.exits, []int{3}) {
		t.Errorf("assess = %q, %q, exits %v; want %q and exit 3", run.stdout, run.stderr, run.exits, want)
	}
	purged := runTargets(w.args("assess", "--purge")...)
	if purged.stderr != readerWarning(w) || purged.stdout != "" || len(purged.exits) != 0 {
		t.Errorf("assess --purge = %q, %q, exits %v; want the reader's warning and the go-ahead", purged.stdout, purged.stderr, purged.exits)
	}
}

func TestInstalledTargetsAssessChangesNothingAndTakesNoLock(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "claude\n")
	held, err := filelock.Acquire(w.layout.Lock(), filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer held()
	run := runTargets(w.args("assess")...)
	if run.stdout != stillInstalled("claude") || !equalInts(run.exits, []int{3}) {
		t.Errorf("assess with the lock held by another = %q, exits %v; want the verdict: it is a look", run.stdout, run.exits)
	}
	if got := w.record(t); got != "claude\n" {
		t.Errorf("record = %q, want it untouched", got)
	}
}

func TestInstalledTargetsRetireRemovesTheBinaryAndTheRecordWhenNothingDependsOnThem(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "")
	w.placeBinary(t)
	run := runTargets(w.args("retire", "--binary", w.binary)...)
	if want := "==> longterm-mem binary removed: " + w.binary + "\n"; run.stdout != want || run.stderr != "" || len(run.exits) != 0 {
		t.Errorf("retire = %q, %q, exits %v; want %q", run.stdout, run.stderr, run.exits, want)
	}
	if w.has(w.binary) || w.has(w.layout.Record()) {
		t.Error("the binary or the record is still there")
	}
}

func TestInstalledTargetsRetireSaysWhenTheBinaryWasAlreadyGoneAndStillRemovesTheRecord(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "")
	run := runTargets(w.args("retire", "--binary", w.binary)...)
	if want := "==> longterm-mem binary already absent: " + w.binary + "\n"; run.stdout != want || run.stderr != "" {
		t.Errorf("retire = %q, %q; want %q", run.stdout, run.stderr, want)
	}
	if w.has(w.layout.Record()) {
		t.Error("the record is still there")
	}
}

func TestInstalledTargetsRetireLeavesEverythingWhileARuntimeMayDependOnTheBinary(t *testing.T) {
	for _, tc := range []struct{ name, record string }{
		{"a target is recorded", "opencode\n"},
		{"there is no record", "-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTargetsWorld(t)
			if tc.record != "-" {
				w.seed(t, tc.record)
			}
			w.placeBinary(t)
			run := runTargets(w.args("retire", "--binary", w.binary)...)
			if run.stdout != "" || run.stderr != "" || len(run.exits) != 0 {
				t.Errorf("retire = %q, %q, exits %v; want it silent", run.stdout, run.stderr, run.exits)
			}
			if !w.has(w.binary) {
				t.Error("the binary was removed")
			}
		})
	}
}

func TestInstalledTargetsRetireWithPurgeRemovesWhateverTheRecordSays(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "claude\nopencode\n")
	w.placeBinary(t)
	run := runTargets(w.args("retire", "--binary", w.binary, "--purge")...)
	if want := "==> longterm-mem binary removed: " + w.binary + "\n"; run.stdout != want {
		t.Errorf("stdout %q, want %q", run.stdout, want)
	}
	if w.has(w.binary) || w.has(w.layout.Record()) {
		t.Error("the binary or the record is still there")
	}
}

func TestInstalledTargetsRetireHoldsTheBinaryWhenTheRecordCannotBeRead(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "claude\n")
	w.placeBinary(t)
	w.unreadable(t)
	run := runTargets(w.args("retire", "--binary", w.binary)...)
	want := readerWarning(w) + "WARN: longterm-mem: the install tracking file could not be read, so whether any runtime still depends on the shared binary is unknown; leaving " + w.binary + " in place. Fix its permissions and re-run uninstall, or pass --purge to remove it anyway.\n"
	if run.stderr != want || run.stdout != "" || len(run.exits) != 0 {
		t.Errorf("retire = %q, %q, exits %v; want %q and no exit", run.stdout, run.stderr, run.exits, want)
	}
	if !w.has(w.binary) {
		t.Error("the binary was removed although the record could not be read")
	}
}

// faultyStore is the real store with a step that fails.
type faultyStore struct {
	*ownershipfs.Store
	lockErr, appendErr, replaceErr, deleteErr error
}

func (f faultyStore) Lock() (func(), error) {
	if f.lockErr != nil {
		return nil, f.lockErr
	}
	return f.Store.Lock()
}

func (f faultyStore) Append(name string) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	return f.Store.Append(name)
}

func (f faultyStore) Replace(content []byte) error {
	if f.replaceErr != nil {
		return f.replaceErr
	}
	return f.Store.Replace(content)
}

func (f faultyStore) Delete() error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.Store.Delete()
}

func faultyWiring(mutate func(*faultyStore)) ownershipWiring {
	return func(stateDir string) (*ownershipapp.Service, ownershipfs.Layout) {
		store := ownershipfs.New(stateDir)
		store.LockOptions = filelock.Options{Wait: 40 * time.Millisecond}
		faulty := faultyStore{Store: store}
		mutate(&faulty)
		return ownershipapp.New(faulty, ownershipfs.Binaries{}, capability.IsTarget), store.Layout
	}
}

func TestInstalledTargetsSaysEachFailureInTheWordsOfTheScriptAndEndsWithExit1(t *testing.T) {
	boom := errors.New("boom")
	w := newTargetsWorld(t)
	for _, tc := range []struct {
		name   string
		mutate func(*faultyStore)
		verb   string
		more   []string
		seed   string
		want   string
	}{
		{"the lock cannot be opened", func(f *faultyStore) { f.lockErr = boom }, "record", []string{"--target", "claude"}, "-",
			"ERROR: longterm-mem: could not take the install-tracking lock at " + w.layout.Lock() + ": boom\n"},
		{"the state directory of the lock cannot be made", func(f *faultyStore) { f.lockErr = &ownershipapp.StateDirError{Err: boom} }, "retire", []string{"--binary", w.binary}, "-",
			"ERROR: longterm-mem: could not create " + w.state + ": boom\n"},
		{"the append fails", func(f *faultyStore) { f.appendErr = boom }, "record", []string{"--target", "claude"}, "-",
			"ERROR: longterm-mem: could not record claude in " + w.layout.Record() + ": boom\n"},
		{"the temporary file cannot be made", func(f *faultyStore) {
			f.replaceErr = &ownershipapp.ReplaceError{Stage: ownershipapp.StageCreate, Err: boom}
		}, "release", []string{"--target", "claude"}, "claude\n",
			"ERROR: longterm-mem: could not create a temporary file in " + w.layout.RecordDir() + ": boom\n"},
		{"the new record cannot be written", func(f *faultyStore) {
			f.replaceErr = &ownershipapp.ReplaceError{Stage: ownershipapp.StageWrite, Err: boom}
		}, "release", []string{"--target", "claude"}, "claude\n",
			"ERROR: longterm-mem: could not rewrite " + w.layout.Record() + ": boom\n"},
		{"the new record cannot take the place of the old", func(f *faultyStore) {
			f.replaceErr = &ownershipapp.ReplaceError{Stage: ownershipapp.StagePublish, Err: boom}
		}, "release", []string{"--target", "claude"}, "claude\n",
			"ERROR: longterm-mem: could not update " + w.layout.Record() + ": boom\n"},
		{"the record cannot be removed", func(f *faultyStore) { f.deleteErr = boom }, "retire", []string{"--binary", w.binary}, "",
			"ERROR: longterm-mem: could not remove " + w.layout.Record() + ": boom\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Every case starts from a state directory of its own making: the words name its paths.
			if err := os.RemoveAll(w.state); err != nil {
				t.Fatal(err)
			}
			if tc.seed != "-" {
				w.seed(t, tc.seed)
			}
			run := runTargetsWith(faultyWiring(tc.mutate), w.args(tc.verb, tc.more...)...)
			if run.stderr != tc.want || run.stdout != "" || !equalInts(run.exits, []int{1}) {
				t.Errorf("stdout %q, stderr %q, exits %v\nwant only %q and exit 1", run.stdout, run.stderr, run.exits, tc.want)
			}
		})
	}
}

func TestInstalledTargetsRefusesAfterTheWaitWhenAnotherWriterKeepsTheLockAndChangesNothing(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "claude\nopencode\n")
	held, err := filelock.Acquire(w.layout.Lock(), filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer held()
	for _, args := range [][]string{
		w.args("record", "--target", "codex"),
		w.args("release", "--target", "claude"),
		w.args("retire", "--binary", w.binary, "--purge"),
	} {
		start := time.Now()
		run := runTargets(args...)
		want := "ERROR: longterm-mem: could not acquire the install-tracking lock at " + w.layout.Lock() + "; another overlay invocation is updating install state. Nothing was changed — re-run once it finishes.\n"
		if run.stderr != want || run.stdout != "" || !equalInts(run.exits, []int{1}) {
			t.Errorf("%s = %q, %q, exits %v; want %q and exit 1", args[0], run.stdout, run.stderr, run.exits, want)
		}
		if time.Since(start) > 5*time.Second {
			t.Errorf("%s waited %v, want about the bound", args[0], time.Since(start))
		}
	}
	if got := w.record(t); got != "claude\nopencode\n" {
		t.Errorf("record = %q, want it untouched", got)
	}
}

func TestInstalledTargetsRecordStopsWhenTheDirectoryOfTheRecordCannotBeMade(t *testing.T) {
	w := newTargetsWorld(t)
	if err := os.MkdirAll(w.state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.layout.RecordDir(), []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := runTargets(w.args("record", "--target", "claude")...)
	if want := "ERROR: longterm-mem: could not create " + w.layout.RecordDir() + ": not a directory\n"; run.stderr != want || !equalInts(run.exits, []int{1}) {
		t.Errorf("stderr %q, exits %v; want %q (the directory named once, then the cause) and exit 1", run.stderr, run.exits, want)
	}
}

func TestInstalledTargetsRetireNamesTheBinaryItCannotRemoveAndKeepsTheRecord(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "")
	// A directory where the binary should be: removing the program is not removing a directory.
	if err := os.MkdirAll(w.binary, 0o755); err != nil {
		t.Fatal(err)
	}
	run := runTargets(w.args("retire", "--binary", w.binary)...)
	if want := "ERROR: longterm-mem: could not remove " + w.binary + ": is a directory\n"; run.stderr != want || !equalInts(run.exits, []int{1}) {
		t.Errorf("stderr %q, exits %v; want %q and exit 1", run.stderr, run.exits, want)
	}
	if !w.has(w.layout.Record()) {
		t.Error("the record was removed although the binary was not")
	}
}

func TestCauseIsWhatTheSystemSaidWithoutTheOperationAndThePathItRepeats(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"an error of a path", &fs.PathError{Op: "open", Path: "/state/longterm-mem/.installed-targets.123", Err: syscall.EACCES}, "permission denied"},
		{"an error of a link", &os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EACCES}, "permission denied"},
		{"an error of a path inside another", fmt.Errorf("wrapped: %w", &fs.PathError{Op: "mkdir", Path: "/x", Err: syscall.ENOTDIR}), "not a directory"},
		{"any other error", boom, "boom"},
	} {
		if got := cause(tc.err).Error(); got != tc.want {
			t.Errorf("%s: cause = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestInstalledTargetsLetGoOfTheLockWhenTheyEnd(t *testing.T) {
	w := newTargetsWorld(t)
	w.seed(t, "claude\n")
	for _, args := range [][]string{
		w.args("record", "--target", "codex"),
		w.args("release", "--target", "codex"),
		w.args("record", "--target", "zed"),
		w.args("retire", "--binary", w.binary),
	} {
		runTargets(args...)
		release, err := filelock.Acquire(w.layout.Lock(), filelock.Options{NoWait: true})
		if err != nil {
			t.Fatalf("the lock is still taken after %s: %v", args[0], err)
		}
		release()
	}
}

func TestInstalledTargetsKeepTheRecordUnderTheCleanedStateDirectory(t *testing.T) {
	w := newTargetsWorld(t)
	messy := w.state + "//deeper/../"
	if run := runTargets("record", "--state-dir", messy, "--target", "claude"); run.stderr != "" {
		t.Fatalf("stderr %q", run.stderr)
	}
	if got := w.record(t); got != "claude\n" {
		t.Errorf("record = %q, want it under the clean state directory", got)
	}
	w2 := newTargetsWorld(t)
	w2.seed(t, "claude\n")
	run := runTargets("assess", "--state-dir", w2.state+"/./")
	if run.stdout != stillInstalled("claude") {
		t.Errorf("assess = %q", run.stdout)
	}
}

// The subcommand as the program runs it: routed by run, wired to the file system, and knowing the
// runtimes the engine knows.
func TestInstalledTargetsRunsOverTheFilesOfTheMachineAndKnowsTheRuntimesOfTheEngine(t *testing.T) {
	w := newTargetsWorld(t)
	for _, name := range []string{"claude", "opencode", "codex", "pi"} {
		p := newCapturedProcess()
		run(p.process, testDeps(), []string{"installed-targets", "record", "--state-dir", w.state, "--target", name})
		if p.err.String() != "" || len(p.exits) != 0 {
			t.Fatalf("record %s = stderr %q, exits %v", name, p.err.String(), p.exits)
		}
	}
	if got := w.record(t); got != "claude\nopencode\ncodex\npi\n" {
		t.Errorf("record = %q, want the four runtimes of the engine", got)
	}
	p := newCapturedProcess()
	run(p.process, testDeps(), []string{"installed-targets", "record", "--state-dir", w.state, "--target", "cursor"})
	if want := "ERROR: longterm-mem: refusing to record an unknown install target: cursor\n"; p.err.String() != want || !equalInts(p.exits, []int{1}) {
		t.Errorf("record cursor = stderr %q, exits %v; want %q and exit 1", p.err.String(), p.exits, want)
	}
	if got := w.record(t); got != "claude\nopencode\ncodex\npi\n" {
		t.Errorf("record = %q after a refusal, want it as it was", got)
	}
}

func TestInstalledTargetsIsDocumentedInTheUsageAndTheREADME(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	for name, text := range map[string]string{
		"README":  docSection(t, string(readme), "gentle-ai-overlay installed-targets", "gentle-ai-overlay status [--only registry]"),
		"usage()": docSection(t, captureUsage(t), "  engine installed-targets", "  engine skills <verb>"),
	} {
		for _, want := range []string{
			"record", "release", "list", "assess", "retire",
			"--state-dir <dir>", "--target <name>", "--binary <path>", "--purge", "--unregister-failed",
			"longterm-mem/installed-targets", ".installed-targets.flock", "exit 3", "ERROR: longterm-mem:",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s of installed-targets does not say %q", name, want)
			}
		}
	}
}
