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
