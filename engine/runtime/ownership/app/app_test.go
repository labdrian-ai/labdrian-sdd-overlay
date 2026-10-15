package app_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
)

// fakeStore is the record in memory. It logs every call, and holds that what is read to be changed
// and what changes it happen while the lock is taken. It models the two directories the real store
// works in, because the contract of the port depends on them: Lock makes the state directory (the
// lock lives there), MakeRecordDir makes the directory of the record, and Append and Replace fail in a
// directory that is not there, as a file system does, while Exists, Read and Delete need none (a
// record in a directory that is not there is absent, and removing it is already done).
type fakeStore struct {
	t         *testing.T
	exists    bool
	state     ownership.State
	content   []byte
	locked    bool
	stateDir  bool
	recordDir bool
	calls     []string

	recordDirErr error
	lockErr      error
	appendErr    error
	replaceErr   error
	deleteErr    error
	// afterLock runs when the lock has been taken, as a writer that got in just before would.
	afterLock func(*fakeStore)
}

// recordHolding is a store whose record is there, readable, with this content.
func recordHolding(t *testing.T, content string) *fakeStore {
	return &fakeStore{t: t, exists: true, state: ownership.Readable, content: []byte(content), stateDir: true, recordDir: true}
}

// noRecord is a store with no record.
func noRecord(t *testing.T) *fakeStore { return &fakeStore{t: t} }

func (f *fakeStore) log(call string) { f.calls = append(f.calls, call) }

func (f *fakeStore) mustBeLocked(op string) {
	f.t.Helper()
	if !f.locked {
		f.t.Errorf("%s ran without the lock", op)
	}
}

func (f *fakeStore) Exists() bool {
	f.log("exists")
	return f.exists
}

func (f *fakeStore) MakeRecordDir() error {
	f.log("make-record-dir")
	if f.recordDirErr != nil {
		return f.recordDirErr
	}
	f.stateDir, f.recordDir = true, true
	return nil
}

func (f *fakeStore) Lock() (func(), error) {
	f.log("lock")
	if f.lockErr != nil {
		return nil, f.lockErr
	}
	f.stateDir = true
	f.locked = true
	if f.afterLock != nil {
		f.afterLock(f)
	}
	return func() {
		f.log("release")
		f.locked = false
	}, nil
}

func (f *fakeStore) Read() (ownership.State, []byte) {
	f.log("read")
	return f.state, f.content
}

func (f *fakeStore) Append(name string) error {
	f.mustBeLocked("append")
	f.log("append:" + name)
	if f.appendErr != nil {
		return f.appendErr
	}
	if !f.recordDir {
		return errors.New("no such file or directory: the directory of the record was not made")
	}
	f.content = append(f.content, name+"\n"...)
	f.exists, f.state = true, ownership.Readable
	return nil
}

func (f *fakeStore) Replace(content []byte) error {
	f.mustBeLocked("replace")
	f.log("replace:" + string(content))
	if f.replaceErr != nil {
		return f.replaceErr
	}
	if !f.recordDir {
		return &app.ReplaceError{Stage: app.StageCreate, Err: errors.New("no such file or directory: the directory of the record was not made")}
	}
	f.content = content
	f.exists, f.state = true, ownership.Readable
	return nil
}

func (f *fakeStore) Delete() error {
	f.mustBeLocked("delete")
	f.log("delete")
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.exists, f.state, f.content = false, ownership.Absent, nil
	return nil
}

func (f *fakeStore) trace() string { return strings.Join(f.calls, " ") }

// fakeBinaries is the shared binary: a path that is there or is not.
type fakeBinaries struct {
	t         *testing.T
	store     *fakeStore
	there     bool
	removeErr error
	calls     []string
}

func (b *fakeBinaries) Exists(path string) bool {
	b.calls = append(b.calls, "exists:"+path)
	b.store.log("binary-exists")
	return b.there
}

func (b *fakeBinaries) Remove(path string) error {
	if !b.store.locked {
		b.t.Error("the binary was removed without the lock")
	}
	b.calls = append(b.calls, "remove:"+path)
	b.store.log("binary-remove")
	if b.removeErr != nil {
		return b.removeErr
	}
	b.there = false
	return nil
}

func knowsEveryRuntime(name string) bool {
	switch name {
	case "claude", "opencode", "codex", "pi":
		return true
	}
	return false
}

func service(store *fakeStore, binaryThere bool) (*app.Service, *fakeBinaries) {
	binaries := &fakeBinaries{t: store.t, store: store, there: binaryThere}
	return app.New(store, binaries, knowsEveryRuntime), binaries
}

func TestRecordAppendsANameTheRecordDoesNotHoldInsideOneLock(t *testing.T) {
	store := recordHolding(t, "claude\n")
	svc, _ := service(store, false)
	if err := svc.Record("codex"); err != nil {
		t.Fatalf("Record(codex) = %v", err)
	}
	if got, want := store.trace(), "make-record-dir lock read append:codex release"; got != want {
		t.Errorf("calls = %q, want %q: the record is read, decided on and written under one hold of the lock", got, want)
	}
	if string(store.content) != "claude\ncodex\n" {
		t.Errorf("record = %q, want the name appended to what it held", store.content)
	}
}

func TestRecordOfANameAlreadyThereWritesNothing(t *testing.T) {
	store := recordHolding(t, "codex\nclaude\nclaude\n")
	svc, _ := service(store, false)
	if err := svc.Record("claude"); err != nil {
		t.Fatalf("Record(claude) = %v", err)
	}
	if got, want := store.trace(), "make-record-dir lock read release"; got != want {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestRecordCreatesTheRecordWhenThereIsNone(t *testing.T) {
	store := noRecord(t)
	svc, _ := service(store, false)
	if err := svc.Record("pi"); err != nil {
		t.Fatal(err)
	}
	if string(store.content) != "pi\n" || !store.exists {
		t.Errorf("record = %q (exists %v), want pi", store.content, store.exists)
	}
}

func TestRecordReadsWhatTheRecordHoldsAsEvidenceBeforeDeciding(t *testing.T) {
	for _, tc := range []struct {
		name, content, record string
		appended              bool
	}{
		{"a name only a newer build knows is not the name", "zed\n", "claude", true},
		{"a name cut short is not the name", "claude\nopenc", "opencode", true},
		{"garbage is not the name", "\x00claude\n", "claude", true},
		{"the name among others", "zed\nclaude\n", "claude", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := recordHolding(t, tc.content)
			svc, _ := service(store, false)
			if err := svc.Record(tc.record); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(store.trace(), "append:"+tc.record); got != tc.appended {
				t.Errorf("appended = %v, want %v (calls %q)", got, tc.appended, store.trace())
			}
		})
	}
}

func TestRecordRefusesANameTheVocabularyDoesNotKnowBeforeTouchingAnything(t *testing.T) {
	store := recordHolding(t, "claude\n")
	svc, _ := service(store, false)
	err := svc.Record("zed")
	var unknown *ownership.UnknownTargetError
	if !errors.As(err, &unknown) || unknown.Name != "zed" {
		t.Fatalf("Record(zed) = %v, want an *UnknownTargetError naming zed", err)
	}
	if len(store.calls) != 0 {
		t.Errorf("calls = %q, want none: no directory is made and no lock is taken for a name that is refused", store.trace())
	}
}

func TestRecordStopsAtEachStepThatFailsAndLetsTheLockGo(t *testing.T) {
	boom := errors.New("boom")
	t.Run("the directory of the record cannot be made", func(t *testing.T) {
		store := recordHolding(t, "")
		store.recordDirErr = boom
		svc, _ := service(store, false)
		var dir *app.RecordDirError
		if err := svc.Record("claude"); !errors.As(err, &dir) || !errors.Is(err, boom) {
			t.Fatalf("Record = %v, want a *RecordDirError that wraps the cause", err)
		}
		if strings.Contains(store.trace(), "lock") {
			t.Errorf("calls = %q: the lock is not taken when there is no directory", store.trace())
		}
	})
	t.Run("the lock stays taken", func(t *testing.T) {
		store := recordHolding(t, "")
		store.lockErr = fmt.Errorf("%w (waited 2s)", app.ErrBusy)
		svc, _ := service(store, false)
		err := svc.Record("claude")
		var lock *app.LockError
		if !errors.As(err, &lock) || !errors.Is(err, app.ErrBusy) {
			t.Fatalf("Record = %v, want a *LockError that is ErrBusy", err)
		}
		if strings.Contains(store.trace(), "read") || strings.Contains(store.trace(), "append") {
			t.Errorf("calls = %q: nothing is read or written without the lock", store.trace())
		}
	})
	t.Run("the state directory of the lock cannot be made", func(t *testing.T) {
		store := recordHolding(t, "")
		store.lockErr = &app.StateDirError{Err: boom}
		svc, _ := service(store, false)
		var state *app.StateDirError
		err := svc.Record("claude")
		if !errors.As(err, &state) || !errors.Is(err, boom) {
			t.Fatalf("Record = %v, want the *StateDirError the store gave", err)
		}
		var lock *app.LockError
		if errors.As(err, &lock) {
			t.Errorf("Record = %v: a state directory that cannot be made is not a lock that could not be taken", err)
		}
	})
	t.Run("the record cannot be read", func(t *testing.T) {
		store := recordHolding(t, "claude\n")
		store.state = ownership.Unreadable
		svc, _ := service(store, false)
		var unreadable *app.UnreadableError
		if err := svc.Record("codex"); !errors.As(err, &unreadable) {
			t.Fatalf("Record = %v, want an *UnreadableError", err)
		}
		if got, want := store.trace(), "make-record-dir lock read release"; got != want {
			t.Errorf("calls = %q, want %q: nothing is appended to a record that cannot be inspected", got, want)
		}
	})
	t.Run("the append fails", func(t *testing.T) {
		store := recordHolding(t, "claude\n")
		store.appendErr = boom
		svc, _ := service(store, false)
		var appendErr *app.AppendError
		if err := svc.Record("codex"); !errors.As(err, &appendErr) || !errors.Is(err, boom) {
			t.Fatalf("Record = %v, want an *AppendError that wraps the cause", err)
		}
		if !strings.HasSuffix(store.trace(), "release") {
			t.Errorf("calls = %q: the lock is let go on the way out", store.trace())
		}
	})
}

func TestReleaseRewritesTheRecordWithoutTheNameInsideOneLock(t *testing.T) {
	store := recordHolding(t, "opencode\nclaude\nclaude\ncodex\n")
	svc, _ := service(store, false)
	if err := svc.Release("claude"); err != nil {
		t.Fatal(err)
	}
	if got, want := store.trace(), "exists make-record-dir lock read replace:codex\nopencode\n release"; got != want {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestReleaseKeepsWhatIsNotTheNameAndCompactsGarbageAway(t *testing.T) {
	for _, tc := range []struct{ name, content, release, want string }{
		{"a name only a newer build knows stays", "claude\nzed\n", "claude", "zed\n"},
		{"the last name leaves an empty record, not none", "codex\n", "codex", ""},
		{"a name that was not there still rewrites the record", "\x00junk\nclaude\n", "codex", "claude\n"},
		{"only the whole name goes", "claude-next\nclaude\n", "claude", "claude-next\n"},
		{"a name cut short is gone with the rewrite", "claude\nopenc", "codex", "claude\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := recordHolding(t, tc.content)
			svc, _ := service(store, false)
			if err := svc.Release(tc.release); err != nil {
				t.Fatal(err)
			}
			if string(store.content) != tc.want || !store.exists {
				t.Errorf("record = %q (exists %v), want %q", store.content, store.exists, tc.want)
			}
		})
	}
}

func TestReleaseWithNoRecordDoesNothingAtAll(t *testing.T) {
	store := noRecord(t)
	svc, _ := service(store, false)
	if err := svc.Release("claude"); err != nil {
		t.Fatal(err)
	}
	if got, want := store.trace(), "exists"; got != want {
		t.Errorf("calls = %q, want %q: no directory is made and no lock is taken for a record that is not there", got, want)
	}
}

func TestReleaseRewritesFromWhatIsThereWhenTheRecordIsGoneUnderTheLock(t *testing.T) {
	// The record was there when it was looked for and a writer removed it before the lock was ours:
	// the rewrite is made from what is there now, which is an empty record.
	store := recordHolding(t, "claude\n")
	store.afterLock = func(f *fakeStore) { f.exists, f.state, f.content = false, ownership.Absent, nil }
	svc, _ := service(store, false)
	if err := svc.Release("claude"); err != nil {
		t.Fatal(err)
	}
	if string(store.content) != "" || !store.exists {
		t.Errorf("record = %q (exists %v), want an empty one", store.content, store.exists)
	}
}

func TestReleaseNeverRewritesARecordItCannotRead(t *testing.T) {
	store := recordHolding(t, "claude\nopencode\n")
	store.state = ownership.Unreadable
	svc, _ := service(store, false)
	var unreadable *app.UnreadableError
	if err := svc.Release("claude"); !errors.As(err, &unreadable) {
		t.Fatalf("Release = %v, want an *UnreadableError", err)
	}
	if strings.Contains(store.trace(), "replace") {
		t.Errorf("calls = %q: a rewrite from a record that could not be read would truncate it", store.trace())
	}
	if string(store.content) != "claude\nopencode\n" {
		t.Errorf("record = %q, want it as it was", store.content)
	}
}

func TestReleaseStopsAtEachStepThatFails(t *testing.T) {
	boom := errors.New("boom")
	t.Run("the directory of the record", func(t *testing.T) {
		store := recordHolding(t, "claude\n")
		store.recordDirErr = boom
		svc, _ := service(store, false)
		var dir *app.RecordDirError
		if err := svc.Release("claude"); !errors.As(err, &dir) {
			t.Fatalf("Release = %v, want a *RecordDirError", err)
		}
	})
	t.Run("the lock stays taken", func(t *testing.T) {
		store := recordHolding(t, "claude\n")
		store.lockErr = fmt.Errorf("%w (waited 2s)", app.ErrBusy)
		svc, _ := service(store, false)
		if err := svc.Release("claude"); !errors.Is(err, app.ErrBusy) {
			t.Fatalf("Release = %v, want ErrBusy", err)
		}
		if string(store.content) != "claude\n" {
			t.Errorf("record = %q, want it untouched", store.content)
		}
	})
	t.Run("the rewrite fails", func(t *testing.T) {
		store := recordHolding(t, "claude\nopencode\n")
		store.replaceErr = &app.ReplaceError{Stage: app.StageCreate, Err: boom}
		svc, _ := service(store, false)
		var replace *app.ReplaceError
		if err := svc.Release("claude"); !errors.As(err, &replace) || replace.Stage != app.StageCreate {
			t.Fatalf("Release = %v, want the *ReplaceError the store gave", err)
		}
		if !strings.HasSuffix(store.trace(), "release") {
			t.Errorf("calls = %q: the lock is let go on the way out", store.trace())
		}
	})
}

func TestTrackedReportsTheRecordAsFoundWithoutTheLock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store *fakeStore
		state ownership.State
		names []string
	}{
		{"absent", noRecord(t), ownership.Absent, nil},
		{"readable", recordHolding(t, "opencode\nclaude\nclaude\n\x00bad\n"), ownership.Readable, []string{"claude", "opencode"}},
		{"unreadable", &fakeStore{t: t, exists: true, state: ownership.Unreadable, content: []byte("claude\n")}, ownership.Unreadable, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := service(tc.store, false)
			got := svc.Tracked()
			if got.State != tc.state || !reflect.DeepEqual(got.Targets.Names(), tc.names) {
				t.Errorf("Tracked() = state %v, %q; want %v, %q", got.State, got.Targets.Names(), tc.state, tc.names)
			}
			if tc.store.trace() != "read" {
				t.Errorf("calls = %q, want one read: a report is no critical section", tc.store.trace())
			}
		})
	}
}

func TestAssessAppliesTheRuleToTheRecordAsFoundWithoutTheLock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store *fakeStore
		in    ownership.Intent
		want  ownership.Verdict
	}{
		{"nothing depends on the binary", recordHolding(t, ""), ownership.Intent{}, ownership.Retire},
		{"a target still does", recordHolding(t, "claude\n"), ownership.Intent{}, ownership.StillInstalled},
		{"no record", noRecord(t), ownership.Intent{}, ownership.NoRecord},
		{"an unregister failed", recordHolding(t, ""), ownership.Intent{UnregisterFailed: true}, ownership.FailedUnregister},
		{"purge", noRecord(t), ownership.Intent{Purge: true}, ownership.Retire},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := service(tc.store, true)
			got := svc.Assess(tc.in)
			if got.Verdict != tc.want {
				t.Errorf("Assess(%+v) = %v, want %v", tc.in, got.Verdict, tc.want)
			}
			if tc.store.trace() != "read" {
				t.Errorf("calls = %q, want one read", tc.store.trace())
			}
		})
	}
	t.Run("the record cannot be read", func(t *testing.T) {
		store := &fakeStore{t: t, exists: true, state: ownership.Unreadable}
		svc, _ := service(store, true)
		got := svc.Assess(ownership.Intent{})
		if got.Verdict != ownership.UnreadableRecord || got.Observation.State != ownership.Unreadable {
			t.Errorf("Assess = %+v, want UnreadableRecord and the observation that says why", got)
		}
	})
	t.Run("the remaining targets come back with the verdict", func(t *testing.T) {
		svc, _ := service(recordHolding(t, "opencode\ncodex\n"), true)
		got := svc.Assess(ownership.Intent{})
		if !reflect.DeepEqual(got.Observation.Targets.Names(), []string{"codex", "opencode"}) {
			t.Errorf("remaining = %q", got.Observation.Targets.Names())
		}
	})
}

const binary = "/state/bin/longterm-mem"

func TestRetireRemovesTheBinaryAndTheRecordInsideOneLockWhenNothingDependsOnThem(t *testing.T) {
	store := recordHolding(t, "")
	svc, binaries := service(store, true)
	got, err := svc.Retire(app.RetireRequest{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposal != ownership.DisposeOfBinary || got.Binary != app.BinaryRemoved {
		t.Errorf("Retire = %+v, want the binary disposed of and removed", got)
	}
	if want := "lock read binary-exists binary-remove delete release"; store.trace() != want {
		t.Errorf("calls = %q, want %q", store.trace(), want)
	}
	if binaries.there || store.exists {
		t.Errorf("binary there %v, record there %v; want neither", binaries.there, store.exists)
	}
}

func TestRetireRemovesTheRecordEvenWhenTheBinaryIsAlreadyGone(t *testing.T) {
	store := recordHolding(t, "")
	svc, binaries := service(store, false)
	got, err := svc.Retire(app.RetireRequest{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if got.Binary != app.BinaryAbsent || store.exists {
		t.Errorf("binary fate %v, record there %v; want absent and removed", got.Binary, store.exists)
	}
	if len(binaries.calls) != 1 {
		t.Errorf("binary calls = %q, want only the look", binaries.calls)
	}
}

func TestRetireLeavesEverythingWhenARuntimeMayDependOnTheBinary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store *fakeStore
		purge bool
		want  ownership.Disposal
	}{
		{"a target is recorded", recordHolding(t, "opencode\n"), false, ownership.KeepBinary},
		{"no record", noRecord(t), false, ownership.KeepBinary},
		{"the record cannot be read", &fakeStore{t: t, exists: true, state: ownership.Unreadable}, false, ownership.HoldBinaryUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, binaries := service(tc.store, true)
			got, err := svc.Retire(app.RetireRequest{Binary: binary, Purge: tc.purge})
			if err != nil {
				t.Fatal(err)
			}
			if got.Disposal != tc.want || got.Binary != app.BinaryUntouched {
				t.Errorf("Retire = %+v, want %v and the binary untouched", got, tc.want)
			}
			if !binaries.there || len(binaries.calls) != 0 {
				t.Errorf("the binary was looked at or removed: %q", binaries.calls)
			}
			if strings.Contains(tc.store.trace(), "delete") {
				t.Errorf("calls = %q: the record stays with the binary", tc.store.trace())
			}
		})
	}
}

// What a record holds is evidence, and the shared binary goes only on evidence that nothing depends on
// it: a name only a newer build knows holds it, and bytes that are not a name (a NUL byte, a name cut
// short by the end of the file, garbage) are not install state and do not.
func TestRetireReadsTheRecordAsEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          ownership.Disposal
	}{
		{"a name only a newer build knows", "zed\n", ownership.KeepBinary},
		{"a NUL byte makes a well-formed name garbage", "\x00zed\n", ownership.DisposeOfBinary},
		{"a name cut short is garbage", "clau", ownership.DisposeOfBinary},
		{"a name cut short does not discard the names above it", "claude\nopenc", ownership.KeepBinary},
		{"garbage alone does not wedge the binary", "\x00garbage\n{not-a-target}\n\n", ownership.DisposeOfBinary},
		{"a carriage return is garbage", "claude\r\n", ownership.DisposeOfBinary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := recordHolding(t, tc.content)
			svc, binaries := service(store, true)
			got, err := svc.Retire(app.RetireRequest{Binary: binary})
			if err != nil {
				t.Fatal(err)
			}
			if got.Disposal != tc.want {
				t.Errorf("Retire = %v, want %v", got.Disposal, tc.want)
			}
			if gone := !binaries.there; gone != (tc.want == ownership.DisposeOfBinary) {
				t.Errorf("binary gone = %v, want %v", gone, tc.want == ownership.DisposeOfBinary)
			}
		})
	}
}

func TestRetireWithPurgeRemovesWhateverTheRecordSays(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store *fakeStore
	}{
		{"a target is recorded", recordHolding(t, "claude\nopencode\n")},
		{"no record", noRecord(t)},
		{"the record cannot be read", &fakeStore{t: t, exists: true, state: ownership.Unreadable}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, binaries := service(tc.store, true)
			got, err := svc.Retire(app.RetireRequest{Binary: binary, Purge: true})
			if err != nil {
				t.Fatal(err)
			}
			if got.Binary != app.BinaryRemoved || binaries.there || tc.store.exists {
				t.Errorf("Retire = %+v, binary there %v, record there %v; want both gone", got, binaries.there, tc.store.exists)
			}
		})
	}
}

func TestRetireDecidesOnTheRecordAsItIsUnderTheLockNotAsItWasAssessed(t *testing.T) {
	// The record was empty when the verdict was Retire; a writer recorded a target before the lock was
	// ours. The binary that target depends on stays.
	store := recordHolding(t, "")
	svc, binaries := service(store, true)
	if verdict := svc.Assess(ownership.Intent{}).Verdict; verdict != ownership.Retire {
		t.Fatalf("Assess = %v, want Retire", verdict)
	}
	store.afterLock = func(f *fakeStore) { f.content = []byte("opencode\n") }
	got, err := svc.Retire(app.RetireRequest{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposal != ownership.KeepBinary || !binaries.there || !store.exists {
		t.Errorf("Retire = %+v, binary there %v, record there %v; want everything kept", got, binaries.there, store.exists)
	}
}

func TestRetireStopsAtEachStepThatFails(t *testing.T) {
	boom := errors.New("boom")
	t.Run("the lock stays taken", func(t *testing.T) {
		store := recordHolding(t, "")
		store.lockErr = fmt.Errorf("%w (waited 2s)", app.ErrBusy)
		svc, binaries := service(store, true)
		if _, err := svc.Retire(app.RetireRequest{Binary: binary}); !errors.Is(err, app.ErrBusy) {
			t.Fatalf("Retire = %v, want ErrBusy", err)
		}
		if !binaries.there || !store.exists {
			t.Error("something was removed without the lock")
		}
	})
	t.Run("the state directory of the lock cannot be made", func(t *testing.T) {
		store := recordHolding(t, "")
		store.lockErr = &app.StateDirError{Err: boom}
		svc, _ := service(store, true)
		var state *app.StateDirError
		_, err := svc.Retire(app.RetireRequest{Binary: binary})
		if !errors.As(err, &state) {
			t.Fatalf("Retire = %v, want the *StateDirError", err)
		}
		var lock *app.LockError
		if errors.As(err, &lock) {
			t.Errorf("Retire = %v: a state directory that cannot be made is not a lock that could not be taken", err)
		}
	})
	t.Run("the binary cannot be removed, and the record stays with it", func(t *testing.T) {
		store := recordHolding(t, "")
		svc, binaries := service(store, true)
		binaries.removeErr = boom
		_, err := svc.Retire(app.RetireRequest{Binary: binary})
		var remove *app.BinaryRemoveError
		if !errors.As(err, &remove) || remove.Path != binary || !errors.Is(err, boom) {
			t.Fatalf("Retire = %v, want a *BinaryRemoveError for the binary", err)
		}
		if !store.exists {
			t.Error("the record was removed although the binary was not")
		}
		if !strings.HasSuffix(store.trace(), "release") {
			t.Errorf("calls = %q: the lock is let go on the way out", store.trace())
		}
	})
	t.Run("the record cannot be removed", func(t *testing.T) {
		store := recordHolding(t, "")
		store.deleteErr = boom
		svc, _ := service(store, true)
		var remove *app.RecordRemoveError
		if _, err := svc.Retire(app.RetireRequest{Binary: binary}); !errors.As(err, &remove) || !errors.Is(err, boom) {
			t.Fatalf("Retire = %v, want a *RecordRemoveError", err)
		}
	})
}

// A store that fails a rewrite in a way it does not place (a plain error, not a *ReplaceError) is
// still a failed rewrite, said the same way: the use case wraps it as it wraps a lock failure, and a
// *ReplaceError the store did place is returned as it is.
func TestReleaseSaysEveryFailedRewriteAsAReplaceError(t *testing.T) {
	boom := errors.New("boom")
	t.Run("a plain error is wrapped, with no stage", func(t *testing.T) {
		store := recordHolding(t, "claude\nopencode\n")
		store.replaceErr = boom
		svc, _ := service(store, false)
		err := svc.Release("claude")
		var replace *app.ReplaceError
		if !errors.As(err, &replace) || replace.Stage != 0 || !errors.Is(err, boom) {
			t.Fatalf("Release = %v, want a *ReplaceError with no stage that wraps the cause", err)
		}
		if !strings.HasSuffix(store.trace(), "release") {
			t.Errorf("calls = %q: the lock is let go on the way out", store.trace())
		}
	})
	t.Run("a wrapped plain error is wrapped too", func(t *testing.T) {
		store := recordHolding(t, "claude\n")
		store.replaceErr = fmt.Errorf("while rewriting: %w", boom)
		svc, _ := service(store, false)
		var replace *app.ReplaceError
		if err := svc.Release("claude"); !errors.As(err, &replace) || !errors.Is(err, boom) {
			t.Fatalf("Release = %v, want a *ReplaceError that wraps the cause", err)
		}
	})
	t.Run("a *ReplaceError the store placed keeps its stage", func(t *testing.T) {
		store := recordHolding(t, "claude\n")
		store.replaceErr = &app.ReplaceError{Stage: app.StageWrite, Err: boom}
		svc, _ := service(store, false)
		var replace *app.ReplaceError
		if err := svc.Release("claude"); !errors.As(err, &replace) || replace.Stage != app.StageWrite {
			t.Fatalf("Release = %v, want the stage the store gave", err)
		}
	})
}

// Record writes into the directory of the record, and makes it first: a machine that has never
// recorded anything has no such directory, and the store (like a file system) refuses to write in one
// that is not there.
func TestRecordMakesTheDirectoryOfTheRecordBeforeItWritesIntoIt(t *testing.T) {
	store := noRecord(t)
	svc, _ := service(store, false)
	if err := svc.Record("claude"); err != nil {
		t.Fatalf("Record in a store with no directory = %v", err)
	}
	if got, want := store.trace(), "make-record-dir lock read append:claude release"; got != want {
		t.Errorf("calls = %q, want %q: the directory is made before the lock is taken and the name written", got, want)
	}
}

// Retire makes no directory and needs none: Lock makes the state directory, the record of a directory
// that is not there is absent, and removing it is already done. The store models a machine that has
// nothing yet.
func TestRetireNeedsNoDirectoryOfTheRecordAndLockMakesTheStateDirectory(t *testing.T) {
	store := noRecord(t)
	svc, binaries := service(store, true)
	got, err := svc.Retire(app.RetireRequest{Binary: binary, Purge: true})
	if err != nil {
		t.Fatalf("Retire in a store with no directory = %v", err)
	}
	if got.Binary != app.BinaryRemoved || binaries.there {
		t.Errorf("Retire = %+v, binary there %v; want the binary removed", got, binaries.there)
	}
	if want := "lock read binary-exists binary-remove delete release"; store.trace() != want {
		t.Errorf("calls = %q, want %q: no directory is made", store.trace(), want)
	}
	if !store.stateDir || store.recordDir {
		t.Errorf("state directory made %v, directory of the record made %v; want the first made by the lock and the second not made", store.stateDir, store.recordDir)
	}
	// Without a record there is nothing that says no runtime depends on the binary: no purge, no removal,
	// and still no directory made.
	kept := noRecord(t)
	svc, binaries = service(kept, true)
	if got, err := svc.Retire(app.RetireRequest{Binary: binary}); err != nil || got.Disposal != ownership.KeepBinary || !binaries.there {
		t.Errorf("Retire without purge and without a record = %+v, %v, binary there %v; want it kept", got, err, binaries.there)
	}
}

// --purge does not skip the rewrite of an unreadable record, and does force the removal after it. Both
// are pinned (engine/installer/testdata/longterm_pins.golden) and look as if they disagreed:
//
//	uninstall/record-unreadable-purge: unregister ends 0, so the script releases the target, and the
//	  release refuses to rewrite a record nobody could read; exit 1, binary and record in place.
//	uninstall/record-unreadable-nothing-rewritten-purge: unregister ends 6, so nothing is released and
//	  the script goes on to assess and retire; purge overrides the unreadable record there; exit 0,
//	  binary and record gone.
//
// Settle and Dispose, which let purge override an unreadable record, are the second half only: the
// first never reaches them. The test follows the script's two paths through the use case.
func TestPurgeDoesNotSkipTheRewriteOfAnUnreadableRecordButForcesTheRemovalAfterIt(t *testing.T) {
	unreadable := func() *fakeStore {
		store := recordHolding(t, "claude\nopencode\n")
		store.state = ownership.Unreadable
		return store
	}

	// uninstall/record-unreadable and uninstall/record-unreadable-purge: the release refuses, the run ends
	// there, and nothing is assessed or removed.
	for _, name := range []string{"without purge", "with purge"} {
		t.Run("the release is refused "+name, func(t *testing.T) {
			store := unreadable()
			svc, binaries := service(store, true)
			var refused *app.UnreadableError
			if err := svc.Release("claude"); !errors.As(err, &refused) {
				t.Fatalf("Release = %v, want an *UnreadableError whatever purge says: it takes no purge", err)
			}
			if strings.Contains(store.trace(), "replace") || strings.Contains(store.trace(), "delete") {
				t.Errorf("calls = %q: the record that could not be read was written or removed", store.trace())
			}
			if !store.exists || !binaries.there {
				t.Errorf("record there %v, binary there %v; want both in place", store.exists, binaries.there)
			}
		})
	}

	// uninstall/record-unreadable-nothing-rewritten: no purge, the guard holds everything.
	t.Run("nothing was rewritten, no purge", func(t *testing.T) {
		store := unreadable()
		svc, binaries := service(store, true)
		if got := svc.Assess(ownership.Intent{}); got.Verdict != ownership.UnreadableRecord {
			t.Fatalf("Assess = %v, want UnreadableRecord", got.Verdict)
		}
		got, err := svc.Retire(app.RetireRequest{Binary: binary})
		if err != nil || got.Disposal != ownership.HoldBinaryUnreadable || !binaries.there || !store.exists {
			t.Errorf("Retire = %+v, %v, binary there %v, record there %v; want everything held", got, err, binaries.there, store.exists)
		}
	})

	// uninstall/record-unreadable-nothing-rewritten-purge: purge overrides the unreadable record.
	t.Run("nothing was rewritten, purge", func(t *testing.T) {
		store := unreadable()
		svc, binaries := service(store, true)
		if got := svc.Assess(ownership.Intent{Purge: true}); got.Verdict != ownership.Retire || got.Observation.State != ownership.Unreadable {
			t.Fatalf("Assess = %+v, want Retire over an unreadable record", got)
		}
		got, err := svc.Retire(app.RetireRequest{Binary: binary, Purge: true})
		if err != nil || got.Disposal != ownership.DisposeOfBinary || got.Binary != app.BinaryRemoved || binaries.there || store.exists {
			t.Errorf("Retire = %+v, %v, binary there %v, record there %v; want both gone", got, err, binaries.there, store.exists)
		}
	})
}

func TestTheErrorsSayWhatTheyAre(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		err   error
		want  string
		cause bool
	}{
		{&app.StateDirError{Err: boom}, "state directory", true},
		{&app.RecordDirError{Err: boom}, "directory of the record", true},
		{&app.LockError{Err: boom}, "lock", true},
		{&app.UnreadableError{}, "cannot be read", false},
		{&app.AppendError{Err: boom}, "record", true},
		{&app.ReplaceError{Stage: app.StageCreate, Err: boom}, "temporary", true},
		{&app.ReplaceError{Stage: app.StageWrite, Err: boom}, "write", true},
		{&app.ReplaceError{Stage: app.StagePublish, Err: boom}, "replace", true},
		{&app.BinaryRemoveError{Path: binary, Err: boom}, binary, true},
		{&app.RecordRemoveError{Err: boom}, "record", true},
	} {
		got := tc.err.Error()
		if !strings.Contains(got, tc.want) {
			t.Errorf("%T says %q, want it to say %q", tc.err, got, tc.want)
		}
		if tc.cause && !strings.Contains(got, "boom") {
			t.Errorf("%T says %q, want it to say why", tc.err, got)
		}
		if tc.cause && !errors.Is(tc.err, boom) {
			t.Errorf("%T does not unwrap to its cause", tc.err)
		}
	}
	// A lock that stayed taken is told from one that could not be taken.
	if err := (&app.LockError{Err: app.ErrBusy}); !errors.Is(err, app.ErrBusy) {
		t.Error("a *LockError over ErrBusy is not ErrBusy")
	}
}
