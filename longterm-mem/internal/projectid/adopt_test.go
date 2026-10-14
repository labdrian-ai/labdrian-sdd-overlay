package projectid

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// These cases run AdoptWith and derivableNames over a table that stands for the repositories (see
// resolver_test.go): which names a repository derives, in what order, and which of them the memory already
// lives under are all the rules' to decide, and none of it needs a disk.

// established builds an Established backed by a fixed set of names, and records the names it was asked about.
func established(asked *[]string, names ...string) Established {
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	return func(name string) (bool, error) {
		if asked != nil {
			*asked = append(*asked, name)
		}
		return have[name], nil
	}
}

func adoptWith(t *testing.T, f *fakeRepositories, dir string, opts AdoptOptions) Adoption {
	t.Helper()
	got, err := NewResolver(f).AdoptWith(dir, opts)
	if err != nil {
		t.Fatalf("AdoptWith(%s): %v", dir, err)
	}
	return got
}

// The cheapest form of integrating fragmented memory is not creating the second pile. A repository whose remote
// normalizes to "github.com/acme/widgets" also derives the plain "widgets", and if THAT is the name the memory
// already lives under, minting the URL-shaped one is fragmentation performed by the resolver itself.
func TestAdopt_EstablishedAliasWinsOverTheChainsAnswer(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))

	got := adoptWith(t, f, mainRoot, AdoptOptions{Established: established(nil, "widgets")})

	if got.Identity.Project != "widgets" {
		t.Fatalf("an established alias must be adopted, not re-minted: got %q", got.Identity.Project)
	}
	if !got.Adopted {
		t.Fatal("Adopted must report that the identity came from storage, not from the chain")
	}
	if got.Identity.Rule != RuleRemote {
		t.Fatalf("the adopted name must still name the rule that derived it: got %q", got.Identity.Rule)
	}
	if got.Identity.WorktreeRoot != mainRoot || got.Identity.CommonDir != commonDir {
		t.Fatalf("the adopted identity lost where the repository is: %+v", got.Identity)
	}
}

// With nothing established there is nothing to integrate WITH, so the chain decides exactly as before. Adoption
// must never invent a name that no rule derives.
func TestAdopt_NothingEstablishedFallsBackToTheChain(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))

	got := adoptWith(t, f, mainRoot, AdoptOptions{Established: established(nil)})

	want := resolveWith(t, f, mainRoot)
	if got.Identity != want {
		t.Fatalf("with nothing established AdoptWith must equal Resolve: got %+v, want %+v", got.Identity, want)
	}
	if got.Adopted {
		t.Fatal("Adopted must be false when no name was established")
	}
	if len(got.PendingIntegration) != 0 {
		t.Fatalf("nothing is pending when nothing is established: got %v", got.PendingIntegration)
	}
}

// Two derivable names both holding memory is the fragmentation itself, and it is PROVABLE rather than guessed:
// both names came out of this one repository's own metadata in a single read. The higher-ranked spelling becomes
// canonical and the other is reported as owed an integration -- reported, because merging Engram's own store is
// not this module's to do (R-002 keeps its connection read-only).
func TestAdopt_OtherEstablishedAliasesArePendingIntegration(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))
	f.declarations[mainRoot] = "acme-widgets\n"

	got := adoptWith(t, f, mainRoot, AdoptOptions{Established: established(nil, "acme-widgets", "widgets", "github.com/acme/widgets")})

	if got.Identity.Project != "acme-widgets" {
		t.Fatalf("the highest-ranked established name is canonical: got %q", got.Identity.Project)
	}
	if want := []string{"github.com/acme/widgets", "widgets"}; !reflect.DeepEqual(got.PendingIntegration, want) {
		t.Fatalf("every other established derivable name is owed an integration, in rank order: got %v, want %v", got.PendingIntegration, want)
	}
}

// The property Resolve guarantees must survive adoption: a main checkout and a linked worktree adopt the same
// name, or adoption has reintroduced exactly what it exists to prevent.
func TestAdopt_MainCheckoutAndWorktreeAdoptIdentically(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))
	opts := AdoptOptions{Established: established(nil, "widgets")}

	fromMain, fromLinked := adoptWith(t, f, mainRoot, opts), adoptWith(t, f, linkedRoot, opts)

	if fromMain.Identity.Project != fromLinked.Identity.Project || fromMain.Adopted != fromLinked.Adopted {
		t.Fatalf("adoption fragmented the repository: main=%+v worktree=%+v", fromMain, fromLinked)
	}
	if !reflect.DeepEqual(fromMain.Derived, fromLinked.Derived) {
		t.Fatalf("the two derive different names: main=%v worktree=%v", fromMain.Derived, fromLinked.Derived)
	}
}

// A storage that cannot be consulted must not silently become "nothing is established" -- that answer is
// indistinguishable from a genuinely fresh repository, and acting on it mints the second pile this whole
// mechanism exists to avoid. The error is surfaced, with the name that could not be asked about, so the caller
// decides.
func TestAdopt_UnreadableStorageIsReportedNotAssumedEmpty(t *testing.T) {
	boom := errors.New("storage unavailable")
	f := widgets(originConfig("https://github.com/acme/widgets.git"))

	_, err := NewResolver(f).AdoptWith(mainRoot, AdoptOptions{Established: func(string) (bool, error) { return false, boom }})

	if !errors.Is(err, boom) {
		t.Fatalf("an unreadable storage must surface, not read as empty: got %v", err)
	}
	if want := `projectid: checking whether "github.com/acme/widgets" already holds memory: storage unavailable`; err.Error() != want {
		t.Errorf("the error is %q, want %q", err, want)
	}
}

// A repository that is in no repository, or cannot be read, is told as it is: nothing is asked of the storage.
func TestAdopt_ALocationErrorIsTheAnswerAndTheStorageIsNotAsked(t *testing.T) {
	var asked []string

	_, err := NewResolver(widgets("")).AdoptWith("/elsewhere", AdoptOptions{Established: established(&asked)})

	if !errors.Is(err, ErrNotARepository) || len(asked) != 0 {
		t.Errorf("AdoptWith = %v (storage asked about %v); want ErrNotARepository and no question", err, asked)
	}
}

// The hole derivation alone cannot close: a repository that MOVES stops deriving its old path, so memory stored
// under that path becomes unreachable -- silently, and forever. A name the repository was known by before is
// still its name, and adopting it is the reunion.
func TestAdoptWith_RemembersANameNothingDerivesAnyMore(t *testing.T) {
	f := widgets("") // no declaration, no remote: path-derived only

	got := adoptWith(t, f, mainRoot, AdoptOptions{
		Established: established(nil, "/somewhere/else/.git"),
		Remembered:  []string{"/somewhere/else/.git"},
	})

	if got.Identity.Project != "/somewhere/else/.git" {
		t.Fatalf("a remembered name that still holds memory must be adopted: got %q", got.Identity.Project)
	}
	if !got.Adopted {
		t.Fatal("Adopted must report that the identity came from storage")
	}
	if got.Identity.Rule != RuleRemembered {
		t.Fatalf("the adopted name must say it was remembered: got %q", got.Identity.Rule)
	}
}

// Remembered names rank BELOW everything the repository still derives. What it looks like now is better evidence
// than what it looked like once, and inverting that would let a stale name outrank a live declaration.
func TestAdoptWith_LiveDerivationOutranksMemory(t *testing.T) {
	f := widgets("")
	f.declarations[mainRoot] = "current-name\n"

	got := adoptWith(t, f, mainRoot, AdoptOptions{
		Established: established(nil, "current-name", "former-name"),
		Remembered:  []string{"former-name"},
	})

	if got.Identity.Project != "current-name" {
		t.Fatalf("a live declaration must outrank a remembered name: got %q", got.Identity.Project)
	}
	if want := []string{"former-name"}; !reflect.DeepEqual(got.PendingIntegration, want) {
		t.Fatalf("the remembered name still holds memory and is owed an integration: got %v", got.PendingIntegration)
	}
}

// Storage is asked about every name there is, the derived ones in rank order and then the remembered ones in the
// order they were given, and each once: a remembered name the repository also derives is not a second question,
// and an empty one is no question at all.
func TestAdoptWith_AsksAboutEveryNameOnceDerivedFirst(t *testing.T) {
	f := widgets(originConfig("https://github.com/acme/widgets.git"))
	var asked []string

	adoptWith(t, f, mainRoot, AdoptOptions{
		Established: established(&asked),
		Remembered:  []string{"older", "widgets", "", "oldest", "older"},
	})

	// "widgets" is derived twice (the origin's last segment and the common directory's own directory name) and
	// remembered once more, and asked about once.
	want := []string{"github.com/acme/widgets", "widgets", commonDir, "older", "oldest"}
	if !reflect.DeepEqual(asked, want) {
		t.Errorf("storage was asked about %v, want %v", asked, want)
	}
}

// What a repository derives is what the caller writes to its ledger, so the rule and the strictness of each name
// are part of the answer: the loose spellings (a bare directory name, the last segment of an origin) may not be
// adopted later on a ledger's word alone, because they can name somebody else's repository.
func TestDerivableNames_ListsEveryNameWithItsRuleAndItsStrictness(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration string
		config      string
		want        []DerivedName
	}{
		{"declared, remote and path", "chosen-name\n", originConfig("https://github.com/acme/widgets.git"), []DerivedName{
			{Name: "chosen-name", Rule: RuleDeclared, Strict: true},
			{Name: "github.com/acme/widgets", Rule: RuleRemote, Strict: true},
			{Name: "widgets", Rule: RuleRemote, Strict: false},
			{Name: commonDir, Rule: RuleCommonDir, Strict: true},
		}},
		{"remote and path", "", originConfig("https://github.com/acme/gadgets.git"), []DerivedName{
			{Name: "github.com/acme/gadgets", Rule: RuleRemote, Strict: true},
			{Name: "gadgets", Rule: RuleRemote, Strict: false},
			{Name: commonDir, Rule: RuleCommonDir, Strict: true},
			{Name: "widgets", Rule: RuleCommonDir, Strict: false},
		}},
		{"path alone", "", "", []DerivedName{
			{Name: commonDir, Rule: RuleCommonDir, Strict: true},
			{Name: "widgets", Rule: RuleCommonDir, Strict: false},
		}},
		{"a declared name that is also the directory's", "widgets\n", "", []DerivedName{
			{Name: "widgets", Rule: RuleDeclared, Strict: true},
			{Name: commonDir, Rule: RuleCommonDir, Strict: true},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := widgets(tc.config)
			if tc.declaration != "" {
				f.declarations[mainRoot] = tc.declaration
			}

			got := adoptWith(t, f, mainRoot, AdoptOptions{Established: established(nil)}).Derived

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Derived = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The names remembered from a ledger are not names the repository derives: they never appear among the ones the
// caller records, which would make a ledger vouch for itself.
func TestAdoptWith_RememberedNamesAreNotAmongTheDerivedOnes(t *testing.T) {
	f := widgets("")

	got := adoptWith(t, f, mainRoot, AdoptOptions{Established: established(nil), Remembered: []string{"former-name"}})

	for _, d := range got.Derived {
		if d.Name == "former-name" {
			t.Errorf("a remembered name was reported as derived: %+v", got.Derived)
		}
	}
}

// A declaration that is broken is refused here as it is by Resolve, with the same error, before storage is
// asked anything: adoption does not paper over the one file a human wrote to control the name.
func TestAdoptWith_ABrokenDeclarationIsRefusedBeforeStorageIsAsked(t *testing.T) {
	f := widgets("")
	f.declarations[mainRoot] = "one\ntwo\n"
	var asked []string

	_, err := NewResolver(f).AdoptWith(mainRoot, AdoptOptions{Established: established(&asked)})

	if !errors.Is(err, ErrDeclaredInvalid) || len(asked) != 0 {
		t.Errorf("AdoptWith = %v (storage asked about %v); want ErrDeclaredInvalid and no question", err, asked)
	}
	if !strings.Contains(err.Error(), DeclaredFileName) {
		t.Errorf("the refusal %q does not name the declaration file", err)
	}
}
