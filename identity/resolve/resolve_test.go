package resolve_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// fakeRepositories answers the port from a table, from no disk, and records every question so that
// a test can say what was asked and what was not.
type fakeRepositories struct {
	repos        map[string]resolve.Repository
	locateErr    error
	declarations map[string]string
	declaredErr  error
	configs      map[string]string
	configErr    error

	located, declared, configRead []string
}

func (f *fakeRepositories) Locate(dir string) (resolve.Repository, error) {
	f.located = append(f.located, dir)
	if f.locateErr != nil {
		return resolve.Repository{}, f.locateErr
	}
	repo, ok := f.repos[dir]
	if !ok {
		return resolve.Repository{}, resolve.ErrNotARepository
	}
	return repo, nil
}

func (f *fakeRepositories) DeclaredProject(root string) (string, bool, error) {
	f.declared = append(f.declared, root)
	if f.declaredErr != nil {
		return "", false, f.declaredErr
	}
	text, ok := f.declarations[root]
	return text, ok, nil
}

func (f *fakeRepositories) GitConfig(commonDir string) (string, error) {
	f.configRead = append(f.configRead, commonDir)
	if f.configErr != nil {
		return "", f.configErr
	}
	return f.configs[commonDir], nil
}

func originConfig(url string) string {
	return "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + url + "\n"
}

var (
	mainRoot   = filepath.FromSlash("/src/widgets")
	linkedRoot = filepath.FromSlash("/src/widgets-feature")
	below      = filepath.FromSlash("/src/widgets/a/b")
	commonDir  = filepath.FromSlash("/src/widgets/.git")
	elsewhere  = filepath.FromSlash("/tmp/scratch-dir")
)

// widgets is a main checkout at /src/widgets, a linked worktree of it at /src/widgets-feature, and a
// directory below the main root, all of one common directory, with the given config.
func widgets(config string) *fakeRepositories {
	main := resolve.Repository{WorktreeRoot: mainRoot, MainWorktreeRoot: mainRoot, CommonDir: commonDir}
	linked := resolve.Repository{WorktreeRoot: linkedRoot, MainWorktreeRoot: mainRoot, CommonDir: commonDir}
	f := &fakeRepositories{
		repos:        map[string]resolve.Repository{mainRoot: main, linkedRoot: linked, below: main},
		declarations: map[string]string{},
		configs:      map[string]string{},
	}
	if config != "" {
		f.configs[commonDir] = config
	}
	return f
}

func resolveWith(t *testing.T, f *fakeRepositories, dir, explicit string) resolve.Identity {
	t.Helper()
	id, err := resolve.New(f).Resolve(dir, explicit)
	if err != nil {
		t.Fatalf("Resolve(%q, %q): %v", dir, explicit, err)
	}
	return id
}

// The owner's order (C23, decision 1 of M2), first match wins: the explicit id, the declared file
// at the main checkout's root, the normalized origin, the name of the main checkout's root.
func TestTheChainIsExplicitThenDeclaredThenOriginThenTheMainCheckoutsName(t *testing.T) {
	for _, tt := range []struct {
		name       string
		explicit   string
		declared   string
		config     string
		dir        string
		want       string
		wantSource resolve.Source
	}{
		{"the explicit id beats everything", "given", "declared-name", originConfig("git@github.com:acme/widgets.git"), mainRoot, "given", resolve.SourceExplicit},
		{"the declared file beats the origin", "", "declared-name\n", originConfig("git@github.com:acme/widgets.git"), mainRoot, "declared-name", resolve.SourceDeclared},
		{"the origin beats the directory", "", "", originConfig("git@github.com:acme/widgets.git"), mainRoot, "github.com/acme/widgets", resolve.SourceRemote},
		{"an origin with no host falls through to the directory", "", "", originConfig("/srv/git/widgets.git"), mainRoot, "widgets", resolve.SourceDirectory},
		{"no origin is the name of the main checkout's root", "", "", "", mainRoot, "widgets", resolve.SourceDirectory},
		{"below the root it is still the main checkout's name", "", "", "", below, "widgets", resolve.SourceDirectory},
		{"a linked worktree is the main checkout's name, not its own", "", "", "", linkedRoot, "widgets", resolve.SourceDirectory},
		{"a linked worktree reads the declaration of the main checkout", "", "declared-name", originConfig("git@github.com:acme/widgets.git"), linkedRoot, "declared-name", resolve.SourceDeclared},
		{"a linked worktree reads the origin of the common directory", "", "", originConfig("https://github.com/acme/widgets"), linkedRoot, "github.com/acme/widgets", resolve.SourceRemote},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := widgets(tt.config)
			if tt.declared != "" {
				f.declarations[mainRoot] = tt.declared
			}
			id := resolveWith(t, f, tt.dir, tt.explicit)
			if id.Project != tt.want || id.Source != tt.wantSource {
				t.Errorf("Resolve = %q via %s, want %q via %s", id.Project, id.Source, tt.want, tt.wantSource)
			}
		})
	}
}

// An explicit id is the person naming the project: the repository is not looked at, so a
// repository that cannot be read is no obstacle.
func TestAnExplicitIdLooksAtNothing(t *testing.T) {
	f := widgets("")
	f.locateErr = errors.New("cannot look")
	id := resolveWith(t, f, mainRoot, "given")
	if id.Project != "given" || id.Source != resolve.SourceExplicit || id.InRepository {
		t.Errorf("Resolve = %+v, want the explicit id and no repository", id)
	}
	if len(f.located)+len(f.declared)+len(f.configRead) != 0 {
		t.Errorf("asked located=%v declared=%v config=%v, want nothing asked", f.located, f.declared, f.configRead)
	}
}

// Outside a git repository, with no explicit id, the project is the name of the directory asked
// about (C23, decision 4 of M2): no error, and no declaration is looked for.
func TestOutsideARepositoryTheProjectIsTheDirectorysName(t *testing.T) {
	f := widgets("")
	id := resolveWith(t, f, elsewhere, "")
	want := resolve.Identity{Project: "scratch-dir", Source: resolve.SourceDirectory}
	if !reflect.DeepEqual(id, want) {
		t.Errorf("Resolve = %+v, want %+v", id, want)
	}
	if len(f.declared) != 0 || len(f.configRead) != 0 {
		t.Errorf("asked declared=%v config=%v outside a repository, want nothing", f.declared, f.configRead)
	}
}

// What the inspector says of a directory that is in no repository is a wrapped sentinel: it is
// recognized through the wrapping.
func TestAWrappedNotARepositoryIsOutsideARepository(t *testing.T) {
	f := widgets("")
	f.locateErr = errors.Join(errors.New("walked to the root"), resolve.ErrNotARepository)
	if id := resolveWith(t, f, elsewhere, ""); id.Project != "scratch-dir" || id.Source != resolve.SourceDirectory {
		t.Errorf("Resolve = %+v, want the directory's name", id)
	}
}

func TestTheIdentityCarriesTheRepositoryItWasResolvedIn(t *testing.T) {
	id := resolveWith(t, widgets(""), linkedRoot, "")
	want := resolve.Repository{WorktreeRoot: linkedRoot, MainWorktreeRoot: mainRoot, CommonDir: commonDir}
	if !id.InRepository || id.Repository != want {
		t.Errorf("Resolve = %+v, want it in the repository %+v", id, want)
	}
}

// The declaration is a property of the repository: it is read at the main checkout's root only, so
// a file on a linked worktree's branch names nothing, and the worktree resolves as the main checkout.
func TestTheDeclarationIsReadAtTheMainCheckoutsRootOnly(t *testing.T) {
	f := widgets("")
	f.declarations[linkedRoot] = "only-in-the-worktree"
	id := resolveWith(t, f, linkedRoot, "")
	if id.Project != "widgets" || id.Source != resolve.SourceDirectory {
		t.Errorf("Resolve = %+v, want the main checkout's name", id)
	}
	if !reflect.DeepEqual(f.declared, []string{mainRoot}) {
		t.Errorf("declarations read at %v, want only %s", f.declared, mainRoot)
	}
}

// A repository whose main checkout cannot be named (a bare one) reads its declaration and takes its
// name from the worktree it was asked about.
func TestARepositoryWithNoMainCheckoutUsesTheWorktreeRoot(t *testing.T) {
	f := widgets("")
	f.repos[linkedRoot] = resolve.Repository{WorktreeRoot: linkedRoot, CommonDir: commonDir}
	id := resolveWith(t, f, linkedRoot, "")
	if id.Project != "widgets-feature" {
		t.Errorf("Resolve = %q, want the worktree's name", id.Project)
	}
	if !reflect.DeepEqual(f.declared, []string{linkedRoot}) {
		t.Errorf("declarations read at %v, want %s", f.declared, linkedRoot)
	}
}

func TestADeclarationThatIsNotOneNonEmptyLineIsRefused(t *testing.T) {
	for name, text := range map[string]string{
		"nothing":         "",
		"white space":     "  \n\t\n",
		"two lines":       "one\ntwo\n",
		"a carriage line": "one\rtwo",
	} {
		t.Run(name, func(t *testing.T) {
			f := widgets(originConfig("git@github.com:acme/widgets.git"))
			f.declarations[mainRoot] = text
			_, err := resolve.New(f).Resolve(mainRoot, "")
			if !errors.Is(err, resolve.ErrDeclaredInvalid) {
				t.Fatalf("err = %v, want ErrDeclaredInvalid", err)
			}
			if path := filepath.Join(mainRoot, resolve.DeclaredFileName); !strings.Contains(err.Error(), path) {
				t.Errorf("the error %q does not name %s", err, path)
			}
		})
	}
}

// A source that cannot tell stops the chain: falling through would name the project by something
// less than what was asked.
func TestASourceThatCannotTellStopsTheChain(t *testing.T) {
	boom := errors.New("cannot read")
	for name, build := range map[string]func(f *fakeRepositories){
		"the repository cannot be located": func(f *fakeRepositories) { f.locateErr = boom },
		"the declaration cannot be read":   func(f *fakeRepositories) { f.declaredErr = boom },
		"the config cannot be read":        func(f *fakeRepositories) { f.configErr = boom },
	} {
		t.Run(name, func(t *testing.T) {
			f := widgets(originConfig("git@github.com:acme/widgets.git"))
			build(f)
			id, err := resolve.New(f).Resolve(mainRoot, "")
			if !errors.Is(err, boom) || id != (resolve.Identity{}) {
				t.Errorf("Resolve = %+v, %v, want the error and no identity", id, err)
			}
		})
	}
}

func TestTheDeclarationIsTrimmed(t *testing.T) {
	f := widgets("")
	f.declarations[mainRoot] = "  declared-name  \n"
	if id := resolveWith(t, f, mainRoot, ""); id.Project != "declared-name" {
		t.Errorf("Resolve = %q, want the trimmed name", id.Project)
	}
}

// The rules a caller needs for the names a repository derives besides the one the chain chose.
func TestDeclaredAndRemoteAnswerForARepository(t *testing.T) {
	f := widgets(originConfig("git@github.com:acme/widgets.git"))
	f.declarations[mainRoot] = "declared-name"
	r := resolve.New(f)
	repo := f.repos[linkedRoot]
	if name, ok, err := r.Declared(repo); err != nil || !ok || name != "declared-name" {
		t.Errorf("Declared = %q, %v, %v", name, ok, err)
	}
	if name, ok, err := r.Remote(repo); err != nil || !ok || name != "github.com/acme/widgets" {
		t.Errorf("Remote = %q, %v, %v", name, ok, err)
	}
	if got := resolve.MainCheckoutName(repo); got != "widgets" {
		t.Errorf("MainCheckoutName = %q, want widgets", got)
	}
}
