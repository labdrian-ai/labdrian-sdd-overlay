// Package enginetest is test support: the engine binary, built once for a test binary, and the place in a
// home where the overlay script looks for it.
//
// bin/labdrian-overlay asks the engine (`manifest routes`) where each row of overlay.manifest goes, and
// looks for it at $HOME/.claude/bin/gentle-ai-overlay; when it is not there the script builds it from
// $OVERLAY_DIR/engine. A test that runs a command which reads the manifest in a home of its own, over an
// overlay that holds no engine source, puts a binary where the script looks, and one that runs the script
// over this checkout leaves the script to build it (and pays for it, in output, once). Building the engine
// takes seconds, so it is done once per test binary and copied into each home:
//
//	func TestMain(m *testing.M) {
//		code := testhome.Run(m)
//		enginetest.Cleanup()
//		os.Exit(code)
//	}
//
//	enginetest.Provision(t, home)
package enginetest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// installedName and installedDir are where the script looks for the engine, under a home.
const (
	installedName = "gentle-ai-overlay"
	installedDir  = ".claude/bin"
)

// built is the engine of this test binary, built on the first call to Binary.
var built struct {
	once sync.Once
	dir  string
	path string
	err  error
}

// Binary returns the path of the engine built from this checkout, building it on the first call. The
// build lives in a directory of its own that Cleanup takes away.
func Binary(t testing.TB) string {
	t.Helper()
	built.once.Do(func() {
		dir, err := os.MkdirTemp("", "enginetest-")
		if err != nil {
			built.err = err
			return
		}
		built.dir = dir
		built.path = filepath.Join(dir, installedName)
		cmd := exec.Command("go", "build", "-o", built.path, "./cmd")
		cmd.Dir = engineRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			built.err = fmt.Errorf("go build ./cmd: %v\n%s", err, out)
		}
	})
	if built.err != nil {
		t.Fatalf("build the engine for the tests: %v", built.err)
	}
	return built.path
}

// engineRoot is the directory of the engine module, found from this file and not from the directory a
// test runs in.
func engineRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

// Cleanup takes away the directory the engine was built in, if it was built. A TestMain calls it after
// the tests have run.
func Cleanup() {
	if built.dir != "" {
		os.RemoveAll(built.dir)
	}
}

// Provision puts the engine where the script looks for it in the given home.
func Provision(t testing.TB, home string) {
	t.Helper()
	body, err := os.ReadFile(Binary(t))
	if err != nil {
		t.Fatalf("read the built engine: %v", err)
	}
	dst := filepath.Join(home, filepath.FromSlash(installedDir), installedName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil {
		t.Fatalf("install the engine at %s: %v", dst, err)
	}
}

// IsProvisioned says whether a path is the engine Provision put in a home. The tools that list what a
// command left in a home do not list it, which is not the command's.
func IsProvisioned(path string) bool {
	return filepath.Base(path) == installedName && filepath.Base(filepath.Dir(path)) == "bin"
}
