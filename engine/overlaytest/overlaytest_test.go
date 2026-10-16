package overlaytest_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/testhome"
)

// TestMain runs the tests of this package in a home directory of their own (testhome), as every package
// whose tests run git and the script does, and takes away the engine NewOverlay provisions.
func TestMain(m *testing.M) {
	code := testhome.Run(m)
	enginetest.Cleanup()
	os.Exit(code)
}

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }

func TestEnvIsTheHomeTheOverlayThePathAndTheSettingsOfGitAndNothingOfTheProcess(t *testing.T) {
	t.Setenv("STATE_DIR", "/the/state/of/whoever/runs/the/tests")
	t.Setenv("PATH", "/the/path")

	env := overlaytest.Env("/h", "/o")

	want := append([]string{"HOME=/h", "OVERLAY_DIR=/o", "PATH=/the/path"}, overlaytest.GitSettings()...)
	if !reflect.DeepEqual(env, want) {
		t.Errorf("Env(\"/h\", \"/o\") = %q, want the home, the overlay, the PATH of the process and the settings of git, in that order: %q", env, want)
	}
}

func TestEnvPutsWhatTheCallerAddsLast(t *testing.T) {
	env := overlaytest.Env("/h", "/o", "LC_ALL=C", "PATH=/mine")

	if got := env[len(env)-2:]; !reflect.DeepEqual(got, []string{"LC_ALL=C", "PATH=/mine"}) {
		t.Errorf("the last entries of Env = %q, want what the caller added, in the order it added it (the last PATH wins when a program reads the environment)", got)
	}
}

// gitConfig asks git what a key is set to in dir, in env: its answer and whether the key is set.
func gitConfig(t *testing.T, dir string, env []string, key string) (string, bool) {
	t.Helper()
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err == nil
}

func TestGitSettingsTurnAutomaticMaintenanceOff(t *testing.T) {
	dir := t.TempDir()
	overlaytest.Git(t, dir, nil, "init", "-q")

	for key, want := range map[string]string{"maintenance.auto": "false", "gc.auto": "0"} {
		if got, ok := gitConfig(t, dir, overlaytest.Env(t.TempDir(), dir), key); !ok || got != want {
			t.Errorf("git config --get %s = %q, %v in the environment of a run, want %q: a commit would start maintenance in the background", key, got, ok, want)
		}
	}
}

func TestGitSettingsTakeNoConfigurationOfWhoeverRunsTheTests(t *testing.T) {
	leak := t.TempDir()
	global := filepath.Join(leak, "global")
	system := filepath.Join(leak, "system")
	if err := os.WriteFile(global, []byte("[leak]\n\tglobal = yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(system, []byte("[leak]\n\tsystem = yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", system)
	dir := t.TempDir()
	overlaytest.Git(t, dir, nil, "init", "-q")

	// The control: the configuration the process was given is there for a git that is given nothing more.
	if got, ok := gitConfig(t, dir, os.Environ(), "leak.global"); !ok || got != "yes" {
		t.Fatalf("control: git config --get leak.global = %q, %v with the environment of the process, want yes: the configuration is not in force, so this test proves nothing", got, ok)
	}
	for _, key := range []string{"leak.global", "leak.system"} {
		if got, ok := gitConfig(t, dir, overlaytest.Env(t.TempDir(), dir), key); ok {
			t.Errorf("git config --get %s = %q in the environment of a run, want it unset: the configuration of whoever runs the tests reaches the fixture", key, got)
		}
		if got, ok := gitConfig(t, dir, append(os.Environ(), overlaytest.GitSettings()...), key); ok {
			t.Errorf("git config --get %s = %q with the settings of git, want it unset", key, got)
		}
	}
}

func TestInitOverlayCommitsTheFilesOnUpstreamAndStartsMainThere(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
		want  [][]string
	}{
		{"all the files", nil, [][]string{
			{"init"}, {"config", "user.email", "test@test.com"}, {"config", "user.name", "test"},
			{"checkout", "-b", "upstream"}, {"add", "."}, {"commit", "-m", "upstream: baseline"}, {"checkout", "-b", "main"},
		}},
		{"the files it is told", []string{"overlay.manifest", "skills/"}, [][]string{
			{"init"}, {"config", "user.email", "test@test.com"}, {"config", "user.name", "test"},
			{"checkout", "-b", "upstream"}, {"add", "overlay.manifest", "skills/"}, {"commit", "-m", "upstream: baseline"}, {"checkout", "-b", "main"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got [][]string
			overlaytest.InitOverlay(func(args ...string) { got = append(got, args) }, tc.paths...)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("InitOverlay ran git as %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewOverlayMakesUpstreamAndMainAtOneCommitHoldingTheFiles(t *testing.T) {
	home := t.TempDir()
	files := map[string]string{"overlay.manifest": "x managed\n", "skills/x/SKILL.md": "# x\n"}

	dir, env := overlaytest.NewOverlay(t, home, files)

	if upstream, main := overlaytest.Git(t, dir, env, "rev-parse", "upstream"), overlaytest.Git(t, dir, env, "rev-parse", "main"); upstream != main {
		t.Errorf("upstream is at %q and main at %q, want both at the baseline commit", upstream, main)
	}
	if got := strings.TrimSpace(overlaytest.Git(t, dir, env, "branch", "--show-current")); got != "main" {
		t.Errorf("the overlay is on branch %q, want main", got)
	}
	for rel, want := range files {
		if got := overlaytest.Git(t, dir, env, "show", "upstream:"+rel); got != want {
			t.Errorf("upstream:%s = %q, want %q", rel, got, want)
		}
	}
	if got := strings.TrimSpace(overlaytest.Git(t, dir, env, "log", "-1", "--format=%an <%ae>", "upstream")); got != "test <test@test.com>" {
		t.Errorf("the baseline commit is by %q, want the identity of the fixtures", got)
	}
	if want := []string{"HOME=" + home, "OVERLAY_DIR=" + dir}; !reflect.DeepEqual(env[:2], want) {
		t.Errorf("the environment begins %q, want the home and the overlay it was made for: %q", env[:2], want)
	}
	if info, err := os.Stat(enginetest.Path(home)); err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("no executable engine at %s (%v): the script would build one", enginetest.Path(home), err)
	}
}

func TestRunKeepsTheStreamsAndTheExitCodeApart(t *testing.T) {
	script := filepath.Join(t.TempDir(), "say")
	if err := os.WriteFile(script, []byte("echo \"out $*\"\necho \"err $*\" >&2\nexit 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := overlaytest.Run(t, script, os.Environ(), "", "a", "b c")

	if want := (overlaytest.Result{Stdout: "out a b c\n", Stderr: "err a b c\n", Exit: 3}); got != want {
		t.Errorf("Run = %+v, want %+v", got, want)
	}
}

func TestRunRunsFromTheDirectoryItIsGiven(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(t.TempDir(), "where")
	if err := os.WriteFile(script, []byte("pwd -P\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := overlaytest.Run(t, script, os.Environ(), dir); strings.TrimSpace(got.Stdout) != want {
		t.Errorf("the script ran in %q, want %q", strings.TrimSpace(got.Stdout), want)
	}
}

func TestCombinedAnswersTheErrorOfANonZeroExit(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fail")
	if err := os.WriteFile(script, []byte("echo said\nexit 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := overlaytest.Combined(t, script, os.Environ(), "")

	var exit *exec.ExitError
	if out != "said\n" || !errors.As(err, &exit) || exit.ExitCode() != 4 {
		t.Errorf("Combined = %q, %v, want the output and an *exec.ExitError for exit 4", out, err)
	}
}

func TestSourceRunsTheSnippetAfterTheScriptDefinedItsFunctionsWithTheArguments(t *testing.T) {
	script := filepath.Join(t.TempDir(), "lib")
	if err := os.WriteFile(script, []byte("greet() { echo \"hello $1 $2\"; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := overlaytest.Source(t, script, os.Environ(), `greet "$@"`, "to", "you")

	if err != nil || out != "hello to you\n" {
		t.Errorf("Source = %q, %v, want %q", out, err, "hello to you\n")
	}
}

func TestStubEngineIsAnExecutableWhereTheScriptLooksForTheEngine(t *testing.T) {
	home := t.TempDir()

	overlaytest.StubEngine(t, home, "#!/bin/sh\nexit 0\n")

	info, err := os.Stat(enginetest.Path(home))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("the stub has mode %v, want 0755", info.Mode().Perm())
	}
	if body, _ := os.ReadFile(enginetest.Path(home)); string(body) != "#!/bin/sh\nexit 0\n" {
		t.Errorf("the stub holds %q, want what it was given", body)
	}
}
