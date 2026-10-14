package projectid

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These cases run the rules over a table that stands for the repositories (fakeRepositories): nothing here
// reads a disk or runs git, so each says what the RULES do with what the inspector tells them. What a real
// repository looks like to the inspector, and that a main checkout and a linked worktree look alike to it, is
// internal/projectidfs's to show.

func resolveWith(t *testing.T, f *fakeRepositories, dir string) Identity {
	t.Helper()
	id, err := NewResolver(f).Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): %v", dir, err)
	}
	return id
}

// The chain is declared, then the normalized origin, then the common directory, and the first link that answers
// wins: a link that answers is never overruled by one below it.
func TestResolve_ChainOrderIsDeclaredThenRemoteThenCommonDir(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration string
		config      string
		wantProject string
		wantRule    Rule
	}{
		{"all three answer", "chosen-name\n", originConfig("https://github.com/acme/widgets.git"), "chosen-name", RuleDeclared},
		{"no declaration", "", originConfig("https://github.com/acme/widgets.git"), "github.com/acme/widgets", RuleRemote},
		{"no declaration and no origin", "", "[core]\n\tbare = false\n", commonDir, RuleCommonDir},
		{"no declaration and no configuration", "", "", commonDir, RuleCommonDir},
		{"a declaration and no origin", "chosen-name\n", "", "chosen-name", RuleDeclared},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := widgets(tc.config)
			if tc.declaration != "" {
				f.declarations[mainRoot] = tc.declaration
			}

			id := resolveWith(t, f, mainRoot)

			if id.Project != tc.wantProject || id.Rule != tc.wantRule {
				t.Errorf("Resolve = %q via %q, want %q via %q", id.Project, id.Rule, tc.wantProject, tc.wantRule)
			}
		})
	}
}

// An identity carries where the working tree is and which common directory the repository shares, and neither is
// part of what names the project.
func TestResolve_IdentityCarriesTheWorktreeRootAndTheCommonDir(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))

	got := resolveWith(t, f, linkedRoot)

	want := Identity{Project: "github.com/acme/widgets", Rule: RuleRemote, WorktreeRoot: linkedRoot, CommonDir: commonDir}
	if got != want {
		t.Errorf("Resolve = %+v, want %+v", got, want)
	}
}

// THE PROPERTY, at every level of the chain: the main checkout and a linked worktree of the same repository
// resolve to the same project by the same rule, wherever the declaration is read from.
func TestResolve_AMainCheckoutAndALinkedWorktreeAgreeAtEveryLevel(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration string
		config      string
	}{
		{"declared", "chosen-name\n", originConfig("https://github.com/acme/widgets.git")},
		{"remote", "", originConfig("https://github.com/acme/widgets.git")},
		{"common dir", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := widgets(tc.config)
			if tc.declaration != "" {
				f.declarations[mainRoot] = tc.declaration
			}

			fromMain, fromLinked := resolveWith(t, f, mainRoot), resolveWith(t, f, linkedRoot)

			if fromMain.Project != fromLinked.Project || fromMain.Rule != fromLinked.Rule {
				t.Errorf("main checkout %q via %q, linked worktree %q via %q: one repository, two identities",
					fromMain.Project, fromMain.Rule, fromLinked.Project, fromLinked.Rule)
			}
			if fromMain.WorktreeRoot == fromLinked.WorktreeRoot {
				t.Errorf("both answer worktree root %q: the fixture did not tell them apart", fromMain.WorktreeRoot)
			}
		})
	}
}

// The declaration is read from the main checkout's root and from nowhere else: a file on a linked worktree is
// never looked for, because a declaration per worktree IS the fragmentation this package exists to prevent.
func TestResolve_TheDeclarationIsReadFromTheMainCheckoutOnly(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))
	f.declarations[linkedRoot] = "worktree-only\n"

	got := resolveWith(t, f, linkedRoot)

	if got.Rule != RuleRemote {
		t.Errorf("a declaration that exists only on the linked worktree decided the identity: %q via %q", got.Project, got.Rule)
	}
	if want := []string{mainRoot}; !reflect.DeepEqual(f.declared, want) {
		t.Errorf("the declaration was read from %v, want %v", f.declared, want)
	}
}

// A repository whose main checkout the inspector cannot name (it has none) is read where it stands: the
// worktree root is the one place there is.
func TestResolve_AMainCheckoutThatCannotBeNamedFallsBackToTheWorktreeRoot(t *testing.T) {
	bare := Repository{WorktreeRoot: linkedRoot, CommonDir: commonDir}
	f := &fakeRepositories{repos: map[string]Repository{linkedRoot: bare}, declarations: map[string]string{linkedRoot: "chosen-name\n"}}

	got := resolveWith(t, f, linkedRoot)

	if got.Project != "chosen-name" || got.Rule != RuleDeclared {
		t.Errorf("Resolve = %q via %q, want chosen-name via declared", got.Project, got.Rule)
	}
	if want := []string{linkedRoot}; !reflect.DeepEqual(f.declared, want) {
		t.Errorf("the declaration was read from %v, want %v", f.declared, want)
	}
}

// A declaration is one non-empty line; around it, white space is nothing. Anything else is refused by name, with
// the file it was read from, and never skipped: a broken declaration hides the one thing the human wrote it to
// control.
func TestResolve_ADeclarationThatIsNotOneNonEmptyLineIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"empty", "", ""},
		{"white space", " \t\n", ""},
		{"two lines", "one\ntwo\n", ""},
		{"a name and a blank line and another", "one\n\ntwo", ""},
		{"a carriage return inside", "one\rtwo", ""},
		{"a name with white space around it", "  chosen-name \r\n", "chosen-name"},
		{"a name with a space in it", "chosen name", "chosen name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := widgets(originConfig("https://github.com/acme/widgets.git"))
			f.declarations[mainRoot] = tc.text

			id, err := NewResolver(f).Resolve(mainRoot)

			if tc.want != "" {
				if err != nil || id.Project != tc.want || id.Rule != RuleDeclared {
					t.Errorf("Resolve = %+v, %v; want %q via declared", id, err, tc.want)
				}
				return
			}
			if !errors.Is(err, ErrDeclaredInvalid) || id != (Identity{}) {
				t.Fatalf("Resolve = %+v, %v; want ErrDeclaredInvalid and no identity", id, err)
			}
			if want := filepath.Join(mainRoot, DeclaredFileName); !strings.HasSuffix(err.Error(), ": "+want) {
				t.Errorf("the refusal is %q, want it to name %s", err, want)
			}
		})
	}
}

// A declaration that is there and cannot be read is the answer: the chain does not fall through to a derived
// rule behind the human's back.
func TestResolve_ADeclarationThatCannotBeReadIsTheAnswer(t *testing.T) {
	boom := errors.New("projectid: reading the declaration: denied")
	f := widgets(originConfig("https://github.com/acme/widgets.git"))
	f.declarationErr = boom

	id, err := NewResolver(f).Resolve(mainRoot)

	if !errors.Is(err, boom) || id != (Identity{}) {
		t.Errorf("Resolve = %+v, %v; want the inspector's error and no identity", id, err)
	}
	if len(f.configRead) != 0 {
		t.Errorf("the configuration was read (%v) after the declaration failed", f.configRead)
	}
}

// Where the directory is no repository, or the repository cannot be read, the inspector's error is the answer
// as it was given, and nothing else is asked.
func TestResolve_AnErrorOfTheInspectorIsTheAnswer(t *testing.T) {
	for _, err := range []error{ErrNotARepository, errors.New("projectid: inspecting /x/.git: denied")} {
		f := widgets("")
		f.locateErr = err

		id, got := NewResolver(f).Resolve(mainRoot)

		if got != err || id != (Identity{}) {
			t.Errorf("Resolve = %+v, %v; want %v and no identity", id, got, err)
		}
		if len(f.declared) != 0 || len(f.configRead) != 0 {
			t.Errorf("declaration %v and configuration %v were read of a repository that could not be located", f.declared, f.configRead)
		}
	}
}

// The directory is handed to the inspector as it is given, with nothing made of it here: making it absolute and
// symlink-free is the inspector's, and the rules do not second-guess the Repository it answers.
func TestResolve_AsksTheInspectorForTheDirectoryAsGiven(t *testing.T) {
	f := widgets("")

	_, err := NewResolver(f).Resolve("./relative/../dir")

	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Resolve = %v, want the fixture to know no such directory", err)
	}
	if want := []string{"./relative/../dir"}; !reflect.DeepEqual(f.located, want) {
		t.Errorf("the inspector was asked for %v, want %v", f.located, want)
	}
}

// What the origin means is the identity module's rule (it is tested on its own vectors there and, through a real
// repository, in internal/projectidfs): here, that the rules hand it the text of the repository's configuration
// and use what it answers, and that a configuration with no origin to key on is no remote.
func TestResolve_ReadsTheOriginFromTheTextOfTheConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		want   string
	}{
		{"an https origin", originConfig("https://GitHub.com/acme/widgets.git"), "github.com/acme/widgets"},
		{"an origin in the scp form", originConfig("git@example.com:acme/widgets.git"), "example.com/acme/widgets"},
		{"an origin that is a path on this machine", originConfig("/srv/git/widgets.git"), commonDir},
		{"a remote that is not origin", "[remote \"upstream\"]\n\turl = https://github.com/acme/widgets.git\n", commonDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := widgets(tc.config)

			id := resolveWith(t, f, mainRoot)

			if id.Project != tc.want {
				t.Errorf("Resolve = %q (via %q), want %q", id.Project, id.Rule, tc.want)
			}
			if want := []string{commonDir}; !reflect.DeepEqual(f.configRead, want) {
				t.Errorf("the configuration was read of %v, want %v", f.configRead, want)
			}
		})
	}
}

// CheckCorrespondence is looser than equality, on purpose: two of the three rules answer something no operator
// types, so the repository's directory name and the origin's last segment count as naming it.
func TestCheckCorrespondence_NamesThatCountAsNamingTheProject(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration string
		config      string
		given       string
		want        bool
	}{
		{"the declared name", "chosen-name\n", "", "chosen-name", true},
		{"another name than the declared one", "chosen-name\n", "", "widgets", false},
		{"the whole normalized origin", "", originConfig("https://github.com/acme/widgets.git"), "github.com/acme/widgets", true},
		{"the last segment of the origin", "", originConfig("https://github.com/acme/widgets.git"), "widgets", true},
		{"white space around a name", "", originConfig("https://github.com/acme/widgets.git"), "  widgets \n", true},
		{"a segment that is not the last", "", originConfig("https://github.com/acme/widgets.git"), "acme", false},
		{"the directory of a repository with no remote", "", "", "widgets", true},
		{"the common directory itself", "", "", commonDir, true},
		{"another name than the directory", "", "", "gadgets", false},
		{"nothing at all", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := widgets(tc.config)
			if tc.declaration != "" {
				f.declarations[mainRoot] = tc.declaration
			}

			c, err := NewResolver(f).CheckCorrespondence(mainRoot, tc.given)

			if err != nil {
				t.Fatalf("CheckCorrespondence: %v", err)
			}
			if c.Match != tc.want {
				t.Errorf("CheckCorrespondence(%q).Match = %v, want %v (canonical %q via %q)", tc.given, c.Match, tc.want, c.Canonical.Project, c.Canonical.Rule)
			}
			if c.Given != strings.TrimSpace(tc.given) {
				t.Errorf("Given = %q, want the name trimmed: %q", c.Given, strings.TrimSpace(tc.given))
			}
		})
	}
}

// An empty name corresponds to nothing, not even to an identity that is empty itself: a project called "" is the
// failure the named errors of this package exist to prevent, and it must not read as a match.
func TestCheckCorrespondence_AnEmptyNameNeverCorresponds(t *testing.T) {
	nowhere := &fakeRepositories{repos: map[string]Repository{mainRoot: {WorktreeRoot: mainRoot, MainWorktreeRoot: mainRoot}}}

	c, err := NewResolver(nowhere).CheckCorrespondence(mainRoot, "")

	if err != nil {
		t.Fatalf("CheckCorrespondence: %v", err)
	}
	if c.Canonical.Project != "" {
		t.Fatalf("the fixture was meant to resolve to an empty project, got %q", c.Canonical.Project)
	}
	if c.Match {
		t.Error("an empty name corresponded to an empty identity")
	}
}

// Where the directory is no repository there is nothing to check; the caller is told so by name.
func TestCheckCorrespondence_OutsideARepositoryIsNamedNotAMismatch(t *testing.T) {
	_, err := NewResolver(widgets("")).CheckCorrespondence(filepath.FromSlash("/elsewhere"), "anything")

	if !errors.Is(err, ErrNotARepository) {
		t.Errorf("CheckCorrespondence = %v, want ErrNotARepository", err)
	}
}

// A mismatch warns, in words that name both the project given and the one the directory resolves to and by what
// rule; a match says nothing.
func TestCorrespondence_WarningNamesBothProjectsAndSaysNothingOnAMatch(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))
	r := NewResolver(f)

	mismatch, err := r.CheckCorrespondence(mainRoot, "somebody-elses")
	if err != nil {
		t.Fatal(err)
	}
	match, err := r.CheckCorrespondence(mainRoot, "widgets")
	if err != nil {
		t.Fatal(err)
	}

	want := `--project somebody-elses does not correspond to this directory's project, which resolves to "github.com/acme/widgets" (via the remote rule); ` +
		"proceeding as asked, but if this was not deliberate the memory you write lands in a project that will never be searched back"
	if got := mismatch.Warning(); got != want {
		t.Errorf("Warning = %q, want %q", got, want)
	}
	if got := match.Warning(); got != "" {
		t.Errorf("a corresponding project warned: %q", got)
	}
}

// CommonDir is the directory the repository shares across its worktrees, as the inspector gave it, and the
// inspector's error otherwise.
func TestCommonDir_IsTheOneTheInspectorGave(t *testing.T) {
	r := NewResolver(widgets(""))

	for _, dir := range []string{mainRoot, linkedRoot} {
		if got, err := r.CommonDir(dir); err != nil || got != commonDir {
			t.Errorf("CommonDir(%s) = %q, %v; want %q", dir, got, err, commonDir)
		}
	}
	if got, err := r.CommonDir(filepath.FromSlash("/elsewhere")); !errors.Is(err, ErrNotARepository) || got != "" {
		t.Errorf("CommonDir of a directory in no repository = %q, %v; want ErrNotARepository", got, err)
	}
}
