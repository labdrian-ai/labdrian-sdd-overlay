package registryyaml_test

// Tests for the adapter as an implementation of skills.RegistryRepository: where a registry is
// read from, what is said of a store that cannot be read, and the one contract that makes the
// domain's judgment come out in the order the file says things (what a decoder that fails hands
// back). What each YAML construct reads as, and each refusal's words, are read_test.go and the
// golden files of engine/cmd.

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

const oneEntry = `version: "1"
skills:
  - id: alpha
    path: alpha
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// anEntry is the YAML of one more valid entry, to be added under skills.
func anEntry(id string) string {
	return "  - id: " + id + "\n    path: " + id + "\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
}

// --- where a registry is read from ---------------------------------------------------------

func TestLoadReadsTheFileAtTheLocationThroughTheFunctionItWasBuiltWith(t *testing.T) {
	var asked []string
	repo := registryyaml.NewRepository(func(path string) ([]byte, error) {
		asked = append(asked, path)
		return []byte(oneEntry), nil
	})
	reg, err := repo.Load("some/dir/skills.registry.yaml")
	if err != nil || len(reg.Skills) != 1 || reg.Skills[0].ID != "alpha" || reg.Version != "1" {
		t.Fatalf("Load() = %+v, %v, want the registry of the file", reg, err)
	}
	if len(asked) != 1 || asked[0] != "some/dir/skills.registry.yaml" {
		t.Errorf("the file read was %v, want exactly the location given", asked)
	}
}

// A store that cannot be read is the one error the domain does not judge: it says so with its own
// type, carries the reader's error whole (its words reach a person), and returns nothing.
func TestLoadTellsAStoreThatCannotBeReadWithTheReadersOwnError(t *testing.T) {
	cause := &fs.PathError{Op: "open", Path: "r.yaml", Err: fs.ErrNotExist}
	repo := registryyaml.NewRepository(func(string) ([]byte, error) { return []byte(oneEntry), cause })
	reg, err := repo.Load("r.yaml")
	var unreadable *skills.RegistryReadError
	if !errors.As(err, &unreadable) || !errors.Is(err, fs.ErrNotExist) || err.Error() != cause.Error() {
		t.Fatalf("Load() = %v, want a *RegistryReadError carrying %v", err, cause)
	}
	if len(reg.Skills) != 0 || reg.Version != "" {
		t.Errorf("Load() returned %+v with its error, want the zero registry", reg)
	}
}

func TestARepositoryBuiltWithNoReaderCannotLoadAndSaysSo(t *testing.T) {
	for name, repo := range map[string]registryyaml.Repository{
		"the zero value": {},
		"a nil function": registryyaml.NewRepository(nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := repo.Load("r.yaml")
			var unreadable *skills.RegistryReadError
			if !errors.As(err, &unreadable) || !strings.Contains(err.Error(), "no way to read a file") {
				t.Errorf("Load() = %v, want a *RegistryReadError that says there is no way to read a file", err)
			}
		})
	}
	// What needs no file still works.
	if _, err := (registryyaml.Repository{}).Decode([]byte(oneEntry)); err != nil {
		t.Errorf("Decode() on the zero repository = %v, want it to decode", err)
	}
}

func TestDecodeAndEncodeAreTheStoredFormOfTheRegistry(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	reg, err := repo.Decode([]byte(oneEntry))
	if err != nil {
		t.Fatal(err)
	}
	out, err := repo.Encode(reg)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != oneEntry {
		t.Errorf("Encode(Decode(file)) = %q, want the file back (it is in the form the program writes)", out)
	}
}

// --- what a decoder that fails hands back ---------------------------------------------------

// A decoder that stops at a fault hands back the entries that were whole before it, in order,
// so the domain can judge them first; a fault of the text itself, before anything was read,
// hands back nothing.
func TestADecoderThatFailsHandsBackTheEntriesThatWereWholeBeforeTheFault(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	for name, tc := range map[string]struct {
		doc     string
		wantIDs []string
		want    string // a piece of the error
	}{
		"an unknown key in the second entry": {
			oneEntry + anEntry("beta")[:len("  - id: beta\n")] + "    color: red\n", []string{"alpha"}, `unknown key "color" in skill entry`},
		"an unknown key at the root, after the entries": {
			oneEntry + anEntry("beta") + "extra: 1\n", []string{"alpha", "beta"}, `unknown top-level key "extra"`},
		"an entry that is whole and a token that is not where it should be": {
			oneEntry + "   stray: 1\n", []string{"alpha"}, "unexpected indentation inside skills sequence"},
		"a version the decoder does not know, after the entries": {
			strings.Replace(oneEntry+anEntry("beta"), `"1"`, `"9"`, 1), []string{"alpha", "beta"}, `version "9" is not supported`},
		"no version, after the entries": {
			strings.Replace(oneEntry+anEntry("beta"), "version: \"1\"\n", "", 1), []string{"alpha", "beta"}, "missing required top-level field 'version'"},
		"a fault of the text itself": {
			oneEntry + "\ttab: here\n", nil, "tab character not allowed"},
		"a fault in the first entry": {
			strings.Replace(oneEntry, "    path: alpha\n", "    path: alpha\n    color: red\n", 1), nil, `unknown key "color"`},
	} {
		t.Run(name, func(t *testing.T) {
			reg, err := repo.Decode([]byte(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Decode() error = %v, want it to say %q", err, tc.want)
			}
			var ids []string
			for _, e := range reg.Skills {
				ids = append(ids, e.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("Decode() handed back entries %v with its error, want %v", ids, tc.wantIDs)
			}
		})
	}
}

// What that contract is for, end to end: an entry that is invalid is named before a later fault of
// the file, as it was when the file was read entry by entry and each was judged as soon as it was
// whole. A line of the entry that is indented wrongly ends the entry early and makes it invalid
// (it has no path), and it is that that is named, not the stray line after it.
func TestTheFirstFaultInTheOrderOfTheFileIsTheOneTheDomainNames(t *testing.T) {
	repo := registryyaml.NewRepository(nil)
	for name, tc := range map[string]struct{ doc, want string }{
		"a path indented too little: the entry has none, which comes before the stray line": {
			strings.Replace(oneEntry, "    path: alpha\n", "   path: alpha\n", 1),
			`skills: entry "alpha": path must not be empty`},
		"an invalid first entry before an unknown key in the second": {
			strings.Replace(oneEntry, "path: alpha", `path: ""`, 1) + "  - id: beta\n    color: red\n",
			`skills: entry "alpha": path must not be empty`},
		"a valid first entry before an unknown key in the second": {
			oneEntry + "  - id: beta\n    color: red\n",
			`line 14: unknown key "color" in skill entry`},
		"an invalid entry before a version the file gets wrong": {
			strings.Replace(strings.Replace(oneEntry, "path: alpha", `path: ""`, 1), `"1"`, `"7"`, 1),
			`skills: entry "alpha": path must not be empty`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := skills.DecodeRegistry(repo, []byte(tc.doc))
			if err == nil || err.Error() != tc.want {
				t.Errorf("DecodeRegistry() = %v, want %q", err, tc.want)
			}
		})
	}
}
