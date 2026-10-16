package gitprov

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"
)

// The checks of Observe read the layout of a worktree: whether .git is a directory or a gitfile,
// what a gitfile names, what the git dir points back at. They read it through Observer.FS, so the
// layouts below exist only in memory, at paths no disk has, and the answers git would give are the
// fake Runner's. Nothing here touches the file system of the machine.

type memEntry struct {
	mode    fs.FileMode
	content string
	target  string // for a symbolic link
}

// memFS is a file system in memory: the entries it holds by their full path, and the paths whose
// resolution is made to fail.
type memFS struct {
	entries map[string]memEntry
	failing map[string]error
}

func newMemFS() *memFS { return &memFS{entries: map[string]memEntry{}, failing: map[string]error{}} }

func (m *memFS) dir(p string) *memFS { m.entries[p] = memEntry{mode: fs.ModeDir | 0o755}; return m }
func (m *memFS) file(p, content string) *memFS {
	m.entries[p] = memEntry{mode: 0o644, content: content}
	return m
}
func (m *memFS) fifo(p string) *memFS {
	m.entries[p] = memEntry{mode: fs.ModeNamedPipe | 0o644}
	return m
}

func (m *memFS) notExist(op, p string) error {
	return &fs.PathError{Op: op, Path: p, Err: fs.ErrNotExist}
}

type memInfo struct {
	name string
	mode fs.FileMode
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return 0 }
func (i memInfo) Mode() fs.FileMode  { return i.mode }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.mode.IsDir() }
func (i memInfo) Sys() any           { return nil }

func (m *memFS) Lstat(p string) (fs.FileInfo, error) {
	e, ok := m.entries[p]
	if !ok {
		return nil, m.notExist("lstat", p)
	}
	return memInfo{name: path.Base(p), mode: e.mode}, nil
}

func (m *memFS) ReadFile(p string) ([]byte, error) {
	e, ok := m.entries[p]
	if !ok {
		return nil, m.notExist("open", p)
	}
	if !e.mode.IsRegular() {
		return nil, &fs.PathError{Op: "read", Path: p, Err: errors.New("not a regular file")}
	}
	return []byte(e.content), nil
}

func (m *memFS) EvalSymlinks(p string) (string, error) {
	if err := m.failing[p]; err != nil {
		return "", err
	}
	e, ok := m.entries[p]
	if !ok {
		return "", m.notExist("lstat", p)
	}
	if e.mode&fs.ModeSymlink != 0 {
		return m.EvalSymlinks(e.target)
	}
	return p, nil
}

// virtualRepo is the answers git gives for a worktree at root whose git dir is gitDir and whose
// common dir is commonDir, with an Observer over fsys that asks no disk.
func virtualRepo(root, gitDir, commonDir string, fsys FS) (*fakeRepo, Observer) {
	repo := &fakeRepo{
		root: root,
		answers: map[string]string{
			"rev-parse --is-bare-repository":   "false\n",
			"rev-parse --is-inside-work-tree":  "true\n",
			"rev-parse --show-toplevel":        root + "\n",
			"rev-parse --absolute-git-dir":     gitDir + "\n",
			"rev-parse --git-common-dir":       commonDir + "\n",
			"rev-parse --verify HEAD^{commit}": testHead + "\n",
		},
		fail: map[string]error{},
	}
	return repo, Observer{Run: repo.run, FS: fsys}
}

func TestObserveReadsTheLayoutOfAMainWorktreeThroughTheFSItIsGiven(t *testing.T) {
	fsys := newMemFS().dir("/virtual/repo").dir("/virtual/repo/.git")
	_, observer := virtualRepo("/virtual/repo", "/virtual/repo/.git", "/virtual/repo/.git", fsys)

	got, err := observer.Observe("/virtual/repo")

	want := Observation{Toplevel: "/virtual/repo", GitDir: "/virtual/repo/.git", CommonDir: "/virtual/repo/.git", Head: testHead}
	if err != nil || got != want {
		t.Errorf("Observe = (%+v, %v), want %+v", got, err, want)
	}
}

func TestObserveReadsTheLayoutOfALinkedWorktreeThroughTheFSItIsGiven(t *testing.T) {
	const (
		root   = "/virtual/wt"
		common = "/virtual/main/.git"
		gitDir = "/virtual/main/.git/worktrees/wt"
	)
	fsys := newMemFS().dir(root).dir(common).dir(gitDir).
		file(root+"/.git", "gitdir: "+gitDir+"\n").
		file(gitDir+"/gitdir", root+"/.git\n")
	_, observer := virtualRepo(root, gitDir, common, fsys)

	got, err := observer.Observe(root)

	want := Observation{Toplevel: root, GitDir: gitDir, CommonDir: common, Head: testHead, Linked: true}
	if err != nil || got != want {
		t.Errorf("Observe = (%+v, %v), want %+v", got, err, want)
	}
}

func TestLocateReadsTheLayoutThroughTheFSItIsGivenFromADirectoryInsideTheWorktree(t *testing.T) {
	fsys := newMemFS().dir("/virtual/repo").dir("/virtual/repo/.git").dir("/virtual/repo/deep")
	repo, observer := virtualRepo("/virtual/repo", "/virtual/repo/.git", "/virtual/repo/.git", fsys)
	repo.fail["rev-parse --verify HEAD^{commit}"] = errors.New("exit status 128")

	got, err := observer.Locate("/virtual/repo/deep")

	want := Observation{Toplevel: "/virtual/repo", GitDir: "/virtual/repo/.git", CommonDir: "/virtual/repo/.git"}
	if err != nil || got != want {
		t.Errorf("Locate = (%+v, %v), want %+v", got, err, want)
	}
}

// Every refusal that reads the layout is the same refusal when the layout is in memory.
func TestObserveRefusesALayoutThatDoesNotAgreeWithGitAndSaysWhyThroughTheFSItIsGiven(t *testing.T) {
	const (
		root   = "/virtual/wt"
		common = "/virtual/main/.git"
		gitDir = "/virtual/main/.git/worktrees/wt"
	)
	linked := func() *memFS {
		return newMemFS().dir(root).dir(common).dir(gitDir).
			file(root+"/.git", "gitdir: "+gitDir+"\n").
			file(gitDir+"/gitdir", root+"/.git\n")
	}
	for _, tc := range []struct {
		name      string
		fsys      *memFS
		gitDir    string
		commonDir string
		mutate    func(*fakeRepo)
		want      string
	}{
		{"no .git at all", newMemFS().dir(root), "/virtual/x/.git", "/virtual/x/.git", nil, "inspect " + root + "/.git"},
		{"a .git that is a FIFO", newMemFS().dir(root).fifo(root + "/.git"), "/virtual/x/.git", "/virtual/x/.git", nil, "is neither a directory nor a regular file"},
		{"a gitfile that leads to another git dir than git reports",
			newMemFS().dir(root).dir("/virtual/other/.git").dir("/virtual/x/.git").file(root+"/.git", "gitdir: /virtual/other/.git\n"),
			"/virtual/x/.git", "/virtual/x/.git", nil, "not the reported git dir"},
		{"a gitfile that is not a pointer",
			newMemFS().dir(root).file(root+"/.git", "something else\n"), "/virtual/x/.git", "/virtual/x/.git", nil, "malformed pointer"},
		{"a forged gitfile: its git dir is its own common dir and names no worktree",
			newMemFS().dir(root).dir("/virtual/main/.git").file(root+"/.git", "gitdir: /virtual/main/.git\n"),
			"/virtual/main/.git", "/virtual/main/.git", func(r *fakeRepo) { r.fail["config --local --get core.worktree"] = errors.New("exit status 1") }, "core.worktree is not usable"},
		{"a linked git dir that does not point back", linked().file(gitDir+"/gitdir", "/virtual/elsewhere/.git\n").dir("/virtual/elsewhere/.git"),
			gitDir, common, nil, "linked worktree mismatch"},
		{"a back-pointer that is a FIFO", linked().fifo(gitDir + "/gitdir"), gitDir, common, nil, "is not a regular file"},
		{"a back-pointer that is missing", func() *memFS { f := linked(); delete(f.entries, gitDir+"/gitdir"); return f }(), gitDir, common, nil, "inspect " + gitDir + "/gitdir"},
		{"a git dir that is not under the common dir's worktrees",
			newMemFS().dir(root).dir(common).file(root+"/.git", "gitdir: /virtual/main/.git/elsewhere/wt\n"),
			"/virtual/main/.git/elsewhere/wt", common, nil, "is not under"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, observer := virtualRepo(root, tc.gitDir, tc.commonDir, tc.fsys)
			for _, p := range []string{tc.gitDir, tc.commonDir} {
				if _, ok := tc.fsys.entries[p]; !ok {
					tc.fsys.dir(p)
				}
			}
			if tc.mutate != nil {
				tc.mutate(repo)
			}

			got, err := observer.Observe(root)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Observe = (%+v, %v), want a refusal that says %q", got, err, tc.want)
			}
			if got != (Observation{}) {
				t.Errorf("Observe returned %+v alongside the refusal", got)
			}
		})
	}
}

func TestObserveSaysWhichPathCouldNotBeResolvedThroughTheFSItIsGiven(t *testing.T) {
	fsys := newMemFS().dir("/virtual/repo").dir("/virtual/repo/.git")
	fsys.failing["/virtual/repo"] = errors.New("too many levels of symbolic links")
	_, observer := virtualRepo("/virtual/repo", "/virtual/repo/.git", "/virtual/repo/.git", fsys)

	_, err := observer.Observe("/virtual/repo")

	if want := `resolve root "/virtual/repo": too many levels of symbolic links`; err == nil || err.Error() != want {
		t.Errorf("Observe error = %v, want %q", err, want)
	}
}

// A symbolic link is followed as the FS resolves it, so a worktree reached through one is judged
// where it really is.
func TestObserveJudgesAWorktreeWhereTheFSResolvesItsPath(t *testing.T) {
	fsys := newMemFS().dir("/virtual/real").dir("/virtual/real/.git")
	fsys.entries["/virtual/link"] = memEntry{mode: fs.ModeSymlink, target: "/virtual/real"}
	_, observer := virtualRepo("/virtual/real", "/virtual/real/.git", "/virtual/real/.git", fsys)

	got, err := observer.Observe("/virtual/link")

	if err != nil || got.Toplevel != "/virtual/real" {
		t.Errorf("Observe = (%+v, %v), want the worktree judged at /virtual/real", got, err)
	}
}
