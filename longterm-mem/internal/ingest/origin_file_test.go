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

// A file or a directory origin with no path has nothing to identify, and says so in the same words in both kinds.
func TestSourceIDOfAFileOrDirectoryNeedsAPath(t *testing.T) {
	for _, kind := range []SourceKind{KindFile, KindDirectory} {
		_, err := SourceID(Origin{Kind: kind})
		if err == nil || err.Error() != "ingest: file/directory origin requires URI" {
			t.Errorf("SourceID(%s with no URI) error = %v, want %q", kind, err, "ingest: file/directory origin requires URI")
		}
	}
}
