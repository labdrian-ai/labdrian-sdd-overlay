package shaper

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// sourceRoot is the worktree root the domain tests name. The fake source never touches the
// file system, so the directory does not exist.
const sourceRoot = "/work/tree"

// readCall is what a ContainedSource was asked for.
type readCall struct{ root, rel, label string }

// fakeSource is a ContainedSource over a map of relative paths to bytes. It records every
// read it is asked for and, when err is set, refuses all of them with it.
type fakeSource struct {
	files map[string][]byte
	err   error
	calls []readCall
}

var _ ContainedSource = (*fakeSource)(nil)

func (f *fakeSource) ReadContained(worktreeRoot, relPath, label string) ([]byte, error) {
	f.calls = append(f.calls, readCall{worktreeRoot, relPath, label})
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.files[relPath]
	if !ok {
		return nil, fmt.Errorf("%s %q is not accessible: no such file", label, relPath)
	}
	return data, nil
}

// entryPoint is one of the three ways the domain reads a source through the port. They
// share their lexical rules and their handling of the source, so those are proved for all
// three at once.
type entryPoint struct {
	name string
	// prefix is the operation named in front of every error, arg the argument named in
	// the lexical refusals.
	prefix, arg string
	read        func(src ContainedSource, root, rel string) error
}

func entryPoints() []entryPoint {
	return []entryPoint{
		{"BindGoal", "bind goal: ", "goalPath", func(src ContainedSource, root, rel string) error {
			_, err := BindGoal(src, sampleHandoff(), root, rel)
			return err
		}},
		{"LoadHandoff", "load handoff: ", "handoffPath", func(src ContainedSource, root, rel string) error {
			_, err := LoadHandoff(src, root, rel)
			return err
		}},
		{"ReadContainedSource", "read source: ", "path", func(src ContainedSource, root, rel string) error {
			_, _, err := ReadContainedSource(src, root, rel)
			return err
		}},
	}
}

// A path the domain refuses by how it is spelled never reaches the source: the source is
// asked about nothing, and the refusal is the domain's, naming the argument.
func TestAPathRefusedByItsSpellingNeverReachesTheSource(t *testing.T) {
	for _, ep := range entryPoints() {
		for _, tc := range []struct{ name, root, rel, want string }{
			{"empty worktree root", "", "goal.json", "worktreeRoot must not be empty"},
			{"relative worktree root", "relative/root", "goal.json", `worktreeRoot must be absolute, got "relative/root"`},
			{"empty path", sourceRoot, "", ep.arg + " must not be empty"},
			{"absolute path", sourceRoot, "/etc/passwd", ep.arg + ` must be relative, got "/etc/passwd"`},
			{"parent of the root", sourceRoot, "../goal.json", ep.arg + ` must not traverse outside the worktree root, got "../goal.json"`},
			{"traversal after a clean", sourceRoot, "a/../../goal.json", ep.arg + ` must not traverse outside the worktree root, got "a/../../goal.json"`},
			{"the root itself", sourceRoot, ".", ep.arg + ` must not resolve to the worktree root itself, got "."`},
			{"a path that cleans to the root", sourceRoot, "a/..", ep.arg + ` must not resolve to the worktree root itself, got "a/.."`},
		} {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				src := &fakeSource{files: map[string][]byte{"goal.json": nil}}
				err := ep.read(src, tc.root, tc.rel)
				if want := ep.prefix + tc.want; err == nil || err.Error() != want {
					t.Errorf("%s(%q, %q) = %v, want %q", ep.name, tc.root, tc.rel, err, want)
				}
				if len(src.calls) != 0 {
					t.Errorf("the source was asked %+v for a path the domain refuses by its spelling", src.calls)
				}
			})
		}
	}
}

// Without a source there is nothing to read from, which is refused; it is never taken for
// an empty file or guessed at.
func TestReadsWithoutAContainedSourceAreRefused(t *testing.T) {
	for _, ep := range entryPoints() {
		t.Run(ep.name, func(t *testing.T) {
			err := ep.read(nil, sourceRoot, "goal.json")
			if want := ep.prefix + "no contained source to read from"; err == nil || err.Error() != want {
				t.Errorf("%s with no source = %v, want %q", ep.name, err, want)
			}
		})
	}
}

// The lexical rules run before the source is looked at: a path refused by its spelling is
// reported as that, whether or not there is a source to ask, so the refusal a caller sees
// does not depend on which of the two faults it happened to have. A well-spelled path with
// no source is the refusal above.
func TestAPathRefusedByItsSpellingIsReportedEvenWithoutASource(t *testing.T) {
	for _, ep := range entryPoints() {
		t.Run(ep.name, func(t *testing.T) {
			err := ep.read(nil, sourceRoot, "../goal.json")
			if want := ep.prefix + ep.arg + ` must not traverse outside the worktree root, got "../goal.json"`; err == nil || err.Error() != want {
				t.Errorf("%s with no source and a traversing path = %v, want %q", ep.name, err, want)
			}
		})
	}
}

// What the source returns as an error is passed on as it is, under the operation's prefix,
// so the domain adds no wording of its own to a refusal that is the source's to explain.
func TestWhatTheSourceRefusesIsPassedOnUnderTheOperationsPrefix(t *testing.T) {
	refusal := errors.New("a refusal in the source's own words")
	for _, ep := range entryPoints() {
		t.Run(ep.name, func(t *testing.T) {
			err := ep.read(&fakeSource{err: refusal}, sourceRoot, "goal.json")
			if want := ep.prefix + refusal.Error(); err == nil || err.Error() != want || !errors.Is(err, refusal) {
				t.Errorf("%s = %v, want %q wrapping the source's error", ep.name, err, want)
			}
		})
	}
}

// ReadContainedSource returns the cleaned path and the exact bytes, unparsed, so a caller
// can hand bytes that failed a strict parse to Evaluate.
func TestReadContainedSourceReturnsTheCleanedPathAndTheExactBytes(t *testing.T) {
	src := &fakeSource{files: map[string][]byte{"h.json": []byte("not json")}}

	cleaned, data, err := ReadContainedSource(src, sourceRoot, "./h.json")
	if err != nil {
		t.Fatalf("ReadContainedSource: %v", err)
	}
	if cleaned != "h.json" || string(data) != "not json" {
		t.Errorf("ReadContainedSource = (%q, %q), want (%q, %q)", cleaned, data, "h.json", "not json")
	}
	want := []readCall{{root: sourceRoot, rel: "h.json", label: "source"}}
	if !reflect.DeepEqual(src.calls, want) {
		t.Errorf("source was asked %+v, want exactly %+v", src.calls, want)
	}

	if _, _, err := ReadContainedSource(src, sourceRoot, "missing.json"); err == nil || !strings.HasPrefix(err.Error(), "read source: ") {
		t.Errorf("ReadContainedSource of a file the source lacks = %v, want a refusal under %q", err, "read source: ")
	}
}

func TestSourceSHA256IsTheLowercaseHexDigestOfTheBytes(t *testing.T) {
	// SHA-256 of "abc", the standard test vector.
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := SourceSHA256([]byte("abc")); got != want {
		t.Errorf("SourceSHA256(%q) = %q, want %q", "abc", got, want)
	}
}
