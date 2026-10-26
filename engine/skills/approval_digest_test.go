package skills

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

// tinyDigest is the digest of tinyTree as the commands in the doc of SkillDigest give it for a
// directory that holds these three files, a/b.txt being the one with its owner execute bit set
// (chmod 755, the others 644). It was made with sha256sum, apart from this code. The list it is the
// digest of is
//
//	3088e5b60779a95389e4ed08d2ecee6eaac2311c590dab3f2e4beb3090a54f00 -  SKILL.md
//	dfb86f4417f5949eca2bed9b8986e96833618f1c03bb43a912ee39ff92190a53 -  a.txt
//	44d66302a35f7ae1cdbc90251b17494c5c55bdbc591cdf91f02028a90c6f856a x  a/b.txt
//
// where a.txt comes before a/b.txt because '.' is a smaller byte than '/', and the character after
// the digest of the bytes is x for the executable file and - for the others.
const tinyDigest = "dabef049475887bddc6a620b5aaecdf1d0f2924e7c2fe791054c52e715edcb3f"

// emptyListDigest is the SHA-256 of no bytes at all, which is the digest of a list of no files.
const emptyListDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// tinyTree is a skill of three regular files in two directories, as a store reports them: the one
// in a directory is executable, so that the tree has both values of the bit.
func tinyTree() []SkillEntry {
	return []SkillEntry{
		{Rel: "SKILL.md", Data: []byte("skill\n")},
		{Rel: "a.txt", Data: []byte("a file at the top\n")},
		{Rel: "a/b.txt", Data: []byte("a file in a directory\n"), Executable: true},
	}
}

func mustDigest(t *testing.T, entries []SkillEntry) string {
	t.Helper()
	digest, err := SkillDigest(entries)
	if err != nil {
		t.Fatalf("SkillDigest(%v) = error %v, want a digest", entries, err)
	}
	return digest
}

func TestSkillDigest_IsTheDigestOfTheSha256sumListOfTheFiles(t *testing.T) {
	if got := mustDigest(t, tinyTree()); got != tinyDigest {
		t.Fatalf("SkillDigest(tiny tree) = %s, want %s (what sha256sum gives for the list of its files)", got, tinyDigest)
	}
}

func TestSkillDigest_OfNoFilesIsTheDigestOfTheEmptyList(t *testing.T) {
	for name, entries := range map[string][]SkillEntry{"nil": nil, "empty": {}} {
		if got := mustDigest(t, entries); got != emptyListDigest {
			t.Errorf("SkillDigest(%s) = %s, want %s", name, got, emptyListDigest)
		}
	}
}

func TestSkillDigest_DoesNotDependOnTheOrderTheStoreReportsTheEntriesIn(t *testing.T) {
	reversed := tinyTree()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	before := append([]SkillEntry(nil), reversed...)
	if got := mustDigest(t, reversed); got != tinyDigest {
		t.Errorf("SkillDigest(reversed) = %s, want %s", got, tinyDigest)
	}
	if !reflect.DeepEqual(reversed, before) {
		t.Errorf("SkillDigest reordered the slice it was given: %v, was %v", reversed, before)
	}
}

func TestSkillDigest_BindsEveryFileItsBytesAndItsPath(t *testing.T) {
	with := func(change func([]SkillEntry) []SkillEntry) []SkillEntry { return change(tinyTree()) }
	for name, entries := range map[string][]SkillEntry{
		"a companion file changed": with(func(e []SkillEntry) []SkillEntry {
			e[1].Data = []byte("a file at the top, changed\n")
			return e
		}),
		"SKILL.md changed": with(func(e []SkillEntry) []SkillEntry {
			e[0].Data = []byte("skill!\n")
			return e
		}),
		"a file in a directory changed": with(func(e []SkillEntry) []SkillEntry {
			e[2].Data = []byte("a file in a directory!\n")
			return e
		}),
		"a file added": with(func(e []SkillEntry) []SkillEntry {
			return append(e, SkillEntry{Rel: "c.txt", Data: []byte("new\n")})
		}),
		"an empty file added": with(func(e []SkillEntry) []SkillEntry {
			return append(e, SkillEntry{Rel: "empty"})
		}),
		"a file removed": with(func(e []SkillEntry) []SkillEntry { return e[:2] }),
		"a file renamed, its bytes the same": with(func(e []SkillEntry) []SkillEntry {
			e[1].Rel = "z.txt"
			return e
		}),
		"a file moved to another directory": with(func(e []SkillEntry) []SkillEntry {
			e[1].Rel = "b/a.txt"
			return e
		}),
		"line endings changed": with(func(e []SkillEntry) []SkillEntry {
			e[0].Data = []byte("skill\r\n")
			return e
		}),
		"a record that is not at the root": with(func(e []SkillEntry) []SkillEntry {
			return append(e, SkillEntry{Rel: "references/" + ApprovalRecordName, Data: []byte("{}")})
		}),
	} {
		if got := mustDigest(t, entries); got == tinyDigest {
			t.Errorf("%s: the digest is still %s, an approval of the tiny tree would hold", name, got)
		}
	}
}

// Setting a script's execute bit changes what the approval lets run without changing one byte of
// it, so the bit is part of what is approved, in both directions and for every file.
func TestSkillDigest_BindsWhetherEachFileIsExecutable(t *testing.T) {
	toggled := func(rel string) []SkillEntry {
		entries := tinyTree()
		for i := range entries {
			if entries[i].Rel == rel {
				entries[i].Executable = !entries[i].Executable
			}
		}
		return entries
	}
	seen := map[string]string{tinyDigest: "the tiny tree as it is"}
	for name, entries := range map[string][]SkillEntry{
		"SKILL.md made executable":                 toggled("SKILL.md"),
		"a file at the top made executable":        toggled("a.txt"),
		"the executable file no longer executable": toggled("a/b.txt"),
	} {
		got := mustDigest(t, entries)
		if before, same := seen[got]; same {
			t.Errorf("%s: the digest is %s, the one of %s: an approval of that would hold while the execute bit changed", name, got, before)
		}
		seen[got] = name
	}
}

func TestSkillDigest_AFileEndingWhereAnotherBeginsIsPartOfWhatIsApproved(t *testing.T) {
	together := []SkillEntry{{Rel: "p", Data: []byte("12")}, {Rel: "q", Data: []byte("34")}}
	moved := []SkillEntry{{Rel: "p", Data: []byte("1")}, {Rel: "q", Data: []byte("234")}}
	if mustDigest(t, together) == mustDigest(t, moved) {
		t.Error("moving a byte from one file to the next left the digest as it was")
	}
}

func TestSkillDigest_LeavesOutTheRecordAtTheRootWhateverItHolds(t *testing.T) {
	for name, record := range map[string]SkillEntry{
		"with bytes":               {Rel: ApprovalRecordName, Data: []byte(`{"version": 2}`)},
		"without bytes":            {Rel: ApprovalRecordName},
		"with its execute bit set": {Rel: ApprovalRecordName, Executable: true},
	} {
		if got := mustDigest(t, append(tinyTree(), record)); got != tinyDigest {
			t.Errorf("a record %s changed the digest to %s, want %s: a digest cannot cover the file that holds it", name, got, tinyDigest)
		}
	}
}

func TestSkillDigest_RefusesWhatItCannotCover(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entry   SkillEntry
		wantAll []string
	}{
		{"a symbolic link", SkillEntry{Rel: "link", Type: fs.ModeSymlink}, []string{"link is a symbolic link"}},
		{"a named pipe", SkillEntry{Rel: "pipe", Type: fs.ModeNamedPipe}, []string{"pipe is a named pipe"}},
		{"a socket", SkillEntry{Rel: "sock", Type: fs.ModeSocket}, []string{"sock is a socket"}},
		{"a device", SkillEntry{Rel: "dev", Type: fs.ModeDevice}, []string{"dev is a device"}},
		{"a character device", SkillEntry{Rel: "tty", Type: fs.ModeDevice | fs.ModeCharDevice}, []string{"tty is a device"}},
		{"something irregular", SkillEntry{Rel: "odd", Type: fs.ModeIrregular}, []string{"odd is not a regular file"}},
		{"a link in the place of the record", SkillEntry{Rel: ApprovalRecordName, Type: fs.ModeSymlink}, []string{".approval.json is a symbolic link"}},
		{"a link in a directory", SkillEntry{Rel: "references/deep/link", Type: fs.ModeSymlink}, []string{"references/deep/link is a symbolic link"}},
		{"a line feed in a name", SkillEntry{Rel: "a\nb", Data: []byte("x")}, []string{`"a\nb" has a line break in its name`}},
		{"a carriage return in a name", SkillEntry{Rel: "a\rb", Data: []byte("x")}, []string{`"a\rb" has a line break in its name`}},
		{"a backslash in a name", SkillEntry{Rel: `a\b`, Data: []byte("x")}, []string{`"a\\b" has a backslash in its name`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			digest, err := SkillDigest(append(tinyTree(), tc.entry))
			if err == nil {
				t.Fatalf("SkillDigest = %q, want an error: an approval that skipped it would hold while it changed", digest)
			}
			if digest != "" {
				t.Errorf("SkillDigest returned the digest %q with its error: nothing may be approved by it", digest)
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
		})
	}
}

func TestSkillDigest_SaysEveryEntryItRefusedInTheOrderOfTheirPaths(t *testing.T) {
	entries := append(tinyTree(),
		SkillEntry{Rel: "pipe", Type: fs.ModeNamedPipe},
		SkillEntry{Rel: "dangling", Type: fs.ModeSymlink},
		SkillEntry{Rel: "link-to-a-file.md", Type: fs.ModeSymlink},
	)
	_, err := SkillDigest(entries)
	if err == nil {
		t.Fatal("SkillDigest = nil error, want one that names every entry it refused")
	}
	want := "dangling is a symbolic link; link-to-a-file.md is a symbolic link; pipe is a named pipe"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}
