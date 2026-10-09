package fsadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/guardmarkers"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// The adapter is a shaper.ClearanceStore: the domain's port, answered from files.
var _ shaper.ClearanceStore = ClearanceStore{}

// newTestStore builds the store over a fresh temporary directory, so no test can reach
// the real user's state, and returns the store and the state home.
func newTestStore(t *testing.T) (ClearanceStore, string) {
	t.Helper()
	state := t.TempDir()
	s, err := NewClearanceStore(state)
	if err != nil {
		t.Fatalf("NewClearanceStore() = %v, want nil", err)
	}
	return s, state
}

func hex64(c string) string { return strings.Repeat(c, 64) }

// recordBytes is one valid clearance record for the given key, as the host captures it.
func recordBytes(t *testing.T, project, goal, handoffSHA string, decision shaper.ClearanceDecision) []byte {
	t.Helper()
	verified := false
	data, err := json.Marshal(shaper.ClearanceRecord{
		Version: shaper.ClearanceRecordVersion,
		Subject: shaper.ClearanceSubject{
			ProjectID: project, GoalID: goal,
			GoalSHA256: hex64("1"), HandoffSHA256: handoffSHA,
			ProvenanceSHA256: hex64("2"), ViewSHA256: hex64("3"),
		},
		FlagResolutions: []shaper.FlagResolution{},
		Decision:        decision,
		Channel:         shaper.ChannelProvenance{Runtime: "pi", Mode: "tui", Verified: &verified},
	})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	return data
}

// storedRecord is the record most tests store: proj, goal-alpha and a handoff digest.
func storedRecord(t *testing.T) (key [3]string, data []byte) {
	t.Helper()
	key = [3]string{"proj", "goal-alpha", hex64("a")}
	return key, recordBytes(t, key[0], key[1], key[2], shaper.DecisionAffirm)
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info.Mode()
}

// recordPath is where the store keeps a record, spelled out so that no test depends on the
// store to say where its own records are.
func recordPath(state string, key [3]string) string {
	return filepath.Join(state, "labdrian", "shaper-clearance", key[0], key[1], key[2]+".json")
}

func TestClearanceStoreResolvesTheDecidedPath(t *testing.T) {
	s, state := newTestStore(t)
	key := [3]string{"proj", "goal-alpha", hex64("a")}
	got, err := s.Path(key[0], key[1], key[2])
	if err != nil {
		t.Fatalf("Path() = %v, want nil", err)
	}
	if want := recordPath(state, key); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// The clearance deny guards of the runtimes refuse any path that contains
// guardmarkers.Store. They can only guard the store where it really is, so the
// layout this adapter keeps and the marker published to the guards are one
// and the same, and a record's path is one the guards match.
func TestTheStoreLayoutIsTheOneTheDenyGuardsMatch(t *testing.T) {
	if want := filepath.Join(storeComponents...); guardmarkers.Store != want {
		t.Fatalf("guardmarkers.Store = %q, want the store path segment %q", guardmarkers.Store, want)
	}
	s, _ := newTestStore(t)
	path, err := s.Path("p", "g", hex64("a"))
	if err != nil {
		t.Fatalf("Path() = %v", err)
	}
	if !shaper.GuardMatches(path) {
		t.Errorf("shaper.GuardMatches(%q) = false for a store record path", path)
	}
}

// The store is built over the state home it is handed, which must be an absolute path.
// Resolving it from the environment, and the text that prints for an unusable
// environment, are the composition root's (cmd pins them).
func TestNewClearanceStoreRefusesAStateHomeThatIsNotAbsolute(t *testing.T) {
	for _, home := range []string{"", "relative/path", "./state"} {
		_, err := NewClearanceStore(home)
		want := fmt.Sprintf("clearance store: state home %q is not an absolute path", home)
		if err == nil || err.Error() != want {
			t.Errorf("NewClearanceStore(%q) = %v, want %q", home, err, want)
		}
	}
}

// A platform without a no-follow open is refused when the store is built, with a sentinel
// callers can match and the message every store of this kind prints.
func TestTheUnsupportedPlatformSentinelKeepsItsMessage(t *testing.T) {
	if got, want := ErrUnsupportedPlatform.Error(), "clearance store: unsupported platform"; got != want {
		t.Errorf("ErrUnsupportedPlatform = %q, want %q", got, want)
	}
}

// A zero store was not built by NewClearanceStore and has no state home to write under.
func TestZeroStoreIsRefused(t *testing.T) {
	want := "clearance store: store is not initialized; use NewClearanceStore"
	var zero ClearanceStore
	if _, err := zero.Path("p", "g", hex64("a")); err == nil || err.Error() != want {
		t.Errorf("Path() = %v, want %q", err, want)
	}
	if _, err := zero.Get("p", "g", hex64("a")); err == nil || err.Error() != want {
		t.Errorf("Get() = %v, want %q", err, want)
	}
	_, data := storedRecord(t)
	if _, err := zero.Put(data); err == nil || err.Error() != want {
		t.Errorf("Put() = %v, want %q", err, want)
	}
}

func TestClearanceStoreRefusesUnsafePathComponents(t *testing.T) {
	s, _ := newTestStore(t)
	sha := hex64("b")
	for _, tc := range []struct {
		name, project, goal, sha, want string
	}{
		{"empty project", "", "g", sha, `clearance store: project_id "" is not a usable path component`},
		{"dot project", ".", "g", sha, `clearance store: project_id "." is not a usable path component`},
		{"dotdot goal", "p", "..", sha, `clearance store: goal_id ".." is not a usable path component`},
		{"slash project", "a/b", "g", sha, `clearance store: project_id "a/b" contains a path separator or NUL`},
		{"backslash goal", "p", `a\b`, sha, `clearance store: goal_id "a\\b" contains a path separator or NUL`},
		{"nul goal", "p", "a\x00b", sha, `clearance store: goal_id "a\x00b" contains a path separator or NUL`},
		{"short sha", "p", "g", "abc", `clearance store: handoff sha256 "abc" is not 64 lowercase hex characters`},
		{"uppercase sha", "p", "g", strings.Repeat("B", 64), `clearance store: handoff sha256 "` + strings.Repeat("B", 64) + `" is not 64 lowercase hex characters`},
		{"traversing sha", "p", "g", "../" + sha[3:], `clearance store: handoff sha256 "../` + sha[3:] + `" is not 64 lowercase hex characters`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if p, err := s.Path(tc.project, tc.goal, tc.sha); err == nil || err.Error() != tc.want {
				t.Errorf("Path(%q, %q, %q) = %q, %v, want %q", tc.project, tc.goal, tc.sha, p, err, tc.want)
			}
			if _, err := s.Get(tc.project, tc.goal, tc.sha); err == nil || err.Error() != tc.want {
				t.Errorf("Get(%q, %q, %q) = %v, want %q", tc.project, tc.goal, tc.sha, err, tc.want)
			}
		})
	}
}

// A record whose subject names an unsafe key is refused at the store, whatever the
// caller verified before it, and nothing is created for it.
func TestPutRefusesARecordWhoseKeyIsUnsafe(t *testing.T) {
	for _, tc := range []struct{ name, project, goal, want string }{
		{"parent directory as project", "../escape", "g", `clearance store: project_id "../escape" contains a path separator or NUL`},
		{"slash in goal", "p", "a/b", `clearance store: goal_id "a/b" contains a path separator or NUL`},
		{"dot goal", "p", ".", `clearance store: goal_id "." is not a usable path component`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, state := newTestStore(t)
			if _, err := s.Put(recordBytes(t, tc.project, tc.goal, hex64("a"), shaper.DecisionAffirm)); err == nil || err.Error() != tc.want {
				t.Fatalf("Put() = %v, want %q", err, tc.want)
			}
			if entries, _ := os.ReadDir(state); len(entries) != 0 {
				t.Errorf("the refused record left %d entries under the state home, want none", len(entries))
			}
		})
	}
}

func TestPutWritesAPrivateImmutableRecord(t *testing.T) {
	s, state := newTestStore(t)
	key, data := storedRecord(t)

	path, err := s.Put(data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if want := recordPath(state, key); path != want {
		t.Errorf("Put path = %q, want %q", path, want)
	}
	if m := mode(t, path); !m.IsRegular() || m.Perm() != 0o600 {
		t.Errorf("record mode = %v, want regular 0600", m)
	}
	for _, dir := range []string{
		filepath.Join(state, "labdrian"),
		filepath.Join(state, "labdrian", "shaper-clearance"),
		filepath.Join(state, "labdrian", "shaper-clearance", key[0]),
		filepath.Join(state, "labdrian", "shaper-clearance", key[0], key[1]),
	} {
		if m := mode(t, dir); !m.IsDir() || m.Perm() != 0o700 {
			t.Errorf("%s mode = %v, want directory 0700", dir, m)
		}
	}
	got, err := s.Get(key[0], key[1], key[2])
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Get returned %q, want the stored bytes", got)
	}

	again, err := s.Put(data)
	if err != nil {
		t.Fatalf("Put identical bytes again: %v", err)
	}
	if again != path {
		t.Errorf("idempotent Put path = %q, want %q", again, path)
	}

	declined := recordBytes(t, key[0], key[1], key[2], shaper.DecisionDecline)
	_, err = s.Put(declined)
	want := fmt.Sprintf("clearance store: refusing to replace immutable record %q with different bytes", path)
	if err == nil || err.Error() != want {
		t.Fatalf("Put(different bytes) = %v, want %q", err, want)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stored record: %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Errorf("stored record changed after a refused Put")
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read store dir: %v", err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("store dir holds %v, want only the record (no temp leftovers)", names)
	}
}

func TestPutRefusesAnInvalidRecord(t *testing.T) {
	s, state := newTestStore(t)
	for _, data := range []string{`{}`, `not json`, `{"version":1,"extra":true}`} {
		if _, err := s.Put([]byte(data)); err == nil || !strings.HasPrefix(err.Error(), "clearance store: parse clearance record: ") {
			t.Errorf("Put(%q) = %v, want a parse refusal", data, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(state, "labdrian")); !os.IsNotExist(err) {
		t.Errorf("Put created store directories for an invalid record (err %v)", err)
	}
}

// An RPC-captured record, which a process rather than a human answered, is kept out of
// the store entirely.
func TestPutRefusesARecordCapturedOverRPC(t *testing.T) {
	s, state := newTestStore(t)
	_, data := storedRecord(t)
	rpc := bytes.Replace(data, []byte(`"mode":"tui"`), []byte(`"mode":"rpc"`), 1)
	if bytes.Equal(rpc, data) {
		t.Fatal("the fixture was not changed to an RPC record")
	}
	if _, err := s.Put(rpc); err == nil || !strings.Contains(err.Error(), "tui") {
		t.Fatalf("Put(rpc record) err = %v, want a refusal naming tui", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "labdrian")); !os.IsNotExist(err) {
		t.Errorf("Put created store directories for an rpc record (err %v)", err)
	}
}

// A key with no record is the port's ErrClearanceNotFound, whether the store directories
// are not there yet or are there without the record, and the text is the file system's,
// as it always was.
func TestGetOfAMissingRecordIsClearanceNotFound(t *testing.T) {
	s, state := newTestStore(t)
	sha := hex64("c")

	_, err := s.Get("proj", "goal-alpha", sha)
	want := fmt.Sprintf("clearance store: lstat %s: no such file or directory", filepath.Join(state, "labdrian"))
	if !errors.Is(err, shaper.ErrClearanceNotFound) || !errors.Is(err, fs.ErrNotExist) || err.Error() != want {
		t.Errorf("Get() with no store = %v, want ErrClearanceNotFound and %q", err, want)
	}

	if err := os.MkdirAll(filepath.Join(state, "labdrian", "shaper-clearance", "proj", "goal-alpha"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get("proj", "goal-alpha", sha)
	want = fmt.Sprintf("clearance store: open %s: no such file or directory", recordPath(state, [3]string{"proj", "goal-alpha", sha}))
	if !errors.Is(err, shaper.ErrClearanceNotFound) || !errors.Is(err, fs.ErrNotExist) || err.Error() != want {
		t.Errorf("Get() with the directories and no record = %v, want ErrClearanceNotFound and %q", err, want)
	}
}

// Only a missing record is not-found: a store that cannot be read is another failure, so
// a caller never takes an unreadable store for an absent clearance.
func TestGetOfAnUnreadableStoreIsNotClearanceNotFound(t *testing.T) {
	s, state := newTestStore(t)
	key, _ := storedRecord(t)
	if err := os.MkdirAll(recordPath(state, key), 0o700); err != nil { // a directory where the record belongs
		t.Fatal(err)
	}
	_, err := s.Get(key[0], key[1], key[2])
	if err == nil || errors.Is(err, shaper.ErrClearanceNotFound) {
		t.Fatalf("Get() of a directory at the record path = %v, want an error that is not ErrClearanceNotFound", err)
	}
}

func TestStoreRefusesSymlinkedComponents(t *testing.T) {
	key, data := storedRecord(t)
	recordRel := []string{"labdrian", "shaper-clearance", key[0], key[1], key[2] + ".json"}
	for _, tc := range []struct {
		name string
		// link is the store-relative path of the component replaced by a symlink to an
		// outside directory (or file).
		link []string
		file bool
	}{
		{name: "labdrian dir", link: []string{"labdrian"}},
		{name: "store root", link: []string{"labdrian", "shaper-clearance"}},
		{name: "project dir", link: []string{"labdrian", "shaper-clearance", key[0]}},
		{name: "goal dir", link: []string{"labdrian", "shaper-clearance", key[0], key[1]}},
		{name: "record file", link: recordRel, file: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, state := newTestStore(t)
			outside := t.TempDir()
			target := outside
			if tc.file {
				target = filepath.Join(outside, "record.json")
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatalf("write outside record: %v", err)
				}
			}
			linkPath := filepath.Join(append([]string{state}, tc.link...)...)
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.Symlink(target, linkPath); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			wantRefusal := fmt.Sprintf("clearance store: refusing symlinked store component %q", linkPath)
			if tc.file {
				wantRefusal = fmt.Sprintf("clearance store: refusing symlinked record %q", linkPath)
			}
			if _, err := s.Put(data); err == nil || err.Error() != wantRefusal {
				t.Errorf("Put through a symlinked %s = %v, want %q", tc.name, err, wantRefusal)
			}
			if !tc.file {
				entries, err := os.ReadDir(outside)
				if err != nil {
					t.Fatalf("read outside dir: %v", err)
				}
				if len(entries) != 0 {
					t.Errorf("Put wrote %d entries through the symlink", len(entries))
				}
				// Place a valid record behind the symlinked directory, so Get can only
				// fail by refusing the symlink itself rather than because nothing exists
				// at the resolved path.
				rest := recordRel[len(tc.link):]
				behind := filepath.Join(append([]string{outside}, rest...)...)
				if err := os.MkdirAll(filepath.Dir(behind), 0o700); err != nil {
					t.Fatalf("mkdir behind symlink: %v", err)
				}
				if err := os.WriteFile(behind, data, 0o600); err != nil {
					t.Fatalf("write record behind symlink: %v", err)
				}
				if _, err := os.Stat(filepath.Join(append([]string{state}, recordRel...)...)); err != nil {
					t.Fatalf("record is not reachable through the symlink: %v", err)
				}
			}
			if _, err := s.Get(key[0], key[1], key[2]); err == nil || err.Error() != wantRefusal {
				t.Errorf("Get through a symlinked %s = %v, want %q", tc.name, err, wantRefusal)
			}
		})
	}
}

// A state home that is itself a symlink is the user's own choice and is followed; only the
// components below it must be plain directories.
func TestStoreFollowsASymlinkedStateHome(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink() = %v", err)
	}
	s, err := NewClearanceStore(link)
	if err != nil {
		t.Fatalf("NewClearanceStore() = %v, want nil", err)
	}
	key, data := storedRecord(t)
	if _, err := s.Put(data); err != nil {
		t.Fatalf("Put() = %v, want nil", err)
	}
	if _, err := os.Stat(recordPath(real, key)); err != nil {
		t.Fatalf("the record is not under the link's target: %v", err)
	}
}

// A state home that does not exist yet is created, private; one that is a file is refused
// in the words the store has always used.
func TestStoreCreatesAMissingStateHomeAndRefusesOneThatIsAFile(t *testing.T) {
	key, data := storedRecord(t)

	missing := filepath.Join(t.TempDir(), "not", "yet")
	s, err := NewClearanceStore(missing)
	if err != nil {
		t.Fatalf("NewClearanceStore() = %v, want nil", err)
	}
	if _, err := s.Put(data); err != nil {
		t.Fatalf("Put() into a missing state home = %v, want nil", err)
	}
	if m := mode(t, missing); !m.IsDir() || m.Perm() != 0o700 {
		t.Errorf("created state home mode = %v, want directory 0700", m)
	}
	if _, err := os.Stat(recordPath(missing, key)); err != nil {
		t.Errorf("the record was not stored: %v", err)
	}

	file := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = NewClearanceStore(file)
	if err != nil {
		t.Fatalf("NewClearanceStore() = %v, want nil: the home is checked when the store is used", err)
	}
	want := fmt.Sprintf("clearance store: state home %q is not a directory", file)
	if _, err := s.Put(data); err == nil || err.Error() != want {
		t.Errorf("Put() into a state home that is a file = %v, want %q", err, want)
	}
}

func TestStoreRefusesANonRegularRecord(t *testing.T) {
	s, state := newTestStore(t)
	key, data := storedRecord(t)
	path := recordPath(state, key)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir at record path: %v", err)
	}
	if _, err := s.Put(data); err == nil {
		t.Errorf("Put accepted a directory at the record path")
	}
	if _, err := s.Get(key[0], key[1], key[2]); err == nil {
		t.Errorf("Get accepted a directory at the record path")
	}
}

// A record that cannot be written because the directory refuses a new file is refused in
// the store's name with the words of the write that failed (engine/atomicfile's). The
// store printed "write temporary record" and "publish record" for these before it was
// built on atomicfile; that wording is the documented difference, and this pins what
// replaced it.
func TestPutReportsAWriteFailureInTheAtomicFileWords(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory does not stop root from writing")
	}
	s, state := newTestStore(t)
	key, data := storedRecord(t)
	dir := filepath.Dir(recordPath(state, key))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	_, err := s.Put(data)
	if err == nil || !strings.HasPrefix(err.Error(), "clearance store: atomicfile: create temporary file: ") || !strings.HasSuffix(err.Error(), "permission denied") {
		t.Fatalf("Put into a directory that refuses a new file = %v, want %q ... %q", err, "clearance store: atomicfile: create temporary file: ", "permission denied")
	}
	if _, err := os.Lstat(recordPath(state, key)); !os.IsNotExist(err) {
		t.Errorf("a record exists after a failed Put (err %v)", err)
	}
}
