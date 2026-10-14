package sourcefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ingest"
)

// sameDirectory reports whether a and b name one directory (the temporary directory of a test may be reached
// through a link, so the names alone are not compared).
func sameDirectory(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	infoB, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(infoA, infoB)
}

func TestReaderAnswersTheAbsoluteDirectoryTheProcessWorksIn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	got, err := Reader{}.WorkingDirectory()
	if err != nil {
		t.Fatalf("WorkingDirectory: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("WorkingDirectory = %q, want an absolute path", got)
	}
	if !sameDirectory(t, got, dir) {
		t.Errorf("WorkingDirectory = %q, want the directory the process changed to, %q", got, dir)
	}
}

// The directory is asked at every call, not remembered: a process that changes directory is in another one.
func TestReaderAnswersTheDirectoryOfTheMomentItIsAsked(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()

	t.Chdir(first)
	gotFirst, err := Reader{}.WorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(second)
	gotSecond, err := Reader{}.WorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}

	if !sameDirectory(t, gotFirst, first) || !sameDirectory(t, gotSecond, second) {
		t.Errorf("answers %q and %q, want the directories %q and %q", gotFirst, gotSecond, first, second)
	}
}

func TestReaderFailsWhenTheDirectoryTheProcessWorksInIsGone(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	if got, err := (Reader{}).WorkingDirectory(); err == nil {
		t.Errorf("WorkingDirectory = %q with no error, want the failure of the process to tell where it is", got)
	}
}

// These are the cases the ingest package pinned while it asked the process itself (the id of a relative path is
// the id of the absolute path it makes from the working directory, which the process can lose), now held through
// the adapter on the real machine: the rule is ingest's and is tested there over a reader that answers a
// directory; this proves that the adapter answers it with the directory the cases are in.
func TestAResolvedRelativeOriginIsIdentifiedByItsPathFromTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"notes/Guide v2.md", "./a.md", "../sibling/x.md", "repo://openspec/specs/x.md"} {
		for _, kind := range []ingest.SourceKind{ingest.KindFile, ingest.KindDirectory} {
			resolved, err := ingest.ResolveOrigin(Reader{}, ingest.Origin{Kind: kind, URI: rel})
			if err != nil {
				t.Fatalf("ResolveOrigin(%s %q): %v", kind, rel, err)
			}
			got, err := ingest.SourceID(resolved)
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", kind, resolved.URI, err)
			}
			want, err := ingest.SourceID(ingest.Origin{Kind: kind, URI: filepath.Join(cwd, rel)})
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", kind, filepath.Join(cwd, rel), err)
			}
			if got != want {
				t.Errorf("id of %s %q = %q, want the id of %q, %q", kind, rel, got, filepath.Join(cwd, rel), want)
			}
		}
	}
}

func TestAResolvedRelativeOriginChangesWithTheDirectoryTheProcessIsIn(t *testing.T) {
	origin := ingest.Origin{Kind: ingest.KindFile, URI: "notes/a.md"}
	idIn := func(dir string) string {
		t.Helper()
		t.Chdir(dir)
		resolved, err := ingest.ResolveOrigin(Reader{}, origin)
		if err != nil {
			t.Fatal(err)
		}
		id, err := ingest.SourceID(resolved)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	first, second := idIn(t.TempDir()), idIn(t.TempDir())

	if first == second {
		t.Errorf("the same relative path got the same id %q in two different directories", first)
	}
}

func TestAnAbsoluteOriginNeedsNoWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	origin := ingest.Origin{Kind: ingest.KindFile, URI: "/srv/notes/Guide v2.md"}

	if _, err := ingest.ResolveOrigin(Reader{}, ingest.Origin{Kind: ingest.KindFile, URI: "notes/a.md"}); err == nil {
		t.Error("a relative path was resolved although the working directory is gone")
	}
	resolved, err := ingest.ResolveOrigin(Reader{}, origin)
	if err != nil {
		t.Fatalf("an absolute path needs no working directory, got %v", err)
	}
	if id, err := ingest.SourceID(resolved); err != nil || id != "guide-v2-191bacc1" {
		t.Errorf("SourceID = %q, %v, want %q", id, err, "guide-v2-191bacc1")
	}
}
