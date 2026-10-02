package fsadapter

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// These tests read real files through the file-backed ContainedSource, directly and end to
// end through the domain's own entry points (shaper.BindGoal, shaper.LoadHandoff,
// shaper.ReadContainedSource). The domain's side, which path it asks for and what it makes
// of the bytes, is proved against a fake source in package shaper; the platform-specific
// races are in contained_race_linux_test.go.

const validHandoffJSON = `{"version":1,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha","architecture":"Layered CLI with a shared jsonstrict wire gate.","stages":["Extract jsonstrict.","Refactor goal.Parse onto jsonstrict."],"acceptance":["go test ./... passes."],"out_of_scope":["Runtime clearance UI."]}`

// documentWith is the valid handoff document with some fields replaced.
func documentWith(t *testing.T, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validHandoffJSON), &fields); err != nil {
		t.Fatalf("decode valid test document: %v", err)
	}
	for key, value := range changes {
		fields[key] = value
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode test document: %v", err)
	}
	return data
}

func sampleHandoff() shaper.Handoff {
	return shaper.Handoff{
		Version:      1,
		ProjectID:    "standalone-shaper-handoff",
		GoalID:       "goal-alpha",
		Architecture: "Layered CLI with a shared jsonstrict wire gate.",
		Stages:       []string{"Extract jsonstrict.", "Refactor goal.Parse onto jsonstrict."},
		Acceptance:   []string{"go test ./... passes."},
		OutOfScope:   []string{"Runtime clearance UI."},
	}
}

func goalV2JSON(projectID, goalID string) string {
	return `{"version":2,"project_id":"` + projectID + `","goal_id":"` + goalID + `",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":[],"acceptance_criteria":["Binding succeeds."],` +
		`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

func writeGoalFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write goal file %s: %v", path, err)
	}
	return path
}

// assertRejectedWithoutPartialBinding fails unless BindGoal returned an error
// together with the zero GoalBinding, so no rejection leaks partial state.
func assertRejectedWithoutPartialBinding(t *testing.T, got shaper.GoalBinding, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("BindGoal accepted %s", what)
	}
	if !reflect.DeepEqual(got, shaper.GoalBinding{}) {
		t.Errorf("BindGoal returned a partial GoalBinding alongside error %v: %#v", err, got)
	}
}

// assertLoadHandoffRejected fails unless LoadHandoff returned an error
// together with the zero HandoffSource, so no rejection leaks partial state.
func assertLoadHandoffRejected(t *testing.T, got shaper.HandoffSource, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("LoadHandoff accepted %s", what)
	}
	if !reflect.DeepEqual(got, shaper.HandoffSource{}) {
		t.Errorf("LoadHandoff returned a partial HandoffSource alongside error %v: %#v", err, got)
	}
}

func TestReadContainedReturnsTheExactBytesOfARegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Bytes the domain would never accept as a document: the source returns them as they
	// are, without trimming, decoding, or normalizing anything.
	data := []byte("  \r\n{\"not\": json}\x00\xff\n\n")
	writeGoalFile(t, filepath.Join(root, "sub"), "any.bin", data)

	got, err := ContainedSource{}.ReadContained(root, filepath.Join("sub", "any.bin"), "test source")
	if err != nil {
		t.Fatalf("ReadContained: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("ReadContained = %q, want the file's bytes %q", got, data)
	}
}

// The root is designated by the caller and trusted, so it is resolved by path: a root that
// is itself a symlink to a directory holds the files in that directory.
func TestReadContainedAcceptsARootThatIsASymlink(t *testing.T) {
	real := t.TempDir()
	writeGoalFile(t, real, "goal.json", []byte("{}"))
	link := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	got, err := ContainedSource{}.ReadContained(link, "goal.json", "test source")
	if err != nil || string(got) != "{}" {
		t.Errorf("ReadContained under a symlinked root = %q, %v, want the file's bytes", got, err)
	}
}

// What is read must lie inside the root by what was opened, whatever the caller did to the
// path: the adapter does not rely on the domain's lexical cleaning, so a path that leaves
// the root is refused by the proof from the open file.
func TestReadContainedRefusesAPathThatLeavesTheRootWithoutTheDomainsCleaning(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoalFile(t, base, "outside.json", []byte("{}"))

	for _, rel := range []string{
		filepath.Join("..", "outside.json"),
		filepath.Join("sub", "..", "..", "outside.json"),
	} {
		got, err := ContainedSource{}.ReadContained(root, rel, "test source")
		if err == nil || got != nil || !strings.Contains(err.Error(), "resolves outside the worktree root") {
			t.Errorf("ReadContained(%q) = %q, %v, want a refusal naming the root", rel, got, err)
		}
	}
}

// The label names the source in every refusal the adapter words, and the path is the one it
// was given.
func TestReadContainedNamesTheSourceByItsLabelInEveryRefusal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeGoalFile(t, outside, "real.json", []byte("{}"))
	if err := os.Symlink(filepath.Join(outside, "real.json"), filepath.Join(root, "link.json")); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ rel, want string }{
		{"absent.json", `widget "absent.json" is not accessible: lstat ` + filepath.Join(root, "absent.json") + `: no such file or directory`},
		{"link.json", `widget "link.json" must not be a symlink`},
		{"dir.json", `widget "dir.json" must be a regular file`},
		{"escape/real.json", `widget "escape/real.json" resolves outside the worktree root`},
	} {
		_, err := ContainedSource{}.ReadContained(root, tc.rel, "widget")
		if err == nil || err.Error() != tc.want {
			t.Errorf("ReadContained(%q) = %v, want %q", tc.rel, err, tc.want)
		}
	}
}

// The test seam is a field of the value, not of the package: a hook set on one source is
// not called by another, so two sources in one process cannot disturb each other's tests.
func TestTheOpenHookBelongsToTheSourceItIsSetOn(t *testing.T) {
	root := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte("{}"))

	var stages []string
	hooked := ContainedSource{openHook: func(stage, joined string) {
		if joined != filepath.Join(root, "goal.json") {
			t.Errorf("hook joined path = %q, want the file under the root", joined)
		}
		stages = append(stages, stage)
	}}
	if _, err := (ContainedSource{}).ReadContained(root, "goal.json", "test source"); err != nil {
		t.Fatalf("ReadContained without a hook: %v", err)
	}
	if len(stages) != 0 {
		t.Fatalf("a hook set on one source was called by another: %v", stages)
	}
	if _, err := hooked.ReadContained(root, "goal.json", "test source"); err != nil {
		t.Fatalf("ReadContained with a hook: %v", err)
	}
	if want := []string{"pre-open", "post-open"}; !reflect.DeepEqual(stages, want) {
		t.Errorf("hook stages = %v, want %v, once each and in that order", stages, want)
	}
}

func TestBindGoalReadsTheGoalFromARealFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	writeGoalFile(t, filepath.Join(root, "sub"), "goal.json", data)
	h := sampleHandoff()

	got, err := shaper.BindGoal(ContainedSource{}, h, root, "./sub/goal.json")
	if err != nil {
		t.Fatalf("BindGoal: %v", err)
	}
	if got.SourcePath != filepath.Clean("./sub/goal.json") {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, filepath.Clean("./sub/goal.json"))
	}
	if string(got.GoalBytes) != string(data) {
		t.Errorf("GoalBytes = %q, want %q", got.GoalBytes, data)
	}
	if got.Goal.ProjectID != h.ProjectID || got.Goal.GoalID != h.GoalID {
		t.Errorf("Goal identity = (%q, %q), want (%q, %q)", got.Goal.ProjectID, got.Goal.GoalID, h.ProjectID, h.GoalID)
	}
}

func TestBindGoalRejectsWhatCannotBeReadAsAPlainContainedFile(t *testing.T) {
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string) string
	}{
		{"an absolute path", func(t *testing.T, root string) string { return writeGoalFile(t, root, "goal.json", data) }},
		{"a traversing path to a real outside file", func(t *testing.T, root string) string {
			outside := t.TempDir()
			writeGoalFile(t, outside, "goal.json", data)
			return filepath.Join("..", filepath.Base(outside), "goal.json")
		}},
		{"a path that cleans to the root", func(t *testing.T, root string) string { return "." }},
		{"a missing file", func(t *testing.T, root string) string { return "absent.json" }},
		{"a symlinked goal source pointing inside the root", func(t *testing.T, root string) string {
			real := writeGoalFile(t, t.TempDir(), "goal.json", data)
			if err := os.Symlink(real, filepath.Join(root, "goal.json")); err != nil {
				t.Skipf("symlink not supported in this environment: %v", err)
			}
			return "goal.json"
		}},
		{"a source reached through a symlinked ancestor directory escaping the root", func(t *testing.T, root string) string {
			outside := t.TempDir()
			writeGoalFile(t, outside, "goal.json", data)
			if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
				t.Skipf("symlink not supported in this environment: %v", err)
			}
			return "escape/goal.json"
		}},
		{"a directory in place of a file", func(t *testing.T, root string) string {
			if err := os.MkdirAll(filepath.Join(root, "goal.json"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			return "goal.json"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := tc.setup(t, root)
			got, err := shaper.BindGoal(ContainedSource{}, sampleHandoff(), root, path)
			assertRejectedWithoutPartialBinding(t, got, err, tc.name)
		})
	}
}

func TestLoadHandoffReadsTheHandoffFromARealFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shaper"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := documentWith(t, nil)
	writeGoalFile(t, filepath.Join(root, "shaper"), "handoff.json", data)

	got, err := shaper.LoadHandoff(ContainedSource{}, root, "./shaper/handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff: %v", err)
	}
	if got.SourcePath != filepath.Join("shaper", "handoff.json") {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, filepath.Join("shaper", "handoff.json"))
	}
	if string(got.Bytes) != string(data) {
		t.Errorf("Bytes = %q, want %q", got.Bytes, data)
	}
	want, err := shaper.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(got.Handoff, want) {
		t.Errorf("Handoff = %#v, want %#v", got.Handoff, want)
	}
}

func TestLoadHandoffRejectsUncontainedOrIrregularSources(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, root string) string
		wantErr string
	}{
		{
			name:    "empty worktree root",
			setup:   func(t *testing.T, root string) string { return "handoff.json" },
			wantErr: "worktreeRoot must not be empty",
		},
		{
			name:    "empty path",
			setup:   func(t *testing.T, root string) string { return "" },
			wantErr: "handoffPath must not be empty",
		},
		{
			name: "absolute path",
			setup: func(t *testing.T, root string) string {
				return writeGoalFile(t, root, "handoff.json", documentWith(t, nil))
			},
			wantErr: "must be relative",
		},
		{
			name:    "traversal",
			setup:   func(t *testing.T, root string) string { return filepath.Join("..", "handoff.json") },
			wantErr: "must not traverse",
		},
		{
			name:    "root itself",
			setup:   func(t *testing.T, root string) string { return "." },
			wantErr: "worktree root itself",
		},
		{
			name:    "missing file",
			setup:   func(t *testing.T, root string) string { return "absent.json" },
			wantErr: "not accessible",
		},
		{
			name: "symlink inside root",
			setup: func(t *testing.T, root string) string {
				target := writeGoalFile(t, t.TempDir(), "handoff.json", documentWith(t, nil))
				if err := os.Symlink(target, filepath.Join(root, "handoff.json")); err != nil {
					t.Skipf("symlink not supported in this environment: %v", err)
				}
				return "handoff.json"
			},
			wantErr: "must not be a symlink",
		},
		{
			name: "symlinked ancestor escaping root",
			setup: func(t *testing.T, root string) string {
				outside := t.TempDir()
				writeGoalFile(t, outside, "handoff.json", documentWith(t, nil))
				if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
					t.Skipf("symlink not supported in this environment: %v", err)
				}
				return "escape/handoff.json"
			},
			wantErr: "outside the worktree root",
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) string {
				if err := os.MkdirAll(filepath.Join(root, "handoff.json"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return "handoff.json"
			},
			wantErr: "must be a regular file",
		},
		{
			name: "invalid handoff",
			setup: func(t *testing.T, root string) string {
				writeGoalFile(t, root, "handoff.json", documentWith(t, map[string]any{"version": 2}))
				return "handoff.json"
			},
			wantErr: "parse shaper handoff",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := tc.setup(t, root)
			if tc.name == "empty worktree root" {
				root = ""
			}
			got, err := shaper.LoadHandoff(ContainedSource{}, root, path)
			assertLoadHandoffRejected(t, got, err, tc.name)
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("LoadHandoff error = %v, want it to contain %q", err, tc.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "load handoff: ") {
				t.Errorf("LoadHandoff error = %v, want the %q prefix", err, "load handoff: ")
			}
		})
	}
}

func TestLoadHandoffRejectsRelativeWorktreeRoot(t *testing.T) {
	got, err := shaper.LoadHandoff(ContainedSource{}, "relative/root", "handoff.json")
	assertLoadHandoffRejected(t, got, err, "a relative worktreeRoot")
}

func TestReadContainedSourceReadsARegularFileInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "h.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleaned, data, err := shaper.ReadContainedSource(ContainedSource{}, root, "./h.json")
	if err != nil {
		t.Fatalf("ReadContainedSource: %v", err)
	}
	if cleaned != "h.json" || string(data) != "not json" {
		t.Errorf("ReadContainedSource = (%q, %q)", cleaned, data)
	}
	for _, bad := range []string{"../h.json", "/etc/passwd", "missing.json"} {
		if _, _, err := shaper.ReadContainedSource(ContainedSource{}, root, bad); err == nil {
			t.Errorf("ReadContainedSource(%q) accepted", bad)
		}
	}
}
