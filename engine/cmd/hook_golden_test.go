package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// The golden files under testdata/hook-golden record what every Claude Code hook the engine
// answers prints: the exit code, stdout and stderr of
//
//   - 'gate-task', the PreToolUse hook on the Agent tool that rewrites a sub-agent prompt
//     (updatedInput) or lets it through unchanged ({}),
//   - 'projection hook --event UserPromptSubmit', which adds the bound workflow to the
//     session's context (additionalContext, systemMessage),
//   - 'projection hook --event PreToolUse', which denies a call (permissionDecision) or
//     warns about it (systemMessage),
//   - 'skills guard-hook', which denies the agent running 'skills approve' or writing the
//     approval record by hand (a deny in JSON, exit 0),
//   - 'shaper guard-hook', which denies, by exit 2 and a message on stderr, the agent that
//     records a clearance or touches the clearance store.
//
// The review-receipt hook, the sixth, is pinned in testdata/review-receipt-golden, which runs
// the built binary against throwaway git repositories.
//
// These bytes are a contract with the runtimes that read them, and the input is a contract
// the runtimes write: every case that feeds a hook an input it cannot use (empty, truncated,
// the wrong shape, a field of the wrong type, over the size bound, a reader that fails) says
// what the hook then does, because each hook decides that for itself (one lets the call go
// through, one denies it) and a decoder shared by all of them must keep each decision. The
// files were recorded from the program as it was before Phase 9 unit H14
// (docs/architecture/hexagonal-target.md) moved the hook wire format into one adapter
// (engine/hookwire); a change to a byte of any of them fails here. Three were rewritten on
// purpose afterwards, in Phase 9 batch 10, and nothing else was: the clearance guard's size
// bound replaced shaper-guard-reads-without-a-size-bound (the same two cases, then the bound
// itself and what is over it, which that guard now denies), and two files hold the same input
// as before, with a character that reorders or hides the text around it now written as text
// (gate-task-writes-html-characters-as-they-are, pretooluse-denies-a-memory-query-for-another-project).
// Rewrite them deliberately with
//
//	go test ./cmd -run TestHookGolden -update-hook-golden
//
// and read the diff before committing it.
//
// The hooks run in process, as the commands do (the same cores 'main' calls, with the exit
// function recorded and the streams captured), against the same throwaway state homes and
// hand-made repositories the behavioral tests use. Each case is its own subtest, so a case
// that skips (a permission the process cannot drop) drops only its own comparison.
var updateHookGolden = flag.Bool("update-hook-golden", false, "rewrite the golden files of the hooks")

// hookGoldenFileName is what a case name may not contain: it is the name of its file.
var hookGoldenFileName = regexp.MustCompile(`[^a-z0-9-]+`)

// hookWorld is the scratch space of one case: the in-memory files the gate-task contract
// is read from, the places a transcript names by a placeholder, and the transcript.
type hookWorld struct {
	t     *testing.T
	b     strings.Builder
	files map[string]string
	names map[string]string
	// deps is what the hooks the world runs are built over: the program's, with a prober that
	// confirms nothing once a case has a bound-repository world (env), and with the decorators a
	// case puts in front of the store or the gate.
	deps deps
}

func newHookWorld(t *testing.T) *hookWorld {
	t.Helper()
	return &hookWorld{t: t, files: map[string]string{}, names: map[string]string{}, deps: testDeps()}
}

// name registers path to be written as placeholder in the transcript.
func (w *hookWorld) name(path, placeholder string) { w.names[path] = placeholder }

// env is a bound-repository world (a state home, a repository, a scratch directory) whose
// prober confirms nothing, so the projected context is the same on every machine.
func (w *hookWorld) env() hookEnv {
	w.t.Helper()
	w.deps = w.deps.withUnavailableProber()
	e := newHookEnv(w.t)
	e.deps = w.deps
	w.name(e.state, "<STATE>")
	w.name(e.dir, "<DIR>")
	w.name(e.repo, "<REPO>")
	return e
}

// tempDir is a directory outside every repository, written as placeholder in the transcript.
func (w *hookWorld) tempDir(placeholder string) string {
	w.t.Helper()
	dir := w.t.TempDir()
	w.name(dir, placeholder)
	return dir
}

// fixtureRepo is a hand-made repository nothing is bound to, written as placeholder in the
// transcript.
func (w *hookWorld) fixtureRepo(name, placeholder string) string {
	w.t.Helper()
	root := fixtureRepo(w.t, name)
	w.name(root, placeholder)
	return root
}

func (w *hookWorld) readFile(path string) ([]byte, error) {
	if content, ok := w.files[path]; ok {
		return []byte(content), nil
	}
	return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}

// replace writes the registered places as their placeholders, longest first, so a path under
// another is replaced as itself.
func (w *hookWorld) replace(text string) string {
	paths := make([]string, 0, len(w.names))
	for p := range w.names {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		text = strings.ReplaceAll(text, p, w.names[p])
	}
	return text
}

// digests are the long hexadecimal digests a transcript may carry: the name of a binding file
// (the digest of the repository's path) and the digests of a workflow's events (which hold
// the time they were recorded). They differ from one run to the next, and so do not belong in
// a golden file; the first 40 characters are enough to tell one from a word.
var digests = regexp.MustCompile(`[0-9a-f]{40,}`)

// text is the transcript, with the registered places replaced, the digests of a run written
// as <DIGEST>, and every control character shown as \xNN.
func (w *hookWorld) text() string {
	return visibleControls(digests.ReplaceAllString(w.replace(w.b.String()), "<DIGEST>"))
}

// visibleControls shows a control character (other than a line break or a tab) as \xNN, and a
// character that no reader can see or that moves the text around it as \uNNNN, so a golden file
// never holds an escape sequence a terminal would act on, nor a character that reorders or hides
// the lines around it in a diff, a pager or an editor: a bidirectional override or isolate, a
// zero-width character, a byte-order mark, a soft hyphen, the line and paragraph separators
// and the controls of the second block (Unicode categories Cf, Zl, Zp and the C1 controls).
func visibleControls(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// largeOutput is the size over which a stream is summarized instead of printed.
const largeOutput = 4096

// shown renders a stream or an input for a transcript: as it is when it is short, and as
// its size, the start of its digest and its start when it is long (the digest and the start
// are of the text with the places replaced, so they are the same wherever the test runs; the
// size is the real one, which a case that cares about it makes the same everywhere). A stream
// that does not end with a line break says so, because the end of a hook's output is part of
// its contract.
func (w *hookWorld) shown(s string) string {
	switch {
	case s == "":
		return ""
	case len(s) > largeOutput:
		return w.summary(s)
	case strings.HasSuffix(s, "\n"):
		return s
	}
	return s + "\n[no line break at the end]\n"
}

// summary is the size of a long text, the start of the digest of its transcript form, and its
// first characters.
func (w *hookWorld) summary(s string) string {
	replaced := w.replace(s)
	sum := sha256.Sum256([]byte(replaced))
	return fmt.Sprintf("<%d bytes, sha256 %s, starting %q>\n", len(s), hex.EncodeToString(sum[:])[:12], replaced[:60])
}

// shownInput renders an input for a transcript: "<empty>" for none, the summary of shown for
// a long one, and otherwise the text with a line break at its end (a hook input is JSON on
// one line, and the line break is the transcript's, not the input's).
func (w *hookWorld) shownInput(s string) string {
	switch {
	case s == "":
		return "<empty>\n"
	case len(s) > largeOutput:
		return w.summary(s)
	}
	return ensureNewline(s)
}

// record writes one invocation: its command line, a label, the input, the exit calls the
// hook made, and both streams.
func (w *hookWorld) record(command, label, stdin string, exits []int, stdout, stderr string) {
	exit := "none"
	if len(exits) > 0 {
		parts := make([]string, len(exits))
		for i, c := range exits {
			parts[i] = fmt.Sprint(c)
		}
		exit = strings.Join(parts, ", ")
	}
	fmt.Fprintf(&w.b, "$ %s\n# %s\n--- stdin ---\n%s--- exit ---\n%s\n--- stdout ---\n%s--- stderr ---\n%s\n", command, label, w.shownInput(stdin), exit, w.shown(stdout), w.shown(stderr))
}

// failingStdin is a stdin that fails: what a transcript says in place of its content.
const failingStdin = "<the reader fails with: read failed>"

// gateTask records one run of 'gate-task' over the in-memory files.
func (w *hookWorld) gateTask(label, stdin string, args ...string) {
	w.t.Helper()
	w.gateTaskReader(label, strings.NewReader(stdin), stdin, args...)
}

// gateTaskReader records one run of 'gate-task' over a stdin that is not a string.
func (w *hookWorld) gateTaskReader(label string, stdin io.Reader, stdinNote string, args ...string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	gateTaskCore(args, stdin, &stdout, &stderr, w.readFile)
	w.record("gate-task "+strings.Join(args, " "), label, stdinNote, nil, stdout.String(), stderr.String())
}

// projection records one run of 'projection <args>' in the process directory processCwd.
func (w *hookWorld) projection(label string, args []string, stdin, processCwd string) {
	w.t.Helper()
	w.projectionReader(label, args, strings.NewReader(stdin), stdin, processCwd)
}

// projectionReader records one run of 'projection <args>' over a stdin that is not a string
// (stdinNote is what the transcript says it holds).
func (w *hookWorld) projectionReader(label string, args []string, stdin io.Reader, stdinNote, processCwd string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	var exits []int
	runProjectionCore(w.deps, args, processCwd, stdin, &stdout, &stderr, func(c int) { exits = append(exits, c) })
	w.record("projection "+strings.Join(args, " "), label, stdinNote, exits, stdout.String(), stderr.String())
}

// projectionWriter records one run of 'projection <args>' whose stdout cannot be written.
func (w *hookWorld) projectionWriter(label string, args []string, stdin, processCwd string, stdout io.Writer) {
	w.t.Helper()
	var stderr bytes.Buffer
	var exits []int
	runProjectionCore(w.deps, args, processCwd, strings.NewReader(stdin), stdout, &stderr, func(c int) { exits = append(exits, c) })
	w.record("projection "+strings.Join(args, " "), label, stdin, exits, "<stdout cannot be written>\n", stderr.String())
}

// hookGoldenCase is one scenario and the golden file its transcript is compared with.
type hookGoldenCase struct {
	name string
	run  func(w *hookWorld)
}

// TestHookGolden runs every case and compares its transcript with its golden file.
func TestHookGolden(t *testing.T) {
	for _, tc := range hookGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newHookWorld(t)
			tc.run(w)
			checkHookGolden(t, tc.name, w.text())
		})
	}
}

func checkHookGolden(t *testing.T, name, got string) {
	t.Helper()
	if hookGoldenFileName.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", "hook-golden", name+".golden")
	if *updateHookGolden {
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
		t.Fatalf("read golden file: %v (record it with -update-hook-golden)", err)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}

// TestHookGoldenCasesAreDistinctFiles guards the case list itself: two cases of one name
// would share a golden file and each pass against the other's recording.
func TestHookGoldenCasesAreDistinctFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range hookGoldenCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "hook-golden"))
	if err != nil {
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/hook-golden/%s belongs to no case", e.Name())
		}
	}
}
