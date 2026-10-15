package installer_test

// The longterm-mem pins of `uninstall` (longterm_pins_world_test.go has the world): which targets the
// script takes out of its record of the installed targets, what it says when others remain, and when it
// removes the engine's registration record and the shared binary with the record itself. Each pin is
// the exit code and both streams of a run, the state directory it left and the calls the longterm-mem
// was given.

import (
	"os"
	"strings"
	"testing"
	"time"
)

// uninstallWorld is a world that has the longterm-mem deployed and the record of the installed targets
// as given (none, for an empty string and no record, is seedNothing).
func uninstallWorld(t *testing.T, tracking string) *pinWorld {
	t.Helper()
	w := longtermWorld(t)
	w.placeBinary()
	if tracking != seedNothing {
		w.seedTracking(tracking)
	}
	return w
}

// seedNothing is the content that stands for a record that is not there at all.
const seedNothing = "\x00no record\x00"

func TestLongtermPins_UninstallTakesOneTargetOutAndKeepsTheBinaryWhileOthersRemain(t *testing.T) {
	for _, tc := range []struct {
		name, tracking string
		args           []string
	}{
		{"one-of-three", "claude\nopencode\ncodex\n", []string{"--target", "claude"}},
		{"sorted-and-once", "opencode\nclaude\nclaude\n", []string{"--target", "opencode"}},
		{"a-target-that-was-not-recorded", "claude\n", []string{"--target", "codex"}},
		{"the-name-of-a-newer-build-stays", "claude\nzed\n", []string{"--target", "all"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := uninstallWorld(t, tc.tracking)
			pinLongterm(t, w, "longterm/uninstall/"+tc.name,
				w.runLongterm(append([]string{"longterm-mem", "uninstall"}, tc.args...)...))
		})
	}
}

func TestLongtermPins_UninstallOfTheLastTargetRemovesTheRecordAndTheBinary(t *testing.T) {
	for _, tc := range []struct {
		name, tracking string
		args           []string
	}{
		{"last-target", "codex\n", []string{"--target", "codex"}},
		{"all-four", "claude\nopencode\ncodex\npi\n", []string{"--target", "all"}},
		{"an-empty-record", "", []string{"--target", "claude"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := uninstallWorld(t, tc.tracking)
			pinLongterm(t, w, "longterm/uninstall/"+tc.name,
				w.runLongterm(append([]string{"longterm-mem", "uninstall"}, tc.args...)...))
		})
	}
}

func TestLongtermPins_UninstallWithoutARecordLeavesTheBinaryUnlessPurged(t *testing.T) {
	w := uninstallWorld(t, seedNothing)
	pinLongterm(t, w, "longterm/uninstall/no-record", w.runLongterm("longterm-mem", "uninstall", "--target", "claude"))

	purged := uninstallWorld(t, seedNothing)
	pinLongterm(t, purged, "longterm/uninstall/no-record-purge", purged.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))

	bare := longtermWorld(t)
	pinLongterm(t, bare, "longterm/uninstall/no-record-no-binary-purge", bare.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))
}

func TestLongtermPins_UninstallPurgeRemovesTheBinaryWhateverIsRecorded(t *testing.T) {
	w := uninstallWorld(t, "claude\nopencode\ncodex\n")
	pinLongterm(t, w, "longterm/uninstall/purge-with-others-recorded", w.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))

	failing := uninstallWorld(t, "claude\n")
	failing.setEnv("FAKE_UNREGISTER_EXIT", "1")
	pinLongterm(t, failing, "longterm/uninstall/purge-through-a-failed-unregister", failing.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))

	absent := longtermWorld(t)
	absent.seedTracking("claude\n")
	pinLongterm(t, absent, "longterm/uninstall/purge-binary-already-absent", absent.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))
}

func TestLongtermPins_UninstallKeepsATargetRecordedWhenItsEntryWasNotRemoved(t *testing.T) {
	for _, tc := range []struct{ name, exit string }{
		{"unmanaged-entry", "6"},
		{"version-skew-clears-it", "2"},
		{"unresolvable-path", "8"},
		{"hard-failure", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := uninstallWorld(t, "claude\n")
			w.setEnv("FAKE_UNREGISTER_EXIT", tc.exit)
			pinLongterm(t, w, "longterm/uninstall/unregister-"+tc.name, w.runLongterm("longterm-mem", "uninstall", "--target", "claude"))
		})
	}

	mixed := uninstallWorld(t, "claude\nopencode\ncodex\npi\n")
	mixed.setEnv("FAKE_UNREGISTER_EXIT_opencode", "6")
	mixed.setEnv("FAKE_UNREGISTER_EXIT_codex", "1")
	pinLongterm(t, mixed, "longterm/uninstall/unregister-mixed-under-all", mixed.runLongterm("longterm-mem", "uninstall", "--target", "all"))
}

func TestLongtermPins_UninstallWithoutAUsableBinaryClearsTheRecordWithoutUnregistering(t *testing.T) {
	missing := longtermWorld(t)
	missing.seedTracking("claude\nopencode\n")
	pinLongterm(t, missing, "longterm/uninstall/binary-missing-others-recorded", missing.runLongterm("longterm-mem", "uninstall", "--target", "claude"))

	last := longtermWorld(t)
	last.seedTracking("codex\n")
	pinLongterm(t, last, "longterm/uninstall/binary-missing-last-target", last.runLongterm("longterm-mem", "uninstall", "--target", "codex"))

	plain := uninstallWorld(t, "claude\nopencode\n")
	if err := os.Chmod(plain.sharedBinary(), 0o644); err != nil {
		t.Fatal(err)
	}
	pinLongterm(t, plain, "longterm/uninstall/binary-not-executable", plain.runLongterm("longterm-mem", "uninstall", "--target", "opencode"))
}

func TestLongtermPins_UninstallReadsTheRecordAsEvidenceAndDropsOnlyWhatIsNotAName(t *testing.T) {
	for _, tc := range []struct {
		name, tracking, target string
	}{
		{"a-nul-byte-makes-a-name-garbage", "\x00zed\n", "claude"},
		{"a-name-cut-short-is-garbage", "clau", "claude"},
		{"a-name-cut-short-keeps-the-lines-above", "claude\nopenc", "codex"},
		{"garbage-is-compacted-away", "\x00garbage\n{not-a-target}\n\nclaude\nopencode\n", "opencode"},
		{"a-carriage-return-is-garbage", "claude\r\ncodex\n", "codex"},
		{"a-padded-name-is-garbage", " claude\nclaude \n", "codex"},
		{"a-name-of-64-bytes-is-a-name", strings.Repeat("a", 64) + "\n", "claude"},
		{"a-name-of-65-bytes-is-garbage", strings.Repeat("a", 65) + "\n", "claude"},
		{"a-name-may-not-begin-with-a-dot", ".hidden\nclaude\n", "claude"},
		{"dots-dashes-and-underscores-are-names", "my-runtime_2.0\nclaude\n", "claude"},
		{"only-the-whole-name-is-taken-out", "claude-next\nmy-claude\nclaude\n", "claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := uninstallWorld(t, tc.tracking)
			pinLongterm(t, w, "longterm/uninstall/record-"+tc.name, w.runLongterm("longterm-mem", "uninstall", "--target", tc.target))
		})
	}
}

// unreadableRecordWorld is a world that has a record of the installed targets nobody can read.
func unreadableRecordWorld(t *testing.T, pins ...string) *pinWorld {
	t.Helper()
	w := uninstallWorld(t, "claude\nopencode\n")
	if canReadWhatIsMode0(t, w.stateDir()) {
		pinSkipped(t, "this user can read a file whose mode is 0", pins...)
	}
	if err := os.Chmod(w.trackingFile(), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(w.trackingFile(), 0o644) })
	return w
}

func TestLongtermPins_UninstallRefusesToRewriteARecordItCannotRead(t *testing.T) {
	w := unreadableRecordWorld(t, "longterm/uninstall/record-unreadable", "longterm/uninstall/record-unreadable-purge")
	pinLongterm(t, w, "longterm/uninstall/record-unreadable", w.runLongterm("longterm-mem", "uninstall", "--target", "claude"))
	// The same, asked to purge: the rewrite of the record is not what --purge skips.
	purged := unreadableRecordWorld(t, "longterm/uninstall/record-unreadable-purge")
	pinLongterm(t, purged, "longterm/uninstall/record-unreadable-purge", purged.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))
}

func TestLongtermPins_UninstallKeepsTheBinaryWhenTheRecordItDoesNotRewriteCannotBeRead(t *testing.T) {
	// An unmanaged entry is not taken out of the record, so the record is read only to decide.
	w := unreadableRecordWorld(t, "longterm/uninstall/record-unreadable-nothing-rewritten", "longterm/uninstall/record-unreadable-nothing-rewritten-purge")
	w.setEnv("FAKE_UNREGISTER_EXIT", "6")
	pinLongterm(t, w, "longterm/uninstall/record-unreadable-nothing-rewritten", w.runLongterm("longterm-mem", "uninstall", "--target", "claude"))

	purged := unreadableRecordWorld(t, "longterm/uninstall/record-unreadable-nothing-rewritten-purge")
	purged.setEnv("FAKE_UNREGISTER_EXIT", "6")
	pinLongterm(t, purged, "longterm/uninstall/record-unreadable-nothing-rewritten-purge", purged.runLongterm("longterm-mem", "uninstall", "--target", "claude", "--purge"))
}

func TestLongtermPins_UninstallStopsWhenTheRecordCannotBeRewritten(t *testing.T) {
	w := uninstallWorld(t, "claude\nopencode\n")
	dir := w.trackingFile()[:strings.LastIndex(w.trackingFile(), "/")]
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	probe := dir + "/probe"
	if f, err := os.Create(probe); err == nil {
		f.Close()
		os.Remove(probe)
		pinSkipped(t, "this user can write into a directory whose mode is 0555", "longterm/uninstall/record-directory-is-read-only")
	}
	pinLongterm(t, w, "longterm/uninstall/record-directory-is-read-only", w.runLongterm("longterm-mem", "uninstall", "--target", "claude"))
}

func TestLongtermPins_UninstallWaitsForTheLockOfTheRecordAndRefusesWhenItStaysTaken(t *testing.T) {
	w := uninstallWorld(t, "claude\nopencode\n")
	release := w.holdLock()
	run := w.runLongterm("longterm-mem", "uninstall", "--target", "claude")
	release()
	pinLongterm(t, w, "longterm/uninstall/lock-held", run)

	// The directory the script's lock used to be, left behind by a run that was killed, is left alone.
	stale := uninstallWorld(t, "claude\nopencode\n")
	stale.leftLockDirectory(10 * time.Minute)
	pinLongterm(t, stale, "longterm/uninstall/lock-abandoned", stale.runLongterm("longterm-mem", "uninstall", "--target", "claude"))
}
