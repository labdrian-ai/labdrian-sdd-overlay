package installer_test

// The longterm-mem pins of `install` (longterm_pins_world_test.go has the world): which targets the
// script writes in its record of the installed targets after the longterm-mem has registered them, and
// what it does with a record it finds as it is (an unknown name, bytes that are not names, a name cut
// short). Each pin is the exit code and both streams of a run, the state directory it left and the
// calls the longterm-mem was given.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLongtermPins_InstallRecordsTheTargetsItRegistered(t *testing.T) {
	w := longtermWorld(t)
	pinLongterm(t, w, "longterm/install/one-target", w.runLongterm("longterm-mem", "install", "--target", "claude"))
	pinLongterm(t, w, "longterm/install/one-target-again", w.runLongterm("longterm-mem", "install", "--target", "claude"))
	pinLongterm(t, w, "longterm/install/a-second-target", w.runLongterm("longterm-mem", "install", "--target", "codex"))

	all := longtermWorld(t)
	pinLongterm(t, all, "longterm/install/all", all.runLongterm("longterm-mem", "install", "--target", "all"))
}

func TestLongtermPins_InstallRecordsNothingForATargetRegisterDidNotRegister(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit string
	}{
		{"refused", "6"},
		{"hard-failure", "1"},
		{"version-skew", "2"},
		{"unresolvable-path", "8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := longtermWorld(t)
			w.setEnv("FAKE_REGISTER_EXIT", tc.exit)
			pinLongterm(t, w, "longterm/install/register-"+tc.name, w.runLongterm("longterm-mem", "install", "--target", "claude"))
		})
	}
}

func TestLongtermPins_InstallUnderAllRecordsWhatRegisteredAndSoftensWhatLooksLikeAMissingRuntime(t *testing.T) {
	mixed := longtermWorld(t)
	mixed.setEnv("FAKE_REGISTER_EXIT_opencode", "1")
	mixed.setEnv("FAKE_REGISTER_EXIT_codex", "6")
	pinLongterm(t, mixed, "longterm/install/all-mixed", mixed.runLongterm("longterm-mem", "install", "--target", "all"))

	for _, tc := range []struct{ name, exit string }{
		{"all-every-register-fails", "1"},
		{"all-every-register-is-version-skewed", "2"},
		{"all-every-path-is-unresolvable", "8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := longtermWorld(t)
			w.setEnv("FAKE_REGISTER_EXIT", tc.exit)
			pinLongterm(t, w, "longterm/install/"+tc.name, w.runLongterm("longterm-mem", "install", "--target", "all"))
		})
	}
}

func TestLongtermPins_InstallStopsBeforeRecordingWhenTheBinaryCannotBeBuilt(t *testing.T) {
	w := longtermWorld(t)
	w.setEnv("FAKE_GO_FAIL", "1")
	pinLongterm(t, w, "longterm/install/build-fails", w.runLongterm("longterm-mem", "install", "--target", "claude"))
}

func TestLongtermPins_InstallAppendsToTheRecordItFindsAndKeepsWhatIsInIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		target  string
	}{
		{"an-unknown-name-stays", "zed\n", "claude"},
		{"the-same-name-twice-is-recorded-once", "claude\nclaude\n", "claude"},
		{"names-in-any-order", "opencode\nclaude\n", "codex"},
		{"bytes-that-are-not-names-stay", "\x00garbage\n{not-a-target}\n\n", "claude"},
		{"a-name-cut-short-runs-into-the-next-one", "claude\nopenc", "opencode"},
		{"a-name-with-a-carriage-return", "claude\r\n", "claude"},
		{"an-empty-file", "", "pi"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := longtermWorld(t)
			w.seedTracking(tc.content)
			pinLongterm(t, w, "longterm/install/record-"+tc.name, w.runLongterm("longterm-mem", "install", "--target", tc.target))
		})
	}
}

func TestLongtermPins_InstallRefusesToRecordAgainstARecordItCannotRead(t *testing.T) {
	w := longtermWorld(t)
	w.seedTracking("claude\n")
	if err := os.Chmod(w.trackingFile(), 0); err != nil {
		t.Fatal(err)
	}
	if canReadWhatIsMode0(t, w.stateDir()) {
		pinSkipped(t, "this user can read a file whose mode is 0", "longterm/install/record-unreadable")
	}
	pinLongterm(t, w, "longterm/install/record-unreadable", w.runLongterm("longterm-mem", "install", "--target", "opencode"))
	// A file the pin could not read back is shown by what the script left of it.
	if err := os.Chmod(w.trackingFile(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLongtermPins_InstallStopsWhenTheRecordCannotHaveItsDirectory(t *testing.T) {
	w := longtermWorld(t)
	// A file where the directory of the record should be.
	w.write(w.stateDir(), "longterm-mem", "in the way\n")
	if err := os.Chmod(filepath.Join(w.stateDir(), "longterm-mem"), 0o644); err != nil {
		t.Fatal(err)
	}
	pinLongterm(t, w, "longterm/install/record-directory-is-a-file", w.runLongterm("longterm-mem", "install", "--target", "claude"))
}

func TestLongtermPins_InstallKeepsTheRecordUnderTheCleanedStateDirectory(t *testing.T) {
	w := longtermWorld(t)
	state := filepath.Join(w.home, "elsewhere") + "//deeper/../state/"
	w.setEnv("STATE_DIR", state)
	run := w.runLongterm("longterm-mem", "install", "--target", "claude")
	// The state is shown under the directory the script cleaned it to, which is where the record is.
	pinLongterm(t, w, "longterm/install/state-directory-is-cleaned", run)
}

func TestLongtermPins_InstallWaitsForTheLockOfTheRecordAndRefusesWhenItStaysTaken(t *testing.T) {
	w := longtermWorld(t)
	w.heldLock(0)
	pinLongterm(t, w, "longterm/install/lock-held", w.runLongterm("longterm-mem", "install", "--target", "claude"))

	stale := longtermWorld(t)
	stale.heldLock(10 * time.Minute)
	pinLongterm(t, stale, "longterm/install/lock-abandoned", stale.runLongterm("longterm-mem", "install", "--target", "claude"))
}
