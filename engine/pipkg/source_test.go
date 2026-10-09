package pipkg_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// fakeRepo is a SourceRepo that answers from a script and logs what it was asked, so a test can
// say what the builder asks of git without a git.
type fakeRepo struct {
	log []string

	worktree    bool
	commits     map[string]bool // refs HasCommit accepts
	paths       map[string]bool // paths HasPath accepts
	ancestor    bool
	resolve     map[string]string // ref -> id
	resolveErr  error
	changes     bool
	changesErr  error
	tag         string
	tagErr      error
	exportBytes func(paths []string) []byte
	exportErr   error
}

func (f *fakeRepo) note(format string, args ...any) {
	f.log = append(f.log, fmt.Sprintf(format, args...))
}

func (f *fakeRepo) IsWorkTree(dir string) bool { f.note("IsWorkTree"); return f.worktree }
func (f *fakeRepo) HasCommit(dir, ref string) bool {
	f.note("HasCommit %s", ref)
	return f.commits[ref]
}
func (f *fakeRepo) HasPath(dir, rev, path string) bool {
	f.note("HasPath %s %s", rev, path)
	return f.paths[path]
}
func (f *fakeRepo) IsAncestor(dir, a, d string) bool {
	f.note("IsAncestor %s %s", a, d)
	return f.ancestor
}
func (f *fakeRepo) Resolve(dir, ref string) (string, error) {
	f.note("Resolve %s", ref)
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	return f.resolve[ref], nil
}
func (f *fakeRepo) HasChanges(dir string, paths ...string) (bool, error) {
	f.note("HasChanges %s", strings.Join(paths, ","))
	return f.changes, f.changesErr
}
func (f *fakeRepo) LatestTag(dir, rev, pattern string) (string, error) {
	f.note("LatestTag %q %s", rev, pattern)
	return f.tag, f.tagErr
}
func (f *fakeRepo) Export(dir, rev string, paths []string) ([]byte, error) {
	f.note("Export %s %s", rev, strings.Join(paths, ","))
	if f.exportErr != nil {
		return nil, f.exportErr
	}
	return f.exportBytes(paths), nil
}

func (f *fakeRepo) asked(prefix string) []string {
	var out []string
	for _, line := range f.log {
		if strings.HasPrefix(line, prefix) {
			out = append(out, line)
		}
	}
	return out
}

// tarOf archives the given paths of root the way `git archive` would: regular files and
// directories, names relative to root.
func tarOf(t *testing.T, root string, paths []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, p := range paths {
		err := filepath.Walk(filepath.Join(root, p), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if info.IsDir() {
				return tw.WriteHeader(&tar.Header{Name: rel + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := tw.WriteHeader(&tar.Header{Name: rel, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
				return err
			}
			_, err = tw.Write(body)
			return err
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("archiving %s: %v", p, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const fakeTip = "0123456789abcdef0123456789abcdef01234567"

// deployingRepo is a fake of a repository that has main at fakeTip and whose archive is the tree
// of overlayRoot.
func deployingRepo(t *testing.T, overlayRoot string) *fakeRepo {
	t.Helper()
	return &fakeRepo{
		worktree: true,
		commits:  map[string]bool{"main": true, "origin/main": true, "HEAD": true},
		resolve:  map[string]string{"main": fakeTip, "HEAD": fakeTip},
		tagErr:   errors.New("no tag"),
		exportBytes: func(paths []string) []byte {
			return tarOf(t, overlayRoot, paths)
		},
	}
}

func TestBuildAsksGitForTheCommitTheSourcesAreAtAndTheVersionTag(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	repo := &fakeRepo{resolve: map[string]string{"HEAD": fakeTip}, tag: "v2.3.4"}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := (pipkg.Packages{Registries: fileRegistries, Source: repo}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(destDir, "package.json"))
	if !strings.Contains(string(raw), `"version": "2.3.4"`) || !strings.Contains(string(raw), `"builtFrom": "`+fakeTip+`"`) {
		t.Errorf("package.json = %s, want the version of the tag without its v and the commit git resolved", raw)
	}
	if got := repo.asked("HasChanges"); len(got) != 1 || got[0] != "HasChanges skills,agents,skills.registry.yaml" {
		t.Errorf("the dirty check asked %v, want the three sources a build copies", got)
	}
	if got := repo.asked("LatestTag"); len(got) != 1 || got[0] != `LatestTag "`+fakeTip+`" v*` {
		t.Errorf("the version lookup asked %v, want the tag matching v* reachable from the recorded commit", got)
	}
}

func TestBuildOfAnUncommittedTreeRecordsNoCommit(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	repo := &fakeRepo{resolve: map[string]string{"HEAD": fakeTip}, changes: true, tagErr: errors.New("no tag")}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := (pipkg.Packages{Registries: fileRegistries, Source: repo}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(destDir, "package.json"))
	if strings.Contains(string(raw), "builtFrom") || !strings.Contains(string(raw), `"version": "0.0.0-dev"`) {
		t.Errorf("package.json = %s, want no builtFrom and the development version", raw)
	}
	if got := repo.asked("Resolve"); len(got) != 0 {
		t.Errorf("a dirty tree still asked git for HEAD: %v", got)
	}
}

func TestAGitThatCannotBeAskedIsATreeWithoutVersionControl(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	repo := &fakeRepo{resolveErr: errors.New("git is not installed"), changesErr: errors.New("git is not installed"), tagErr: errors.New("git is not installed")}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	packages := pipkg.Packages{Registries: fileRegistries, Source: repo}

	if err := packages.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	report, err := packages.Compare(overlayRoot, registryPath, destDir)
	if err != nil || report.Basis != "worktree" {
		t.Fatalf("Compare = %+v, %v, want the worktree basis and no drift", report, err)
	}
}

func TestCompareExportsTheDeployRefAndNamesItsTip(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	repo := deployingRepo(t, overlayRoot)
	repo.paths = map[string]bool{"pi": false}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	packages := pipkg.Packages{Registries: fileRegistries, Source: repo}
	// Build as if the sources were at the tip, so the package matches the export.
	buildRepo := &fakeRepo{resolve: map[string]string{"HEAD": fakeTip}, tagErr: errors.New("no tag")}
	if err := (pipkg.Packages{Registries: fileRegistries, Source: buildRepo}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	report, err := packages.Compare(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if report.Basis != "deploy" || report.DeployRef != "main" || report.DeployTip != fakeTip[:12] || report.BuiltFrom != fakeTip || report.Stale {
		t.Errorf("report = %+v, want a deploy basis on main at %s, built from the same commit and not stale", report, fakeTip[:12])
	}
	if got := repo.asked("Export"); len(got) != 1 || got[0] != "Export main skills,agents,skills.registry.yaml" {
		t.Errorf("the export asked %v, want the three sources and no pi path the commit lacks", got)
	}
}

func TestCompareExportsThePiPathOnlyWhenTheCommitHasIt(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	writeFile(t, filepath.Join(overlayRoot, "pi", "agents", "GADU.md"), "---\nname: GADU\n---\npi\n")
	repo := deployingRepo(t, overlayRoot)
	repo.paths = map[string]bool{"pi": true}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := (pipkg.Packages{Registries: fileRegistries, Source: &fakeRepo{resolve: map[string]string{"HEAD": fakeTip}, tagErr: errors.New("none")}}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, err := (pipkg.Packages{Registries: fileRegistries, Source: repo}).Compare(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got := repo.asked("Export"); len(got) != 1 || got[0] != "Export main skills,agents,skills.registry.yaml,pi" {
		t.Errorf("the export asked %v, want the pi path too", got)
	}
}

func TestCompareTriesTheRefsInOrderAndTheOverrideFirst(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	repo := deployingRepo(t, overlayRoot)
	repo.commits = map[string]bool{"HEAD": true} // no main, no origin/main
	repo.resolve = map[string]string{"HEAD": fakeTip}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := (pipkg.Packages{Registries: fileRegistries, Source: &fakeRepo{resolve: map[string]string{"HEAD": fakeTip}, tagErr: errors.New("none")}}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	report, err := (pipkg.Packages{Registries: fileRegistries, Source: repo, Options: pipkg.Options{DeployRef: " release "}}).Compare(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got := strings.Join(repo.asked("HasCommit"), "|"); got != "HasCommit release|HasCommit main|HasCommit origin/main|HasCommit HEAD" {
		t.Errorf("the refs were tried as %q, want the override (trimmed) first, then main, origin/main, HEAD", got)
	}
	if report.DeployRef != "HEAD" {
		t.Errorf("deploy ref = %q, want HEAD, the first that names a commit", report.DeployRef)
	}
}

func TestCompareReportsAPackageBehindTheDeployRefAsStale(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	old := "fedcba9876543210fedcba9876543210fedcba98"
	repo := deployingRepo(t, overlayRoot)
	repo.ancestor = true
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := (pipkg.Packages{Registries: fileRegistries, Source: &fakeRepo{resolve: map[string]string{"HEAD": old}, tagErr: errors.New("none")}}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	report, err := (pipkg.Packages{Registries: fileRegistries, Source: repo}).Compare(overlayRoot, registryPath, destDir)
	if err == nil || !report.Stale {
		t.Fatalf("Compare = %+v, %v, want a stale package reported as drift", report, err)
	}
	if got := repo.asked("IsAncestor"); len(got) != 1 || got[0] != "IsAncestor "+old+" "+fakeTip {
		t.Errorf("ancestry asked %v, want the recorded commit against the tip", got)
	}
}

// What the refusals say when git cannot give the tree: the words are the ones the builder used
// when it ran git itself.
func TestCompareSaysWhyGitCouldNotGiveTheTree(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := (pipkg.Packages{Registries: fileRegistries, Source: pipkg.NoRepository{}}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cases := map[string]struct {
		mutate func(f *fakeRepo)
		want   string
	}{
		"the ref cannot be resolved": {func(f *fakeRepo) { f.resolveErr = errors.New("exit status 128") }, "pipkg: resolving main: exit status 128"},
		"git archive fails":          {func(f *fakeRepo) { f.exportErr = errors.New("git archive main: exit status 128 (fatal)") }, "pipkg: exporting main for comparison: pipkg: git archive main: exit status 128 (fatal)"},
		"the archive is not a tar": {func(f *fakeRepo) {
			f.exportBytes = func([]string) []byte { return []byte("this is not a tar archive at all, not even close to one") }
		}, "pipkg: exporting main for comparison: pipkg: extracting git archive main: "},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			repo := deployingRepo(t, overlayRoot)
			c.mutate(repo)
			_, err := (pipkg.Packages{Registries: fileRegistries, Source: repo}).Compare(overlayRoot, registryPath, destDir)
			if err == nil || !strings.HasPrefix(err.Error(), c.want) {
				t.Fatalf("Compare error = %v, want it to start with %q", err, c.want)
			}
		})
	}
}

func TestNoRepositoryAnswersNoToEverything(t *testing.T) {
	var repo pipkg.SourceRepo = pipkg.NoRepository{}
	if repo.IsWorkTree("/x") || repo.HasCommit("/x", "main") || repo.HasPath("/x", "main", "pi") || repo.IsAncestor("/x", "a", "b") {
		t.Error("a predicate answered yes")
	}
	if _, err := repo.Resolve("/x", "HEAD"); !errors.Is(err, pipkg.ErrNoRepository) {
		t.Errorf("Resolve err = %v, want ErrNoRepository", err)
	}
	if _, err := repo.HasChanges("/x", "skills"); !errors.Is(err, pipkg.ErrNoRepository) {
		t.Errorf("HasChanges err = %v, want ErrNoRepository", err)
	}
	if _, err := repo.LatestTag("/x", "", "v*"); !errors.Is(err, pipkg.ErrNoRepository) {
		t.Errorf("LatestTag err = %v, want ErrNoRepository", err)
	}
	if _, err := repo.Export("/x", "main", nil); !errors.Is(err, pipkg.ErrNoRepository) {
		t.Errorf("Export err = %v, want ErrNoRepository", err)
	}
}

// A status that git cannot give counts as a clean tree, as it always did: the build records the
// commit it could resolve rather than refusing or leaving it out.
func TestAStatusGitCannotGiveIsACleanTree(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	repo := &fakeRepo{resolve: map[string]string{"HEAD": fakeTip}, changesErr: errors.New("status failed"), changes: true, tagErr: errors.New("none")}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := (pipkg.Packages{Registries: fileRegistries, Source: repo}).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(destDir, "package.json"))
	if !strings.Contains(string(raw), `"builtFrom": "`+fakeTip+`"`) {
		t.Errorf("package.json = %s, want the commit recorded", raw)
	}
}
