package main

// Every error the stop hook prints is one line of stderr (S6, L72): CR, LF, VT, FF and the UTF-8 forms of NEL, LS and PS
// fold (L74), and the control bytes are escaped (L76): every C0 byte but those four, DEL, a lone byte 0x80-0x9F and the
// UTF-8 form of U+0080-U+009F but NEL, each byte written as \x and two lowercase hex digits. Every other byte is kept. The
// line form is the printer's concern, so the errors of review/app keep their own texts.

import (
	"errors"
	"strings"
	"testing"
)

// SH11: an error of many lines is printed on one line, and so is a recovered panic. A relative cwd is read by
// hookwire.DecodeStop as no cwd, so the hook asks the working directory, which is the print site reached here.
func TestStopHookPrintsAnErrorOfManyLinesOnOneLine(t *testing.T) {
	s := newStopWorld(t)
	t.Run("an error", func(t *testing.T) {
		d := s.hookDeps()
		d.getwd = func() (string, error) { return "", errors.New("first\nsecond") }

		stdout, stderr, exits := runStopHook(d, stopInput("s1", "relative/dir", false))

		want := "review stop-hook: cannot determine the working directory: first second\n"
		if stdout != "" || stderr != want || !equalInts(exits, []int{0}) {
			t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q and [0]", stdout, stderr, exits, want)
		}
	})
	t.Run("bytes that are not text", func(t *testing.T) {
		d := s.hookDeps()
		d.getwd = func() (string, error) { return "", errors.New("a\xffb\xe2\x80\xa8c") }

		stdout, stderr, exits := runStopHook(d, stopInput("s1", "relative/dir", false))

		want := "review stop-hook: cannot determine the working directory: a\xffb c\n"
		if stdout != "" || stderr != want || !equalInts(exits, []int{0}) {
			t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q and [0]", stdout, stderr, exits, want)
		}
	})
	t.Run("control bytes", func(t *testing.T) { // SH14
		d := s.hookDeps()
		d.getwd = func() (string, error) { return "", errors.New("a\x1b[31mb\x9bc") }

		stdout, stderr, exits := runStopHook(d, stopInput("s1", "relative/dir", false))

		want := "review stop-hook: cannot determine the working directory: a\\x1b[31mb\\x9bc\n"
		if stdout != "" || stderr != want || !equalInts(exits, []int{0}) {
			t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q and [0]", stdout, stderr, exits, want)
		}
	})
	t.Run("a panic", func(t *testing.T) {
		d := s.hookDeps()
		d.getenv = func(string) string { panic("boom\nsecond") }

		stdout, stderr, exits := runStopHook(d, stopInput("s1", s.root, false))

		want := "review stop-hook: internal error: boom second\n"
		if stdout != "" || stderr != want || !equalInts(exits, []int{0}) {
			t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q and [0]", stdout, stderr, exits, want)
		}
	})
}

func TestOneLineJoinsTheLinesOfAText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"a\nb", "a b"},
		{"a\r\nb", "a b"},
		{"a\rb", "a b"},
		{"a\n\n b", "a  b"},
		{"\na\n", "a"},
		{"a", "a"},
		{"", ""},
		// the folded set (L74): VT, FF and the UTF-8 forms of NEL, LS and PS, written as byte escapes.
		{"a\vb", "a b"},
		{"a\fb", "a b"},
		{"a\xc2\x85b", "a b"},
		{"a\xe2\x80\xa8b", "a b"},
		{"a\xe2\x80\xa9b", "a b"},
		{"a\r\xe2\x80\xa8\v\xe2\x80\xa9b", "a b"},
		{"\xe2\x80\xa9a\f", "a"},
		// every other byte is printed as it is.
		{"a\xffb\nc", "a\xffb c"},
		{"\xc3\xa9\n\xc3\xbc", "\xc3\xa9 \xc3\xbc"},
		// a control is written as \x and two lowercase hex digits per byte (L76): the escape is four ASCII characters.
		{"a\x85b", "a\\x85b"},
		{"a\xe2\x80", "a\xe2\\x80"},
		{"a\x1bb", "a\\x1bb"},
		{"a\x00b", "a\\x00b"},
		{"a\tb", "a\\x09b"},
		{"a\x7fb", "a\\x7fb"},
		{"a\x9bb", "a\\x9bb"},
		{"a\xc2\x9bb", "a\\xc2\\x9bb"},
		{"\x1ba", "\\x1ba"},
		{"a\x1b\nb", "a\\x1b b"},
		{"\xe2\x82\xac", "\xe2\x82\xac"},
		{"a\xc2\x85b", "a b"},
	} {
		if got := oneLine(tc.in); got != tc.want {
			t.Errorf("oneLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if strings.Contains(oneLine("x\r\n\r\ny"), "\n") {
		t.Error("oneLine kept a line break")
	}
}
