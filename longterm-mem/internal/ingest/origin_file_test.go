package ingest

import "testing"

// A file or a directory origin is identified by its cleaned absolute path alone (the contract's Source-Id
// derivation table, skills/_shared/ingested-observation-contract.md): the id is the slug of the base name without
// its extension, cut at 40 bytes, a dash, and the first eight hex digits of the sha256 of that path.
//
// The digests below were computed with sha256sum from the formula, not by running the code, and the table was
// recorded as passing on the package as it stood when it read the working directory (5607a75): it holds the
// derivation fixed while the part that read the directory moves behind a port.
func TestSourceIDOfAnAbsoluteFileOrDirectory(t *testing.T) {
	cases := []struct {
		name string
		kind SourceKind
		uri  string
		want string
	}{
		{"a file", KindFile, "/srv/notes/Guide v2.md", "guide-v2-191bacc1"},
		{"the same path as a directory has the same id", KindDirectory, "/srv/notes/Guide v2.md", "guide-v2-191bacc1"},
		{"an unclean path is cleaned first", KindFile, "/srv/notes/../notes/./Guide v2.md", "guide-v2-191bacc1"},
		{"a directory with a trailing slash", KindDirectory, "/srv/notes/", "notes-b9c5ef13"},
		{"only the last extension is dropped", KindFile, "/srv/archive.tar.gz", "archive-tar-d3b4aebd"},
		{"the case of the extension does not matter to what is dropped", KindFile, "/srv/notes/draft.TXT", "draft-8f9f11cc"},
		{"a name that is only an extension has no slug", KindFile, "/srv/.env", "-2a4cffc2"},
		{"the root", KindDirectory, "/", "-8a5edab2"},
		{"a long name is cut at the last word that fits in 40 bytes", KindFile, "/srv/a-very-long-document-name-that-keeps-going-on-and-on-forever.md", "a-very-long-document-name-that-keeps-900effbf"},
		{"letters outside a-z0-9 are separators", KindFile, "/srv/Résumé Ünï.md", "r-sum-n-86104f79"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SourceID(Origin{Kind: c.kind, URI: c.uri})
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", c.kind, c.uri, err)
			}
			if got != c.want {
				t.Errorf("SourceID(%s %q) = %q, want %q", c.kind, c.uri, got, c.want)
			}
		})
	}
}

// An override is the identity the caller chose for a file or a directory origin, so that one source has one id on
// every machine: SourceID hashes its exact bytes, neither cleaned nor joined to any directory, and does not read
// the path beside it. The readable part is the override's base name without its extension, as for a path. On a
// URL or a pasted origin the override is ignored, as Label is ignored on every kind but pasted.
//
// The digests below were computed with sha256sum from the formula, not by running the code.
func TestSourceIDOfAnOverride(t *testing.T) {
	cases := []struct {
		name string
		in   Origin
		want string
	}{
		{"a file", Origin{Kind: KindFile, URI: "/srv/notes/x.md", Override: "repo://openspec/specs/x.md"}, "x-8bfa69fe"},
		{"a directory", Origin{Kind: KindDirectory, URI: "/srv/notes", Override: "repo://openspec/specs/x.md"}, "x-8bfa69fe"},
		{"an unclean override keeps its own bytes", Origin{Kind: KindFile, Override: "repo://openspec//specs/./x.md"}, "x-7fd6464b"},
		{"beside a relative path", Origin{Kind: KindFile, URI: "notes/a.md", Override: "repo://openspec/specs/x.md"}, "x-8bfa69fe"},
		{"beside no path", Origin{Kind: KindFile, Override: "repo://openspec/specs/x.md"}, "x-8bfa69fe"},
		{"the readable part is the base name without its extension", Origin{Kind: KindFile, Override: "repo://openspec/specs/Guide v2.md"}, "guide-v2-31db09a7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SourceID(c.in)
			if err != nil {
				t.Fatalf("SourceID(%+v): %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("SourceID(%+v) = %q, want %q", c.in, got, c.want)
			}
		})
	}

	ignored := []struct {
		name string
		in   Origin
	}{
		{"a URL", Origin{Kind: KindURL, URI: "https://example.com/docs/config"}},
		{"a pasted text", Origin{Kind: KindPasted, Label: "my notes"}},
	}
	for _, c := range ignored {
		t.Run(c.name+" ignores the override", func(t *testing.T) {
			want, err := SourceID(c.in)
			if err != nil {
				t.Fatalf("SourceID(%+v): %v", c.in, err)
			}
			with := c.in
			with.Override = "repo://openspec/specs/x.md"
			got, err := SourceID(with)
			if err != nil {
				t.Fatalf("SourceID(%+v): %v", with, err)
			}
			if got != want {
				t.Errorf("SourceID(%+v) = %q, want %q, the id without the override", with, got, want)
			}
		})
	}
}

// A file or a directory origin with no path has nothing to identify, and says so in the same words in both kinds.
func TestSourceIDOfAFileOrDirectoryNeedsAPath(t *testing.T) {
	for _, kind := range []SourceKind{KindFile, KindDirectory} {
		_, err := SourceID(Origin{Kind: kind})
		if err == nil || err.Error() != "ingest: file/directory origin requires URI" {
			t.Errorf("SourceID(%s with no URI) error = %v, want %q", kind, err, "ingest: file/directory origin requires URI")
		}
	}
}
