package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// The golden files named embedding-ops-* under testdata/retrieval-golden pin what `longterm-mem status` and
// `longterm-mem doctor` report of the embedding index and of the embedding backend, in text and in JSON,
// with the exit code and the files the state directory holds afterwards: no index, a complete one, one behind
// the corpus (by a count and by a set), one that cannot be read in each of the ways a load refuses, a backend
// without its model, a backend that fails, an Engram database that is not there, and a state directory nobody
// can name. They were recorded from the program as it stood before ops read the index and told the backend's
// failures through ports it owns (Phase 9, L4, the ops half), and that change must not alter one byte of them.
//
// Each world is deterministic: its index is built here with the fixed clock of the fixtures, from the live rows
// of the fixture database, by the embedding backend of the test (never the wall clock, never a server of the
// machine); the vault is an empty directory, so the checks of the vault say what they say of one; python3 is a
// file in a directory of the test's own, so that the runtime-prerequisites check does not depend on the machine;
// and every path the program prints is one of the world's, replaced by where it is. The recording runs under
// two lengths of TMPDIR (TestEmbeddingOpsGolden_DoesNotDependOnWhereTheWorldIs).
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestEmbeddingOpsGolden -update-retrieval-golden
//
// and read the diff before committing it.
//
// World 14 (a state directory nobody can name) was re-recorded on the owner's decision of 2026-10-12: `status`
// refuses with exit 8 and `doctor` fails the two checks that read the index, saying the directory could not be
// resolved, where both had looked in the working directory. It is the only world whose recording changed for a
// change of behavior; worlds 05, 08 and 13 changed only in the words of the libraries (libraryError).

// opsGoldenCase is one world and the commands run in it.
type opsGoldenCase struct {
	name  string
	setup func(t *testing.T, w *retrievalWorld)
	// masks are the words of the libraries the world prints, which its golden pins by placeholder.
	masks []libraryError
}

// opsGoldenCommands are the commands run in every world, in this order.
func opsGoldenCommands() [][]string {
	return [][]string{
		{"status", "--project", queryGoldenProject},
		{"status", "--project", queryGoldenProject, "--json"},
		{"doctor", "--project", queryGoldenProject},
		{"doctor", "--project", queryGoldenProject, "--json"},
	}
}

func opsGoldenCases() []opsGoldenCase {
	return []opsGoldenCase{
		{name: "01-no-index-was-built"},
		{name: "02-a-complete-index", setup: func(t *testing.T, w *retrievalWorld) { w.buildIndexOfLiveRows(t, nil, nil) }},
		{name: "03-an-index-behind-the-corpus-by-two-rows", setup: func(t *testing.T, w *retrievalWorld) {
			live := w.liveRows(t)
			w.buildIndexOfLiveRows(t, func(r vecindex.Row) bool { return r.EngramID != live[0].EngramID && r.EngramID != live[1].EngramID }, nil)
		}},
		{name: "04-an-index-as-long-as-the-corpus-but-not-the-same-rows", setup: func(t *testing.T, w *retrievalWorld) {
			live := w.liveRows(t)
			w.buildIndexOfLiveRows(t, func(r vecindex.Row) bool { return r.EngramID != live[0].EngramID },
				[]vecindex.Row{{EngramID: 9999, Title: "A row since deleted", Content: "gone"}})
		}},
		{name: "05-a-manifest-that-is-not-json", masks: []libraryError{jsonSyntaxError}, setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRows(t, nil, nil)
			w.writeIndexFile(t, "manifest.json", "{not json")
		}},
		{name: "06-a-manifest-without-its-vectors", setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRows(t, nil, nil)
			if err := os.Remove(filepath.Join(w.embeddingIndexDir(), "vectors.blob")); err != nil {
				t.Fatalf("remove the vector blob: %v", err)
			}
		}},
		{name: "07-a-manifest-edited-after-it-was-written", setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRows(t, nil, nil)
			path := filepath.Join(w.embeddingIndexDir(), "manifest.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read the manifest: %v", err)
			}
			w.writeIndexFile(t, "manifest.json", strings.Replace(string(data), `"dimension": 3`, `"dimension": 4`, 1))
		}},
		{name: "08-an-index-directory-that-is-a-file", masks: []libraryError{notADirectoryError}, setup: func(t *testing.T, w *retrievalWorld) {
			w.writeFile(t, w.embeddingIndexDir(), "not a directory")
		}},
		{name: "09-an-index-of-another-project-only", setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRowsFor(t, "otherproj", nil, nil)
		}},
		{name: "10-a-complete-index-and-a-backend-without-the-model", setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRows(t, nil, nil)
			w.backend.mode.Store(int32(backendLacksTheModel))
		}},
		{name: "11-a-complete-index-and-a-backend-that-fails", setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRows(t, nil, nil)
			w.backend.mode.Store(int32(backendFails))
		}},
		{name: "12-no-index-and-a-backend-without-the-model", setup: func(t *testing.T, w *retrievalWorld) {
			w.backend.mode.Store(int32(backendLacksTheModel))
		}},
		{name: "13-a-complete-index-and-an-engram-database-that-is-not-there", masks: []libraryError{sqliteOpenError}, setup: func(t *testing.T, w *retrievalWorld) {
			w.buildIndexOfLiveRows(t, nil, nil)
			t.Setenv(engramDBEnvVar, filepath.Join(w.home, "absent", "engram.db"))
		}},
		{name: "14-a-state-directory-nobody-can-name", setup: func(t *testing.T, w *retrievalWorld) {
			t.Setenv(stateDirEnvVar, "")
			t.Setenv("HOME", "")
		}},
	}
}

// liveRows are the live rows of the golden project in the shape an index is built from.
func (w *retrievalWorld) liveRows(t *testing.T) []vecindex.Row {
	t.Helper()
	store, err := engram.Open(w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer store.Close()
	observations, err := store.ListObservations(queryGoldenProject)
	if err != nil {
		t.Fatalf("list the live observations: %v", err)
	}
	rows := make([]vecindex.Row, len(observations))
	for i, o := range observations {
		rows[i] = vecindex.Row{EngramID: o.ID, Title: o.Title, Content: o.Content}
	}
	return rows
}

// buildIndexOfLiveRows builds the index of the golden project from its live rows that keep accepts (all when
// keep is nil) and the rows in extra, at the fixed instant of the fixtures, through the backend of the world.
func (w *retrievalWorld) buildIndexOfLiveRows(t *testing.T, keep func(vecindex.Row) bool, extra []vecindex.Row) {
	t.Helper()
	w.buildIndexOfLiveRowsFor(t, queryGoldenProject, keep, extra)
}

func (w *retrievalWorld) buildIndexOfLiveRowsFor(t *testing.T, project string, keep func(vecindex.Row) bool, extra []vecindex.Row) {
	t.Helper()
	var rows []vecindex.Row
	for _, row := range w.liveRows(t) {
		if keep == nil || keep(row) {
			rows = append(rows, row)
		}
	}
	rows = append(rows, extra...)
	client, err := w.commands.newEmbedClient(embed.Config{Model: vecindex.DefaultModel})
	if err != nil {
		t.Fatalf("build the embedding client: %v", err)
	}
	builtAt := func() string { return fixtureInstant.Format(time.RFC3339) }
	if _, _, err := vecindex.Build(context.Background(), vecindex.Dir(w.stateDir, project), vecindex.DefaultModel, 3, vecindex.DefaultInputLimit, rows, client, builtAt); err != nil {
		t.Fatalf("build the embedding index of %s: %v", project, err)
	}
}

// writeIndexFile replaces a file of the golden project's index directory.
func (w *retrievalWorld) writeIndexFile(t *testing.T, name, content string) {
	t.Helper()
	w.writeFile(t, filepath.Join(w.embeddingIndexDir(), name), content)
}

// writeFile writes content at path, replacing what is there and making its directory.
func (w *retrievalWorld) writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the directory of %s: %v", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("clear %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// putPython3OnPath makes python3 a file of a directory of the test's own and that directory the whole PATH, so
// that the check of the runtime prerequisite finds it whatever the machine has, and nothing else.
func putPython3OnPath(t *testing.T) {
	t.Helper()
	dir := realTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write the stand-in for python3: %v", err)
	}
	t.Setenv("PATH", dir)
}

// libraryError is the words of an error that the program prints but does not write, and the placeholder the
// goldens pin in their place. The words are the toolchain's, the driver's or the platform's to change, so the
// goldens of this set pin the sentence the program builds around them and not their wording (a golden that failed
// on a new Go release or driver would say nothing about the program). The pattern keeps the words that come
// before them (group 1) because those are the program's: a mask applies where the library's words arrive in the
// sentence that carries them, and not to the same words in any other.
type libraryError struct {
	pattern *regexp.Regexp
	mask    string
}

var (
	// jsonSyntaxError is the JSON parser's message for a manifest it cannot parse.
	jsonSyntaxError = libraryError{regexp.MustCompile(`(parse manifest: )invalid character '[^']*' looking for beginning of object key string`), "${1}<json syntax error>"}
	// sqliteOpenError is the SQLite driver's message for a database it cannot open, as the primary and the
	// fallback connection each report it.
	sqliteOpenError = libraryError{regexp.MustCompile(`(open failed[: (]+)unable to open database file \(\d+\)`), "${1}<sqlite open error>"}
	// notADirectoryError is the operating system's message for a manifest read through a path that has a file in it.
	notADirectoryError = libraryError{regexp.MustCompile(`(manifest\.json): not a directory`), "${1}: <not a directory>"}
)

// maskLibraryErrors replaces the words of the given library errors in a rendering, and nothing else: a world
// names the ones it is about (opsGoldenCase.masks), so no other world, and no other sentence, is masked.
func maskLibraryErrors(text string, masks ...libraryError) string {
	for _, r := range masks {
		text = r.pattern.ReplaceAllString(text, r.mask)
	}
	return text
}

func TestMaskLibraryErrorsMasksTheWordsOfTheLibrariesAndNothingElse(t *testing.T) {
	got := maskLibraryErrors("parse manifest: invalid character 'n' looking for beginning of object key string)\n"+
		"engram: connect x: primary open failed (unable to open database file (14)), fallback open failed: unable to open database file (14)\n"+
		"open /s/index/p/manifest.json: not a directory\n"+
		"vecindex: index is corrupted: no such file or directory; is a directory\n",
		jsonSyntaxError, sqliteOpenError, notADirectoryError)
	want := "parse manifest: <json syntax error>)\n" +
		"engram: connect x: primary open failed (<sqlite open error>), fallback open failed: <sqlite open error>\n" +
		"open /s/index/p/manifest.json: <not a directory>\n" +
		"vecindex: index is corrupted: no such file or directory; is a directory\n"
	if got != want {
		t.Errorf("masked:\n%s\nwant:\n%s", got, want)
	}
}

// A world masks the words of the libraries it is about and no others: the same words anywhere else in the
// rendering, or in a world that did not ask for them, are the program's to have changed, and the golden says so.
func TestMaskLibraryErrorsOnlyMasksWhatTheWorldAskedFor(t *testing.T) {
	const text = "parse manifest: invalid character 'n' looking for beginning of object key string)\n" +
		"engram: connect x: primary open failed (unable to open database file (14))\n" +
		"open /s/index/p/manifest.json: not a directory\n"
	for _, tc := range []struct {
		name   string
		masks  []libraryError
		masked string
	}{
		{"nothing", nil, ""},
		{"json only", []libraryError{jsonSyntaxError}, "<json syntax error>"},
		{"sqlite only", []libraryError{sqliteOpenError}, "<sqlite open error>"},
		{"not a directory only", []libraryError{notADirectoryError}, "<not a directory>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := maskLibraryErrors(text, tc.masks...)
			for _, mask := range []string{"<json syntax error>", "<sqlite open error>", "<not a directory>"} {
				if has, want := strings.Contains(got, mask), mask == tc.masked; has != want {
					t.Errorf("%s in the masked text = %v, want %v:\n%s", mask, has, want, got)
				}
			}
		})
	}
}

// Each mask is anchored to the sentence the libraries' words arrive in, so the same words in a sentence the
// program wrote itself are not hidden: a refusal that names a path that "is not a directory", a manifest of
// another file, or a different parser error.
func TestMaskLibraryErrorsLeavesTheProgramsOwnSentencesAlone(t *testing.T) {
	for _, text := range []string{
		"longterm-mem: skills-stale: --project-root is not a directory: /w/root\n",
		"open /w/notes.txt: not a directory\n",
		"parse config: invalid character 'n' looking for beginning of object key string\n",
		"parse manifest: invalid character 'n' looking for beginning of value\n",
		"the vault said: unable to open database file (14)\n",
		"parse manifest: invalid character 'n' and then more before looking for beginning of object key string\n",
	} {
		if got := maskLibraryErrors(text, jsonSyntaxError, sqliteOpenError, notADirectoryError); got != text {
			t.Errorf("masked a sentence of the program's own:\n%q\nbecame\n%q", text, got)
		}
	}
}

// Status prints "never" for an index it cannot read, as it does for one nobody built (worlds 05 to 08): it
// never invents an instant it cannot stand behind (R-065, status.go), and the doctor, which says what is wrong
// with the index, is where the difference is told. The goldens pin that as the intended behavior.

// runOpsGoldenCase runs the commands of the case in a world of its own and renders what they did.
func runOpsGoldenCase(t *testing.T, tc opsGoldenCase) (w *retrievalWorld, rendering string) {
	t.Helper()
	putPython3OnPath(t)
	w = newRetrievalWorld(t)
	if tc.setup != nil {
		tc.setup(t, w)
	}
	var b strings.Builder
	for _, args := range opsGoldenCommands() {
		code, stdout, stderr := w.run(t, args)
		fmt.Fprintf(&b, "args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s\n", strings.Join(args, " "), code, stdout, stderr)
	}
	fmt.Fprintf(&b, "--- state ---\n%s", w.renderState(t))
	return w, maskLibraryErrors(w.normalise(b.String()), tc.masks...)
}

func TestEmbeddingOpsGolden(t *testing.T) {
	for _, tc := range opsGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w, got := runOpsGoldenCase(t, tc)
			w.check(t, "embedding-ops-"+tc.name, got)
		})
	}
}

// What the commands print must not depend on where the world is: the program's paths are the world's, and the
// recording replaces them by name. Every case is run in two directories of very different lengths and must say
// the same thing in both.
func TestEmbeddingOpsGolden_DoesNotDependOnWhereTheWorldIs(t *testing.T) {
	base := t.TempDir()
	for _, tc := range opsGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]string{}
			for name, dir := range map[string]string{
				"short": filepath.Join(base, "s"),
				"long":  filepath.Join(base, strings.Repeat("a-long-directory-", 6)),
			} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("create the %s temporary directory: %v", name, err)
				}
				t.Run(name, func(t *testing.T) {
					t.Setenv("TMPDIR", dir)
					_, answers[name] = runOpsGoldenCase(t, tc)
				})
			}
			if answers["short"] == "" || answers["short"] != answers["long"] {
				t.Errorf("the rendering depends on the length of the temporary directory the world is in:\n--- short ---\n%s\n--- long ---\n%s", answers["short"], answers["long"])
			}
		})
	}
}
