package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// executablePlan produces a real, validated plan over a real (empty) t.TempDir
// project root and returns it with that root. Nothing here touches $HOME, the
// live .claude/skills, .agents/skills, .pi or skills-lock.json.
func executablePlan(t *testing.T, id string) (ProjectPlan, string) {
	t.Helper()
	in := registerInput(t, id)
	p, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal while building the fixture plan: %v", err)
	}
	return p, filepath.Clean(in.ProjectRoot)
}

// planOrder is the executor's own commit order: the Writes in projectTargets
// order, then the lock LAST.
func planOrder(p ProjectPlan) []ProjectWrite {
	return append(append([]ProjectWrite{}, p.Writes...), p.Lock)
}

// snapshotTree records every path under root with its bytes and permission
// bits, so "the pre-run tree is byte-identical" is a single comparison rather
// than a handful of spot checks. Directories are recorded too, because a
// rollback that leaves an empty `.claude/skills/<id>/` behind has not restored
// the tree.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			target, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			out[rel+"@"] = "symlink " + target
			return nil
		}
		if d.IsDir() {
			out[rel+"/"] = fmt.Sprintf("dir %04o", info.Mode().Perm())
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		out[rel] = fmt.Sprintf("file %04o %s", info.Mode().Perm(), b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// assertSameTree reports every difference rather than the first, because a
// rollback usually leaves several traces at once (a file, its directory, a
// leftover temp) and seeing one at a time hides the shape of the bug.
func assertSameTree(t *testing.T, want, got map[string]string) {
	t.Helper()
	for _, k := range sortedKeys(want) {
		if g, ok := got[k]; !ok {
			t.Errorf("rollback lost %q (was %q)", k, want[k])
		} else if g != want[k] {
			t.Errorf("rollback changed %q: got %q, want %q", k, g, want[k])
		}
	}
	for _, k := range sortedKeys(got) {
		if _, ok := want[k]; !ok {
			t.Errorf("rollback left %q behind (%q)", k, got[k])
		}
	}
}

// fakeProjectFS is the injected ProjectFS of tasks.md 3b-ii.1/3b-ii.3: every
// call is delegated to the real filesystem over a t.TempDir, and `fail` may
// turn any single call into an error. Delegating rather than simulating is
// deliberate — the rollback claim is about the real tree, so the fake injects
// failures and nothing else.
type fakeProjectFS struct {
	real  ProjectFS
	after bool // set by the injector: perform the call, THEN report failure
	fail  func(f *fakeProjectFS, op, path string) error
	n     map[string]int
	log   []string
}

func newFakeProjectFS(fail func(f *fakeProjectFS, op, path string) error) *fakeProjectFS {
	return &fakeProjectFS{real: testProjectFS(), fail: fail, n: map[string]int{}}
}

func (f *fakeProjectFS) check(op, path string) error {
	f.n[op]++
	f.log = append(f.log, op+" "+path)
	if f.fail == nil {
		return nil
	}
	return f.fail(f, op, path)
}

func (f *fakeProjectFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.check("stat", name); err != nil {
		return nil, err
	}
	return f.real.Stat(name)
}

func (f *fakeProjectFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := f.check("readdir", name); err != nil {
		return nil, err
	}
	return f.real.ReadDir(name)
}

func (f *fakeProjectFS) MkdirAll(dir string, perm fs.FileMode) error {
	f.after = false
	if err := f.check("mkdirall", dir); err != nil {
		if f.after {
			// Model os.MkdirAll faithfully: it creates every ancestor it can
			// before failing on a deeper component, and it cannot report which
			// ones it created (review round 4, D2).
			_ = f.real.MkdirAll(filepath.Dir(dir), perm)
		}
		return err
	}
	return f.real.MkdirAll(dir, perm)
}

func (f *fakeProjectFS) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	if err := f.check("writetemp", dir); err != nil {
		return "", err
	}
	return f.real.WriteTemp(dir, data, perm)
}

func (f *fakeProjectFS) Rename(oldPath, newPath string) error {
	f.after = false
	if err := f.check("rename", newPath); err != nil {
		if f.after {
			// The rename lands and still reports failure.
			_ = f.real.Rename(oldPath, newPath)
		}
		return err
	}
	return f.real.Rename(oldPath, newPath)
}

func (f *fakeProjectFS) Remove(name string) error {
	if err := f.check("remove", name); err != nil {
		return err
	}
	return f.real.Remove(name)
}

func (f *fakeProjectFS) ResolvePath(name string) (string, error) {
	if err := f.check("resolvepath", name); err != nil {
		return "", err
	}
	return f.real.ResolvePath(name)
}

// failAt returns a `fail` func that fails EVERY call of op whose path equals
// want. It keeps no counter: the previous doc comment claimed it failed "the
// n-th (1-based) call", which nothing here ever implemented or needed — the
// injection points each name a path that the executor touches once, and where
// a single failure is wanted (the commit rename but not the restoring rename)
// failAfterRename owns its own `fired` latch (review round 4, D6).
func failAt(op, want string, err error) func(*fakeProjectFS, string, string) error {
	return func(f *fakeProjectFS, gotOp, gotPath string) error {
		if gotOp == op && gotPath == want {
			return err
		}
		return nil
	}
}

// failPartialMkdir fails the MkdirAll of `want` AFTER letting its ancestors be
// created, which is what os.MkdirAll does on a real filesystem: it walks down
// creating what it can and only then reports the component it could not make.
// Nothing weaker can witness D2, because a fake that refuses the call outright
// leaves a clean tree the buggy code also leaves clean.
func failPartialMkdir(want string, err error) func(*fakeProjectFS, string, string) error {
	return func(f *fakeProjectFS, gotOp, gotPath string) error {
		if gotOp != "mkdirall" || gotPath != want {
			return nil
		}
		f.after = true
		return err
	}
}
