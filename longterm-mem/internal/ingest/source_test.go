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
		// URI is only ever a path; an identity the caller chooses goes in Override.
		{"a path that looks like a URL is still a path", KindFile, "/work", "repo://openspec/specs/x.md", "/work/repo:/openspec/specs/x.md"},
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

// ResolveOrigin resolves the path beside an override as it resolves any path, asking the reader only for a
// relative one, and returns the override as it came; the id of the override is then the same whatever directory
// the reader works in.
func TestResolveOriginPassesAnOverrideUnchanged(t *testing.T) {
	const override = "repo://openspec//specs/./x.md"
	cases := []struct {
		name    string
		uri     string
		wantURI string
		asked   int
	}{
		{"beside a relative path", "notes/x.md", "/work/notes/x.md", 1},
		{"beside no path", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader := &directory{dir: "/work"}
			in := Origin{Kind: KindFile, URI: c.uri, Override: override, Title: "T"}

			got, err := ResolveOrigin(reader, in)
			if err != nil {
				t.Fatalf("ResolveOrigin: %v", err)
			}
			want := in
			want.URI = c.wantURI
			if got != want {
				t.Errorf("ResolveOrigin = %+v, want %+v (the override unchanged)", got, want)
			}
			if reader.asked != c.asked {
				t.Errorf("the reader was asked %d times, want %d", reader.asked, c.asked)
			}
		})
	}

	idIn := func(dir string) string {
		t.Helper()
		resolved, err := ResolveOrigin(&directory{dir: dir}, Origin{Kind: KindFile, URI: "notes/x.md", Override: override})
		if err != nil {
			t.Fatal(err)
		}
		id, err := SourceID(resolved)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if first, second := idIn("/work"), idIn("/elsewhere"); first != second || first != "x-7fd6464b" {
		t.Errorf("the override's id is %q in /work and %q in /elsewhere, want %q in both", first, second, "x-7fd6464b")
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
// rather than choose a directory, and says what makes the path absolute. It is the one thing that tells a
// caller that forgot ResolveOrigin, in a file origin and in a directory origin alike, and the same path made
// absolute is accepted: the refusal is about the path and nothing else.
func TestSourceIDRefusesAFileOrDirectoryOriginThatIsNotAbsolute(t *testing.T) {
	for _, kind := range []SourceKind{KindFile, KindDirectory} {
		for _, uri := range []string{"notes/a.md", "./a.md", "../a.md", "."} {
			_, err := SourceID(Origin{Kind: kind, URI: uri})
			want := `ingest: file/directory origin "` + uri + `" is not absolute; resolve it with ResolveOrigin first`
			if err == nil || err.Error() != want {
				t.Errorf("SourceID(%s %q) error = %v, want %q", kind, uri, err, want)
			}
		}
		if id, err := SourceID(Origin{Kind: kind, URI: "/srv/notes/a.md"}); err != nil || id == "" {
			t.Errorf("SourceID(%s, an absolute path) = %q, %v, want an id", kind, id, err)
		}
	}
}
