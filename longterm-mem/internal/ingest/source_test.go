package ingest

import (
	"errors"
	"testing"
)

// directory is a SourceReader that works in the directory it was given and counts the times it was asked, so the
// cases below are a function of what the reader answers and of nothing about the machine they run on.
type directory struct {
	dir   string
	err   error
	asked int
}

func (d *directory) WorkingDirectory() (string, error) {
	d.asked++
	return d.dir, d.err
}

var _ SourceReader = (*directory)(nil)

func TestResolveOriginBeginsARelativeFileOrDirectoryAtTheWorkingDirectory(t *testing.T) {
	cases := []struct {
		name string
		kind SourceKind
		wd   string
		uri  string
		want string
	}{
		{"a file", KindFile, "/work", "notes/Guide v2.md", "/work/notes/Guide v2.md"},
		{"a directory", KindDirectory, "/work", "./notes", "/work/notes"},
		{"a path that climbs out of the working directory", KindFile, "/work/project", "../sibling/x.md", "/work/sibling/x.md"},
		// The override of the design (repo://openspec/specs/x.md) is taken for a relative path, as it was when the
		// package asked the process for the directory: pinned as it is, and reported in the ledger of the batch.
		{"an override that looks like a URL", KindFile, "/work", "repo://openspec/specs/x.md", "/work/repo:/openspec/specs/x.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader := &directory{dir: c.wd}
			in := Origin{Kind: c.kind, URI: c.uri, Title: "A title", Label: "a label"}

			got, err := ResolveOrigin(reader, in)
			if err != nil {
				t.Fatalf("ResolveOrigin: %v", err)
			}
			want := in
			want.URI = c.want
			if got != want {
				t.Errorf("ResolveOrigin = %+v, want %+v (only the path changes)", got, want)
			}
			if reader.asked != 1 {
				t.Errorf("the reader was asked %d times, want once", reader.asked)
			}
		})
	}
}

func TestResolveOriginLeavesAnOriginThatNeedsNoDirectoryAsItIsAndDoesNotAsk(t *testing.T) {
	cases := []struct {
		name string
		in   Origin
	}{
		{"an absolute file", Origin{Kind: KindFile, URI: "/srv/notes/../notes/Guide v2.md", Title: "T"}},
		{"an absolute directory", Origin{Kind: KindDirectory, URI: "/srv/notes/"}},
		{"a URL", Origin{Kind: KindURL, URI: "https://example.com/docs/config"}},
		{"a relative-looking URL", Origin{Kind: KindURL, URI: "docs/config"}},
		{"a pasted text", Origin{Kind: KindPasted, Label: "my notes"}},
		{"a file with no path", Origin{Kind: KindFile}},
		{"a source kind nobody knows", Origin{Kind: SourceKind("tape"), URI: "reel/1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader := &directory{dir: "/work"}

			got, err := ResolveOrigin(reader, c.in)
			if err != nil {
				t.Fatalf("ResolveOrigin: %v", err)
			}
			if got != c.in {
				t.Errorf("ResolveOrigin = %+v, want %+v unchanged", got, c.in)
			}
			if reader.asked != 0 {
				t.Errorf("the reader was asked %d times, want never", reader.asked)
			}
		})
	}
}

func TestResolveOriginTellsTheReadersFailureUnchanged(t *testing.T) {
	failure := errors.New("getwd: no such file or directory")
	reader := &directory{err: failure}

	got, err := ResolveOrigin(reader, Origin{Kind: KindFile, URI: "notes/a.md", Title: "T"})

	if !errors.Is(err, failure) || err.Error() != failure.Error() {
		t.Errorf("ResolveOrigin error = %v, want the reader's own: %v", err, failure)
	}
	if got != (Origin{}) {
		t.Errorf("ResolveOrigin = %+v with an error, want the zero Origin", got)
	}
}

func TestResolveOriginRefusesAWorkingDirectoryThatIsNotAbsolute(t *testing.T) {
	for _, dir := range []string{"", "work", "./work"} {
		_, err := ResolveOrigin(&directory{dir: dir}, Origin{Kind: KindFile, URI: "notes/a.md"})
		if err == nil {
			t.Errorf("a reader that works in %q got a path resolved from it", dir)
		}
	}
}

// The id of a file that was named relative to a directory is the id of the file's absolute path, the id the
// table of TestSourceIDOfAnAbsoluteFileOrDirectory holds for it.
func TestSourceIDOfAResolvedRelativeOriginIsTheIDOfItsAbsolutePath(t *testing.T) {
	cases := []struct {
		name string
		kind SourceKind
		wd   string
		uri  string
		want string
	}{
		{"a file", KindFile, "/srv", "notes/Guide v2.md", "guide-v2-191bacc1"},
		{"the same path as a directory", KindDirectory, "/srv", "notes/Guide v2.md", "guide-v2-191bacc1"},
		{"a path that climbs out and comes back", KindFile, "/srv/other", "../notes/./Guide v2.md", "guide-v2-191bacc1"},
		{"a directory", KindDirectory, "/srv", "notes/", "notes-b9c5ef13"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			origin, err := ResolveOrigin(&directory{dir: c.wd}, Origin{Kind: c.kind, URI: c.uri})
			if err != nil {
				t.Fatalf("ResolveOrigin: %v", err)
			}
			got, err := SourceID(origin)
			if err != nil {
				t.Fatalf("SourceID: %v", err)
			}
			if got != c.want {
				t.Errorf("SourceID = %q, want %q", got, c.want)
			}
		})
	}
}

// SourceID is a function of its argument: it does not learn where a relative path begins, so it refuses one
// rather than choose a directory, and says what makes the path absolute.
func TestSourceIDRefusesAFileOrDirectoryOriginThatIsNotAbsolute(t *testing.T) {
	for _, kind := range []SourceKind{KindFile, KindDirectory} {
		_, err := SourceID(Origin{Kind: kind, URI: "notes/a.md"})
		want := `ingest: file/directory origin "notes/a.md" is not absolute; resolve it with ResolveOrigin first`
		if err == nil || err.Error() != want {
			t.Errorf("SourceID(%s, a relative path) error = %v, want %q", kind, err, want)
		}
	}
}
