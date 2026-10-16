package shelltest

// The script finds itself, and what it is made of, from where it really is, and not from where it was run
// or what it was called: from another directory, through a symlink (absolute, relative, or a chain of them,
// which is what install-alias makes), executed as a program or run by bash, and sourced. These pass on the
// script as one file and on the script as an entry point with modules beside it: they are the pins of the
// move, and the only thing they may ever be told apart by is a script that no longer finds its parts.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// locationEnv is the environment of a run that has to find the checkout from where the script really is: a
// home of the test's own and the PATH, and not OVERLAY_DIR, STATE_DIR or anything else that would tell the
// script where to look instead.
func locationEnv(home string) []string {
	return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
}

// checkoutRoot is the directory of the checkout as the script sees it: physical, symlinks resolved.
func checkoutRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(overlaytest.RepoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// aWayToRun is one way of starting the script: how to build the command for the arguments, given the
// environment, and the directory it is started in.
type aWayToRun struct {
	name  string
	start func(t *testing.T, env []string, args ...string) *exec.Cmd
}

// symlinkTo makes a symlink called name in dir to target (as given: relative or not) and returns its path.
func symlinkTo(t *testing.T, dir, name, target string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, name)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// waysToRunTheScript is every way the script is started that a person or a program does start it, each from a
// directory that is not the checkout (but the first, which is the checkout and is there to be the same).
func waysToRunTheScript(t *testing.T) []aWayToRun {
	t.Helper()
	real := overlaytest.Script(t)
	elsewhere := t.TempDir()
	relativeTarget, err := filepath.Rel(filepath.Join(elsewhere, "rel"), real)
	if err != nil {
		t.Fatal(err)
	}
	absolute := symlinkTo(t, filepath.Join(elsewhere, "abs"), "labdrian", real)
	relative := symlinkTo(t, filepath.Join(elsewhere, "rel"), "labdrian", relativeTarget)
	symlinkTo(t, filepath.Join(elsewhere, "chain"), "second", real)
	first := symlinkTo(t, filepath.Join(elsewhere, "chain"), "first", "second")

	byBash := func(script, dir string) func(*testing.T, []string, ...string) *exec.Cmd {
		return func(t *testing.T, env []string, args ...string) *exec.Cmd {
			cmd := exec.Command("bash", append([]string{script}, args...)...)
			cmd.Dir, cmd.Env = dir, env
			return cmd
		}
	}
	asProgram := func(script, dir string) func(*testing.T, []string, ...string) *exec.Cmd {
		return func(t *testing.T, env []string, args ...string) *exec.Cmd {
			cmd := exec.Command(script, args...)
			cmd.Dir, cmd.Env = dir, env
			return cmd
		}
	}
	return []aWayToRun{
		{"bash, by its path, from the checkout", byBash(real, overlaytest.RepoRoot(t))},
		{"bash, by its path, from another directory", byBash(real, elsewhere)},
		{"as a program, by its path, from another directory", asProgram(real, elsewhere)},
		{"as a program, through an absolute symlink, from another directory", asProgram(absolute, elsewhere)},
		{"as a program, through a relative symlink, from another directory", asProgram(relative, elsewhere)},
		{"as a program, through a chain of two symlinks, from another directory", asProgram(first, elsewhere)},
		{"bash, through an absolute symlink, from another directory", byBash(absolute, elsewhere)},
	}
}

const theCatalog = "claude\tcopy\nopencode\tcopy\ncodex\tcopy\npi\tpackage\n"

func TestTheScriptRunsWhereverItIsRunFromAndWhateverItIsCalled(t *testing.T) {
	overlaytest.RequireBash(t)
	for _, way := range waysToRunTheScript(t) {
		t.Run(way.name, func(t *testing.T) {
			env := locationEnv(t.TempDir())

			targets := overlaytest.Capture(t, way.start(t, env, "targets"))
			help := overlaytest.Capture(t, way.start(t, env, "--help"))

			if targets.Exit != 0 || targets.Stdout != theCatalog || targets.Stderr != "" {
				t.Errorf("targets = exit %d, stdout %q, stderr %q, want exit 0, %q and nothing on stderr", targets.Exit, targets.Stdout, targets.Stderr, theCatalog)
			}
			if help.Exit != 0 || !strings.HasPrefix(help.Stdout, "Usage: labdrian <command> [options]\n") || help.Stderr != "" {
				t.Errorf("--help = exit %d, stdout starting %.60q, stderr %q, want exit 0, the usage and nothing on stderr", help.Exit, help.Stdout, help.Stderr)
			}
		})
	}
}

// install-alias writes the path of the script into the link it makes, and takes it from where the script
// really is: a script that found its checkout from the name it was called by would link the symlink it was
// called through, or a directory that is not the checkout at all.
func TestInstallAliasLinksTheRealScriptWhateverItWasRunThrough(t *testing.T) {
	overlaytest.RequireBash(t)
	wantTarget := filepath.Join(checkoutRoot(t), "bin", "labdrian-overlay")
	for _, way := range waysToRunTheScript(t) {
		t.Run(way.name, func(t *testing.T) {
			home := t.TempDir()
			env := locationEnv(home)

			installed := overlaytest.Capture(t, way.start(t, env, "install-alias", "myalias"))

			if installed.Exit != 0 {
				t.Fatalf("install-alias = exit %d, stdout %q, stderr %q", installed.Exit, installed.Stdout, installed.Stderr)
			}
			link := filepath.Join(home, ".local", "bin", "myalias")
			if got, err := os.Readlink(link); err != nil || got != wantTarget {
				t.Fatalf("the alias points at %q (%v), want the script of the checkout, %q", got, err, wantTarget)
			}
			cmd := exec.Command(link, "targets")
			cmd.Dir, cmd.Env = t.TempDir(), env
			if ran := overlaytest.Capture(t, cmd); ran.Exit != 0 || ran.Stdout != theCatalog {
				t.Errorf("the alias, run from another directory: targets = exit %d, stdout %q, stderr %q, want exit 0 and %q", ran.Exit, ran.Stdout, ran.Stderr, theCatalog)
			}
		})
	}
}

// Sourced, the script defines what it defines and does nothing else, whatever it is sourced through: this is
// the seam the tests of the script use, the TUI's backend tests among them.
func TestSourcingTheScriptDefinesItsFunctionsAndPrintsNothingWhateverItIsSourcedThrough(t *testing.T) {
	overlaytest.RequireBash(t)
	script := overlaytest.Script(t)
	link := symlinkTo(t, filepath.Join(t.TempDir(), "elsewhere"), "labdrian-overlay", script)
	// One function of each concern the script is made of: the helpers, the targets, the engine, the manifest,
	// the state, the release, the usage and one command of each kind.
	const snippet = `for f in path_clean resolve_targets ensure_engine_binary manifest_routes_load compute_target_digest ` +
		`resolve_latest_release_tag usage cmd_apply cmd_doctor cmd_longterm_mem cmd_self_update; do ` +
		`declare -F "$f" >/dev/null || echo "missing function $f"; done; echo "$OVERLAY_DIR"`

	for _, through := range []struct{ name, path string }{{"its path", script}, {"a symlink to it", link}} {
		t.Run(through.name, func(t *testing.T) {
			shell := overlaytest.Shell{Env: locationEnv(t.TempDir()), Dir: t.TempDir()}

			out, err := overlaytest.Source(t, through.path, shell, snippet)

			if want := checkoutRoot(t) + "\n"; err != nil || out != want {
				t.Errorf("sourcing it through %s, then asking for the functions and OVERLAY_DIR = %q, %v, want %q: every function, the checkout it really is in, and nothing printed by the sourcing", through.name, out, err, want)
			}
		})
	}
}
