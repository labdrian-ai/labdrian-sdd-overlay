package registryyaml

// The dispatch on the version of the file, tested with decoders of its own: with only version 1
// written, nothing outside the package could tell a dispatch from a decoder that is always the
// one, so the package tests the dispatch with a second version that does something else.

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

func TestTheVersionOfTheFileSelectsTheDecoderThatReadsIt(t *testing.T) {
	var ran []string
	versionRan := func(version string) decoder {
		return func(tokens []tok) (skills.Registry, error) {
			ran = append(ran, version)
			return skills.Registry{Skills: []skills.Entry{{ID: "read-by-" + version}}}, nil
		}
	}
	versions := map[string]decoder{"1": versionRan("1"), "2": versionRan("2")}

	for _, version := range []string{"2", "1"} {
		ran = nil
		reg, err := decodeWith(versions, strings.NewReader("version: \""+version+"\"\nskills:\n"))
		if err != nil || strings.Join(ran, ",") != version {
			t.Fatalf("version %s: decoders run %v, error %v, want only the decoder of version %s", version, ran, err, version)
		}
		if reg.Version != version || len(reg.Skills) != 1 || reg.Skills[0].ID != "read-by-"+version {
			t.Errorf("version %s: read as %+v, want what its decoder read and the version it was chosen by", version, reg)
		}
	}

	ran = nil
	if _, err := decodeWith(versions, strings.NewReader("version: \"3\"\nskills:\n")); err == nil || len(ran) != 0 {
		t.Errorf("version 3: decoders run %v, error %v, want a refusal and no decoder run", ran, err)
	}
}

// A decoder is handed the tokens of the whole file, and what it returns with an error is what it
// had read: the dispatch adds the version and nothing else.
func TestAFailingDecoderHandsBackWhatItReadWithTheVersion(t *testing.T) {
	versions := map[string]decoder{"1": func(tokens []tok) (skills.Registry, error) {
		return skills.Registry{Skills: []skills.Entry{{ID: "whole"}}, Unread: []string{"line 1: a"}}, errTest
	}}
	reg, err := decodeWith(versions, strings.NewReader("version: \"1\"\nskills:\n"))
	if err != errTest || reg.Version != "1" || len(reg.Skills) != 1 || len(reg.Unread) != 1 {
		t.Errorf("decodeWith() = %+v, %v, want the entries and the notes of the decoder, with the version and its error", reg, err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

const errTest = testError("the decoder failed")

func TestTheRefusalOfAVersionSaysWhichVersionsAreValid(t *testing.T) {
	for name, tc := range map[string]struct {
		versions []string
		want     string
	}{
		"one version":     {[]string{"1"}, `skills: version "9" is not supported; only version "1" is valid`},
		"two versions":    {[]string{"1", "2"}, `skills: version "9" is not supported; the valid versions are "1", "2"`},
		"sorted, as text": {[]string{"10", "2", "1"}, `skills: version "9" is not supported; the valid versions are "1", "10", "2"`},
	} {
		t.Run(name, func(t *testing.T) {
			versions := map[string]decoder{}
			for _, v := range tc.versions {
				versions[v] = func([]tok) (skills.Registry, error) { return skills.Registry{}, nil }
			}
			_, err := decodeWith(versions, strings.NewReader("version: \"9\"\nskills:\n"))
			if err == nil || err.Error() != tc.want {
				t.Errorf("decodeWith() = %v, want %q", err, tc.want)
			}
		})
	}
}

// A byte-order mark before the first key makes that key not "version", and the file is refused
// for having no version. The refusal names the key as it is, with the mark, because that is the one
// clue that points at the encoding of the file and not at a version that is missing.
func TestAByteOrderMarkBeforeTheVersionIsNamedInTheRefusal(t *testing.T) {
	_, err := decodeWith(decoders, strings.NewReader("\ufeffversion: \"1\"\nskills:\n"))
	want := `line 1: the top-level key "\ufeffversion" starts with a byte-order mark; save the file without one`
	if err == nil || err.Error() != want {
		t.Errorf("decodeWith() = %v, want %q", err, want)
	}

	// A file that has no version at all is still refused for that.
	_, err = decodeWith(decoders, strings.NewReader("skills:\n"))
	if err == nil || err.Error() != "skills: missing required top-level field 'version'" {
		t.Errorf("decodeWith() of a file with no version = %v, want it refused for having none", err)
	}
}

// The versions this package reads are listed twice, by the decoder that reads each and by the
// table of its fields (and so its must-understand set). A version in one and not the other would
// read a file without the set that guards it, which is what this test is here to catch.
func TestEveryVersionThatHasADecoderHasItsFields(t *testing.T) {
	for version := range decoders {
		if _, ok := schemas[version]; !ok {
			t.Errorf("version %q has a decoder and no table of fields", version)
		}
	}
	for version := range schemas {
		if _, ok := decoders[version]; !ok {
			t.Errorf("version %q has a table of fields and no decoder", version)
		}
	}
}

// A decoder that asks for a field its table does not have is a fault of the program, and it is
// refused like one instead of crashing the verb that read the file.
func TestAFieldTheSchemaDoesNotHaveIsRefusedNotAPanic(t *testing.T) {
	p := &tokParser{schema: newSchema(field{path: "id", shape: shapeScalar})}
	read, err := p.open("path", tok{kind: tokKeyValue, key: "path", val: "x", lineNum: 3}, 0)
	if read || err == nil || !strings.Contains(err.Error(), `no field "path"`) {
		t.Errorf("open() of a field the schema does not have = %v, %v, want it not read and an error that names the field", read, err)
	}
}
