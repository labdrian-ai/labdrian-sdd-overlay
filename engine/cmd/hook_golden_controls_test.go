package main

import (
	"fmt"
	"testing"
)

// A golden file is read by people, in diffs, pagers and editors, and holds what a hook was
// handed: a transcript that carries a character which reorders the text around it, or hides
// some of it, is a file that cannot be trusted to say what it records. visibleControls writes
// each of them as text, and leaves every other character as it is.
//
// The characters are given by their code points and never spelled in this source, as the
// escape of one would be decoded by the first tool that handled the file.
func TestVisibleControlsShowsWhatMovesOrHidesTheTextAroundIt(t *testing.T) {
	for name, r := range map[string]rune{
		"a right-to-left override":      0x202e,
		"a left-to-right isolate":       0x2066,
		"a pop of an isolate":           0x2069,
		"a zero-width space":            0x200b,
		"a zero-width joiner":           0x200d,
		"a left-to-right mark":          0x200e,
		"a word joiner":                 0x2060,
		"a byte-order mark":             0xfeff,
		"a soft hyphen":                 0x00ad,
		"the line separator":            0x2028,
		"the paragraph separator":       0x2029,
		"a control of the second block": 0x0085,
		"the last control of that":      0x009f,
	} {
		t.Run(name, func(t *testing.T) {
			want := fmt.Sprintf("a%sb", `\u`+fmt.Sprintf("%04x", r))
			if got := visibleControls("a" + string(r) + "b"); got != want {
				t.Errorf("visibleControls(a, U+%04X, b) = %q, want %q", r, got, want)
			}
		})
	}
	for name, tc := range map[string]struct{ in, want string }{
		"plain text":                  {"abc", "abc"},
		"a line break and a tab stay": {"a\nb\tc", "a\nb\tc"},
		"an escape sequence":          {"a\x1b[31mb", `a\x1b[31mb`},
		"a nul and a delete":          {"a\x00b\x7fc", `a\x00b\x7fc`},
		"a carriage return":           {"a\rb", `a\x0db`},
		"letters and symbols stay":    {"é ñ → ✓ 日本 😀", "é ñ → ✓ 日本 😀"},
		"a no-break space is visible": {"a" + string(rune(0xa0)) + "b", "a" + string(rune(0xa0)) + "b"},
		"an ideographic space stays":  {"a" + string(rune(0x3000)) + "b", "a" + string(rune(0x3000)) + "b"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := visibleControls(tc.in); got != tc.want {
				t.Errorf("visibleControls(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
