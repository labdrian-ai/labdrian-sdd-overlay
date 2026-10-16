package shelltest

// How the script reaches a machine, and that every file it is made of goes with it. The script is shipped by
// git and by nothing else: a clone of this repository, kept current by `self-update` (which fast-forwards
// main to the latest release tag) and by `apply`, started by the name `install-alias` links to it. There is
// no tarball of it and no row of overlay.manifest (the manifest lists what is deployed to the runtimes, and
// bin/ is not that). So "the modules are shipped" comes to two things, and these are the tests of them:
// every file the script is made of is one git carries (it is tracked or at least not ignored), and an
// update of a clone, to a release, brings a change to the file that holds a piece of the script.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// gitCarriedFilesUnderBin is the files under bin/ of this checkout that a commit of everything would hold:
// tracked, or untracked and not ignored. A file that git ignores is not in it, which is how a module that
// a pattern of .gitignore swallowed would be missed by every clone.
func gitCarriedFilesUnderBin(t *testing.T) []string {
	t.Helper()
	out := overlaytest.Git(t, overlaytest.RepoRoot(t), nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "bin")
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

func TestEveryFileTheScriptIsMadeOfIsOneGitCarries(t *testing.T) {
	root := overlaytest.RepoRoot(t)
	carried := map[string]bool{}
	for _, f := range gitCarriedFilesUnderBin(t) {
		carried[f] = true
	}

	for _, file := range overlaytest.ScriptFiles(t) {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		if !carried[filepath.ToSlash(rel)] {
			t.Errorf("%s is part of the script and git does not carry it (is it ignored?): a clone would not have it", rel)
		}
		if ignoredByAPattern(t, root, rel) {
			t.Errorf("a pattern of .gitignore matches %s, which is part of the script: it is tracked today, and the next module that a pattern like it swallows is not", rel)
		}
	}
}

// ignoredByAPattern says whether a pattern of the ignore files matches path, whether or not git tracks the
// file (a tracked file is carried, and says nothing about the next file with a name like it).
func ignoredByAPattern(t *testing.T, root, path string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "--no-index", "-q", "--", path)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), overlaytest.GitSettings()...)
	switch err := cmd.Run(); {
	case err == nil:
		return true
	case overlaytest.ExitCode(err) == 1:
		return false
	default:
		t.Fatalf("git check-ignore %s: %v", path, err)
		return false
	}
}

// theClone is a clone of a repository that holds what this checkout carries under bin/, with the script's
// first release tagged, and the repository it was cloned from.
type theClone struct {
	dir    string // the clone: the overlay the script in it runs on
	seed   string // the working repository the origin is pushed from
	script string // the entry point in the clone
	env    []string
}

// newClone builds the origin from what git carries of bin/ in this checkout, tags it v1.0.0, and clones it.
func newClone(t *testing.T) theClone {
	t.Helper()
	root := overlaytest.RepoRoot(t)
	base := t.TempDir()
	seed, origin, clone := filepath.Join(base, "seed"), filepath.Join(base, "origin.git"), filepath.Join(base, "clone")
	for _, dir := range []string{seed, base} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	overlaytest.Git(t, seed, nil, "init", "-q", "-b", "main")
	for _, rel := range gitCarriedFilesUnderBin(t) {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		to := filepath.Join(seed, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, data, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	overlaytest.Git(t, seed, nil, "add", "-A")
	overlaytest.Git(t, seed, nil, "commit", "-q", "-m", "chore: the script, first release")
	overlaytest.Git(t, seed, nil, "tag", "-a", "v1.0.0", "-m", "first release")
	overlaytest.Git(t, base, nil, "clone", "-q", "--bare", seed, origin)
	overlaytest.Git(t, seed, nil, "remote", "add", "origin", origin)
	overlaytest.Git(t, base, nil, "clone", "-q", origin, clone)

	// No OVERLAY_DIR: the script of the clone has to find the clone from where it is.
	home := t.TempDir()
	env := append(locationEnv(home), overlaytest.GitSettings()...)
	return theClone{dir: clone, seed: seed, script: filepath.Join(clone, "bin", "labdrian-overlay"), env: env}
}

// helpLine is the last line of the usage, which the new release changes.
const helpLine = "  --help                           Show this help"

// release publishes a second release from the seed: the same script with the last line of its usage
// changed, in whichever file of the script holds it, tagged v2.0.0 and pushed to the origin. It returns the
// path, relative to the seed, of the file it changed.
func (c theClone) release(t *testing.T) string {
	t.Helper()
	script := filepath.Join(c.seed, "bin", "labdrian-overlay")
	for _, file := range overlaytest.ScriptFilesOf(t, script) {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), helpLine+"\n") {
			continue
		}
		changed := strings.Replace(string(data), helpLine+"\n", helpLine+" (second release)\n", 1)
		if err := os.WriteFile(file, []byte(changed), 0o755); err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(c.seed, file)
		if err != nil {
			t.Fatal(err)
		}
		overlaytest.Git(t, c.seed, nil, "commit", "-q", "-a", "-m", "feat: the second release")
		overlaytest.Git(t, c.seed, nil, "tag", "-a", "v2.0.0", "-m", "second release")
		overlaytest.Git(t, c.seed, nil, "push", "-q", "origin", "main", "v2.0.0")
		return rel
	}
	t.Fatalf("no file of the script holds the line %q", helpLine)
	return ""
}

func TestSelfUpdateBringsAChangeToTheFileThatHoldsAPieceOfTheScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	overlaytest.RequireBash(t)
	c := newClone(t)
	helpBefore := overlaytest.Run(t, c.script, c.env, "", "--help")
	if helpBefore.Exit != 0 || !strings.Contains(helpBefore.Stdout, helpLine+"\n") {
		t.Fatalf("the clone, before the update: --help = exit %d, stdout %.200q, stderr %q, want the usage as it is", helpBefore.Exit, helpBefore.Stdout, helpBefore.Stderr)
	}

	changed := c.release(t)
	update := overlaytest.Run(t, c.script, c.env, "", "self-update")

	if update.Exit != 0 {
		t.Fatalf("self-update = exit %d, stdout %q, stderr %q, want 0", update.Exit, update.Stdout, update.Stderr)
	}
	if !strings.Contains(update.Stdout, "Updated main:") || !strings.Contains(update.Stdout, "v2.0.0") {
		t.Errorf("self-update said %q, want it to say it fast-forwarded main to the release v2.0.0", update.Stdout)
	}
	if want, got := strings.TrimSpace(overlaytest.Git(t, c.seed, nil, "rev-parse", "v2.0.0^{commit}")), strings.TrimSpace(overlaytest.Git(t, c.dir, nil, "rev-parse", "HEAD")); got != want {
		t.Errorf("the clone is at %s, want the release v2.0.0 at %s", got, want)
	}
	if want, got := mustRead(t, filepath.Join(c.seed, changed)), mustRead(t, filepath.Join(c.dir, changed)); got != want {
		t.Errorf("%s in the clone is not the one of the release: the update did not bring it", changed)
	}
	helpAfter := overlaytest.Run(t, c.script, c.env, "", "--help")
	if helpAfter.Exit != 0 || !strings.Contains(helpAfter.Stdout, helpLine+" (second release)\n") {
		t.Errorf("the clone, after the update: --help = exit %d, stdout %.200q, stderr %q, want the usage of the second release", helpAfter.Exit, helpAfter.Stdout, helpAfter.Stderr)
	}
}

func TestEveryFileOfTheScriptIsInTheTreeOfARelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	c := newClone(t)

	tree := strings.Fields(overlaytest.Git(t, c.dir, nil, "ls-tree", "-r", "--name-only", "v1.0.0", "--", "bin"))

	in := map[string]bool{}
	for _, f := range tree {
		in[f] = true
	}
	for _, file := range overlaytest.ScriptFilesOf(t, c.script) {
		rel, err := filepath.Rel(c.dir, file)
		if err != nil {
			t.Fatal(err)
		}
		if !in[filepath.ToSlash(rel)] {
			t.Errorf("the release v1.0.0 has no %s: a machine that updates to it would not have the file", rel)
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
