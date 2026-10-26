package piguard

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// The guard keeps every test away from the real `pi`. A test that has to run the real one (the
// live test of the Pi global tier) does not weaken it: it asks, and the way past the guard opens
// for that one request, for the exact value 1 of LiveVariable. The test then starts the program
// that runs `pi` with a HOME of its own, which it has had RefuseRealHome accept first. Nothing
// here starts a process, and the PATH of the run is never changed.

// GuardDirPrefix begins the name of the directory Install makes for the guard. It is how a request
// for the real pi tells the guard's entry on a PATH from the others.
const GuardDirPrefix = "pi-guard-"

// LiveVariable is the environment variable that lets a test run the real `pi`. Only the exact
// value 1 does: any other value, the empty one included, leaves the guard in force.
const LiveVariable = "LABDRIAN_PI_LIVE_TEST"

// ErrLiveNotRequested is what RequestLive returns when the person running the tests did not ask
// for the real pi. A test skips on it, with this text as the reason.
var ErrLiveNotRequested = errors.New("the real pi is not requested: export " + LiveVariable + "=1 to run it against a throwaway HOME")

// Live is what a request for the real pi grants.
type Live struct {
	// Pi is the pi of the machine: the first one on the PATH of the run outside the guard.
	Pi string
	// Path is the PATH to hand the program that starts it: the PATH of the run without the guard,
	// in the order it had. The PATH of the run itself is left as it was, guard first.
	Path string
}

// RequestLive grants the real pi to a test that asked for it. It returns ErrLiveNotRequested
// unless LiveVariable is exactly 1, and an error of its own when it is and no pi stands on the PATH
// behind the guard, so a person who opted in is never told a test passed that had nothing to run.
func RequestLive() (Live, error) {
	if os.Getenv(LiveVariable) != "1" {
		return Live{}, ErrLiveNotRequested
	}
	path := withoutGuard(os.Getenv("PATH"))
	pi, ok := firstPi(path)
	if !ok {
		return Live{}, fmt.Errorf("%s=1 asks for the real pi, but no pi stands on the PATH of the run behind the guard", LiveVariable)
	}
	return Live{Pi: pi, Path: path}, nil
}

// withoutGuard is pathList without the directories of a guard, which are the ones whose name
// begins with GuardDirPrefix, wherever they stand, and in the order the rest had.
func withoutGuard(pathList string) string {
	var kept []string
	for _, dir := range filepath.SplitList(pathList) {
		if strings.HasPrefix(filepath.Base(dir), GuardDirPrefix) {
			continue
		}
		kept = append(kept, dir)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

// firstPi is the first executable file named pi in the absolute directories of pathList. A
// relative or empty entry means the working directory, which is not where the pi of a machine is
// looked for; a directory, or a file that cannot be run, is not a pi.
func firstPi(pathList string) (string, bool) {
	for _, dir := range filepath.SplitList(pathList) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, "pi")
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

// RealHome is the home directory of the account that runs the tests, as the user database names
// it and not as HOME says: a TestMain pins HOME to a throwaway directory, and the person's own
// home is the one a test must never hand to the real pi. Where the account has no entry in the
// user database the standard library falls back to HOME, so there the answer is HOME's.
func RealHome() (string, error) {
	account, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("piguard: the current account is unknown: %w", err)
	}
	if account.HomeDir == "" {
		return "", errors.New("piguard: the current account has no home directory")
	}
	return account.HomeDir, nil
}

// RefuseRealHome is nil when home is a directory a test may hand to a program as its HOME, and an
// error that says why not otherwise: it is not an absolute path, it is the home of the account
// running the tests (RealHome), it is the HOME this test process runs with, or it already holds a
// Pi installation (~/.pi/agent/settings.json), which a throwaway home never does. Directories are
// compared as directories, so a symlink to the real home is the real home.
func RefuseRealHome(home string) error {
	if home == "" || !filepath.IsAbs(home) {
		return fmt.Errorf("piguard: the HOME of a test that may run the real pi must be an absolute path, got %q", home)
	}
	if real, err := RealHome(); err == nil && sameDirectory(home, real) {
		return fmt.Errorf("piguard: %q is the home of the account running the tests (%s): a test may not hand it to the real pi", home, real)
	}
	if running := os.Getenv("HOME"); running != "" && sameDirectory(home, running) {
		return fmt.Errorf("piguard: %q is the HOME this test process runs with: a throwaway HOME is a directory of its own", home)
	}
	settings := filepath.Join(home, ".pi", "agent", "settings.json")
	if _, err := os.Stat(settings); err == nil {
		return fmt.Errorf("piguard: %q already holds a Pi installation (%s): a throwaway HOME is a fresh directory", home, settings)
	}
	return nil
}

// sameDirectory reports whether a and b are the same directory. A path that cannot be read is
// compared by its spelling, which is all there is to compare.
func sameDirectory(a, b string) bool {
	left, err := os.Stat(a)
	if err != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	right, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(left, right)
}
