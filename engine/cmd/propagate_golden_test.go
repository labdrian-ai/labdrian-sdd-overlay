package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/propagate-golden record what 'propagate' does across the
// attempts of its read-decide-write-verify loop: what it read and wrote in what order, what it
// printed, and how it exited. The contract goldens (contract_golden_test.go) record single runs;
// these record the runs where a foreign writer tears the registry between the attempts, where the
// registry vanishes and comes back, and where a write is lost. They were recorded from the program
// as it was before Phase 9 unit H28 (docs/architecture/hexagonal-target.md) turned the loop into a
// use case that branches on a typed outcome instead of on the text of the attempt's output: the
// loop told a torn read from a hard failure by the exact words of a message, and these files are
// what the words became once the loop was done. Rewrite them deliberately with
//
//	go test ./cmd -run TestPropagateGolden -update-propagate-golden
//
// and read the diff before committing it.
//
// The program runs in process over an in-memory file system the case scripts: the reads that are
// answered in order before the files are consulted, the failure of a write, and what a foreign
// writer does right after a write. Each case is its own subtest.
var updatePropagateGolden = flag.Bool("update-propagate-golden", false, "rewrite the golden files of the propagate loop")

// propagateRead is one scripted answer to a read: the content, or the error to fail with.
type propagateRead struct {
	content string
	err     error
}

// propagateWorld is the scratch space of one case.
type propagateWorld struct {
	t     *testing.T
	files map[string]string
	// reads are the scripted answers per path, consumed in order; when a path has none left the
	// file is read.
	reads map[string][]propagateRead
	// writeErrors fails the nth write (counting from 1) of the case with the error.
	writeErrors map[int]error
	// afterWrite runs after the nth write of the case succeeded (a foreign writer acting).
	afterWrite map[int]func(w *propagateWorld)
	writes     int
	calls      strings.Builder
	b          strings.Builder
}

func newPropagateWorld(t *testing.T) *propagateWorld {
	return &propagateWorld{
		t:           t,
		files:       map[string]string{},
		reads:       map[string][]propagateRead{},
		writeErrors: map[int]error{},
		afterWrite:  map[int]func(*propagateWorld){},
	}
}

// script queues answers for the next reads of path.
func (w *propagateWorld) script(path string, answers ...propagateRead) {
	w.reads[path] = append(w.reads[path], answers...)
}

// absent is the answer of a path that does not exist.
func absent(path string) propagateRead {
	return propagateRead{err: &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}}
}

func content(text string) propagateRead { return propagateRead{content: text} }

func failure(message string) propagateRead { return propagateRead{err: errors.New(message)} }

func (w *propagateWorld) readFile(path string) ([]byte, error) {
	answer, scripted := propagateRead{}, false
	if queue := w.reads[path]; len(queue) > 0 {
		answer, scripted = queue[0], true
		w.reads[path] = queue[1:]
	}
	if !scripted {
		text, ok := w.files[path]
		if !ok {
			answer = absent(path)
		} else {
			answer = content(text)
		}
	}
	if answer.err != nil {
		fmt.Fprintf(&w.calls, "read %s -> error: %v\n", path, answer.err)
		return nil, answer.err
	}
	fmt.Fprintf(&w.calls, "read %s -> %d bytes\n", path, len(answer.content))
	return []byte(answer.content), nil
}

func (w *propagateWorld) writeFile(path string, data []byte, perm os.FileMode) error {
	w.writes++
	if err := w.writeErrors[w.writes]; err != nil {
		fmt.Fprintf(&w.calls, "write %s (%d bytes, %v) -> error: %v\n", path, len(data), perm, err)
		return err
	}
	fmt.Fprintf(&w.calls, "write %s (%d bytes, %v) -> ok\n", path, len(data), perm)
	w.files[path] = string(data)
	if act := w.afterWrite[w.writes]; act != nil {
		act(w)
	}
	return nil
}

func (w *propagateWorld) write(format string, args ...any) { fmt.Fprintf(&w.b, format, args...) }

func (w *propagateWorld) text() string { return w.b.String() }

// propagate records one run of 'propagate' with the whole loop.
func (w *propagateWorld) propagate(label string, args ...string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := -1
	before, hadRegistry := w.files[registryFile]
	w.calls.Reset()
	runPropagateVerified(args, &stdout, &stderr, w.readFile, w.writeFile, func(code int) { exitCode = code })
	exit := "none"
	if exitCode >= 0 {
		exit = fmt.Sprint(exitCode)
	}
	w.write("$ propagate %s\n# %s\n--- io ---\n%sexit: %s\n--- stdout ---\n%s--- stderr ---\n%s",
		strings.Join(args, " "), label, ensureNewline(w.calls.String()), exit, ensureNewline(stdout.String()), ensureNewline(stderr.String()))
	if after, ok := w.files[registryFile]; ok && (after != before || !hadRegistry) {
		w.write("--- registry after ---\n%s", ensureNewline(after))
	} else {
		w.write("--- registry unchanged ---\n")
	}
	w.write("\n")
}

// propagateArgs are the arguments of a run with a file contract.
var propagateArgs = []string{"--registry", registryFile, "--contract-file", contractFile}

// propagateCase is one scenario and the golden file that records it.
type propagateCase struct {
	name string
	run  func(w *propagateWorld)
}

// A propagate world starts with the contract of the overlay on disk and no registry.
func (w *propagateWorld) withContract() {
	w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject)
}

func propagateGoldenCases() []propagateCase {
	var cases []propagateCase
	cases = append(cases, propagateAbsentCases()...)
	cases = append(cases, propagateEmptyCases()...)
	cases = append(cases, propagateMixedReadCases()...)
	cases = append(cases, propagateFailureCases()...)
	return cases
}

var propagateGoldenFileName = regexp.MustCompile(`[^a-z0-9-]+`)

// TestPropagateGolden runs every case and compares its transcript with its golden file.
func TestPropagateGolden(t *testing.T) {
	for _, tc := range propagateGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newPropagateWorld(t)
			tc.run(w)
			checkPropagateGolden(t, tc.name, w.text())
		})
	}
}

func checkPropagateGolden(t *testing.T, name, got string) {
	t.Helper()
	if propagateGoldenFileName.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", "propagate-golden", name+".golden")
	if *updatePropagateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-propagate-golden)", err)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}
