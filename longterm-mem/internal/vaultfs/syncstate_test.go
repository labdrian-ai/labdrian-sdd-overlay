package vaultfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// Vault is the adapter behind the port promote owns for the record of the last sync.
var _ promote.SyncStateRecorder = (*Vault)(nil)

// syncStateRel is where the vault keeps the record, written out as the literal a vault written by an earlier
// build holds, not derived from the layout under test.
const syncStateRel = ".vault-meta/longterm-mem-sync-state.json"

// The record is a JSON object indented by two spaces and ended by a newline, with the schema version and the
// instant the sync completed, in the form the vault's readers have always read.
func TestRecordingASyncWritesTheRecordTheVaultHasAlwaysHeld(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	if err := New(root).RecordSyncCompleted(at); err != nil {
		t.Fatalf("RecordSyncCompleted: %v", err)
	}

	path := filepath.Join(root, filepath.FromSlash(syncStateRel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "{\n  \"schema\": 1,\n  \"last_sync_completed_at\": \"2026-10-10T12:00:00Z\"\n}\n"
	if string(data) != want {
		t.Errorf("record = %q, want %q", data, want)
	}
	if got := modeOf(t, path); got != 0o600 {
		t.Errorf("mode of the new record = %v, want 0600", got)
	}
}

// The record is replaced the way the vault's other files are: whole and atomically, keeping the mode of the
// one that is there, written through a link, leaving nothing beside it.
func TestRecordingASyncReplacesTheRecordThatIsThere(t *testing.T) {
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	t.Run("keeps its mode and leaves no temporary file", func(t *testing.T) {
		root := t.TempDir()
		path := writeVaultFile(t, root, syncStateRel, "{\"a much longer\": \"earlier record than the new one\"}\n", 0o644)
		if err := New(root).RecordSyncCompleted(at); err != nil {
			t.Fatalf("RecordSyncCompleted: %v", err)
		}
		if got := modeOf(t, path); got != 0o644 {
			t.Errorf("mode = %v, want the 0644 it had", got)
		}
		if got, err := New(root).LastSyncCompletedAt(); err != nil || got != "2026-10-10T12:00:00Z" {
			t.Errorf("LastSyncCompletedAt = (%q, %v), want the new instant", got, err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 {
			t.Errorf(".vault-meta holds %v (%v), want the record alone", entries, err)
		}
	})

	t.Run("through a link", func(t *testing.T) {
		root := t.TempDir()
		target := writeVaultFile(t, root, "notes/state.json", "{}\n", 0o644)
		if err := os.MkdirAll(filepath.Join(root, ".vault-meta"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, filepath.FromSlash(syncStateRel))
		symlinkOrSkip(t, "../notes/state.json", link)
		if err := New(root).RecordSyncCompleted(at); err != nil {
			t.Fatalf("RecordSyncCompleted: %v", err)
		}
		if data, _ := os.ReadFile(target); !strings.Contains(string(data), "2026-10-10T12:00:00Z") {
			t.Errorf("the file the link names holds %q, want the new record", data)
		}
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the link was replaced by a file (%v, %v)", info, err)
		}
	})

	t.Run("a failure names what failed and leaves the record as it was", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, filepath.FromSlash(syncStateRel))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		err := New(root).RecordSyncCompleted(at)
		if err == nil || !strings.HasPrefix(err.Error(), "durable: ") {
			t.Fatalf("error = %v, want one that begins with what failed", err)
		}
		if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
			t.Errorf("the directory in the record's place was disturbed (%v, %v)", info, statErr)
		}
	})
}

// The instant is written as it was given, in RFC 3339: the caller decides the zone.
func TestRecordingASyncWritesTheInstantInTheZoneItWasGiven(t *testing.T) {
	root := t.TempDir()
	zone := time.FixedZone("UTC+2", 2*60*60)

	if err := New(root).RecordSyncCompleted(time.Date(2026, 10, 10, 14, 0, 0, 0, zone)); err != nil {
		t.Fatalf("RecordSyncCompleted: %v", err)
	}
	if got, err := New(root).LastSyncCompletedAt(); err != nil || got != "2026-10-10T14:00:00+02:00" {
		t.Fatalf("LastSyncCompletedAt = (%q, %v), want %q", got, err, "2026-10-10T14:00:00+02:00")
	}
}

// The last completion is what the record holds, as it holds it. A vault that has never synced has no record,
// and a record that holds no completion has none either: both answer nothing, and no error.
func TestTheLastSyncIsWhatTheRecordHolds(t *testing.T) {
	for _, tc := range []struct {
		name, record, want string
	}{
		{"a completion", `{"schema":1,"last_sync_completed_at":"2026-08-31T12:00:00Z"}`, "2026-08-31T12:00:00Z"},
		{"a completion that is not an instant", `{"last_sync_completed_at":"yesterday"}`, "yesterday"},
		{"no completion", `{"schema":1}`, ""},
		{"an empty completion", `{"last_sync_completed_at":""}`, ""},
		{"the JSON null", `null`, ""},
		{"fields it does not know", `{"schema":2,"last_sync_completed_at":"x","note":[1,2]}`, "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeVaultFile(t, root, syncStateRel, tc.record, 0o644)
			got, err := New(root).LastSyncCompletedAt()
			if err != nil || got != tc.want {
				t.Fatalf("LastSyncCompletedAt = (%q, %v), want (%q, nil)", got, err, tc.want)
			}
		})
	}

	t.Run("a vault with no record", func(t *testing.T) {
		got, err := New(t.TempDir()).LastSyncCompletedAt()
		if err != nil || got != "" {
			t.Fatalf("LastSyncCompletedAt = (%q, %v), want nothing", got, err)
		}
	})

	t.Run("a record that is a link", func(t *testing.T) {
		root := t.TempDir()
		writeVaultFile(t, root, "notes/state.json", `{"last_sync_completed_at":"2026-01-01T00:00:00Z"}`, 0o644)
		if err := os.MkdirAll(filepath.Join(root, ".vault-meta"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, "../notes/state.json", filepath.Join(root, filepath.FromSlash(syncStateRel)))
		if got, err := New(root).LastSyncCompletedAt(); err != nil || got != "2026-01-01T00:00:00Z" {
			t.Fatalf("LastSyncCompletedAt = (%q, %v), want the instant of the file the link names", got, err)
		}
	})
}

// A record that cannot be read, or is not the object it should be, is an error that names the file and says whether
// it could not read it or could not understand it; it is no completion.
func TestARecordThatCannotBeReadOrUnderstoodIsAnErrorThatNamesIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root, path string)
		want  string
	}{
		{"not JSON", func(t *testing.T, root, _ string) { writeVaultFile(t, root, syncStateRel, "{not json", 0o644) }, "parse "},
		{"empty", func(t *testing.T, root, _ string) { writeVaultFile(t, root, syncStateRel, "", 0o644) }, "parse "},
		{"a completion that is a number", func(t *testing.T, root, _ string) {
			writeVaultFile(t, root, syncStateRel, `{"last_sync_completed_at":5}`, 0o644)
		}, "parse "},
		{"a directory", func(t *testing.T, _, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}, "read "},
		{"a link to nothing", func(t *testing.T, _, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/absent.json", path)
		}, ""},
		{".vault-meta a file", func(t *testing.T, root, _ string) { writeVaultFile(t, root, ".vault-meta", "x", 0o644) }, "read "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(syncStateRel))
			tc.setup(t, root, path)

			got, err := New(root).LastSyncCompletedAt()
			if tc.want == "" {
				// A link to nothing is no record: the file is not there.
				if err != nil || got != "" {
					t.Fatalf("LastSyncCompletedAt = (%q, %v), want nothing and no error", got, err)
				}
				return
			}
			if err == nil || got != "" {
				t.Fatalf("LastSyncCompletedAt = (%q, %v), want an error and no completion", got, err)
			}
			if want := tc.want + path + ": "; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
		})
	}
}
