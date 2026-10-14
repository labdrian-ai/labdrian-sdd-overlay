package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

// This file pins what SourceID does with a path that is not absolute while the package still asks the process
// where it works (filepath.Abs): the relative path is taken to be relative to the working directory, and the id
// is the id of the absolute path that makes. The next commit moves that question behind a port and deletes this
// file; the same cases are then held over a reader that answers a directory (ingest) and over the file system
// (sourcefs).

func TestSourceIDOfARelativeOriginIsTheIDOfThePathFromTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"notes/Guide v2.md",
		"./a.md",
		"../sibling/x.md",
		// Origin.URI is documented as the override that gives a file a machine-independent identity (design.md,
		// "Known limitation"), e.g. repo://openspec/specs/x.md. The code does not tell an override from a path: this
		// one is taken for a relative path, its // folded into one slash and the working directory put before it.
		// Pinned as it is, not as the design says; the ledger of this batch reports it.
		"repo://openspec/specs/x.md",
	} {
		for _, kind := range []SourceKind{KindFile, KindDirectory} {
			got, err := SourceID(Origin{Kind: kind, URI: rel})
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", kind, rel, err)
			}
			want, err := SourceID(Origin{Kind: kind, URI: filepath.Join(cwd, rel)})
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", kind, filepath.Join(cwd, rel), err)
			}
			if got != want {
				t.Errorf("SourceID(%s %q) = %q, want the id of %q, %q", kind, rel, got, filepath.Join(cwd, rel), want)
			}
		}
	}
}

func TestSourceIDOfARelativeOriginChangesWithTheDirectoryTheProcessIsIn(t *testing.T) {
	origin := Origin{Kind: KindFile, URI: "notes/a.md"}

	t.Chdir(t.TempDir())
	first, err := SourceID(origin)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	second, err := SourceID(origin)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("the same relative path got the same id %q in two different directories", first)
	}
}

func TestSourceIDOfARelativeOriginFailsWhenTheWorkingDirectoryIsGone(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := SourceID(Origin{Kind: KindFile, URI: "notes/a.md"}); err == nil {
		t.Error("a relative path got an id although the working directory is gone")
	}
	if _, err := SourceID(Origin{Kind: KindFile, URI: "/srv/notes/Guide v2.md"}); err != nil {
		t.Errorf("an absolute path needs no working directory, got %v", err)
	}
}
