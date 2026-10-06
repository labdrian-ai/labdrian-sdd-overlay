package fsstore

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

// The adapter is the four ports of the domain, in one value.
var (
	_ reviewreceipt.TransactionStores = Store{}
	_ reviewreceipt.ReceiptSource     = Store{}
	_ reviewreceipt.ReceiptSink       = Store{}
	_ reviewreceipt.ChangeCatalog     = Store{}
)

// locator answers Locate with a fixed observation, recording what it was asked.
type locator struct {
	obs   gitprov.Observation
	err   error
	asked []string
}

func (l *locator) Locate(dir string) (gitprov.Observation, error) {
	l.asked = append(l.asked, dir)
	return l.obs, l.err
}

func newStore(t *testing.T, root string, l *locator) Store {
	t.Helper()
	s, err := New(root, l)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewRefusesWhatItCannotWorkWithout(t *testing.T) {
	if _, err := New("", &locator{}); err == nil {
		t.Error("New accepted an empty project root")
	}
	if _, err := New(t.TempDir(), nil); err == nil {
		t.Error("New accepted no locator")
	}
}

// The stores are the review-transaction store of the working tree's own git directory and
// then the one under the directory every working tree of the repository shares, when that is
// another; one store when they are the same.
func TestStoresAreTheWorkingTreesOwnThenTheSharedOne(t *testing.T) {
	root := t.TempDir()
	const rel = "gentle-ai/review-transactions/v2"
	for _, tc := range []struct {
		name string
		obs  gitprov.Observation
		want []reviewreceipt.Store
	}{
		{"a main working tree", gitprov.Observation{GitDir: "/r/.git", CommonDir: "/r/.git"},
			[]reviewreceipt.Store{reviewreceipt.Store(filepath.FromSlash("/r/.git/" + rel))}},
		{"a linked working tree", gitprov.Observation{GitDir: "/r/.git/worktrees/w", CommonDir: "/r/.git", Linked: true},
			[]reviewreceipt.Store{
				reviewreceipt.Store(filepath.FromSlash("/r/.git/worktrees/w/" + rel)),
				reviewreceipt.Store(filepath.FromSlash("/r/.git/" + rel)),
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newStore(t, root, &locator{obs: tc.obs}).Stores()
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Stores = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}

// Git is asked about the absolute root, whatever spelling the project root came in, and its
// refusal is passed on in git's words.
func TestStoresAskTheLocatorForTheAbsoluteRootAndPassItsRefusalOn(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	l := &locator{err: errors.New("git rev-parse --show-toplevel: exit status 128")}

	_, err = newStore(t, "some/relative/root", l).Stores()

	if err == nil || err.Error() != "git rev-parse --show-toplevel: exit status 128" {
		t.Errorf("Stores error = %v, want git's refusal as it is", err)
	}
	if want := []string{filepath.Join(cwd, "some", "relative", "root")}; !reflect.DeepEqual(l.asked, want) {
		t.Errorf("the locator was asked %v, want %v", l.asked, want)
	}
}

func TestDocumentsAreTheLineagesInNameOrderLegacyBeforeState(t *testing.T) {
	store := filepath.Join(t.TempDir(), "v2")
	write(t, filepath.Join(store, "review-b", "review-state.json"), "b-state")
	write(t, filepath.Join(store, "review-b", "review-receipt.json"), "b-receipt")
	write(t, filepath.Join(store, "review-a", "review-state.json"), "a-state")
	write(t, filepath.Join(store, "review-c", "other.json"), "not a document")
	write(t, filepath.Join(store, "a-plain-file"), "not a lineage")
	if err := os.MkdirAll(filepath.Join(store, "review-d"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := newStore(t, t.TempDir(), &locator{}).Documents(reviewreceipt.Store(store))
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	want := []reviewreceipt.Document{
		{Shape: reviewreceipt.ShapeState, Data: []byte("a-state"), Origin: filepath.Join(store, "review-a", "review-state.json")},
		{Shape: reviewreceipt.ShapeReceipt, Data: []byte("b-receipt"), Origin: filepath.Join(store, "review-b", "review-receipt.json")},
		{Shape: reviewreceipt.ShapeState, Data: []byte("b-state"), Origin: filepath.Join(store, "review-b", "review-state.json")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Documents = %q, want %q", got, want)
	}
}

func TestAStoreThatDoesNotExistHoldsNothing(t *testing.T) {
	got, err := newStore(t, t.TempDir(), &locator{}).Documents(reviewreceipt.Store(filepath.Join(t.TempDir(), "absent")))
	if err != nil || got != nil {
		t.Errorf("Documents of an absent store = %v, %v, want nothing and no error", got, err)
	}
}

// A store that is there and cannot be read is an error that names it.
func TestAStoreThatCannotBeReadIsAnErrorThatNamesIt(t *testing.T) {
	dir := t.TempDir()
	notADirectory := filepath.Join(dir, "v2")
	write(t, notADirectory, "a file where the store should be")

	_, err := newStore(t, dir, &locator{}).Documents(reviewreceipt.Store(notADirectory))
	if err == nil || !strings.HasPrefix(err.Error(), "read "+notADirectory+": ") {
		t.Errorf("Documents of a file = %v, want an error that names it", err)
	}
}

// Only a missing document is absent. A document that is there and cannot be read is an error
// that names it, never a silent absence: the hook would otherwise count fewer approved
// receipts than exist, allow the acknowledgement, and lose a receipt with nothing said.
func TestADocumentThatIsThereAndCannotBeReadIsAnErrorThatNamesIt(t *testing.T) {
	t.Run("a directory where a document should be", func(t *testing.T) {
		dir := t.TempDir()
		store := filepath.Join(dir, "store")
		doc := filepath.Join(store, "review-a", "review-receipt.json")
		if err := os.MkdirAll(doc, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := newStore(t, dir, &locator{}).Documents(reviewreceipt.Store(store))
		if err == nil || !strings.HasPrefix(err.Error(), "read "+doc+": ") || strings.Count(err.Error(), doc) != 1 || got != nil {
			t.Errorf("Documents = %v, %v, want no documents and an error that names %s", got, err, doc)
		}
	})
	t.Run("a document without permissions", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("a file without permissions does not stop root")
		}
		dir := t.TempDir()
		store := filepath.Join(dir, "store")
		secret := filepath.Join(store, "review-a", "review-receipt.json")
		write(t, secret, "unreadable")
		write(t, filepath.Join(store, "review-a", "review-state.json"), "readable")
		if err := os.Chmod(secret, 0); err != nil {
			t.Fatal(err)
		}
		got, err := newStore(t, dir, &locator{}).Documents(reviewreceipt.Store(store))
		if err == nil || !strings.HasPrefix(err.Error(), "read "+secret+": ") || strings.Count(err.Error(), secret) != 1 || got != nil {
			t.Errorf("Documents = %v, %v, want no documents and an error that names %s", got, err, secret)
		}
	})
}

func TestLocationIsUnderTheChangesReviewReceiptsOfTheRootAsGiven(t *testing.T) {
	s := newStore(t, "rel/root", &locator{})
	want := filepath.Join("rel", "root", "openspec", "changes", "my-change", "review-receipts", "review-a.json")
	if got := s.Location("my-change", "review-a.json"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// Where openspec/ is looked for is the toplevel the locator reports when it is another
// directory than the one given and has openspec/changes, and the directory as it was given
// otherwise: when it is the toplevel itself (so the paths printed keep the form they always
// had), when the locator refuses or says nothing, and when the toplevel has none. The locator
// is asked once, however many ports are used.
func TestOpenspecIsLookedForFromTheToplevelTheLocatorReports(t *testing.T) {
	top := t.TempDir()
	sub := filepath.Join(top, "sub")
	write(t, filepath.Join(top, "openspec", "changes", "top-change", "tasks.md"), "x")
	write(t, filepath.Join(sub, "openspec", "changes", "sub-change", "tasks.md"), "x")
	bareTop := t.TempDir()
	bareSub := filepath.Join(bareTop, "sub")
	write(t, filepath.Join(bareSub, "openspec", "changes", "sub-change", "tasks.md"), "x")

	for _, tc := range []struct {
		name       string
		root       string
		loc        *locator
		wantRoot   string
		wantChange string
	}{
		{"a subdirectory of a toplevel with openspec", sub, &locator{obs: gitprov.Observation{Toplevel: top}}, top, "top-change"},
		{"the toplevel itself keeps the form it was given", top, &locator{obs: gitprov.Observation{Toplevel: top}}, top, "top-change"},
		{"a toplevel with no openspec falls back to the directory", bareSub, &locator{obs: gitprov.Observation{Toplevel: bareTop}}, bareSub, "sub-change"},
		{"a locator that refuses falls back to the directory", sub, &locator{err: errors.New("not a repository")}, sub, "sub-change"},
		{"a locator that reports no toplevel falls back to the directory", sub, &locator{}, sub, "sub-change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t, tc.root, tc.loc)
			changes, err := s.Changes()
			if err != nil || !reflect.DeepEqual(changes, []string{tc.wantChange}) {
				t.Fatalf("Changes = %v, %v, want [%s]", changes, err, tc.wantChange)
			}
			if !s.HasArtifact(tc.wantChange, "tasks.md") {
				t.Error("HasArtifact does not find the change's tasks.md under the same root")
			}
			want := filepath.Join(tc.wantRoot, "openspec", "changes", tc.wantChange, "review-receipts", "review-a.json")
			if got := s.Location(tc.wantChange, "review-a.json"); got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			if err := s.Write(tc.wantChange, "review-a.json", []byte("{}")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(want); err != nil {
				t.Errorf("Write did not put the receipt where Location says: %v", err)
			}
			if len(tc.loc.asked) > 1 {
				t.Errorf("the locator was asked %d times, want at most once", len(tc.loc.asked))
			}
		})
	}
}

func TestReadReturnsWhatWasPersistedAndSaysWhenNothingWas(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})
	write(t, s.Location("c", "review-a.json"), "persisted")

	got, err := s.Read("c", "review-a.json")
	if err != nil || string(got) != "persisted" {
		t.Errorf("Read = %q, %v", got, err)
	}

	_, err = s.Read("c", "review-b.json")
	if !errors.Is(err, reviewreceipt.ErrNotPersisted) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read of an absent receipt = %v, want ErrNotPersisted (and the file system's own)", err)
	}
	if want := "read " + s.Location("c", "review-b.json") + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Read error = %q, want it to start with %q", err, want)
	}

	if err := os.MkdirAll(s.Location("c", "a-directory.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read("c", "a-directory.json"); err == nil || errors.Is(err, reviewreceipt.ErrNotPersisted) {
		t.Errorf("Read of a directory = %v, want an error that is not 'not persisted'", err)
	}
}

// Write puts the exact bytes under the name, in a folder it creates, readable only by the
// owner (as the receipts have always been), and leaves nothing else behind.
func TestWritePersistsTheExactBytesPrivatelyAndLeavesNothingElse(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})
	data := []byte("{\"exact\": \"bytes\"}\r\n\x00")

	if err := s.Write("my-change", "review-a.review-state.json", data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	dest := s.Location("my-change", "review-a.review-state.json")
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(data) {
		t.Errorf("persisted %q, %v, want %q", got, err, data)
	}
	if info, err := os.Stat(dest); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode of the persisted receipt = %v, %v, want 0600", info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil || len(entries) != 1 {
		t.Errorf("the folder holds %v, %v, want only the receipt", entries, err)
	}
}

func TestWriteFailsInTheWordsOfTheStepThatFailedAndLeavesNoTemporaryFile(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})

	// A file where the change folder should be: the folder cannot be created.
	write(t, filepath.Join(root, "openspec", "changes", "blocked"), "a file")
	err := s.Write("blocked", "review-a.json", []byte("x"))
	if wantDir := filepath.Join(root, "openspec", "changes", "blocked", "review-receipts"); err == nil || !strings.HasPrefix(err.Error(), "create "+wantDir+": ") {
		t.Errorf("Write under a file = %v, want an error that names the folder it could not create", err)
	}

	if os.Geteuid() == 0 {
		return
	}
	// A folder that refuses a new file.
	dir := filepath.Dir(s.Location("closed", "x"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if err := s.Write("closed", "review-a.json", []byte("x")); err == nil || !strings.HasPrefix(err.Error(), "atomicfile: create temporary file: ") {
		t.Errorf("Write into a closed folder = %v, want the words of the write that failed", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a failed Write left %v behind", entries)
	}
}

// The changes are the directories under openspec/changes, by name; files and the like are not
// changes. A project with no such folder has none, and says so without an error.
func TestChangesAreTheDirectoriesUnderOpenspecChanges(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})

	if got, err := s.Changes(); err != nil || got != nil {
		t.Errorf("Changes of a project with no openspec folder = %v, %v, want none", got, err)
	}

	base := filepath.Join(root, "openspec", "changes")
	for _, d := range []string{"zeta", "alpha", "archive"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(base, "a-file"), "not a directory")
	if err := os.Symlink(filepath.Join(base, "alpha"), filepath.Join(base, "a-link")); err != nil {
		t.Logf("symlinks unavailable: %v", err)
	}

	got, err := s.Changes()
	if want := []string{"alpha", "archive", "zeta"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Changes = %v, %v, want %v", got, err, want)
	}
}

func TestChangesReportsAFolderThatCannotBeRead(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes"), "a file where the folder should be")
	_, err := newStore(t, root, &locator{}).Changes()
	if want := "read " + filepath.Join(root, "openspec", "changes") + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Changes = %v, want an error that starts with %q", err, want)
	}
}

func TestHasArtifactFindsAFileOrADirectoryByName(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})
	write(t, filepath.Join(root, "openspec", "changes", "c", "tasks.md"), "# tasks")
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "c", "design.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]bool{"tasks.md": true, "design.md": true, "proposal.md": false} {
		if got := s.HasArtifact("c", name); got != want {
			t.Errorf("HasArtifact(c, %s) = %v, want %v", name, got, want)
		}
	}
	if s.HasArtifact("absent", "tasks.md") {
		t.Error("HasArtifact found an artifact of a change that is not there")
	}
}

// Git is reached through the locator the composition root hands in, which is gitprov in
// production: no file of this package starts a process, so there is no second way of asking
// git where a repository is.
func TestThePackageStartsNoProcess(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("found no source files: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			if path := strings.Trim(imp.Path.Value, `"`); path == "os/exec" || path == "syscall" {
				t.Errorf("%s imports %s: git is reached through gitprov, and the file system through the shared primitives", file, path)
			}
		}
	}
}

// Only an openspec/changes that does not exist at the toplevel lets the store serve the directory
// it was given. Any other answer of the file system (here, no permission to look inside openspec/)
// is not an absence: the receipts could be at the toplevel, and serving the directory below would
// put them somewhere else. The ports that can say so refuse, naming the path they could not look
// at, and the hook, which is fail-closed, denies (service_test.go and the golden files).
func TestAToplevelOpenspecThatCannotBeLookedAtIsAnErrorAndNotAnAbsence(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a directory without permissions does not stop root")
	}
	top := t.TempDir()
	sub := filepath.Join(top, "sub")
	write(t, filepath.Join(sub, "openspec", "changes", "sub-change", "tasks.md"), "x")
	closed := filepath.Join(top, "openspec")
	if err := os.MkdirAll(closed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(closed, 0o755) })
	loc := &locator{obs: gitprov.Observation{Toplevel: top}}
	s := newStore(t, sub, loc)

	wantStat := "stat " + filepath.Join(top, "openspec", "changes") + ": "
	if changes, err := s.Changes(); err == nil || !strings.HasPrefix(err.Error(), wantStat) || changes != nil {
		t.Errorf("Changes = %v, %v, want no changes and an error that starts %q", changes, err, wantStat)
	}
	if _, err := s.Read("sub-change", "review-a.json"); err == nil || !strings.HasPrefix(err.Error(), wantStat) || errors.Is(err, reviewreceipt.ErrNotPersisted) {
		t.Errorf("Read = %v, want the error of the stat, which is not an absence of the receipt", err)
	}
	if err := s.Write("sub-change", "review-a.json", []byte("{}")); err == nil || !strings.HasPrefix(err.Error(), wantStat) {
		t.Errorf("Write = %v, want the error of the stat", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "openspec", "changes", "sub-change", "review-receipts")); !os.IsNotExist(err) {
		t.Errorf("Write put a receipt in the directory below while refusing (stat err=%v)", err)
	}
	if s.HasArtifact("sub-change", "tasks.md") {
		t.Error("HasArtifact found an artifact of the directory below although the toplevel could not be looked at")
	}
	if len(loc.asked) > 1 {
		t.Errorf("the locator was asked %d times, want at most once", len(loc.asked))
	}
}

// Where openspec/ is looked for is decided once for a store, by what the file system held when it
// was first asked: a change folder that appears at the toplevel afterwards does not move a store
// that has already answered from the directory it was given, and a store built afterwards sees it.
// A hook is one short process, so the decision cannot go stale in it, and every port of one
// invocation answers from the same place; this pins that, since the comment of the store says it.
func TestWhereOpenspecIsLookedForIsDecidedOnceForAStore(t *testing.T) {
	top := t.TempDir()
	sub := filepath.Join(top, "sub")
	write(t, filepath.Join(sub, "openspec", "changes", "sub-change", "tasks.md"), "x")
	loc := &locator{obs: gitprov.Observation{Toplevel: top}}
	first := newStore(t, sub, loc)

	if changes, err := first.Changes(); err != nil || !reflect.DeepEqual(changes, []string{"sub-change"}) {
		t.Fatalf("before the toplevel has a change: Changes = %v, %v, want [sub-change]", changes, err)
	}
	write(t, filepath.Join(top, "openspec", "changes", "top-change", "tasks.md"), "x")

	if changes, err := first.Changes(); err != nil || !reflect.DeepEqual(changes, []string{"sub-change"}) {
		t.Errorf("after the toplevel got a change: Changes = %v, %v, want the store to answer from where it first did", changes, err)
	}
	if changes, err := newStore(t, sub, loc).Changes(); err != nil || !reflect.DeepEqual(changes, []string{"top-change"}) {
		t.Errorf("a store built after: Changes = %v, %v, want [top-change]", changes, err)
	}
}
