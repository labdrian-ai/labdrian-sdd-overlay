package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/shaper-golden record what the shaper verbs print:
// 'shaper assess' (the JSON, the presented view, and the refusals of a source that cannot
// be read), the verb that stores a clearance, and 'roles match-shaper', which reads a
// handoff through the same contained read. They were recorded from the program as it was
// before the contained read moved behind a port (H10 S2) and before the disclosure file
// was renamed (S3), and they did not change when those landed: a change to what a verb
// prints, or to the wording of a refusal, fails here. Rewrite them deliberately with
//
//	go test ./cmd -run TestShaperGolden -update-shaper-golden
//
// and read the diff before committing it.
var updateShaperGolden = flag.Bool("update-shaper-golden", false, "rewrite the golden files of the shaper verbs")

// provenanceDigest is the one value in the output that depends on where the test ran: the
// digest of the worktree paths. Every other digest comes from file content.
var provenanceDigest = regexp.MustCompile(`"provenance_sha256": "[0-9a-f]{64}"`)

// goldenTranscript collects the runs of one scenario as text, with the temporary paths of
// this run replaced by placeholders so the text is the same wherever the test runs.
type goldenTranscript struct {
	b          strings.Builder
	root, home string
}

func newGoldenTranscript(root, home string) *goldenTranscript {
	return &goldenTranscript{root: root, home: home}
}

func (g *goldenTranscript) write(format string, args ...any) {
	fmt.Fprintf(&g.b, format, args...)
}

// run records one invocation: its command line, its exit code, and both output streams.
func (g *goldenTranscript) run(verb string, args []string, r interface{ out() (int, string, string) }) {
	code, stdout, stderr := r.out()
	g.write("$ %s %s\nexit: %d\n--- stdout ---\n%s", verb, strings.Join(args, " "), code, ensureNewline(stdout))
	g.write("--- stderr ---\n%s\n", ensureNewline(stderr))
}

func (g *goldenTranscript) text() string {
	text := g.b.String()
	text = strings.ReplaceAll(text, g.root, "<ROOT>")
	text = strings.ReplaceAll(text, g.home, "<STATE>")
	return provenanceDigest.ReplaceAllString(text, `"provenance_sha256": "<PROVENANCE-SHA256>"`)
}

func ensureNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func (r shaperRun) out() (int, string, string) { return r.code, r.stdout, r.stderr }
func (r rolesRun) out() (int, string, string)  { return r.code, r.stdout, r.stderr }

// checkShaperGolden compares got with testdata/shaper-golden/<name>.golden, or rewrites
// the file under -update-shaper-golden.
func checkShaperGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "shaper-golden", name+".golden")
	if *updateShaperGolden {
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
		t.Fatalf("read golden file: %v (record it with -update-shaper-golden)", err)
	}
	if got == string(want) {
		return
	}
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Fatalf("%s differs from its golden file at line %d:\n got: %s\nwant: %s", name, i+1, g, w)
		}
	}
}

// A handoff that stays a draft: the JSON assessment, the presented view, and the same
// assessment of a clearance that was recorded for a draft.
func TestShaperGoldenDraftOutput(t *testing.T) {
	root, home := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	g := newGoldenTranscript(root, home)

	assess := runShaperTest(assessArgs(root), "")
	g.run("shaper", assessArgs(root), assess)
	g.run("shaper", assessArgs(root, "--view"), runShaperTest(assessArgs(root, "--view"), ""))

	rec := recordFromAssess(t, decodeAssess(t, assess), "affirm", "tui", nil)
	g.run("shaper", recordArgs(root, "--stdin"), runShaperTest(recordArgs(root, "--stdin"), rec))
	g.run("shaper", assessArgs(root), runShaperTest(assessArgs(root), ""))

	checkShaperGolden(t, "draft", g.text())
}

// A handoff that reaches ready, for each handoff version that can: what the verbs print
// before and after the clearance, the stored record's report, and the disclosure that
// version must carry.
func TestShaperGoldenReadyOutput(t *testing.T) {
	for _, tc := range []struct{ name, handoff string }{
		{"ready-v2", shaperTestHandoffV2},
		{"ready-v3", shaperTestHandoffV3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home := shaperWorktree(t, tc.handoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
			g := newGoldenTranscript(root, home)

			before := runShaperTest(assessArgs(root), "")
			g.run("shaper", assessArgs(root), before)
			rec := recordFromAssess(t, decodeAssess(t, before), "affirm", "tui", nil)
			g.run("shaper", recordArgs(root, "--stdin"), runShaperTest(recordArgs(root, "--stdin"), rec))
			g.run("shaper", assessArgs(root), runShaperTest(assessArgs(root), ""))
			g.run("shaper", assessArgs(root, "--view"), runShaperTest(assessArgs(root, "--view"), ""))

			checkShaperGolden(t, tc.name, g.text())
		})
	}
}

// goldenSourceCase is one way the handoff or the Goal source can fail to be read, set up
// inside a worktree and then pointed at with --handoff or --goal.
type goldenSourceCase struct {
	name string
	// setup changes the worktree and returns the --handoff and --goal arguments.
	setup func(t *testing.T, root string) (handoff, goal string)
}

func goldenSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func goldenSourceCases() []goldenSourceCase {
	return []goldenSourceCase{
		{"handoff missing", func(t *testing.T, root string) (string, string) { return "absent.json", "goal.json" }},
		{"goal missing", func(t *testing.T, root string) (string, string) { return "handoff.json", "absent.json" }},
		{"handoff is a symlink", func(t *testing.T, root string) (string, string) {
			outside := filepath.Join(t.TempDir(), "handoff.json")
			if err := os.WriteFile(outside, []byte(shaperTestHandoff), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(root, "handoff.json")); err != nil {
				t.Fatal(err)
			}
			goldenSymlink(t, outside, filepath.Join(root, "handoff.json"))
			return "handoff.json", "goal.json"
		}},
		{"goal is a symlink", func(t *testing.T, root string) (string, string) {
			outside := filepath.Join(t.TempDir(), "goal.json")
			if err := os.WriteFile(outside, []byte(shaperTestGoal("standalone-shaper-handoff", `[]`)), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(root, "goal.json")); err != nil {
				t.Fatal(err)
			}
			goldenSymlink(t, outside, filepath.Join(root, "goal.json"))
			return "handoff.json", "goal.json"
		}},
		{"handoff is a directory", func(t *testing.T, root string) (string, string) {
			if err := os.Remove(filepath.Join(root, "handoff.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "handoff.json"), 0o755); err != nil {
				t.Fatal(err)
			}
			return "handoff.json", "goal.json"
		}},
		{"goal is a directory", func(t *testing.T, root string) (string, string) {
			if err := os.Remove(filepath.Join(root, "goal.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "goal.json"), 0o755); err != nil {
				t.Fatal(err)
			}
			return "handoff.json", "goal.json"
		}},
		{"handoff through a symlinked directory out of the root", func(t *testing.T, root string) (string, string) {
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "handoff.json"), []byte(shaperTestHandoff), 0o600); err != nil {
				t.Fatal(err)
			}
			goldenSymlink(t, outside, filepath.Join(root, "escape"))
			return "escape/handoff.json", "goal.json"
		}},
		{"goal through a symlinked directory out of the root", func(t *testing.T, root string) (string, string) {
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "goal.json"), []byte(shaperTestGoal("standalone-shaper-handoff", `[]`)), 0o600); err != nil {
				t.Fatal(err)
			}
			goldenSymlink(t, outside, filepath.Join(root, "escape"))
			return "handoff.json", "escape/goal.json"
		}},
		{"handoff traverses above the root", func(t *testing.T, root string) (string, string) { return "../handoff.json", "goal.json" }},
		{"goal traverses above the root", func(t *testing.T, root string) (string, string) { return "handoff.json", "sub/../../goal.json" }},
		{"handoff is an absolute path", func(t *testing.T, root string) (string, string) {
			return filepath.Join(root, "handoff.json"), "goal.json"
		}},
		{"goal is an absolute path", func(t *testing.T, root string) (string, string) {
			return "handoff.json", filepath.Join(root, "goal.json")
		}},
		{"handoff is the root itself", func(t *testing.T, root string) (string, string) { return ".", "goal.json" }},
		{"goal is empty", func(t *testing.T, root string) (string, string) { return "handoff.json", "" }},
		{"handoff in a nested directory", func(t *testing.T, root string) (string, string) {
			if err := os.MkdirAll(filepath.Join(root, "nested", "dir"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(root, "handoff.json"), filepath.Join(root, "nested", "dir", "handoff.json")); err != nil {
				t.Fatal(err)
			}
			return "./nested/dir/../dir/handoff.json", "goal.json"
		}},
		{"handoff is not a valid handoff", func(t *testing.T, root string) (string, string) {
			if err := os.WriteFile(filepath.Join(root, "handoff.json"), []byte(`{"version":9}`), 0o600); err != nil {
				t.Fatal(err)
			}
			return "handoff.json", "goal.json"
		}},
		{"goal is not a valid goal", func(t *testing.T, root string) (string, string) {
			if err := os.WriteFile(filepath.Join(root, "goal.json"), []byte(`not json`), 0o600); err != nil {
				t.Fatal(err)
			}
			return "handoff.json", "goal.json"
		}},
		{"goal names another project", func(t *testing.T, root string) (string, string) {
			if err := os.WriteFile(filepath.Join(root, "goal.json"), []byte(shaperTestGoal("another-project", `[]`)), 0o600); err != nil {
				t.Fatal(err)
			}
			return "handoff.json", "goal.json"
		}},
	}
}

// Every way a source can fail to be read, as 'shaper assess' reports it: an unreadable
// source is an error naming why, and one that is readable but not a valid handoff or Goal
// reaches the assessment as a blocker.
func TestShaperGoldenSourceRefusals(t *testing.T) {
	var all strings.Builder
	for _, tc := range goldenSourceCases() {
		root, home := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
		handoff, goal := tc.setup(t, root)
		args := []string{"assess", "--root", root, "--handoff", handoff, "--goal", goal}
		g := newGoldenTranscript(root, home)
		g.write("== %s\n", tc.name)
		g.run("shaper", args, runShaperTest(args, ""))
		all.WriteString(g.text())
		all.WriteString("\n")
	}
	checkShaperGolden(t, "source-refusals", all.String())
}

// 'roles match-shaper' reads the handoff through the same contained read, and prints the
// same refusals with its own prefix.
func TestShaperGoldenRolesMatchShaper(t *testing.T) {
	var all strings.Builder
	for _, tc := range goldenSourceCases() {
		root, home := shaperWorktree(t, shaperTestHandoffV3, shaperTestGoal("standalone-shaper-handoff", `[]`))
		handoff, _ := tc.setup(t, root)
		args := []string{"match-shaper", "--root", root, "--handoff", handoff}
		g := newGoldenTranscript(root, home)
		g.write("== %s\n", tc.name)
		g.run("roles", args, runRolesTest(args, ""))
		all.WriteString(g.text())
		all.WriteString("\n")
	}
	checkShaperGolden(t, "roles-match-shaper", all.String())
}
