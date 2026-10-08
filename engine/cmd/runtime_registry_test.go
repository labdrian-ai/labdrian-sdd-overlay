package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// TestRunRuntimeCore_ConfigRootIsThePiPackageDirectory: --config-root reaches Pi like it reaches
// the other runtimes, as the directory the package is built in. It used to be dropped for Pi, so
// `status --target pi --config-root X` looked in the default package directory and not in X.
func TestRunRuntimeCore_ConfigRootIsThePiPackageDirectory(t *testing.T) {
	pkgDir := filepath.Join(t.TempDir(), "labdrian-pi")

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"status", "--target", "pi", "--config-root", pkgDir},
		&outBuf, &errBuf, func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("status of an unbuilt Pi package should exit 1, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if want := "labdrian-pi package is not built at " + pkgDir + " "; !strings.Contains(outBuf.String(), want) {
		t.Fatalf("status should look for the Pi package at the --config-root %s; got %q", pkgDir, outBuf.String())
	}
}

// TestRunRuntimeCore_ConfigRootReachesPiInTheAllForm: the same for `--target all`, whose Pi line
// names the given directory too.
func TestRunRuntimeCore_ConfigRootReachesPiInTheAllForm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-root")

	var outBuf, errBuf bytes.Buffer
	runRuntimeCore(
		[]string{"status", "--target", "all", "--config-root", root},
		&outBuf, &errBuf, func(int) {},
	)

	var piLine string
	for _, line := range strings.Split(outBuf.String(), "\n") {
		if strings.HasPrefix(line, "[pi] ") {
			piLine = line
		}
	}
	if !strings.Contains(piLine, "package is not built at "+root+" ") {
		t.Fatalf("the Pi line of status --target all should name the --config-root %s; got %q", root, piLine)
	}
}

// TestNewRuntimeRegistry_RegistersTheDeclaredRuntimesInTheOrderAllExpandsTo: the roster the
// program ships is the capability vocabulary, so a target declared and never registered, or the
// reverse, fails here; and `all` acts on the runtimes in the order they have always been acted on.
func TestNewRuntimeRegistry_RegistersTheDeclaredRuntimesInTheOrderAllExpandsTo(t *testing.T) {
	reg, err := newRuntimeRegistry(nil)
	if err != nil {
		t.Fatalf("newRuntimeRegistry: %v", err)
	}

	wantOrder := []runtimepkg.Target{runtimepkg.TargetClaude, runtimepkg.TargetOpenCode, runtimepkg.TargetCodex, runtimepkg.TargetPi}
	if got := reg.Expand(runtimepkg.TargetAll); !reflect.DeepEqual(got, wantOrder) {
		t.Errorf("Expand(all) = %v, want %v", got, wantOrder)
	}

	var registered []string
	for _, target := range reg.Targets() {
		registered = append(registered, string(target))
	}
	declared := capability.Targets()
	sort.Strings(registered)
	sort.Strings(declared)
	if !reflect.DeepEqual(registered, declared) {
		t.Errorf("registered runtimes = %v, declared targets = %v; they must be the same set", registered, declared)
	}
}

func TestRuntimeConfigFromEnv(t *testing.T) {
	environ := map[string]string{
		"HOME":            "  /home/p  ",
		"XDG_CONFIG_HOME": "/xdg",
		"CODEX_HOME":      "/codex",
		"OVERLAY_DIR":     "/overlay",
		"STATE_DIR":       "/state",
	}
	env := func(k string) string { return environ[k] }
	failing := func() (string, error) { return "", errors.New("no home") }
	profile := func() (string, error) { return "/from/profile", nil }

	t.Run("reads each value once and trims the home", func(t *testing.T) {
		got := runtimeConfigFromEnv(env, failing, "/given")
		want := runtimepkg.Config{
			Home: "/home/p", XDGConfigHome: "/xdg", CodexHome: "/codex",
			OverlayDir: "/overlay", StateDir: "/state", ConfigRoot: "/given",
		}
		if got != want {
			t.Errorf("runtimeConfigFromEnv = %+v, want %+v", got, want)
		}
	})

	t.Run("a blank HOME falls back to the user's profile directory", func(t *testing.T) {
		environ["HOME"] = "   "
		defer func() { environ["HOME"] = "  /home/p  " }()
		if got := runtimeConfigFromEnv(env, profile, "").Home; got != "/from/profile" {
			t.Errorf("Home = %q, want the profile directory", got)
		}
	})

	t.Run("a home that cannot be determined stays empty", func(t *testing.T) {
		environ["HOME"] = ""
		defer func() { environ["HOME"] = "  /home/p  " }()
		if got := runtimeConfigFromEnv(env, failing, "").Home; got != "" {
			t.Errorf("Home = %q, want none: an unknown home is not a place to guess", got)
		}
	})
}
