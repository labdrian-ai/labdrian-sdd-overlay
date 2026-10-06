package main

import (
	"errors"
	"reflect"
	"testing"
)

// The parser is held to a table: what each form of a command line splits into, and what is
// refused in which words.

func TestSkillsFlagParserSplitsACommandLine(t *testing.T) {
	spec := skillsFlagSpec{
		verb:         "demo",
		values:       []string{"--registry", "--id"},
		switches:     []string{"--rules"},
		wrapper:      []string{"--manifest", "--source-root"},
		words:        2,
		extraWord:    "skills demo: unexpected word %q",
		endOfOptions: true,
	}
	for _, tc := range []struct {
		name     string
		args     []string
		values   map[string]string
		switches map[string]bool
		words    []string
	}{
		{"nothing", nil, nil, nil, nil},
		{"a value flag", []string{"--registry", "r.yaml"}, map[string]string{"--registry": "r.yaml"}, nil, nil},
		{"the last of a repeated value flag wins", []string{"--registry", "a", "--registry", "b"}, map[string]string{"--registry": "b"}, nil, nil},
		{"a value flag with no value, last, is unset", []string{"--registry"}, nil, nil, nil},
		{"a value flag takes the next word even when it is a flag", []string{"--registry", "--rules"}, map[string]string{"--registry": "--rules"}, nil, nil},
		{"the empty word is a value", []string{"--registry", ""}, map[string]string{"--registry": ""}, nil, nil},
		{"a switch", []string{"--rules"}, nil, map[string]bool{"--rules": true}, nil},
		{"a switch given twice", []string{"--rules", "--rules"}, nil, map[string]bool{"--rules": true}, nil},
		{"the flags of the wrapper are taken with their value and read by no one", []string{"--manifest", "m", "--source-root", "s", "a.md"}, nil, nil, []string{"a.md"}},
		{"a flag of the wrapper with no value, last", []string{"a.md", "--source-root"}, nil, nil, []string{"a.md"}},
		{"a flag of the wrapper takes the next word, which is then no word", []string{"--source-root", "a.md"}, nil, nil, nil},
		{"words in order", []string{"a", "b"}, nil, nil, []string{"a", "b"}},
		{"-- ends the flags", []string{"--", "--rules", "-x"}, nil, nil, []string{"--rules", "-x"}},
		{"-- is not a word", []string{"--"}, nil, nil, nil},
		{"-- twice: the second is a word", []string{"--", "--"}, nil, nil, []string{"--"}},
		{"a flag after a word is still a flag", []string{"a", "--rules"}, nil, map[string]bool{"--rules": true}, []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := spec.parse(tc.args)
			if err != nil {
				t.Fatalf("parse(%q): %v", tc.args, err)
			}
			if !reflect.DeepEqual(got.values, orEmptyValues(tc.values)) {
				t.Errorf("values = %v, want %v", got.values, tc.values)
			}
			if !reflect.DeepEqual(got.switches, orEmptySwitches(tc.switches)) {
				t.Errorf("switches = %v, want %v", got.switches, tc.switches)
			}
			if !reflect.DeepEqual(got.words, tc.words) {
				t.Errorf("words = %q, want %q", got.words, tc.words)
			}
		})
	}
}

func orEmptyValues(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func orEmptySwitches(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return m
}

func TestSkillsFlagParserRefusesAFlagNobodyNamed(t *testing.T) {
	spec := skillsFlagSpec{verb: "demo", values: []string{"--registry"}, switches: []string{"--rules"}, wrapper: []string{"--manifest"}, words: -1, endOfOptions: true}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"a long flag", []string{"--frobnicate"}, `skills demo: unknown flag "--frobnicate"`},
		{"a short flag", []string{"-v"}, `skills demo: unknown flag "-v"`},
		{"a lone dash", []string{"-"}, `skills demo: unknown flag "-"`},
		{"the equals form of a flag it reads", []string{"--registry=r"}, `skills demo: unknown flag "--registry=r"`},
		{"the equals form of a flag of the wrapper", []string{"--manifest=m"}, `skills demo: unknown flag "--manifest=m"`},
		{"an unknown flag after words and valid flags", []string{"a", "--registry", "r", "--nope", "x"}, `skills demo: unknown flag "--nope"`},
		{"the first unknown flag is the one named", []string{"--one", "--two"}, `skills demo: unknown flag "--one"`},
		{"a case that differs", []string{"--Registry", "r"}, `skills demo: unknown flag "--Registry"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.parse(tc.args)
			var usage *skillsUsageError
			if !errors.As(err, &usage) || err.Error() != tc.want {
				t.Errorf("parse(%q) error = %v, want %q", tc.args, err, tc.want)
			}
		})
	}
}

func TestSkillsFlagParserTakesAValueThatLooksLikeAFlag(t *testing.T) {
	// A value is whatever word follows its flag: it is not read as a flag, so it is not refused.
	spec := skillsFlagSpec{verb: "demo", values: []string{"--registry"}, words: -1}
	got, err := spec.parse([]string{"--registry", "--not-a-flag"})
	if err != nil || got.value("--registry", "default") != "--not-a-flag" {
		t.Errorf("parse = %+v, %v, want the word taken as the value", got, err)
	}
}

func TestSkillsFlagParserLimitsTheWords(t *testing.T) {
	one := skillsFlagSpec{verb: "demo", words: 1, extraWord: "skills demo: unexpected extra argument %q"}
	if _, err := one.parse([]string{"a"}); err != nil {
		t.Errorf("one word under a limit of one: %v", err)
	}
	_, err := one.parse([]string{"a", "b", "c"})
	if err == nil || err.Error() != `skills demo: unexpected extra argument "b"` {
		t.Errorf("error = %v, want the first word past the limit named", err)
	}
	none := skillsFlagSpec{verb: "demo", words: 0, extraWord: "skills demo takes no word, got %q"}
	if _, err := none.parse([]string{"x"}); err == nil || err.Error() != `skills demo takes no word, got "x"` {
		t.Errorf("error = %v, want the word refused", err)
	}
	any := skillsFlagSpec{verb: "demo", words: -1}
	if got, err := any.parse([]string{"a", "b", "c"}); err != nil || len(got.words) != 3 {
		t.Errorf("parse = %+v, %v, want every word kept when there is no limit", got, err)
	}
}

func TestSkillsFlagParserWithoutEndOfOptionsRefusesTheDoubleDash(t *testing.T) {
	spec := skillsFlagSpec{verb: "demo", words: -1}
	if _, err := spec.parse([]string{"--"}); err == nil {
		t.Error(`a verb that does not take "--" must refuse it as an unknown flag`)
	}
}

func TestSkillsRegistryReaderSpecReadsTheRegistryAndTakesTheRest(t *testing.T) {
	spec := skillsRegistryReaderSpec("list")
	got, err := spec.parse([]string{"--registry", "r", "--manifest", "m", "--source-root", "s", "list", "extra"})
	if err != nil {
		t.Fatal(err)
	}
	if got.value(flagRegistry, "d") != "r" || len(got.words) != 2 {
		t.Errorf("parse = %+v, want the registry read and both words kept and ignored", got)
	}
	if _, err := spec.parse([]string{"--project-id", "x"}); err == nil || err.Error() != `skills list: unknown flag "--project-id"` {
		t.Errorf("error = %v, want a flag of another verb refused", err)
	}
}

func TestSkillsLintSpecAcceptsOnePathAndTheRules(t *testing.T) {
	got, err := skillsLintSpec.parse([]string{"--rules", "--registry", "r", "a.md"})
	if err != nil || !got.switches["--rules"] || !reflect.DeepEqual(got.words, []string{"a.md"}) {
		t.Errorf("parse = %+v, %v", got, err)
	}
	_, err = skillsLintSpec.parse([]string{"a.md", "b.md"})
	if err == nil || err.Error() != `skills lint: unexpected extra argument "b.md" (lint accepts exactly one path)` {
		t.Errorf("error = %v, want the second path refused in the words of lint", err)
	}
}

// A verb whose value is never a flag (approve) refuses a value that begins with a dash, and a value
// flag that is the last word, instead of taking the next flag for its value or leaving it unset.
func TestSkillsFlagParserCanRefuseAValueThatIsAFlagOrMissing(t *testing.T) {
	spec := skillsFlagSpec{verb: "demo", values: []string{"--id", "--label"}, wrapper: []string{"--manifest"}, words: 0,
		extraWord: "skills demo: unexpected argument %q", valueIsNeverAFlag: true,
		dashValue: func(flag, value string) string {
			if flag == "--label" {
				return "skills demo: the label " + value + " starts with a dash"
			}
			return ""
		}}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"a value that is a flag", []string{"--id", "--label", "x"}, `skills demo: flag "--id" requires a value; got flag token "--label"`},
		{"a value that is a short flag", []string{"--id", "-x"}, `skills demo: flag "--id" requires a value; got flag token "-x"`},
		{"the last word is a value flag", []string{"--label", "x", "--id"}, `skills demo: flag "--id" requires a value`},
		{"a flag whose refusal is worded for it", []string{"--label", "-x"}, "skills demo: the label -x starts with a dash"},
		{"a flag of the wrapper that is the last word", []string{"--manifest"}, `skills demo: flag "--manifest" requires a value`},
		{"a flag of the wrapper followed by a flag", []string{"--manifest", "--id"}, `skills demo: flag "--manifest" requires a value; got flag token "--id"`},
		{"a word, where there is none", []string{"word"}, `skills demo: unexpected argument "word"`},
		{"-- is no end of options and is the flag it looks like", []string{"--"}, `skills demo: unknown flag "--"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.parse(tc.args)
			var usage *skillsUsageError
			if !errors.As(err, &usage) || err.Error() != tc.want {
				t.Errorf("parse(%q) error = %v, want %q", tc.args, err, tc.want)
			}
		})
	}
	got, err := spec.parse([]string{"--id", "a", "--manifest", "m", "--label", "b"})
	if err != nil || got.value("--id", "") != "a" || got.value("--label", "") != "b" {
		t.Errorf("parse of the valid form = %+v, %v", got, err)
	}
}

// What each verb that writes reads: the flags it takes, the ones the wrapper appends that it does
// not, and what is no flag.
func TestSkillsFlagSpecsOfTheWritersSayWhatEachVerbReads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		spec   skillsFlagSpec
		args   []string
		values map[string]string
		words  []string
	}{
		{"add reads the registry, the manifest, the source root, the repo and the ref", skillsAddSpec,
			[]string{"--registry", "r", "--manifest", "m", "--source-root", "s", "--repo", "u", "--ref", "v", "foo"},
			map[string]string{"--registry": "r", "--manifest": "m", "--source-root": "s", "--repo": "u", "--ref": "v"}, []string{"foo"}},
		{"add reads the id from the first word and ignores the rest", skillsAddSpec, []string{"foo", "bar"}, nil, []string{"foo", "bar"}},
		{"add ends its flags at --", skillsAddSpec, []string{"--", "foo"}, nil, []string{"foo"}},
		{"remove takes the source root of the wrapper and does not read it", skillsRemoveSpec,
			[]string{"--registry", "r", "--source-root", "s", "foo"}, map[string]string{"--registry": "r"}, []string{"foo"}},
		{"sync-manifest reads the registry and the manifest", skillsSyncSpec,
			[]string{"--manifest", "m", "--registry", "r"}, map[string]string{"--registry": "r", "--manifest": "m"}, nil},
		{"approve reads the id, the approver, the source root and the registry whose lock it takes", skillsApproveSpec,
			[]string{"--id", "foo", "--approver", "A B", "--source-root", "s", "--registry", "r", "--manifest", "m"},
			map[string]string{"--id": "foo", "--approver": "A B", "--source-root": "s", "--registry": "r"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.spec.parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.values, orEmptyValues(tc.values)) || !reflect.DeepEqual(got.words, tc.words) {
				t.Errorf("parse = values %v, words %q, want %v and %q", got.values, got.words, tc.values, tc.words)
			}
		})
	}
	for name, spec := range map[string]skillsFlagSpec{"remove": skillsRemoveSpec, "sync-manifest": skillsSyncSpec} {
		if _, err := spec.parse([]string{"--repo", "u"}); err == nil {
			t.Errorf("%s takes a flag of add, which it does not read", name)
		}
	}
}
