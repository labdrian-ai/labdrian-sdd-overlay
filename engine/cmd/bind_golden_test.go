package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/bind-golden record what the binding verbs print and leave:
// 'workflow bind', 'workflow unbind' and 'workflow binding', the exit code, stdout and stderr of
// each, and the files of the binding store they leave behind (names, permission bits and the
// binding documents). testdata/repo-golden records how the repository a verb is run in is
// found: the key that names its binding, and the worktree root and HEAD a workflow records
// (see repo_golden_cases_test.go). Both were recorded from the program as it was before Phase 9
// unit H29 (docs/architecture/hexagonal-target.md) moved the finding of the repository behind a
// RepoLocator and the verbs and the projection hook into services; a change to a byte of any of
// them fails here. Rewrite them deliberately with
//
//	go test ./cmd -run TestBindGolden -update-bind-golden
//	go test ./cmd -run TestRepoGolden -update-repo-golden
//
// and read the diff before committing it.
//
// The verbs run in process, as the commands do (the same cores 'main' calls, with the exit
// function recorded and the streams captured), against a state home and hand-made repositories
// under directories with neutral names. No case runs git.
var updateBindGolden = flag.Bool("update-bind-golden", false, "rewrite the golden files of the binding verbs")
var updateRepoGolden = flag.Bool("update-repo-golden", false, "rewrite the golden files of the finding of a repository")

// goldenTime is a time as the program writes it: UTC, to the second, with a Z. A time written
// any other way (a fraction, an offset) is not replaced, so it shows in the transcript and
// differs from the golden file.
var goldenTime = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`)

// repoWorld is the scratch space of one case: a state home, a directory for the Goal files, the
// places a transcript names by a placeholder, the keys it names by a label, and the transcript.
type repoWorld struct {
	*hookWorld
	state string
	goals string
	keys  map[string]string
}

// neutralDir makes a directory under the system temporary directory whose name carries no word
// of the test, and returns it with its symbolic links resolved.
func neutralDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "g")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// newRepoWorld makes a world whose workflows confirm no dependency, so a transcript is the
// same on every machine.
func newRepoWorld(t *testing.T) *repoWorld {
	t.Helper()
	useUnavailableProber(t)
	w := &repoWorld{hookWorld: newHookWorld(t), state: neutralDir(t), goals: neutralDir(t), keys: map[string]string{}}
	t.Setenv("XDG_STATE_HOME", w.state)
	w.name(w.state, "<STATE>")
	w.name(w.goals, "<GOALS>")
	return w
}

// place makes a directory with a neutral name, written as label in the transcript.
func (w *repoWorld) place(label string) string {
	w.t.Helper()
	dir := neutralDir(w.t)
	w.name(dir, label)
	return dir
}

// key registers the key the design specifies for the git common directory commonDir: the digest
// of its path with the symbolic links resolved. The transcript writes it as label, so a key that
// is computed any other way is a digest the golden file does not hold.
func (w *repoWorld) key(commonDir, label string) {
	w.t.Helper()
	w.keys[wantRepoKey(w.t, commonDir)] = label
}

// run records one run of 'workflow <args>' in the directory cwd.
func (w *repoWorld) run(label, cwd string, args ...string) workflowRun {
	w.t.Helper()
	r := runWorkflowTest(args, cwd)
	fmt.Fprintf(&w.b, "$ workflow %s\n# %s (in %s)\n--- exit ---\n%d\n--- stdout ---\n%s--- stderr ---\n%s\n",
		strings.Join(args, " "), label, w.cwdName(cwd), r.code, w.shown(r.stdout), w.shown(r.stderr))
	return r
}

// note records a fact the case checked itself, which the times and digests of a transcript hide.
func (w *repoWorld) note(format string, args ...any) {
	fmt.Fprintf(&w.b, "# "+format+"\n", args...)
}

// cwdName is the place a run happened in, as the transcript names it.
func (w *repoWorld) cwdName(cwd string) string {
	if cwd == "" {
		return "no directory"
	}
	return cwd
}

// setup runs 'workflow <args>' without recording it, and fails the case when it does not exit 0.
func (w *repoWorld) setup(cwd string, args ...string) {
	w.t.Helper()
	if r := runWorkflowTest(args, cwd); r.code != 0 {
		w.t.Fatalf("setup workflow %v: exit %d, stderr %q", args, r.code, r.stderr)
	}
}

// workflowIn creates workflow wf of project from cwd and moves it to status: created, running,
// paused or closed.
func (w *repoWorld) workflowIn(cwd, project, wf, status string) {
	w.t.Helper()
	goal := writeMemoryTestFile(w.t, w.goals, project+"-"+wf+".json", memoryTestGoalJSON(project, "goal-1"))
	w.setup(cwd, "create", "--project", project, "--workflow", wf, "--goal", goal, "--profile", "standalone-minimal")
	steps := map[string][][]string{
		"created": {},
		"running": {{"start"}},
		"paused":  {{"start"}, {"pause"}},
		"closed":  {{"close", "--outcome", "abandoned", "--reason", "test"}},
	}[status]
	for _, step := range steps {
		w.setup(cwd, append(step, "--project", project, "--workflow", wf)...)
	}
}

// storeFiles records what the binding store holds: every path under it, with its permission
// bits, and the binding document of each file that holds one.
func (w *repoWorld) storeFiles(label string) {
	w.t.Helper()
	root := filepath.Join(w.state, "labdrian", "bindings")
	fmt.Fprintf(&w.b, "# %s: the binding store\n", label)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(w.state, path)
		fmt.Fprintf(&w.b, "%s %v\n", filepath.ToSlash(rel), info.Mode())
		if info.Mode().IsRegular() && strings.HasSuffix(path, ".json") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(&w.b, "%s", w.shown(string(data)))
		}
		return nil
	})
	if os.IsNotExist(err) {
		fmt.Fprintf(&w.b, "<no binding store>\n")
		return
	}
	if err != nil {
		w.t.Fatal(err)
	}
}

// text is the transcript: the places replaced, the keys written as their labels, the times of a
// run as <TIME>, the other digests as <DIGEST>, and every control character shown as \xNN.
func (w *repoWorld) text() string {
	out := w.replace(w.b.String())
	for key, label := range w.keys {
		out = strings.ReplaceAll(out, key, label)
	}
	return visibleControls(digests.ReplaceAllString(goldenTime.ReplaceAllString(out, "<TIME>"), "<DIGEST>"))
}

// checkGolden compares the transcript with testdata/<dir>/<name>.golden, or rewrites the file
// under the update flag.
func checkGolden(t *testing.T, dir, name, got string, update bool) {
	t.Helper()
	if hookGoldenFileName.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", dir, name+".golden")
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-%s)", err, strings.Split(dir, "-")[0]+"-golden")
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}

// repoGoldenCase is one scenario and the golden file its transcript is compared with.
type repoGoldenCase struct {
	name string
	run  func(w *repoWorld)
}

// checkRepoGoldenCases runs each case as its own subtest against testdata/<dir>, and guards the
// list itself: two cases of one name would share a file, and a file that belongs to no case is a
// recording nothing reads.
func checkRepoGoldenCases(t *testing.T, dir string, cases []repoGoldenCase, update bool) {
	t.Helper()
	seen := map[string]bool{}
	for _, tc := range cases {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
		t.Run(tc.name, func(t *testing.T) {
			w := newRepoWorld(t)
			tc.run(w)
			checkGolden(t, dir, tc.name, w.text(), update)
		})
	}
	entries, err := os.ReadDir(filepath.Join("testdata", dir))
	if err != nil {
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/%s/%s belongs to no case", dir, e.Name())
		}
	}
}

// TestBindGolden runs every case of the binding verbs and compares its transcript with its
// golden file.
func TestBindGolden(t *testing.T) {
	checkRepoGoldenCases(t, "bind-golden", bindGoldenCases(), *updateBindGolden)
}
