package registryyaml_test

// Tests of the reader policy of Phase 9 decision Q5, which is where this reader differs from the
// strict one that came before it (the golden files of engine/cmd pin each of those differences from
// the outside, in the words a person reads):
//
//   - the version of the file selects the decoder, and a version no decoder is for is refused first,
//     naming it;
//   - a field the reader does not know is left out and said so (skills.Registry.Unread);
//   - a field of the must-understand set is never left out: in a shape the decoder does not read, it
//     is refused, naming the field and the line.

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// policyDoc is a registry of two entries that holds every field of version 1: the first a core skill
// of one project, the second an external one. The tests that need a document with one thing
// different build it from this one, by the text of a line.
const policyDoc = `version: "1"
skills:
  - id: alpha
    path: alpha
    source:
      type: core
      upstream:
        owner: gentleman-programming
    install:
      defaultScope: project
      allowedProjects:
        - demo
      targets:
        - claude
        - pi
    lifecycle:
      updateStrategy: vendor-merge
  - id: beta
    path: beta
    source:
      type: external
      repo: https://example.test/org/beta
      ref: v1
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// lineOf is the number, from 1, of the first line of text that is exactly line.
func lineOf(t *testing.T, text, line string) int {
	t.Helper()
	for i, l := range strings.Split(text, "\n") {
		if l == line {
			return i + 1
		}
	}
	t.Fatalf("no line %q in:\n%s", line, text)
	return 0
}

// replaceLine returns text with its first line that is exactly old replaced by repl, which may be
// several lines or none.
func replaceLine(t *testing.T, text, old, repl string) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l == old {
			out := append(append(append([]string{}, lines[:i]...), repl), lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	t.Fatalf("no line %q in:\n%s", old, text)
	return ""
}

func decoded(t *testing.T, text string) skills.Registry {
	t.Helper()
	reg, err := registryyaml.NewRepository(nil).Decode([]byte(text))
	if err != nil {
		t.Fatalf("Decode() = %v, want it to read:\n%s", err, text)
	}
	return reg
}

// withNothingUnread is reg without the note of what was left out, to compare what was read.
func withNothingUnread(reg skills.Registry) skills.Registry {
	reg.Unread = nil
	return reg
}

// --- the version selects the decoder ----------------------------------------------------------

func TestVersionOneIsReadByItsDecoder(t *testing.T) {
	reg := decoded(t, policyDoc)
	if reg.Version != "1" || len(reg.Skills) != 2 || len(reg.Unread) != 0 {
		t.Errorf("version 1 read as %+v, want its two entries and nothing left out", reg)
	}
}

// A version no decoder is for is refused first, naming it: nothing else of the file is read, so its
// entries cannot be read the wrong way, and what the person is told is the version.
func TestAVersionNoDecoderIsForIsRefusedNamingItBeforeAnythingIsRead(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	for name, tc := range map[string]struct{ doc, version string }{
		"version 2":        {strings.Replace(policyDoc, `"1"`, `"2"`, 1), `"2"`},
		"version 1.0":      {strings.Replace(policyDoc, `"1"`, `"1.0"`, 1), `"1.0"`},
		"version v1":       {strings.Replace(policyDoc, `"1"`, `v1`, 1), `"v1"`},
		"an empty version": {strings.Replace(policyDoc, `"1"`, `""`, 1), `""`},
		"a version with no value": {
			strings.Replace(policyDoc, `version: "1"`, "version:", 1), `""`},
		"a later format, whose entries this decoder could not read": {
			"version: \"2\"\nskills:\n  - id: alpha\n    color: red\n    shape: 3\n", `"2"`},
		"a version before an entry that is not valid": {
			strings.Replace(strings.Replace(policyDoc, `"1"`, `"7"`, 1), "    path: alpha\n", "", 1), `"7"`},
	} {
		t.Run(name, func(t *testing.T) {
			reg, err := repo.Decode([]byte(tc.doc))
			want := fmt.Sprintf("skills: version %s is not supported; only version \"1\" is valid", tc.version)
			if err == nil || err.Error() != want {
				t.Errorf("Decode() = %v, want %q", err, want)
			}
			if reg.Version != "" || len(reg.Skills) != 0 || len(reg.Unread) != 0 {
				t.Errorf("Decode() returned %+v with its error, want nothing read", reg)
			}
		})
	}
}

// The version is found before the entries are, wherever the file puts it. A file that has none is
// refused for that before any fault of its entries; a fault of the text itself comes before both,
// because the text is read before it is understood.
func TestTheVersionIsFoundBeforeTheEntriesWhereverTheFilePutsIt(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	noVersion := strings.Replace(policyDoc, "version: \"1\"\n", "", 1)

	last := noVersion + "version: \"1\"\n"
	if reg, err := repo.Decode([]byte(last)); err != nil || reg.Version != "1" || len(reg.Skills) != 2 {
		t.Errorf("a version at the end = %+v, %v, want the registry", reg, err)
	}

	const missing = "skills: missing required top-level field 'version'"
	if _, err := repo.Decode([]byte(noVersion)); err == nil || err.Error() != missing {
		t.Errorf("no version = %v, want %q", err, missing)
	}
	broken := strings.Replace(noVersion, "    path: beta\n", "   path: beta\n", 1)
	if _, err := repo.Decode([]byte(broken)); err == nil || err.Error() != missing {
		t.Errorf("no version and a mis-indented entry = %v, want %q first", err, missing)
	}

	tab := strings.Replace(policyDoc, `"1"`, `"2"`, 1) + "\ttab\n"
	if _, err := repo.Decode([]byte(tab)); err == nil || !strings.Contains(err.Error(), "tab character not allowed") {
		t.Errorf("a tab in a file of version 2 = %v, want the refusal of the tab, not of the version", err)
	}
}

// Only the top-level key "version" is the version: a key of that name inside something else is not
// (it is under a key the reader does not know, here), and a "- version" item at the top of the file
// is not a key at all.
func TestOnlyTheTopLevelKeyIsTheVersion(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	nested := "extra:\n  version: \"9\"\n" + policyDoc
	reg, err := repo.Decode([]byte(nested))
	if err != nil || reg.Version != "1" || len(reg.Unread) != 1 {
		t.Errorf("a version inside an unknown block = %+v, %v, want version 1 and the block left out", reg, err)
	}

	item := policyDoc + "- version: \"9\"\n"
	_, err = repo.Decode([]byte(item))
	want := fmt.Sprintf("line %d: unexpected token at document root", lineOf(t, item, "- version: \"9\""))
	if err == nil || err.Error() != want {
		t.Errorf("a version item at the top of the file = %v, want %q", err, want)
	}
}

// A field that is a block, as the first key of an entry, is in its own shape without a value on
// the line, and refused with one, as it is anywhere else in the entry.
func TestAFirstKeyOfAnEntryHasTheShapeOfItsField(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	doc := "version: \"1\"\nskills:\n  - source: custom\n    id: alpha\n"
	_, err := repo.Decode([]byte(doc))
	want := "line 3: source must be a block of fields (type, upstream, repo, ref), not a value"
	if err == nil || err.Error() != want {
		t.Errorf("a first key that is a block with a value = %v, want %q", err, want)
	}
	_, err = repo.Decode([]byte("version: \"1\"\nskills:\n  - lifecycle: overlay-only\n    id: alpha\n"))
	if err != nil {
		t.Errorf("a first key that is a block nothing depends on, with a value = %v, want it left out", err)
	}
}

// A file that says its version twice is refused, naming both lines, before a decoder is chosen:
// the version is the one field that selects how the rest is read, so there is no reading of the
// file in which two of it are not a fault (decision 5 of the owner; the strict reader took the last).
func TestAFileThatSaysItsVersionTwiceIsRefusedBeforeAnyDecoderIsChosen(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	const want = `line 2: duplicate key "version" at the top level (first on line 1)`
	for name, doc := range map[string]string{
		"2 then 1": "version: \"2\"\n" + policyDoc,
		"1 then 2": "version: \"1\"\n" + strings.Replace(policyDoc, `"1"`, `"2"`, 1),
		"3 then 3": "version: \"3\"\nversion: \"3\"\nskills:\n",
	} {
		if reg, err := repo.Decode([]byte(doc)); err == nil || err.Error() != want || len(reg.Skills) != 0 {
			t.Errorf("version %s = %+v, %v, want the refusal %q and no entries", name, reg, err, want)
		}
	}
}

// --- what the reader does not know is left out, and said ---------------------------------------

func TestAFieldTheReaderDoesNotKnowIsLeftOutAndSaid(t *testing.T) {
	clean := decoded(t, policyDoc)
	if len(clean.Unread) != 0 {
		t.Fatalf("the clean document leaves out %q, want nothing", clean.Unread)
	}
	cases := map[string]struct {
		doc  string
		what string // the note, after "line N: "
		line string // the line of the document the note names
	}{
		"at the top of the file, with a value": {"extra: 1\n" + policyDoc, `unknown top-level key "extra"`, "extra: 1"},
		"at the top of the file, with none":    {"extra:\n" + policyDoc, `unknown top-level key "extra"`, "extra:"},
		"at the top of the file, after the entries": {policyDoc + "generatedBy: some tool\n",
			`unknown top-level key "generatedBy"`, "generatedBy: some tool"},
		"at the top of the file, with a block under it": {"extra:\n  nested: 1\n  other:\n    - a\n    - b\n" + policyDoc,
			`unknown top-level key "extra"`, "extra:"},
		"in an entry, first": {replaceLine(t, policyDoc, "  - id: alpha", "  - color: red\n    id: alpha"),
			`unknown key "color" in skill entry`, "  - color: red"},
		"in an entry, between its fields": {replaceLine(t, policyDoc, "    path: alpha", "    path: alpha\n    color: red"),
			`unknown key "color" in skill entry`, "    color: red"},
		"in an entry, with a block under it": {replaceLine(t, policyDoc, "    path: alpha", "    path: alpha\n    notes:\n      - one\n      - two"),
			`unknown key "notes" in skill entry`, "    notes:"},
		"in an entry, last": {replaceLine(t, policyDoc, "      updateStrategy: vendor-merge", "      updateStrategy: vendor-merge\n    owner: me"),
			`unknown key "owner" in skill entry`, "    owner: me"},
		"in a source":    {replaceLine(t, policyDoc, "      type: core", "      type: core\n      mirror: x"), `unknown key "mirror" in source`, "      mirror: x"},
		"in an upstream": {replaceLine(t, policyDoc, "        owner: gentleman-programming", "        owner: gentleman-programming\n        name: x"), `unknown key "name" in upstream`, "        name: x"},
		"in an install":  {replaceLine(t, policyDoc, "      defaultScope: project", "      defaultScope: project\n      mode: x"), `unknown key "mode" in install`, "      mode: x"},
		"in a lifecycle": {replaceLine(t, policyDoc, "      updateStrategy: vendor-merge", "      updateStrategy: vendor-merge\n      phase: x"), `unknown key "phase" in lifecycle`, "      phase: x"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reg := decoded(t, tc.doc)
			want := []string{fmt.Sprintf("line %d: %s", lineOf(t, tc.doc, tc.line), tc.what)}
			if !reflect.DeepEqual(reg.Unread, want) {
				t.Errorf("left out %q, want %q", reg.Unread, want)
			}
			if !reflect.DeepEqual(withNothingUnread(reg), clean) {
				t.Errorf("what was read differs from the document without the unknown field:\n got %+v\nwant %+v", withNothingUnread(reg), clean)
			}
		})
	}
}

// Several are told in the order of the file.
func TestWhatIsLeftOutIsToldInTheOrderOfTheFile(t *testing.T) {
	doc := "first: 1\n" +
		replaceLine(t, policyDoc, "    path: alpha", "    path: alpha\n    second: 2") +
		"third: 3\nfourth: 4\n"
	reg := decoded(t, doc)
	var keys []string
	for _, note := range reg.Unread {
		keys = append(keys, regexp.MustCompile(`"([a-z]+)"`).FindStringSubmatch(note)[1])
	}
	if strings.Join(keys, ",") != "first,second,third,fourth" {
		t.Errorf("left out %q, want first, second, third and fourth, in the order of the file", reg.Unread)
	}
}

// A key that repeats inside a mapping is a damaged file whether or not the key is known: the
// tolerance is for what the reader does not know, not for what it cannot make sense of.
func TestAnUnknownKeyThatRepeatsInAMappingIsStillRefused(t *testing.T) {
	doc := replaceLine(t, policyDoc, "    path: alpha", "    path: alpha\n    color: red\n    color: blue")
	_, err := registryyaml.NewRepository(nil).Decode([]byte(doc))
	want := fmt.Sprintf(`line %d: duplicate key "color" in skill entry`, lineOf(t, doc, "    color: blue"))
	if err == nil || err.Error() != want {
		t.Errorf("Decode() = %v, want %q", err, want)
	}
}

// What follows an unknown field is read as ever, and the domain still judges it: the tolerance is
// not an escape from the rules.
func TestWhatIsKnownIsStillJudgedWhenSomethingElseIsLeftOut(t *testing.T) {
	doc := replaceLine(t, policyDoc, "        - pi", "        - vim")
	doc = replaceLine(t, doc, "      defaultScope: project", "      defaultScope: project\n      mode: x")
	_, err := skills.DecodeRegistry(registryyaml.NewRepository(nil), []byte(doc))
	if err == nil || !strings.Contains(err.Error(), `install.targets contains invalid value "vim"`) {
		t.Errorf("DecodeRegistry() = %v, want the refusal of the target", err)
	}
}

// --- the must-understand set -------------------------------------------------------------------

// The set the package documents, which is the contract: a change to it is a decision, so the test
// says what it is. The rationale of each field is at the top of the package (doc.go), and the next
// test reads it from there.
var documentedMustUnderstand = []string{
	"id", "install", "install.allowedProjects", "install.defaultScope", "install.targets",
	"path", "skills", "source", "source.type", "version",
}

func TestTheMustUnderstandSetIsTheDocumentedOne(t *testing.T) {
	got := registryyaml.MustUnderstand("1")
	if !sort.StringsAreSorted(got) {
		t.Errorf("MustUnderstand(\"1\") = %v, want it sorted", got)
	}
	if !reflect.DeepEqual(got, documentedMustUnderstand) {
		t.Errorf("MustUnderstand(\"1\") = %v, want %v", got, documentedMustUnderstand)
	}
	if len(got) > 0 {
		got[0] = "tampered"
		if registryyaml.MustUnderstand("1")[0] == "tampered" {
			t.Error("MustUnderstand returned its own list: a caller changed the set")
		}
	}
	if registryyaml.MustUnderstand("2") != nil || registryyaml.MustUnderstand("") != nil {
		t.Error("MustUnderstand of a version no decoder is for must be nil: there is no set to say")
	}
}

// The package documents the set at its top, one line of rationale per field, and that text is the
// one the code answers to: a field added to the set and not explained, or explained and not in the
// set, fails here.
func TestTheMustUnderstandSetIsExplainedAtTheTopOfThePackage(t *testing.T) {
	text, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	section := string(text)
	start := strings.Index(section, "The must-understand set of version 1")
	end := strings.Index(section, "Fields the reader may leave out")
	if start < 0 || end < start {
		t.Fatal("doc.go has no section 'The must-understand set of version 1' followed by 'Fields the reader may leave out'")
	}
	var documented []string
	line := regexp.MustCompile(`^//\t([a-zA-Z.]+) {2,}(\S.*)$`)
	for _, l := range strings.Split(section[start:end], "\n") {
		if m := line.FindStringSubmatch(l); m != nil {
			documented = append(documented, m[1])
		}
	}
	sort.Strings(documented)
	if !reflect.DeepEqual(documented, registryyaml.MustUnderstand("1")) {
		t.Errorf("doc.go explains the fields %v, the set is %v", documented, registryyaml.MustUnderstand("1"))
	}
}

// For every field of version 1, in a shape that is not its own: a field of the set is refused,
// naming the field and the line; any other is left out and said, like one the reader does not
// know. Together with the test of the set, this is what makes the set the set: it is read from the
// behavior of each field, not from a list.
func TestAFieldIsReadInItsOwnShapeAndOtherwiseRefusedOrLeftOutByTheSet(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	const (
		scalarWord = "a single value, not a block"
		blockWord  = "a block of fields"
		listWord   = `a list of values, one "- value" line each`
	)
	type shaped struct {
		field string
		doc   string
		line  string // the line the document gives the field a shape that is not its own
		must  string // the refusal, after "line N: ", for a field of the set
		note  string // the note, after "line N: ", for a field that is not
	}
	cases := []shaped{
		{field: "version", doc: "version:\n  nested: 1\nskills:\n", line: "version:", must: "version must be " + scalarWord},
		{field: "skills", doc: "version: \"1\"\nskills: nope\n", line: "skills: nope", must: "skills must be a list of entries, not a value"},
		{field: "id", doc: replaceLine(t, policyDoc, "  - id: alpha", "  - id: alpha\n      nested: 1"), line: "  - id: alpha", must: "id must be " + scalarWord},
		{field: "path", doc: replaceLine(t, policyDoc, "    path: alpha", "    path: alpha\n      nested: 1"), line: "    path: alpha", must: "path must be " + scalarWord},
		{field: "source", doc: replaceLine(t, policyDoc, "    source:", "    source: custom"), line: "    source: custom",
			must: "source must be " + blockWord + " (type, upstream, repo, ref), not a value"},
		{field: "source.type", doc: replaceLine(t, policyDoc, "      type: core", "      type: core\n        nested: 1"), line: "      type: core", must: "source.type must be " + scalarWord},
		{field: "install", doc: replaceLine(t, policyDoc, "    install:", "    install: global"), line: "    install: global",
			must: "install must be " + blockWord + " (defaultScope, allowedProjects, targets), not a value"},
		{field: "install.defaultScope", doc: replaceLine(t, policyDoc, "      defaultScope: project", "      defaultScope: project\n        nested: 1"), line: "      defaultScope: project",
			must: "install.defaultScope must be " + scalarWord},
		{field: "install.allowedProjects", doc: replaceLine(t, policyDoc, "      allowedProjects:", "      allowedProjects: demo"), line: "      allowedProjects: demo",
			must: "install.allowedProjects must be " + listWord},
		{field: "install.targets", doc: replaceLine(t, policyDoc, "      targets:", "      targets: claude"), line: "      targets: claude",
			must: "install.targets must be " + listWord},
		// What nothing decides on: provenance, and what is only shown.
		{field: "source.upstream", doc: replaceLine(t, policyDoc, "      upstream:", "      upstream: someone"), line: "      upstream: someone",
			note: "source.upstream has a value, which is left out"},
		{field: "source.upstream.owner", doc: replaceLine(t, policyDoc, "        owner: gentleman-programming", "        owner:\n          nested: 1"), line: "        owner:",
			note: "source.upstream.owner is not a single value"},
		{field: "source.ref", doc: replaceLine(t, policyDoc, "      ref: v1", "      ref:\n        nested: 1"), line: "      ref:",
			note: "source.ref is not a single value"},
		{field: "lifecycle", doc: replaceLine(t, policyDoc, "    lifecycle:", "    lifecycle: vendor-merge"), line: "    lifecycle: vendor-merge",
			note: "lifecycle has a value, which is left out"},
		{field: "lifecycle.updateStrategy", doc: replaceLine(t, policyDoc, "      updateStrategy: vendor-merge", "      updateStrategy:\n        nested: 1"), line: "      updateStrategy:",
			note: "lifecycle.updateStrategy is not a single value"},
	}
	// source.repo is the one field whose absence the reader itself refuses (an external source
	// without a repository), so it has a test of its own, below.
	covered := map[string]bool{"source.repo": true}
	var must []string
	for _, tc := range cases {
		covered[tc.field] = true
		if tc.must != "" {
			must = append(must, tc.field)
		}
		t.Run(tc.field, func(t *testing.T) {
			reg, err := repo.Decode([]byte(tc.doc))
			if tc.must != "" {
				want := fmt.Sprintf("line %d: %s", lineOf(t, tc.doc, tc.line), tc.must)
				if err == nil || err.Error() != want {
					t.Errorf("Decode() = %v, want %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode() = %v, want %s left out and the rest read", err, tc.field)
			}
			want := []string{fmt.Sprintf("line %d: %s", lineOf(t, tc.doc, tc.line), tc.note)}
			if !reflect.DeepEqual(reg.Unread, want) {
				t.Errorf("left out %q, want %q", reg.Unread, want)
			}
			if len(reg.Skills) != 2 {
				t.Errorf("read %d entries, want both: the rest of the file is read as ever", len(reg.Skills))
			}
		})
	}
	// The table covers every field of version 1, and the fields it expects refused are the set.
	if len(covered) != 16 {
		t.Errorf("the table covers %d fields, want the 16 of version 1", len(covered))
	}
	sort.Strings(must)
	if !reflect.DeepEqual(must, registryyaml.MustUnderstand("1")) {
		t.Errorf("the fields refused in another shape are %v, the must-understand set is %v", must, registryyaml.MustUnderstand("1"))
	}
}

// What a field left out leaves behind is what the file does not say: a block under a value that
// should be one is not read, and the rest is as the file says.
func TestABlockUnderAValueIsLeftOutOfTheRegistryThatIsRead(t *testing.T) {
	clean := decoded(t, policyDoc)

	noStrategy := decoded(t, replaceLine(t, policyDoc, "      updateStrategy: vendor-merge", "      updateStrategy:\n        nested: 1"))
	if got := noStrategy.Skills[0].Lifecycle.UpdateStrategy; got != "" {
		t.Errorf("an update strategy with a block under it was read as %q, want none", got)
	}
	noRef := decoded(t, replaceLine(t, policyDoc, "      ref: v1", "      ref:\n        nested: 1"))
	if got := noRef.Skills[1].Source.Ref; got != "" {
		t.Errorf("a ref with a block under it was read as %q, want none", got)
	}
	noOwner := decoded(t, replaceLine(t, policyDoc, "        owner: gentleman-programming", "        owner:\n          nested: 1"))
	if up := noOwner.Skills[0].Source.Upstream; up == nil || up.Owner != "" {
		t.Errorf("an owner with a block under it was read as %+v, want an upstream with no owner", up)
	}
	// Nothing else of the file is read differently.
	noRef.Skills = append([]skills.Entry(nil), noRef.Skills...)
	noRef.Skills[1].Source.Ref = "v1"
	if !reflect.DeepEqual(withNothingUnread(noRef), clean) {
		t.Errorf("the rest of the file was read differently: %+v, want %+v", withNothingUnread(noRef), clean)
	}
}

// The value on the line of a block that nothing depends on is what is left out, and the block under
// it is read, as it was when the value was ignored without a word: no registry that was read is read
// differently, it only says so now.
func TestTheValueOnTheLineOfABlockIsLeftOutAndTheBlockIsRead(t *testing.T) {
	clean := decoded(t, policyDoc)
	for name, line := range map[string][2]string{
		"an upstream": {"      upstream:", "      upstream: someone"},
		"a lifecycle": {"    lifecycle:", "    lifecycle: vendor-merge"},
	} {
		t.Run(name, func(t *testing.T) {
			reg := decoded(t, replaceLine(t, policyDoc, line[0], line[1]))
			if len(reg.Unread) != 1 || !reflect.DeepEqual(withNothingUnread(reg), clean) {
				t.Errorf("read %+v, want the registry the file says and one note", reg)
			}
		})
	}
	// With nothing under it, the block is empty, as it always was (an upstream that is there and has
	// no owner, which the domain refuses for a core skill).
	bare := replaceLine(t, replaceLine(t, policyDoc, "      upstream:", "      upstream: someone"), "        owner: gentleman-programming", "")
	if up := decoded(t, bare).Skills[0].Source.Upstream; up == nil || up.Owner != "" {
		t.Errorf("an upstream with a value and nothing under it was read as %+v, want an upstream with no owner", up)
	}
}

// An external source whose repository cannot be read is an external source without one, which the
// reader refuses as it refuses any: leaving the field out never admits a source the file does not
// describe.
func TestARepositoryLeftOutOfAnExternalSourceIsMissing(t *testing.T) {
	doc := replaceLine(t, policyDoc, "      repo: https://example.test/org/beta", "      repo:\n        nested: 1")
	_, err := registryyaml.NewRepository(nil).Decode([]byte(doc))
	want := fmt.Sprintf("skills: entry %q: source.repo is required when source.type is 'external' (line %d)", "beta", lineOf(t, doc, "      type: external")-1)
	if err == nil || err.Error() != want {
		t.Errorf("Decode() = %v, want %q", err, want)
	}
}

// --- a registry that was not read whole is not written ------------------------------------------

func TestTheFileIsNotWrittenFromARegistryThatLeftFieldsOut(t *testing.T) {
	reg := decoded(t, "extra: 1\n"+policyDoc)
	out, err := registryyaml.NewRepository(nil).Encode(reg)
	if err == nil || out != nil {
		t.Fatalf("Encode() = %q, %v, want a refusal and no bytes: they would drop what was left out", out, err)
	}
	// The rule is the domain's, and so are its words: one rule, one wording, whichever way the
	// registry is refused (a verb that changes it, or the adapter that would write it).
	if want := reg.CheckWritable(); want == nil || err.Error() != want.Error() {
		t.Errorf("Encode() error = %q, want the words of the domain's rule, %v", err, want)
	}
	if want := "line 1: unknown top-level key \"extra\""; !strings.Contains(err.Error(), want) {
		t.Errorf("Encode() error = %q, want it to say %q", err, want)
	}
	// What was read whole is written as ever.
	if _, err := registryyaml.NewRepository(nil).Encode(withNothingUnread(reg)); err != nil {
		t.Errorf("Encode() of the registry without the note = %v, want it written", err)
	}
}

// An unknown key under source does not refuse the file: the entry is read as it is and usable, and
// the key is left out and reported with its line (the scenario "An unknown key under source is
// tolerated and reported" of openspec/specs/skill-package-manager/spec.md).
func TestAnUnknownKeyUnderSourceIsToleratedAndReportedWithItsLine(t *testing.T) {
	const doc = `version: "1"
skills:
  - id: alpha
    path: alpha
    source:
      type: external
      repo: https://example.test/skills
      mirror: elsewhere
      ref: v1
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
	reg, err := readRegistry(doc)
	if err != nil {
		t.Fatalf("a registry with an unknown key under source = %v, want it read", err)
	}
	if want := []string{fmt.Sprintf(`line %d: unknown key "mirror" in source`, lineOf(t, doc, "      mirror: elsewhere"))}; !reflect.DeepEqual(reg.Unread, want) {
		t.Errorf("left out %q, want %q", reg.Unread, want)
	}
	if len(reg.Skills) != 1 || reg.Skills[0].Source.Repo != "https://example.test/skills" || reg.Skills[0].Source.Ref != "v1" {
		t.Errorf("the entry came back as %+v, want it read, with the repo and the ref it says on both sides of the key", reg.Skills)
	}
}
