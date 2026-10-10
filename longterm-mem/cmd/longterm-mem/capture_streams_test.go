package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// A command that panics must not leave the process's streams pointing at the capture files: the
// test framework writes its own report to them afterwards, and a redirected stdout would lose it.
func TestCaptureStreamsRestoresTheStreamsWhenTheCommandPanics(t *testing.T) {
	realOut, realErr := os.Stdout, os.Stderr

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic of the command did not reach the caller")
			}
		}()
		captureStreams(t, func() {
			fmt.Fprintln(os.Stdout, "written before the panic")
			panic("the command failed")
		})
	}()

	if os.Stdout != realOut || os.Stderr != realErr {
		t.Fatal("os.Stdout or os.Stderr still point at the capture files after the command panicked")
	}
}

func TestCaptureStreamsReturnsWhatTheCommandWroteToEachStream(t *testing.T) {
	realOut, realErr := os.Stdout, os.Stderr

	stdout, stderr := captureStreams(t, func() {
		fmt.Fprint(os.Stdout, "to stdout")
		fmt.Fprint(os.Stderr, "to stderr")
	})

	if stdout != "to stdout" || stderr != "to stderr" {
		t.Errorf("captured (%q, %q), want (%q, %q)", stdout, stderr, "to stdout", "to stderr")
	}
	if os.Stdout != realOut || os.Stderr != realErr {
		t.Fatal("the streams were not restored after a normal run")
	}
}

// The text form of `query` renders a rank, the sources, the id and the title of each row, and nothing of
// its body: no snippet and no truncation mark. The JSON form carries both. The text goldens therefore
// cannot show the snippet rule, and this test says so instead of leaving it to be inferred: if the text
// form ever starts to print snippets, the goldens must grow with it.
func TestTheTextFormOfQueryPrintsNoSnippet(t *testing.T) {
	dbPath := queryGoldenDatabase(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
	t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
	t.Chdir(t.TempDir())

	_, text, _ := runCaptured(t, []string{"query", "--project", queryGoldenProject, "dragonscale"})
	_, jsonOut, _ := runCaptured(t, []string{"query", "--project", queryGoldenProject, "--json", "dragonscale"})

	if !strings.Contains(jsonOut, "needle in the middle") || !strings.Contains(jsonOut, "snippet_truncated") {
		t.Fatalf("the JSON form should carry the snippet and its truncation flag:\n%s", jsonOut)
	}
	for _, snippetText := range []string{"needle in the middle", "\u2026", "snippet"} {
		if strings.Contains(text, snippetText) {
			t.Errorf("the text form printed %q, which belongs to a snippet:\n%s", snippetText, text)
		}
	}
}
