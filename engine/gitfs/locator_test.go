package gitfs

// Fixture repos here are hand-built plain directories and files, never a
// real git repository and never the git binary: the Locator must work (or
// fail soft) without one.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// locator is the adapter under test; it has no state.
var locator Locator

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("os.MkdirAll(%q) = %v, want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) = %v, want nil", path, err)
	}
}

func TestProvenanceRejectsRelativeOrEmptyCwd(t *testing.T) {
	for _, cwd := range []string{"", "relative/path", "."} {
		p := locator.Provenance(cwd)
		if p.WorktreeRoot != "" || p.GitHead != "" {
			t.Fatalf("locator.Provenance(%q) = %+v, want empty Provenance", cwd, p)
		}
	}
}

func TestProvenanceDetachedHead(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("a", 40)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), head+"\n")

	p := locator.Provenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q", p.GitHead, head)
	}
}

func TestProvenanceWalksUpFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("b", 64)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), head+"\n")
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() = %v, want nil", err)
	}

	p := locator.Provenance(sub)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q (found by walking up from %q)", p.WorktreeRoot, root, sub)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q", p.GitHead, head)
	}
}

func TestProvenanceSymbolicRefResolvedFromLooseRefFile(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("c", 40)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFixtureFile(t, filepath.Join(root, ".git", "refs", "heads", "main"), head+"\n")

	p := locator.Provenance(root)
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (resolved from the loose ref file)", p.GitHead, head)
	}
}

func TestProvenanceSymbolicRefResolvedFromPackedRefs(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("d", 40)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	// No loose refs/heads/main file: only packed-refs names it.
	writeFixtureFile(t, filepath.Join(root, ".git", "packed-refs"),
		"# pack-refs with: peeled fully-peeled sorted\n"+
			strings.Repeat("e", 40)+" refs/heads/other\n"+
			head+" refs/heads/main\n"+
			"^"+strings.Repeat("f", 40)+"\n")

	p := locator.Provenance(root)
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (resolved from packed-refs)", p.GitHead, head)
	}
}

func TestProvenanceLinkedWorktreeGitfile(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "worktree")
	actualGitDir := filepath.Join(t.TempDir(), "main-repo", ".git", "worktrees", "worktree")
	head := strings.Repeat("1", 40)
	writeFixtureFile(t, filepath.Join(actualGitDir, "HEAD"), head+"\n")
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+actualGitDir+"\n")

	p := locator.Provenance(worktree)
	if p.WorktreeRoot != worktree {
		t.Fatalf("WorktreeRoot = %q, want %q (this worktree's own root, not the main repo)", p.WorktreeRoot, worktree)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q", p.GitHead, head)
	}
}

// TestObserveProvenanceLinkedWorktreeSymbolicRefUsesCommonDir mirrors the
// layout `git worktree add` creates: the worktree's gitdir holds a symbolic
// HEAD and a commondir file, while the branch ref (loose or packed) lives in
// the main repository's .git.
func TestProvenanceLinkedWorktreeSymbolicRefUsesCommonDir(t *testing.T) {
	for _, packed := range []bool{false, true} {
		root := t.TempDir()
		mainGit := filepath.Join(root, "main-repo", ".git")
		worktreeGitDir := filepath.Join(mainGit, "worktrees", "wt")
		worktree := filepath.Join(root, "wt")
		head := strings.Repeat("2", 40)
		writeFixtureFile(t, filepath.Join(worktreeGitDir, "HEAD"), "ref: refs/heads/feat/x\n")
		writeFixtureFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
		if packed {
			writeFixtureFile(t, filepath.Join(mainGit, "packed-refs"), "# pack-refs with: peeled\n"+head+" refs/heads/feat/x\n")
		} else {
			writeFixtureFile(t, filepath.Join(mainGit, "refs", "heads", "feat", "x"), head+"\n")
		}
		writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")

		if p := locator.Provenance(worktree); p.GitHead != head {
			t.Errorf("packed=%v: GitHead = %q, want %q resolved through commondir", packed, p.GitHead, head)
		}
	}
}

func TestProvenanceMissingGitYieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := locator.Provenance(dir)
	if p.WorktreeRoot != "" || p.GitHead != "" {
		t.Fatalf("locator.Provenance(%q) = %+v, want empty Provenance (no .git anywhere above it)", dir, p)
	}
}

func TestProvenanceMalformedGitfileYieldsEmpty(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git"), "not a gitdir pointer\n")

	p := locator.Provenance(root)
	if p.WorktreeRoot != "" || p.GitHead != "" {
		t.Fatalf("locator.Provenance() = %+v, want empty Provenance for a malformed .git file", p)
	}
}

func TestProvenanceGitfilePointingNowhereYieldsEmpty(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git"), "gitdir: "+filepath.Join(root, "does-not-exist")+"\n")

	p := locator.Provenance(root)
	if p.WorktreeRoot != "" || p.GitHead != "" {
		t.Fatalf("locator.Provenance() = %+v, want empty Provenance when the gitdir target does not exist", p)
	}
}

func TestProvenanceUnreadableHeadYieldsEmptyHeadOnly(t *testing.T) {
	root := t.TempDir()
	// .git exists as a directory but HEAD is missing entirely.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("os.MkdirAll() = %v, want nil", err)
	}

	p := locator.Provenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q (the worktree is still observed even without HEAD)", p.WorktreeRoot, root)
	}
	if p.GitHead != "" {
		t.Fatalf("GitHead = %q, want empty when HEAD is missing", p.GitHead)
	}
}

func TestProvenanceHeadNotHexYieldsEmptyHead(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "garbage\n")

	p := locator.Provenance(root)
	if p.GitHead != "" {
		t.Fatalf("GitHead = %q, want empty for a HEAD that is neither a hex object id nor a ref line", p.GitHead)
	}
}

func TestProvenanceUnresolvableSymbolicRefYieldsEmptyHead(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/does-not-exist\n")

	p := locator.Provenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != "" {
		t.Fatalf("GitHead = %q, want empty when the ref resolves nowhere", p.GitHead)
	}
}

// TestObserveProvenanceFollowsSymlinkedGitDirectory covers the decision
// that a symlinked .git IS followed, matching git's own behavior: git
// itself does not care whether .git is a plain directory or a symlink to
// one, so this helper must not either.
func TestProvenanceFollowsSymlinkedGitDirectory(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(t.TempDir(), "actual-git-dir")
	head := strings.Repeat("3", 40)
	writeFixtureFile(t, filepath.Join(actual, "HEAD"), head+"\n")
	if err := os.Symlink(actual, filepath.Join(root, ".git")); err != nil {
		t.Skipf("os.Symlink() = %v, symlinks unsupported here", err)
	}

	p := locator.Provenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (a symlinked .git directory must be followed)", p.GitHead, head)
	}
}

// TestObserveProvenanceFollowsSymlinkedGitfile covers a symlink named .git
// that itself points at a gitdir-pointer FILE (rather than a directory),
// the shape a hand-rolled or unusual worktree tool might produce.
func TestProvenanceFollowsSymlinkedGitfile(t *testing.T) {
	root := t.TempDir()
	actualGitDir := filepath.Join(t.TempDir(), "main-repo", ".git", "worktrees", "worktree")
	head := strings.Repeat("4", 40)
	writeFixtureFile(t, filepath.Join(actualGitDir, "HEAD"), head+"\n")
	gitfile := filepath.Join(t.TempDir(), "real.gitfile")
	writeFixtureFile(t, gitfile, "gitdir: "+actualGitDir+"\n")
	if err := os.Symlink(gitfile, filepath.Join(root, ".git")); err != nil {
		t.Skipf("os.Symlink() = %v, symlinks unsupported here", err)
	}

	p := locator.Provenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (a symlinked .git pointer file must be followed)", p.GitHead, head)
	}
}

// TestObserveProvenanceGitdirPointerRelativeToBase covers readGitdirPointer's
// relative-target branch: the common on-disk shape `git worktree add`
// actually produces, where the "gitdir:" line names a path relative to the
// worktree root rather than an absolute one.
func TestProvenanceGitdirPointerRelativeToBase(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	actualGitDir := filepath.Join(root, "main-repo", ".git", "worktrees", "wt")
	head := strings.Repeat("5", 40)
	writeFixtureFile(t, filepath.Join(actualGitDir, "HEAD"), head+"\n")
	rel, err := filepath.Rel(worktree, actualGitDir)
	if err != nil {
		t.Fatalf("filepath.Rel() = %v, want nil", err)
	}
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+rel+"\n")

	p := locator.Provenance(worktree)
	if p.WorktreeRoot != worktree {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, worktree)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (resolved from a relative gitdir pointer)", p.GitHead, head)
	}
}

// TestRefDirsLooksInTheGitDirectoryThenItsCommonDirectory characterizes what
// refDirs returns, so factoring the commondir lookup into commonDir cannot
// change where HEAD's symbolic ref is searched.
func TestRefDirsLooksInTheGitDirectoryThenItsCommonDirectory(t *testing.T) {
	base := t.TempDir()
	tests := []struct {
		name      string
		commondir *string // nil: no commondir file at all
		want      func(gitDir string) []string
	}{
		{"no commondir file", nil, func(g string) []string { return []string{g} }},
		{"an empty commondir file", strPtr("\n"), func(g string) []string { return []string{g} }},
		{"a relative commondir", strPtr("../..\n"), func(g string) []string { return []string{g, filepath.Dir(filepath.Dir(g))} }},
		{"an absolute commondir", strPtr(filepath.Join(base, "elsewhere") + "\n"), func(g string) []string { return []string{g, filepath.Join(base, "elsewhere")} }},
		{"a commondir that needs cleaning", strPtr("../.././/\n"), func(g string) []string { return []string{g, filepath.Dir(filepath.Dir(g))} }},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir := filepath.Join(base, "main"+strconv.Itoa(i), ".git", "worktrees", "wt")
			writeFixtureFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
			if tt.commondir != nil {
				writeFixtureFile(t, filepath.Join(gitDir, "commondir"), *tt.commondir)
			}
			got := refDirs(gitDir)
			want := tt.want(gitDir)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("refDirs(%q) = %v, want %v", gitDir, got, want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

// --- Locator.RepoKey and commonDir ---------------------------------------------

var repoKeyShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// wantRepoKey is the key the design specifies for a git common directory: the
// lowercase hex SHA-256 of its symlink-resolved path. The tests compute it
// independently of Locator.RepoKey, from the fixture they built.
func wantRepoKey(t *testing.T, commonDir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) = %v, want nil", commonDir, err)
	}
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:])
}

// fixtureRepo builds a hand-made plain repository (a .git directory with a
// HEAD) named name under a fresh temporary directory, and returns its root.
func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), strings.Repeat("a", 40)+"\n")
	return root
}

// fixtureLinkedWorktree adds a linked worktree called name to the repository
// at mainRoot, in the layout `git worktree add` creates: the worktree's .git
// file points at .git/worktrees/<name>, whose commondir file leads back to the
// main .git. It returns the worktree root.
func fixtureLinkedWorktree(t *testing.T, mainRoot, name string) string {
	t.Helper()
	worktreeGitDir := filepath.Join(mainRoot, ".git", "worktrees", name)
	writeFixtureFile(t, filepath.Join(worktreeGitDir, "HEAD"), strings.Repeat("b", 40)+"\n")
	writeFixtureFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
	worktree := filepath.Join(t.TempDir(), name)
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")
	return worktree
}

func mustRepoKey(t *testing.T, cwd string) string {
	t.Helper()
	key, ok := locator.RepoKey(cwd)
	if !ok {
		t.Fatalf("locator.RepoKey(%q) = _, false, want a key", cwd)
	}
	if !repoKeyShape.MatchString(key) {
		t.Fatalf("locator.RepoKey(%q) = %q, want 64 lowercase hex characters", cwd, key)
	}
	return key
}

func TestRepoKeyIsTheDigestOfTheGitDirectory(t *testing.T) {
	root := fixtureRepo(t, "repo")
	if got, want := mustRepoKey(t, root), wantRepoKey(t, filepath.Join(root, ".git")); got != want {
		t.Fatalf("locator.RepoKey() = %q, want the SHA-256 of the .git directory, %q", got, want)
	}
}

func TestRepoKeyWalksUpFromASubdirectory(t *testing.T) {
	root := fixtureRepo(t, "repo")
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, want := mustRepoKey(t, sub), mustRepoKey(t, root); got != want {
		t.Fatalf("key from a subdirectory = %q, want the repository's key %q", got, want)
	}
}

// TestObserveRepoKeyLinkedWorktreesShareTheRepositoryKey is the point of the
// key: every worktree of one repository must resolve to one binding, so the
// key is the digest of the common directory, not of each worktree's own git
// directory.
func TestRepoKeyLinkedWorktreesShareTheRepositoryKey(t *testing.T) {
	root := fixtureRepo(t, "main-repo")
	first := fixtureLinkedWorktree(t, root, "wt1")
	second := fixtureLinkedWorktree(t, root, "wt2")

	want := wantRepoKey(t, filepath.Join(root, ".git"))
	for name, cwd := range map[string]string{"the main checkout": root, "the first worktree": first, "the second worktree": second} {
		if got := mustRepoKey(t, cwd); got != want {
			t.Errorf("%s: key = %q, want the common directory's key %q", name, got, want)
		}
	}
}

func TestRepoKeyFollowsAnAbsoluteCommondir(t *testing.T) {
	root := fixtureRepo(t, "main-repo")
	worktree := fixtureLinkedWorktree(t, root, "wt")
	writeFixtureFile(t, filepath.Join(root, ".git", "worktrees", "wt", "commondir"), filepath.Join(root, ".git")+"\n")

	if got, want := mustRepoKey(t, worktree), mustRepoKey(t, root); got != want {
		t.Fatalf("key with an absolute commondir = %q, want %q", got, want)
	}
}

func TestRepoKeyDiffersBetweenRepositories(t *testing.T) {
	a := mustRepoKey(t, fixtureRepo(t, "repo"))
	b := mustRepoKey(t, fixtureRepo(t, "repo")) // same name, different parent directory
	if a == b {
		t.Fatalf("two different repositories share the key %q", a)
	}
}

// TestObserveRepoKeyIsTheSameThroughASymlinkedSpelling covers a repository
// reached through a symlink, for both the plain checkout and a linked
// worktree whose gitdir pointer was written through the symlink.
func TestRepoKeyIsTheSameThroughASymlinkedSpelling(t *testing.T) {
	root := fixtureRepo(t, "real-repo")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("os.Symlink() = %v, symlinks unsupported here", err)
	}
	want := mustRepoKey(t, root)

	if got := mustRepoKey(t, alias); got != want {
		t.Errorf("key through the symlink = %q, want %q", got, want)
	}
	if got := mustRepoKey(t, filepath.Join(alias, "sub")); got != want {
		t.Errorf("key from a subdirectory through the symlink = %q, want %q", got, want)
	}

	// A worktree whose gitdir pointer names the main .git through the alias.
	aliasGitDir := filepath.Join(alias, ".git", "worktrees", "wt")
	writeFixtureFile(t, filepath.Join(aliasGitDir, "HEAD"), strings.Repeat("c", 40)+"\n")
	writeFixtureFile(t, filepath.Join(aliasGitDir, "commondir"), "../..\n")
	worktree := filepath.Join(t.TempDir(), "wt")
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+aliasGitDir+"\n")
	if got := mustRepoKey(t, worktree); got != want {
		t.Errorf("key of a worktree pointing through the symlink = %q, want %q", got, want)
	}
}

// TestObserveRepoKeyFallsBackToTheCleanedPathWhenSymlinksCannotBeResolved
// covers a commondir that names a directory that does not exist: the symlinks
// cannot be resolved, so the key is the digest of the cleaned path, and it is
// still deterministic.
func TestRepoKeyFallsBackToTheCleanedPathWhenSymlinksCannotBeResolved(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "wt")
	gitDir := filepath.Join(t.TempDir(), "gitdir")
	writeFixtureFile(t, filepath.Join(gitDir, "HEAD"), strings.Repeat("d", 40)+"\n")
	missing := filepath.Join(t.TempDir(), "gone", "..", "gone-too")
	writeFixtureFile(t, filepath.Join(gitDir, "commondir"), missing+"\n")
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")

	sum := sha256.Sum256([]byte(filepath.Clean(missing)))
	want := hex.EncodeToString(sum[:])
	if got := mustRepoKey(t, worktree); got != want {
		t.Fatalf("key = %q, want the digest of the cleaned, unresolvable path, %q", got, want)
	}
}

func TestRepoKeyOfAGitDirectoryWithoutCommondirIsItsOwn(t *testing.T) {
	// The shape of a submodule: .git is a file pointing at a git directory that
	// has no commondir, so that directory is its own common directory.
	worktree := filepath.Join(t.TempDir(), "sub")
	gitDir := filepath.Join(t.TempDir(), "modules", "sub")
	writeFixtureFile(t, filepath.Join(gitDir, "HEAD"), strings.Repeat("e", 40)+"\n")
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")

	if got, want := mustRepoKey(t, worktree), wantRepoKey(t, gitDir); got != want {
		t.Fatalf("key = %q, want the digest of the git directory itself, %q", got, want)
	}
}

func TestRepoKeyTreatsAnEmptyCommondirAsNone(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "wt")
	gitDir := filepath.Join(t.TempDir(), "gitdir")
	writeFixtureFile(t, filepath.Join(gitDir, "HEAD"), strings.Repeat("f", 40)+"\n")
	writeFixtureFile(t, filepath.Join(gitDir, "commondir"), "  \n")
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")

	if got, want := mustRepoKey(t, worktree), wantRepoKey(t, gitDir); got != want {
		t.Fatalf("key = %q, want the git directory's own digest %q", got, want)
	}
}

func TestRepoKeyReportsNotOKWithoutARepository(t *testing.T) {
	tests := []struct {
		name string
		cwd  func(t *testing.T) string
	}{
		{"no .git anywhere above", func(t *testing.T) string { return t.TempDir() }},
		{"an empty working directory", func(t *testing.T) string { return "" }},
		{"a relative working directory", func(t *testing.T) string { return "relative/path" }},
		{"a dot", func(t *testing.T) string { return "." }},
		{"a malformed .git file", func(t *testing.T) string {
			root := t.TempDir()
			writeFixtureFile(t, filepath.Join(root, ".git"), "not a gitdir pointer\n")
			return root
		}},
		{"a .git file pointing nowhere", func(t *testing.T) string {
			root := t.TempDir()
			writeFixtureFile(t, filepath.Join(root, ".git"), "gitdir: "+filepath.Join(root, "missing")+"\n")
			return root
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if key, ok := locator.RepoKey(tt.cwd(t)); ok || key != "" {
				t.Fatalf("locator.RepoKey() = %q, %v, want \"\", false", key, ok)
			}
		})
	}
}

func TestCommonDir(t *testing.T) {
	base := t.TempDir()
	gitDir := filepath.Join(base, "main", ".git", "worktrees", "wt")

	// No commondir file: a plain repository is its own common directory.
	if got := commonDir(gitDir); got != gitDir {
		t.Errorf("commonDir without a commondir file = %q, want the git directory %q", got, gitDir)
	}

	writeFixtureFile(t, filepath.Join(gitDir, "commondir"), "../..\n")
	if got, want := commonDir(gitDir), filepath.Join(base, "main", ".git"); got != want {
		t.Errorf("commonDir with a relative commondir = %q, want %q", got, want)
	}

	writeFixtureFile(t, filepath.Join(gitDir, "commondir"), filepath.Join(base, "elsewhere")+"\n")
	if got, want := commonDir(gitDir), filepath.Join(base, "elsewhere"); got != want {
		t.Errorf("commonDir with an absolute commondir = %q, want %q", got, want)
	}

	writeFixtureFile(t, filepath.Join(gitDir, "commondir"), "\n")
	if got := commonDir(gitDir); got != gitDir {
		t.Errorf("commonDir with an empty commondir = %q, want the git directory %q", got, gitDir)
	}
}

// TestObserveRepoKeyNeverResolvesARelativeCwdAgainstTheProcessDirectory pins
// why a relative cwd is refused outright: were it walked, "." would silently
// mean whatever repository the process happens to be standing in. The test
// stands the process in a repository to make that visible.
func TestRepoKeyNeverResolvesARelativeCwdAgainstTheProcessDirectory(t *testing.T) {
	root := fixtureRepo(t, "repo")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(previous) })

	for _, cwd := range []string{".", "", "./"} {
		if key, ok := locator.RepoKey(cwd); ok || key != "" {
			t.Errorf("locator.RepoKey(%q) = %q, %v, want \"\", false while the process stands in a repository", cwd, key, ok)
		}
	}
	// Control: the same repository, named absolutely, does resolve.
	mustRepoKey(t, root)
}
