package overlaytest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// The identity every commit of a fixture is made by.
const (
	identityName  = "test"
	identityEmail = "test@test.com"
)

// GitSettings is what every git of a fixture takes, whether the test runs it or the script does: no
// configuration of whoever runs the tests (neither the global nor the system one), an identity of the
// test's own, and no automatic maintenance. A commit may otherwise start `git maintenance run --auto`
// in the background, whose files in .git come and go while a test compares two snapshots of a tree,
// and the program under test would be blamed for a file git wrote.
func GitSettings() []string {
	return []string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=maintenance.auto", "GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=gc.auto", "GIT_CONFIG_VALUE_1=0",
		"GIT_AUTHOR_NAME=" + identityName, "GIT_AUTHOR_EMAIL=" + identityEmail,
		"GIT_COMMITTER_NAME=" + identityName, "GIT_COMMITTER_EMAIL=" + identityEmail,
	}
}

// Git runs git in dir and fails the test when it fails; it returns what git wrote on both streams. A
// nil env is the environment of the test process with GitSettings added.
func Git(t testing.TB, dir string, env []string, args ...string) string {
	t.Helper()
	return GitBin(t, "git", dir, env, args...)
}

// GitBin is Git for the git program at bin, which a test that dates its commits puts first on its PATH.
func GitBin(t testing.TB, bin, dir string, env []string, args ...string) string {
	t.Helper()
	if env == nil {
		env = append(os.Environ(), GitSettings()...)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// WriteFile puts content at rel under root, making the directories it needs.
func WriteFile(t testing.TB, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// WriteFiles puts every file of the map under root: the keys are the paths below it.
func WriteFiles(t testing.TB, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		WriteFile(t, root, rel, content)
	}
}

// InitOverlay makes of the files in a directory the repository the script expects, through git, the
// function that runs git in that directory and stops the test when it fails: the files named by paths
// (all of them when there are none) are committed on a branch called upstream, and main starts at the
// same commit, so that merging upstream into it is a no-op.
func InitOverlay(git func(args ...string), paths ...string) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	git("init")
	git("config", "user.email", identityEmail)
	git("config", "user.name", identityName)
	git("checkout", "-b", "upstream")
	git(append([]string{"add"}, paths...)...)
	git("commit", "-m", "upstream: baseline")
	git("checkout", "-b", "main")
}

// NewOverlay makes an overlay repository holding the files, in a directory of its own, with the
// branches InitOverlay makes, and puts the engine where the script looks for it in home. It returns
// the directory and the environment the script runs in (Env).
func NewOverlay(t testing.TB, home string, files map[string]string) (dir string, env []string) {
	t.Helper()
	dir = t.TempDir()
	WriteFiles(t, dir, files)
	env = Env(home, dir)
	InitOverlay(func(args ...string) { Git(t, dir, env, args...) })
	enginetest.Provision(t, home)
	return dir, env
}
