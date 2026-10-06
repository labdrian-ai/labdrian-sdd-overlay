package skills

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Decision 6 of the owner (2026-10-05): two ids on the same path stay accepted (they have always
// been: the golden 'two entries with the same path'), and 'validate' says so in a note, which is
// not a failure.

func entryAt(id, path string) Entry {
	e := validEntry(id)
	e.Path = path
	return e
}

func TestTheSharedPathsOfARegistryAreTheOnesTwoIdsHold(t *testing.T) {
	for name, tc := range map[string]struct {
		reg  Registry
		want []SharedPath
	}{
		"no entries":   {registryOfEntries(), nil},
		"paths of one": {registryOfEntries(validEntry("alpha"), validEntry("beta")), nil},
		"two on one":   {registryOfEntries(entryAt("alpha", "dir"), entryAt("beta", "dir")), []SharedPath{{Path: "dir", IDs: []string{"alpha", "beta"}}}},
		"three on one": {registryOfEntries(entryAt("c", "dir"), entryAt("a", "dir"), entryAt("b", "dir")), []SharedPath{{Path: "dir", IDs: []string{"c", "a", "b"}}}},
		"in the order of the first one": {
			registryOfEntries(entryAt("a", "x"), entryAt("b", "y"), entryAt("c", "y"), entryAt("d", "x")),
			[]SharedPath{{Path: "x", IDs: []string{"a", "d"}}, {Path: "y", IDs: []string{"b", "c"}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.reg.SharedPaths(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SharedPaths() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTheNoteOfASharedPathNamesTheIdsAndThePath(t *testing.T) {
	for want, shared := range map[string]SharedPath{
		`note: the skills "alpha" and "beta" share the path "dir"`:    {Path: "dir", IDs: []string{"alpha", "beta"}},
		`note: the skills "a", "b" and "c" share the path "dir"`:      {Path: "dir", IDs: []string{"a", "b", "c"}},
		`note: the skills "a", "b", "c" and "d" share the path "x/y"`: {Path: "x/y", IDs: []string{"a", "b", "c", "d"}},
	} {
		if got := shared.Note(); got != want {
			t.Errorf("Note() = %q, want %q", got, want)
		}
	}
}

// The rule that refuses a registry still accepts two ids on one path.
func TestARegistryWithTwoIdsOnOnePathIsValid(t *testing.T) {
	if err := registryOfEntries(entryAt("alpha", "dir"), entryAt("beta", "dir")).Validate(); err != nil {
		t.Errorf("Validate() = %v, want two ids on one path accepted", err)
	}
}

func TestValidateNotesTheIdsThatShareAPathAndStillPasses(t *testing.T) {
	regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\n")
	data, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	second := strings.Replace(string(data), "id: sdd-spec", "id: sdd-spec-copy", 1)
	second = strings.TrimPrefix(second, "version: \"1\"\nskills:\n")
	if err := os.WriteFile(regPath, append(data, second...), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := 0
	RenderValidateCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
		os.ReadFile, testRegistries(os.ReadFile), stubScan([]string{"sdd-spec/SKILL.md"}), &out, &errBuf,
		func(c int) { code = c },
	)
	const note = `note: the skills "sdd-spec" and "sdd-spec-copy" share the path "sdd-spec"` + "\n"
	if errBuf.String() != note {
		t.Errorf("stderr = %q, want only %q", errBuf.String(), note)
	}
	if code != 0 || !strings.Contains(out.String(), "registry and manifest aligned (2 skills)") {
		t.Errorf("exit %d, stdout %q, want the note to change nothing of the outcome", code, out.String())
	}
}
