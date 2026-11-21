package skillregistry

import "testing"

// Every case below is an answer of gentle-ai 4.2.0 recorded in a sandbox HOME, except the ones
// marked "whole", where the difference this package states applies.
func TestParseSkillReadsTheFrontmatterAsGentleDoes(t *testing.T) {
	for _, tc := range []struct {
		name, dir, text string
		want            Skill
	}{
		{"plain", "d", "---\nname: plain\ndescription: A plain description. Trigger: x.\n---\nbody\n", Skill{"plain", "A plain description. Trigger: x."}},
		{"single quotes are dropped", "d", "---\nname: sq\ndescription: 'single quoted: yes'\n---\n", Skill{"sq", "single quoted: yes"}},
		{"every quote at both ends is dropped, escapes are not read", "d", "---\nname: dq\ndescription: \"double quoted, with \\\"esc\\\"\"\n---\n", Skill{"dq", `double quoted, with \"esc\`}},
		{"quotes of both kinds are dropped", "d", "---\nname: m\ndescription: \"it's 'q'\"\n---\n", Skill{"m", "it's 'q"}},
		{"white space inside the quotes is dropped too", "d", "---\nname: p\ndescription: \" padded \"\n---\n", Skill{"p", "padded"}},
		{"the name loses its quotes and white space", "d", "---\nname:  \"sp name\"  \ndescription: d\n---\n", Skill{"sp name", "d"}},
		{"a tab around the value is dropped", "d", "---\nname: tabs\ndescription: \ttabbed\tvalue\t\n---\n", Skill{"tabs", "tabbed\tvalue"}},
		{"a colon in the value stays", "d", "---\nname: c\ndescription: a: b: c # not comment\n---\n", Skill{"c", "a: b: c # not comment"}},
		{"the last description wins", "d", "---\nname: dup\ndescription: first\ndescription: second\n---\n", Skill{"dup", "second"}},
		{"the last name wins", "d", "---\nname: first-n\nname: second-n\ndescription: d\n---\n", Skill{"second-n", "d"}},
		{"the name can follow the description", "d", "---\ndescription: d\nname: late\n---\n", Skill{"late", "d"}},
		{"keys are read in lower case only", "d", "---\nNAME: upper\nDescription: caps keys\n---\n", Skill{"d", ""}},
		{"an indented key is not a key of the skill", "d", "---\nname: n\nmetadata:\n  description: nested\n---\n", Skill{"n", ""}},
		{"a closing bar line may have trailing white space", "d", "---\nname: n\ndescription: d\n--- \nname: other\n", Skill{"n", "d"}},
		{"carriage returns are line ends", "d", "---\r\nname: crlf\r\ndescription: crlf desc\r\n---\r\n", Skill{"crlf", "crlf desc"}},
		{"no name takes the directory", "dirname", "---\ndescription: has no name\n---\n", Skill{"dirname", "has no name"}},
		{"an empty name takes the directory", "dirname", "---\nname: \"\"\ndescription: en\n---\n", Skill{"dirname", "en"}},
		{"no description says nothing", "d", "---\nname: nodesc\n---\n", Skill{"nodesc", ""}},
		{"quotes with nothing between say nothing", "d", "---\nname: n\ndescription: \"\"\n---\n", Skill{"n", ""}},
		{"a description after the frontmatter is not read", "d", "---\nname: afterfm\n---\ndescription: after fm\n", Skill{"afterfm", ""}},
		{"no frontmatter is no name and no description", "dirname", "# heading\ndescription: in body\n", Skill{"dirname", ""}},
		{"frontmatter that never closes is none", "dirname", "---\nname: nocloseFM\ndescription: no close\n", Skill{"dirname", ""}},
		{"a byte order mark hides the frontmatter", "dirname", "\xef\xbb\xbf---\nname: bom\ndescription: bom desc\n---\n", Skill{"dirname", ""}},
		{"a blank line before the frontmatter hides it", "dirname", "\n---\nname: lb\ndescription: lb\n---\n", Skill{"dirname", ""}},
		{"a long description is not cut", "d", "---\nname: l\ndescription: " + repeat("x", 3000) + "\n---\n", Skill{"l", repeat("x", 3000)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseSkill(tc.dir, tc.text); got != tc.want {
				t.Fatalf("ParseSkill = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseSkillFoldsBlockScalarsWithSingleSpaces(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
	}{
		{"folded", "---\nname: n\ndescription: >\n  folded one\n  folded two\n\n  after blank\nlicense: MIT\n---\n", "folded one folded two  after blank"},
		{"literal", "---\nname: n\ndescription: |\n  lit one\n  lit two\nmetadata:\n  x: 1\n---\n", "lit one lit two"},
		{"folded, strip", "---\nname: n\ndescription: >-\n  a\n  b\n---\n", "a b"},
		{"literal, strip", "---\nname: n\ndescription: |-\n  a\n  b\n---\n", "a b"},
		{"leading blank lines and deeper indents", "---\nname: n\ndescription: >\n\n  a\n   b indented more\n  c\n---\n", "a b indented more c"},
		{"a tab indent", "---\nname: n\ndescription: >\n  a\n\tb tab\n  c\nx: y\n---\n", "a b tab c"},
		{"it ends at the closing bars", "---\nname: n\ndescription: |\n  a\n  b\n---\nbody\n", "a b"},
		{"keep indicator is the text itself", "---\nname: n\ndescription: |+\n  a\n  b\n\nnext: z\n---\n", "|+"},
		{"an indent indicator is the text itself", "---\nname: n\ndescription: >2\n  a\n  b\n---\n", ">2"},
		{"a comment after the indicator is the text itself", "---\nname: n\ndescription: > # c\n  a\n---\n", "> # c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseSkill("d", tc.text).Description; got != tc.want {
				t.Fatalf("description = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseSkillKeepsOnlyTheFirstLineOfAPlainValue(t *testing.T) {
	text := "---\nname: plainmulti\ndescription: one\n  two\n  three\nlicense: x\n---\n"
	if got := ParseSkill("d", text).Description; got != "one" {
		t.Fatalf("description = %q, want %q (gentle-ai keeps the first line of a plain value)", got, "one")
	}
}

// The difference from gentle-ai: its first line of a quoted value, which ends the sentence in the
// middle, is the whole value here.
func TestParseSkillReadsAQuotedValueOverSeveralLinesWhole(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
	}{
		{"double quotes", "---\nname: multi\ndescription: \"first line\n  second line\n  third\"\n---\n", "first line second line third"},
		{"single quotes", "---\nname: sqmulti\ndescription: 'one\n  two'\nlicense: x\n---\n", "one two"},
		{"a blank line inside adds no space", "---\nname: dqmulti\ndescription: \"a\n\n  b\"\n---\n", "a b"},
		{"carriage returns", "---\r\nname: crlfmulti\r\ndescription: \"a\r\n  b\"\r\n---\r\n", "a b"},
		{"the other keys after it are read", "---\nname: multi\ndescription: \"a\n  b\"\nname: later\n---\n", "a b"},
		{"a quote that does not close before a nested mapping keeps its first line", "---\nname: multi\ndescription: \"Starts quoted\n  metadata:\n    note: \"x\"\nlicense: x\n---\n", "Starts quoted"},
		{"a continuation line may say Trigger:", "---\nname: multi\ndescription: \"first\n  Trigger: second\"\n---\n", "first Trigger: second"},
		{"a continuation line may say note: in prose", "---\nname: multi\ndescription: \"first\n  note: see the docs\"\n---\n", "first note: see the docs"},
		{"a quote that never closes keeps its first line", "---\nname: multi\ndescription: \"a\n  b\nlicense: x\n---\n", "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseSkill("d", tc.text).Description; got != tc.want {
				t.Fatalf("description = %q, want %q", got, tc.want)
			}
		})
	}
	if got := ParseSkill("d", "---\nname: multi\ndescription: \"a\n  b\"\nname: later\n---\n").Name; got != "later" {
		t.Fatalf("name after a multi-line description = %q, want later", got)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
