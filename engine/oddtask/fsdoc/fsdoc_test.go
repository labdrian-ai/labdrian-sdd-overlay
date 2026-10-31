package fsdoc_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask/fsdoc"
)

// repo is a repository with odd/tasks/f.md holding text.
func repo(t *testing.T, text string) (root, doc string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, "odd", "tasks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc = filepath.Join(dir, "f.md")
	if err := os.WriteFile(doc, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, doc
}

func wantReadError(t *testing.T, err error, path, cause string) {
	t.Helper()
	var re *oddtask.ReadError
	if !errors.As(err, &re) {
		t.Fatalf("error = %v, want an *oddtask.ReadError", err)
	}
	if re.Path != path || re.Cause != cause {
		t.Fatalf("ReadError = {%q, %q}, want {%q, %q}", re.Path, re.Cause, path, cause)
	}
}

func TestReadReturnsTheTextOfAnAbsolutePath(t *testing.T) {
	_, doc := repo(t, "hello")
	got, err := fsdoc.Reader{}.Read(doc, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != doc || got.Text != "hello" {
		t.Fatalf("Read = %+v, want {%q hello}", got, doc)
	}
}

// recordingRoot is a Root that answers root for dir and records every dir it is asked about.
type recordingRoot struct {
	root, dir string
	asked     []string
}

func (r *recordingRoot) Root(dir string) string {
	r.asked = append(r.asked, dir)
	if dir == r.dir {
		return r.root
	}
	return ""
}

func TestReadJoinsARelativePathToTheRootOfTheWorktreeOfTheCwd(t *testing.T) {
	root, doc := repo(t, "hello")
	sub := filepath.Join(root, "engine")
	rec := &recordingRoot{root: root, dir: sub}
	got, err := fsdoc.Reader{Root: rec.Root}.Read("odd/x/../tasks/f.md", sub)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != doc || got.Text != "hello" {
		t.Fatalf("Read = %+v, want {%q hello}", got, doc)
	}
}

func TestReadRefusesARelativePathNoRepositoryHolds(t *testing.T) {
	root, _ := repo(t, "hello")
	rec := &recordingRoot{}
	for _, r := range []fsdoc.Reader{{}, {Root: rec.Root}} {
		_, err := r.Read("odd/tasks/f.md", root)
		wantReadError(t, err, "odd/tasks/f.md", oddtask.CauseNoRepository)
	}
}

func TestReadRefusesARelativePathThatLeavesTheRepository(t *testing.T) {
	root, _ := repo(t, "hello")
	rec := &recordingRoot{root: root, dir: root}
	for _, path := range []string{"../odd/tasks/f.md", "odd/../../odd/tasks/f.md"} {
		_, err := fsdoc.Reader{Root: rec.Root}.Read(path, root)
		wantReadError(t, err, path, oddtask.CauseOutsideRepository)
	}
	if len(rec.asked) != 0 {
		t.Fatalf("Root was asked about %q, want it never called", rec.asked)
	}
}

func TestReadRefusesARelativePathWithNoCwd(t *testing.T) {
	rec := &recordingRoot{}
	_, err := fsdoc.Reader{Root: rec.Root}.Read("odd/tasks/f.md", "")
	wantReadError(t, err, "odd/tasks/f.md", oddtask.CauseNoCwd)
	if len(rec.asked) != 0 {
		t.Fatalf("Root was asked about %q, want it never called", rec.asked)
	}
}

func TestReadRefusesAMissingDocument(t *testing.T) {
	root, _ := repo(t, "x")
	missing := filepath.Join(root, "odd", "tasks", "gone.md")
	_, err := fsdoc.Reader{}.Read(missing, "")
	wantReadError(t, err, missing, oddtask.CauseNotFound)
}

func TestReadRefusesADirectory(t *testing.T) {
	root, _ := repo(t, "x")
	dir := filepath.Join(root, "odd", "tasks", "d.md")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := fsdoc.Reader{}.Read(dir, "")
	wantReadError(t, err, dir, oddtask.CauseNotRegular)
}

func TestReadRefusesALinkAsTheLastComponent(t *testing.T) {
	root, doc := repo(t, "x")
	link := filepath.Join(root, "odd", "tasks", "link.md")
	if err := os.Symlink(doc, link); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	_, err := fsdoc.Reader{}.Read(link, "")
	wantReadError(t, err, link, oddtask.CauseLink)
}

func TestReadFollowsALinkInTheDirectoryPath(t *testing.T) {
	root, _ := repo(t, "hello")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Join(root, "odd"), alias); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	got, err := fsdoc.Reader{}.Read(filepath.Join(alias, "tasks", "f.md"), "")
	if err != nil || got.Text != "hello" {
		t.Fatalf("Read = %+v, %v, want the text through a linked directory", got, err)
	}
}

func TestReadBoundsTheDocumentAtEightMiB(t *testing.T) {
	_, doc := repo(t, strings.Repeat("a", oddtask.MaxDocumentBytes))
	got, err := fsdoc.Reader{}.Read(doc, "")
	if err != nil || len(got.Text) != oddtask.MaxDocumentBytes {
		t.Fatalf("a document of exactly the bound: len %d, err %v, want it read", len(got.Text), err)
	}
	_, over := repo(t, strings.Repeat("a", oddtask.MaxDocumentBytes+1))
	_, err = fsdoc.Reader{}.Read(over, "")
	wantReadError(t, err, over, oddtask.CauseTooLarge)
}
