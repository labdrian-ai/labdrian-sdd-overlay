// Package overlaytest is test support: the world the tests of the overlay script (bin/labdrian-overlay)
// run it in. It is imported by test files only (the tests of engine/installer and engine/shelltest).
//
// A test of the script needs the same few things each time, and before this package each test file made
// them again: where the script and the checkout are, an environment that names a home and an overlay
// and takes nothing from the machine, a throwaway overlay repository with the branches the script
// merges and compares, programs of its own first on the PATH, and a way to run the script that tells
// the two output streams and the exit code apart. They are here once.
//
//   - Script, RepoRoot: where the script and the checkout are.
//   - Env, GitSettings: the environment a run takes, and the part of it every git of a fixture takes.
//   - NewOverlay, InitOverlay, Git: the fixture repository.
//   - Run, Combined, Source: running the script, or the functions it defines.
//   - WriteTool, StubEngine, ToolchainEnv: programs of the test's own, and the caches of the go command.
//
// The engine the script asks for is enginetest's.
package overlaytest

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// RepoRoot is the root of this checkout, found from this file and not from the directory a test runs in
// (as enginetest finds the engine). It fails the test when the directory it finds holds no overlay.
func RepoRoot(t testing.TB) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	if _, err := os.Stat(filepath.Join(root, "overlay.manifest")); err != nil {
		t.Fatalf("the root of the checkout is not at %s: %v", root, err)
	}
	return root
}

// Script is the absolute path of bin/labdrian-overlay.
func Script(t testing.TB) string {
	t.Helper()
	p := filepath.Join(RepoRoot(t), "bin", "labdrian-overlay")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("overlay script not found at %s: %v", p, err)
	}
	return p
}

// RequireBash skips the test on a machine that has no bash: the script is a bash script.
func RequireBash(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not available: %v", err)
	}
}

// Env is the environment a run of the script takes, and only that: the home and the overlay it is told
// of, the PATH of the machine, and what GitSettings says. The environment of the test process, which
// holds whatever whoever runs the tests has exported, is not in it. extra goes last, so it wins.
func Env(home, overlay string, extra ...string) []string {
	env := []string{"HOME=" + home, "OVERLAY_DIR=" + overlay, "PATH=" + os.Getenv("PATH")}
	env = append(env, GitSettings()...)
	return append(env, extra...)
}

// Result is what one run of the script left: what it wrote on each stream and how it ended.
type Result struct {
	Stdout string
	Stderr string
	Exit   int
}

// Run runs the script with the arguments, from dir (the directory of the test when it is empty), in
// env (the environment of the test process when it is nil), and keeps the two streams apart. A script
// that ends with a code is a Result; one that could not be started fails the test.
func Run(t testing.TB, script string, env []string, dir string, args ...string) Result {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		result.Exit = exit.ExitCode()
	default:
		t.Fatalf("run %v: %v", args, err)
	}
	return result
}

// Combined runs the script as Run does and returns what it wrote on both streams, in the order it
// wrote it, and the error the run ended with (an *exec.ExitError for a non-zero exit).
func Combined(t testing.TB, script string, env []string, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Source runs snippet in a bash that has sourced the script, which when it is sourced only defines its
// functions and variables, with args as "$@". It returns what the snippet wrote on both streams and
// the error it ended with.
func Source(t testing.TB, script string, env []string, snippet string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"-c", `source "$1"; shift; ` + snippet, "_", script}, args...)...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}
