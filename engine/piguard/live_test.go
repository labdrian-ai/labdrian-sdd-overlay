package piguard_test

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

// writeExecutable makes an executable file at path and returns the path.
func writeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// unsetEnv removes name for the length of the test; t.Setenv puts the original back afterwards.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

// listSeparator is what joins the entries of a PATH.
const listSeparator = string(os.PathListSeparator)

// The way past the guard opens for one request only: the exact value 1 of the variable. A real pi
// stands on the PATH here, so what refuses is the missing request and nothing else.
func TestRequestLiveStaysClosedUnlessTheRunAskedForTheRealPi(t *testing.T) {
	skipOnWindows(t)
	machine := t.TempDir()
	writeExecutable(t, filepath.Join(machine, "pi"))
	t.Setenv("PATH", machine)

	cases := []struct {
		name  string
		value string
		set   bool
	}{
		{"unset", "", false},
		{"empty", "", true},
		{"zero", "0", true},
		{"true", "true", true},
		{"yes", "yes", true},
		{"padded", " 1", true},
		{"eleven", "11", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv(piguard.LiveVariable, c.value)
			} else {
				unsetEnv(t, piguard.LiveVariable)
			}
			live, err := piguard.RequestLive()
			if !errors.Is(err, piguard.ErrLiveNotRequested) {
				t.Fatalf("RequestLive with %s %s = %v, want ErrLiveNotRequested", piguard.LiveVariable, c.name, err)
			}
			if live != (piguard.Live{}) {
				t.Errorf("a refused request still returned %+v", live)
			}
		})
	}
}

// What the request hands over is the PATH of the run without the guard, in the order it had, and
// the pi of the machine that stands first on it. The PATH of the run itself is left alone, so the
// guard still stands for every test that did not ask.
func TestRequestLiveHandsOverThePathWithoutTheGuard(t *testing.T) {
	skipOnWindows(t)
	machine, rest := t.TempDir(), t.TempDir()
	realPi := writeExecutable(t, filepath.Join(machine, "pi"))
	t.Setenv("PATH", machine+listSeparator+rest)
	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	t.Setenv(piguard.LiveVariable, "1")

	live, err := piguard.RequestLive()
	if err != nil {
		t.Fatalf("RequestLive: %v", err)
	}
	if live.Pi != realPi {
		t.Errorf("Pi = %q, want the pi of the machine %q", live.Pi, realPi)
	}
	if want := machine + listSeparator + rest; live.Path != want {
		t.Errorf("Path = %q, want the PATH of the run without the guard, %q", live.Path, want)
	}
	path, err := exec.LookPath("pi")
	if err != nil || !strings.HasPrefix(filepath.Base(filepath.Dir(path)), piguard.GuardDirPrefix) {
		t.Errorf("after RequestLive a pi looked up by name is %q (%v), want the guard still first on the PATH of the run", path, err)
	}
}

// A guard is known by the name of its directory, wherever it stands on the PATH, and the real pi
// found is the first one outside it.
func TestRequestLiveDropsAGuardWhereverItStandsOnThePath(t *testing.T) {
	skipOnWindows(t)
	first, second, rest := t.TempDir(), t.TempDir(), t.TempDir()
	guardDir := filepath.Join(t.TempDir(), piguard.GuardDirPrefix+"abc123")
	if err := os.Mkdir(guardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(guardDir, "pi"))
	realPi := writeExecutable(t, filepath.Join(second, "pi"))
	t.Setenv("PATH", strings.Join([]string{first, guardDir, second, rest}, listSeparator))
	t.Setenv(piguard.LiveVariable, "1")

	live, err := piguard.RequestLive()
	if err != nil {
		t.Fatalf("RequestLive: %v", err)
	}
	if live.Pi != realPi {
		t.Errorf("Pi = %q, want %q, the first pi outside the guard", live.Pi, realPi)
	}
	if want := strings.Join([]string{first, second, rest}, listSeparator); live.Path != want {
		t.Errorf("Path = %q, want %q", live.Path, want)
	}
}

// A request with nothing to run says so, in words that are not the refusal of a missing request: a
// directory called pi and a file that is not executable are not a pi, and neither is the guard.
func TestRequestLiveFailsWhenNoRealPiStandsBehindTheGuard(t *testing.T) {
	skipOnWindows(t)
	notExecutable, withDirectory := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(notExecutable, "pi"), []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(withDirectory, "pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", notExecutable+listSeparator+withDirectory)
	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	t.Setenv(piguard.LiveVariable, "1")

	live, err := piguard.RequestLive()
	if err == nil {
		t.Fatalf("RequestLive = %+v, want an error: there is no real pi on the PATH", live)
	}
	if errors.Is(err, piguard.ErrLiveNotRequested) {
		t.Errorf("the error is the refusal of a missing request, but the request was made: %v", err)
	}
	if !strings.Contains(err.Error(), "no pi") {
		t.Errorf("error = %q, want it to say that no pi stands on the PATH behind the guard", err)
	}
}

// A relative entry of the PATH means the working directory, and a pi left there is not the pi of
// the machine.
func TestRequestLiveIgnoresARelativeEntryOfThePath(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(dir, "bin", "pi"))
	// t.Chdir needs a newer Go than this module declares, so the working directory is put back by hand.
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(before); err != nil {
			t.Errorf("put the working directory back: %v", err)
		}
	})
	t.Setenv("PATH", "bin"+listSeparator+listSeparator) // a relative entry, and an empty one
	t.Setenv(piguard.LiveVariable, "1")

	if live, err := piguard.RequestLive(); err == nil {
		t.Fatalf("RequestLive = %+v, want an error: the only pi stands in a relative directory", live)
	}
}

// Install makes the directory of the guard under the prefix the live request knows it by.
func TestInstallNamesTheGuardDirectoryByThePrefix(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("PATH", os.Getenv("PATH"))
	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	path, err := exec.LookPath("pi")
	if err != nil {
		t.Fatalf("LookPath(pi): %v", err)
	}
	if base := filepath.Base(filepath.Dir(path)); !strings.HasPrefix(base, piguard.GuardDirPrefix) {
		t.Errorf("the guard directory is %q, want a name that starts with %q", base, piguard.GuardDirPrefix)
	}
}

// RealHome is the home the user database names, not the one HOME says: a TestMain pins HOME to a
// throwaway directory, and the person's own home is the one a test must never reach.
func TestRealHomeIsTheHomeOfTheAccountNotHOME(t *testing.T) {
	account, err := user.Current()
	if err != nil {
		t.Skipf("the current account is unknown here: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	if account.HomeDir == os.Getenv("HOME") {
		t.Skip("the account has no entry in the user database here, so its home is what HOME says")
	}
	got, err := piguard.RealHome()
	if err != nil {
		t.Fatalf("RealHome: %v", err)
	}
	if got != account.HomeDir {
		t.Errorf("RealHome = %q, want the home of the account, %q", got, account.HomeDir)
	}
}

func TestRefuseRealHomeRefusesWhatIsNotAnAbsolutePath(t *testing.T) {
	for _, home := range []string{"", ".", "relative/home"} {
		if err := piguard.RefuseRealHome(home); err == nil {
			t.Errorf("RefuseRealHome(%q) = nil, want a refusal: a throwaway HOME is an absolute path", home)
		}
	}
}

func TestRefuseRealHomeRefusesTheHomeOfTheAccount(t *testing.T) {
	home, err := piguard.RealHome()
	if err != nil {
		t.Skipf("the home of the account is unknown here: %v", err)
	}
	if _, err := os.Stat(home); err != nil {
		t.Skipf("the home of the account does not exist here: %v", err)
	}
	t.Setenv("HOME", t.TempDir()) // as a TestMain leaves it: the real home is not what HOME says
	err = piguard.RefuseRealHome(home)
	if err == nil {
		t.Fatalf("RefuseRealHome(%q) = nil, want a refusal: it is the home of the account running the tests", home)
	}
	// The real home may well hold a Pi installation, which would refuse it for another reason: the
	// reason is what proves the account's home is known as such.
	if !strings.Contains(err.Error(), home) || !strings.Contains(err.Error(), "home of the account") {
		t.Errorf("error = %q, want it to name the home it refused and to say that it is the home of the account", err)
	}
}

func TestRefuseRealHomeRefusesTheHomeThisProcessRunsWith(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := piguard.RefuseRealHome(home)
	if err == nil {
		t.Fatalf("RefuseRealHome(%q) = nil, want a refusal: it is the HOME of this process", home)
	}
	if !strings.Contains(err.Error(), "HOME this test process runs with") {
		t.Errorf("error = %q, want it to say that the directory is the HOME this test process runs with", err)
	}
}

// A directory is the same home through a link: the refusal reads the directory, not the spelling.
func TestRefuseRealHomeRefusesTheSameHomeThroughASymlink(t *testing.T) {
	skipOnWindows(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	if err := piguard.RefuseRealHome(link); err == nil {
		t.Errorf("RefuseRealHome(%q) = nil, want a refusal: it leads to the HOME of this process", link)
	}
}

// A directory that already holds a Pi installation is not a throwaway one, whatever its name.
func TestRefuseRealHomeRefusesADirectoryThatHoldsAPiInstallation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home := t.TempDir()
	settings := filepath.Join(home, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := piguard.RefuseRealHome(home)
	if err == nil {
		t.Fatalf("RefuseRealHome(%q) = nil, want a refusal: it holds %s", home, settings)
	}
	if !strings.Contains(err.Error(), "Pi installation") {
		t.Errorf("error = %q, want it to say that the directory holds a Pi installation", err)
	}
}

func TestRefuseRealHomeAcceptsAFreshDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := piguard.RefuseRealHome(t.TempDir()); err != nil {
		t.Errorf("RefuseRealHome(a fresh directory) = %v, want nil", err)
	}
}
