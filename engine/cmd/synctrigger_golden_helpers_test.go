//go:build unix

package main

import "testing"

// The parts of the sync-trigger golden harness that decide what it waits for and how it writes
// a transcript are pure, and are tested here, not trusted.

func TestALogHoldsTheChildsLineOnlyOnceItIsComplete(t *testing.T) {
	for _, tc := range []struct {
		name, log string
		want      bool
	}{
		{"an empty log", "", false},
		{"a line that is not the child's", "partial\n", false},
		{"the child's line, cut before its end", "2026-10-10T00:00:00Z event=archive cwd=/p outcome=ok exit=0 dura", false},
		{"a complete line with an outcome and no exit", "x outcome=ok\n", false},
		{"the child's line, complete", "2026-10-10T00:00:00Z event=archive cwd=/p outcome=ok exit=0 duration=1ms\n", true},
		{"a complete line after an incomplete text", "garbage outcome=\nx outcome=ok exit=0\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := logHoldsAChildLine([]byte(tc.log)); got != tc.want {
				t.Errorf("logHoldsAChildLine(%q) = %v, want %v", tc.log, got, tc.want)
			}
		})
	}
}

func TestMaskingNamesTheLongestPathFirst(t *testing.T) {
	got := maskPaths("in /w/state/sub and /w/state and /w/other", []pathName{
		{"/w/state", "<STATE>"},
		{"/w/state/sub", "<SUB>"},
		{"/w/other", "<OTHER>"},
	})
	if want := "in <SUB> and <STATE> and <OTHER>"; got != want {
		t.Errorf("maskPaths = %q, want %q: a path inside another must be named as itself", got, want)
	}
}

func TestPathsNestWhenOneIsInsideTheOtherAndNotWhenTheyOnlyShareAPrefix(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"/w/a", "/w/a/b", true},
		{"/w/a/b", "/w/a", true},
		{"/w/a", "/w/a", true},
		{"/w/a", "/w/ab", false},
		{"/w/a", "/x/a", false},
	} {
		if got := pathsNest(tc.a, tc.b); got != tc.want {
			t.Errorf("pathsNest(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
