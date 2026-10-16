package shelltest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// TestOverlayPathCleanMatchesFilepathClean checks the POPULATION, not the
// one case that was reported. The binary-path test next to this one pins
// the shapes of $STATE_DIR that actually reach the MCP entry; this one pins
// path_clean itself against the function it exists to reproduce, over every
// lexical shape filepath.Clean has an opinion about — because a normalizer
// that agrees on a trailing slash and disagrees on "/.." has not fixed the
// class, it has fixed one member of it.
//
// One bash process handles the whole corpus: the loop is inside the shell,
// so adding a case costs nothing.
func TestOverlayPathCleanMatchesFilepathClean(t *testing.T) {
	overlay := overlayScript(t)

	inputs := []string{
		"", ".", "..", "/", "//", "/.", "/..", "/../..",
		"a", "a/", "a//", "a/.", "a/..", "a/../..", "a/b/..",
		"/a", "/a/", "/a//b", "/a/./b", "/a/../b", "/a/b/../..",
		"/a/b/../../..", "./a", "../a", "../../a", "a/./b/../c",
		"/home/u/.labdrian-overlay", "/home/u/.labdrian-overlay/",
		"/home/u//.labdrian-overlay", "/home/u/x/../.labdrian-overlay",
		"/home/u/.labdrian-overlay/.", "relative/state",
		"/a/b/c/../../d/", "/./a/./b/./", "...", "/a/.../b",
		"/spaces in/a path/", "/a-b_c.d/e/", "/a/*/b", "/a/[x]/b",
	}

	out, err := overlaytest.Source(t, overlay, nil, "", `for p in "$@"; do path_clean "$p"; done`, inputs...)
	if err != nil {
		t.Fatalf("path_clean run failed: %v\n%s", err, out)
	}

	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != len(inputs) {
		t.Fatalf("path_clean printed %d lines for %d inputs:\n%s", len(got), len(inputs), out)
	}
	for i, in := range inputs {
		want := filepath.Clean(in)
		if got[i] != want {
			t.Errorf("path_clean %q = %q, filepath.Clean = %q", in, got[i], want)
		}
	}
}
