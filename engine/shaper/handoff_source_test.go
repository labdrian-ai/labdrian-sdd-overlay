package shaper

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// assertLoadHandoffRejected fails unless LoadHandoff returned an error
// together with the zero HandoffSource, so no rejection leaks partial state.
func assertLoadHandoffRejected(t *testing.T, got HandoffSource, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("LoadHandoff accepted %s", what)
	}
	if !reflect.DeepEqual(got, HandoffSource{}) {
		t.Errorf("LoadHandoff returned a partial HandoffSource alongside error %v: %#v", err, got)
	}
}

func TestLoadHandoffReturnsParsedHandoffRawBytesAndDigest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shaper"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := documentWith(t, nil)
	writeGoalFile(t, filepath.Join(root, "shaper"), "handoff.json", data)

	got, err := LoadHandoff(root, "./shaper/handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff: %v", err)
	}
	if got.SourcePath != filepath.Join("shaper", "handoff.json") {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, filepath.Join("shaper", "handoff.json"))
	}
	if string(got.Bytes) != string(data) {
		t.Errorf("Bytes = %q, want %q", got.Bytes, data)
	}
	sum := sha256.Sum256(data)
	if want := hex.EncodeToString(sum[:]); got.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", got.SHA256, want)
	}
	want, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(got.Handoff, want) {
		t.Errorf("Handoff = %#v, want %#v", got.Handoff, want)
	}
}

func TestLoadHandoffSHA256ChangesOnWhitespaceOnlyEdit(t *testing.T) {
	root := t.TempDir()
	data := documentWith(t, nil)
	writeGoalFile(t, root, "handoff.json", data)
	before, err := LoadHandoff(root, "handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff before edit: %v", err)
	}

	writeGoalFile(t, root, "handoff.json", append(append([]byte{}, data...), ' ', '\n'))
	after, err := LoadHandoff(root, "handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff after edit: %v", err)
	}
	if !reflect.DeepEqual(before.Handoff, after.Handoff) {
		t.Fatalf("whitespace-only edit changed the parsed Handoff")
	}
	if before.SHA256 == after.SHA256 {
		t.Errorf("SHA256 did not change on a whitespace-only edit: %q", before.SHA256)
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
			got, err := LoadHandoff(root, path)
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
	got, err := LoadHandoff("relative/root", "handoff.json")
	assertLoadHandoffRejected(t, got, err, "a relative worktreeRoot")
}
