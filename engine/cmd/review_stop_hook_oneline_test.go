package main

// Every error the stop hook prints is one line of stderr, whatever line breaks its text holds (S6, L72): the line form is
// the printer's concern, so the errors of review/app keep their own texts.

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
	} {
		if got := oneLine(tc.in); got != tc.want {
			t.Errorf("oneLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if strings.Contains(oneLine("x\r\n\r\ny"), "\n") {
		t.Error("oneLine kept a line break")
	}
}
