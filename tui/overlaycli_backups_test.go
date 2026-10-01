package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseBackupList pins what the TUI assumes about `labdrian-overlay
// restore --target <t> --list`: either the no-backups sentence, or a header
// naming the target followed by one "  <timestamp> (<version>)" line per
// retained backup, newest last. Anything else is an error, never a guess,
// because restore is destructive and is only offered for a backup the backend
// confirmed.
func TestParseBackupList(t *testing.T) {
	const header = "Backups for claude (newest last):\n"

	t.Run("no backups is a clear answer, not an error", func(t *testing.T) {
		b, ok, err := parseBackupList("No backups available for target 'claude'.\n", "claude")
		if err != nil || ok || b != (Backup{}) {
			t.Errorf("parseBackupList = %+v, %v, %v; want no backup, no error", b, ok, err)
		}
	})

	t.Run("the newest backup is the last line", func(t *testing.T) {
		b, ok, err := parseBackupList(header+"  20260101T000000Z (v1.2.0)\n  20260215T120000Z (v1.3.0)\n", "claude")
		if err != nil || !ok {
			t.Fatalf("parseBackupList: ok=%v err=%v", ok, err)
		}
		if b != (Backup{Timestamp: "20260215T120000Z", Version: "v1.3.0"}) {
			t.Errorf("backup = %+v, want the last (newest) entry", b)
		}
	})

	t.Run("a same-second suffix stays part of the timestamp", func(t *testing.T) {
		b, ok, err := parseBackupList(header+"  20260215T120000Z (v1.3.0)\n  20260215T120000Z-1 (v1.4.0)\n", "claude")
		if err != nil || !ok || b.Timestamp != "20260215T120000Z-1" {
			t.Errorf("parseBackupList = %+v, %v, %v; want timestamp 20260215T120000Z-1", b, ok, err)
		}
	})

	t.Run("a version the backend reports as unknown is left empty", func(t *testing.T) {
		b, ok, err := parseBackupList(header+"  20260101T000000Z (unknown)\n", "claude")
		if err != nil || !ok {
			t.Fatalf("parseBackupList: ok=%v err=%v", ok, err)
		}
		if b.Timestamp != "20260101T000000Z" || b.Version != "" {
			t.Errorf("backup = %+v, want the timestamp with an empty version", b)
		}
	})

	t.Run("a release-less version is kept verbatim", func(t *testing.T) {
		b, _, err := parseBackupList(header+"  20260101T000000Z (untagged)\n", "claude")
		if err != nil || b.Version != "untagged" {
			t.Errorf("backup = %+v, err = %v; want version untagged", b, err)
		}
	})

	rejected := []struct{ name, output string }{
		{"empty output", ""},
		{"a header for another target", "Backups for opencode (newest last):\n  20260101T000000Z (v1.2.0)\n"},
		{"the no-backups sentence for another target", "No backups available for target 'opencode'.\n"},
		{"a header without entries", header},
		{"an entry without a version", header + "  20260101T000000Z\n"},
		{"an entry that is not indented", header + "20260101T000000Z (v1.2.0)\n"},
		{"text after the entries", header + "  20260101T000000Z (v1.2.0)\nand then something else\n"},
		{"unrelated output", "Usage: labdrian <command> [options]\n"},
	}
	for _, tc := range rejected {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			b, ok, err := parseBackupList(tc.output, "claude")
			if err == nil {
				t.Fatalf("parseBackupList(%q) = %+v, %v; want an error", tc.output, b, ok)
			}
			if ok || b != (Backup{}) {
				t.Errorf("a rejected listing must report no backup, got %+v, %v", b, ok)
			}
		})
	}
}

// TestOverlayCLI_LatestBackup_AsksTheBackendToListThatTarget: the adapter's
// whole contract with the process -- `restore --target <t> --list`, nothing
// else, from the repo root.
func TestOverlayCLI_LatestBackup_AsksTheBackendToListThatTarget(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	writeStubBackend(t, root, recorder, "Backups for opencode (newest last):\n  20260301T093000Z (v1.5.0)", 0)

	b, ok, err := newOverlayCLI(root).LatestBackup("opencode")
	if err != nil || !ok || b != (Backup{Timestamp: "20260301T093000Z", Version: "v1.5.0"}) {
		t.Fatalf("LatestBackup = %+v, %v, %v", b, ok, err)
	}

	data, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatalf("the backend was never invoked: %v", err)
	}
	_, args, _ := strings.Cut(strings.TrimSpace(string(data)), "|")
	if args != "restore --target opencode --list" {
		t.Errorf("backend invoked with %q, want %q", args, "restore --target opencode --list")
	}
}

// TestOverlayCLI_LatestBackup_FailsClosed: a backend that cannot answer is an
// error, which the model reads as "not confirmed", never as "no backups" nor
// as a backup.
func TestOverlayCLI_LatestBackup_FailsClosed(t *testing.T) {
	t.Run("a backend that exits non-zero, reporting why", func(t *testing.T) {
		root := t.TempDir()
		writeScriptBackend(t, root, `echo "ERROR: restore does not support --target pi" >&2; exit 1`)

		b, ok, err := newOverlayCLI(root).LatestBackup("pi")
		if err == nil || ok || b != (Backup{}) {
			t.Fatalf("LatestBackup = %+v, %v, %v; want an error and no backup", b, ok, err)
		}
		if !strings.Contains(err.Error(), "restore does not support --target pi") {
			t.Errorf("error %q should carry the backend's own explanation", err)
		}
	})

	t.Run("a backend that prints something that is not a listing", func(t *testing.T) {
		root := t.TempDir()
		writeScriptBackend(t, root, `echo "hello"`)

		if b, ok, err := newOverlayCLI(root).LatestBackup("claude"); err == nil || ok {
			t.Fatalf("LatestBackup = %+v, %v, %v; want an error", b, ok, err)
		}
	})

	t.Run("no repo root at all", func(t *testing.T) {
		if _, ok, err := newOverlayCLI("").LatestBackup("claude"); err == nil || ok {
			t.Fatalf("LatestBackup with no backend: ok=%v err=%v; want an error", ok, err)
		}
	})
}
