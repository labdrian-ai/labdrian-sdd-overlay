package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// The candidate the fakes answer for: two full commit names, the bytes of a diff and the two paths it
// changes. "HEAD~1" and "HEAD" are the revisions a person types for them.
const (
	baseSHA  = "1111111111111111111111111111111111111111"
	headSHA  = "2222222222222222222222222222222222222222"
	otherSHA = "3333333333333333333333333333333333333333"
	diffText = "diff --git a/a.go b/a.go\n"
)

var changedPaths = []string{"engine/guard.go", "engine/guard_test.go"}

// otherWriter is what the file of a writer that raced this one holds.
const otherWriter = "written by the other writer"

// fakeRepo is the repository the fake git locates: a linked worktree of a main checkout.
var fakeRepo = app.Repository{Root: "/work/wt/feature", Main: "/work/main", CommonDir: "/work/main/.git"}

// fakeGit is the Git port with canned answers. It records every call, in order, as "Name arg ..." so a
// test says which calls were made and which were not. A method named in fail returns that error.
// Worktree answers worktree to its first call; worktreeLater are the answers to its second and later calls,
// in order (a worktree that another process made in between), and worktree again once they run out.
type fakeGit struct {
	diff          []byte
	paths         []string
	parents       map[string]string // the first parent of a commit; none for a commit that is not a key
	standing      app.Standing      // what Standing answers
	worktree      app.WorktreeState
	worktreeLater []worktreeAnswer
	fail          map[string]error
	calls         []string
}

// worktreeAnswer is one answer of Worktree: what is at the path, or why git could not say.
type worktreeAnswer struct {
	state app.WorktreeState
	err   error
}

func newFakeGit() *fakeGit {
	return &fakeGit{diff: []byte(diffText), paths: changedPaths, parents: map[string]string{headSHA: baseSHA}, fail: map[string]error{}}
}

func (g *fakeGit) record(call string) { g.calls = append(g.calls, call) }

// count says how many calls were exactly call.
func (g *fakeGit) count(call string) int {
	n := 0
	for _, c := range g.calls {
		if c == call {
			n++
		}
	}
	return n
}

// called says how many calls began with prefix.
func (g *fakeGit) called(prefix string) int {
	n := 0
	for _, c := range g.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (g *fakeGit) Locate(dir string) (app.Repository, error) {
	g.record("Locate " + dir)
	if err := g.fail["Locate"]; err != nil {
		return app.Repository{}, err
	}
	return fakeRepo, nil
}

func (g *fakeGit) Commit(repo app.Repository, rev string) (string, error) {
	g.record("Commit " + rev)
	if err := g.fail["Commit "+rev]; err != nil {
		return "", err
	}
	switch rev {
	case "HEAD~1", baseSHA:
		return baseSHA, nil
	case "HEAD", headSHA:
		return headSHA, nil
	case otherSHA:
		return otherSHA, nil
	}
	return "", fmt.Errorf("fakeGit: unknown revision %q", rev)
}

func (g *fakeGit) Parent(repo app.Repository, commit string) (string, error) {
	g.record("Parent " + commit)
	if err := g.fail["Parent"]; err != nil {
		return "", err
	}
	return g.parents[commit], nil
}

func (g *fakeGit) Diff(repo app.Repository, base, head string) ([]byte, error) {
	g.record("Diff " + base + " " + head)
	if err := g.fail["Diff"]; err != nil {
		return nil, err
	}
	return g.diff, nil
}

func (g *fakeGit) Paths(repo app.Repository, base, head string) ([]string, error) {
	g.record("Paths " + base + " " + head)
	if err := g.fail["Paths"]; err != nil {
		return nil, err
	}
	return g.paths, nil
}

func (g *fakeGit) Worktree(repo app.Repository, path string) (app.WorktreeState, error) {
	g.record("Worktree " + path)
	if err := g.fail["Worktree"]; err != nil {
		return app.WorktreeState{}, err
	}
	if later := g.count("Worktree "+path) - 2; later >= 0 && later < len(g.worktreeLater) {
		return g.worktreeLater[later].state, g.worktreeLater[later].err
	}
	return g.worktree, nil
}

func (g *fakeGit) AddWorktree(repo app.Repository, path, head string) error {
	g.record("AddWorktree " + path + " " + head)
	return g.fail["AddWorktree"]
}

func (g *fakeGit) Standing(repo app.Repository) (app.Standing, error) {
	g.record("Standing")
	if err := g.fail["Standing"]; err != nil {
		return app.Standing{}, err
	}
	return g.standing, nil
}

// fakeResults is the Results port over a map: the names of a feature's directory and what was created.
// races is how many of the first Create calls find their name taken by another writer that got there a
// moment before: the fake stores that writer's file under the name, so the next listing shows it.
type fakeResults struct {
	files map[string]map[string][]byte // feature -> name -> content
	// tasks says that the tree has odd/tasks, which makes it one that keeps features even with no result stored.
	tasks bool
	fail  map[string]error
	calls []string
	races int
	// unflushed, when set, is what Create returns after it stored the file: a store that published the
	// result but could not flush its directory.
	unflushed error
}

func newFakeResults() *fakeResults {
	return &fakeResults{files: map[string]map[string][]byte{}, fail: map[string]error{}}
}

func (r *fakeResults) put(feature, name string, data []byte) {
	if r.files[feature] == nil {
		r.files[feature] = map[string][]byte{}
	}
	r.files[feature][name] = data
}

// callsTo says how many calls began with prefix.
func (r *fakeResults) callsTo(prefix string) int {
	n := 0
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (r *fakeResults) created() int { return r.callsTo("Create ") }

func (r *fakeResults) listed() int { return r.callsTo("Names ") }

// pauses records the waits a Service asks for instead of making them, so that no test sleeps.
type pauses struct{ asked []time.Duration }

func (p *pauses) sleep(d time.Duration) { p.asked = append(p.asked, d) }

func (r *fakeResults) Names(root, feature string) ([]string, error) {
	r.calls = append(r.calls, "Names "+root+" "+feature)
	if err := r.fail["Names"]; err != nil {
		return nil, err
	}
	var names []string
	for name := range r.files[feature] {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (r *fakeResults) Features(root string) (app.Features, error) {
	r.calls = append(r.calls, "Features "+root)
	if err := r.fail["Features"]; err != nil {
		return app.Features{}, err
	}
	features := app.Features{Tracked: r.tasks || len(r.files) > 0}
	for name := range r.files {
		features.Names = append(features.Names, name)
	}
	sort.Strings(features.Names)
	return features, nil
}

func (r *fakeResults) Read(root, feature, name string) ([]byte, error) {
	r.calls = append(r.calls, "Read "+root+" "+feature+" "+name)
	if err := r.fail["Read"]; err != nil {
		return nil, err
	}
	data, stored := r.files[feature][name]
	if !stored {
		return nil, app.ErrNotFound
	}
	return data, nil
}

func (r *fakeResults) Lock(root, feature string) (func(), error) {
	r.calls = append(r.calls, "Lock "+root+" "+feature)
	if err := r.fail["Lock"]; err != nil {
		return nil, err
	}
	return func() { r.calls = append(r.calls, "Unlock "+root+" "+feature) }, nil
}

func (r *fakeResults) Create(root, feature, name string, data []byte) error {
	r.calls = append(r.calls, "Create "+root+" "+feature+" "+name)
	if err := r.fail["Create"]; err != nil {
		return err
	}
	if r.races > 0 {
		r.races--
		r.put(feature, name, []byte(otherWriter))
		return app.ErrExists
	}
	if _, taken := r.files[feature][name]; taken {
		return app.ErrExists
	}
	r.put(feature, name, data)
	return r.unflushed
}

// candidateOf is the candidate the fake git gives, as the domain binds it.
func candidateOf(t *testing.T, g *fakeGit) review.Candidate {
	t.Helper()
	c, err := review.NewCandidate(baseSHA, headSHA, g.diff, g.paths)
	if err != nil {
		t.Fatalf("setup: NewCandidate() error = %v", err)
	}
	return c
}

// replyFor is a reply of a reviewer that inspected every path of c and found nothing, echoing the
// candidate it was given.
func replyFor(t *testing.T, c review.Candidate) []byte {
	t.Helper()
	return replyWith(t, c.Base, c.Head, c.DiffSHA256, "completed", c.Paths)
}

// replyWith is a clean reply that echoes the given values and lists the given paths.
func replyWith(t *testing.T, base, head, digest, status string, paths []string) []byte {
	t.Helper()
	reply := map[string]any{
		"candidate":  map[string]string{"base": base, "head": head, "diff_sha256": digest},
		"inspection": map[string]any{"status": status, "paths": paths, "reason": ""},
		"findings":   []any{},
		"evidence":   []string{"read every changed path"},
	}
	data, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return data
}

// storedWithoutRows is the stored file of a clean result of c with its rows member taken out: the shape of a result
// stored before the rows were recorded.
func storedWithoutRows(t *testing.T, c review.Candidate) []byte {
	t.Helper()
	file, err := review.EncodeResult(c, []string{"default"}, replyFor(t, c))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(file, &members); err != nil {
		t.Fatalf("setup: %v", err)
	}
	delete(members, "rows")
	out, err := json.Marshal(members)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// sha256Of is the SHA-256 of data, written without the code under test.
func sha256Of(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
