package shaper

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardStoreMarkerIsTheStorePathSegment(t *testing.T) {
	if want := filepath.Join(storeComponents...); GuardStoreMarker != want {
		t.Fatalf("GuardStoreMarker = %q, want the store path segment %q", GuardStoreMarker, want)
	}
	store := FileStore{stateHome: "/state"}
	path, err := store.Path("p", "g", strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !GuardMatches(path) {
		t.Errorf("GuardMatches(%q) = false for a store record path", path)
	}
}

func TestGuardMatchesRecordEntryPointAndStorePath(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"record verb", "gentle-ai-overlay shaper clearance record --stdin --root /r", true},
		{"record verb mid pipeline", "echo '{}' | ~/.claude/bin/gentle-ai-overlay shaper clearance record --stdin", true},
		{"record verb inside sh -c", `sh -c "gentle-ai-overlay shaper clearance record --stdin"`, true},
		{"extra whitespace", "gentle-ai-overlay shaper   clearance\trecord --stdin", true},
		{"line continuation", "gentle-ai-overlay shaper \\\n clearance \\\n record --stdin", true},
		{"store write", "echo x > ~/.local/state/labdrian/shaper-clearance/p/g/a.json", true},
		{"store listing", "ls $XDG_STATE_HOME/labdrian/shaper-clearance", true},
		{"assess is allowed", "gentle-ai-overlay shaper assess --root /r --handoff h.json --goal g.json", false},
		{"branch name is allowed", "git checkout feat/shaper-clearance", false},
		{"unrelated", "go test ./...", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := GuardMatches(tc.text); got != tc.want {
				t.Errorf("GuardMatches(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestRunGuardHook(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		wantCode int
	}{
		{"bash record verb denied", `{"tool_name":"Bash","tool_input":{"command":"gentle-ai-overlay shaper clearance record --stdin"}}`, 2},
		{"bash store path denied", `{"tool_name":"Bash","tool_input":{"command":"cat ~/.local/state/labdrian/shaper-clearance/p/g/x.json"}}`, 2},
		{"write into store denied", `{"tool_name":"Write","tool_input":{"file_path":"/home/u/.local/state/labdrian/shaper-clearance/p/g/x.json","content":"{}"}}`, 2},
		{"edit into store denied", `{"tool_name":"Edit","tool_input":{"file_path":"/s/labdrian/shaper-clearance/p/g/x.json"}}`, 2},
		{"notebook into store denied", `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/s/labdrian/shaper-clearance/n.ipynb"}}`, 2},
		{"bash unrelated allowed", `{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`, 0},
		{"write content mentioning verb allowed", `{"tool_name":"Write","tool_input":{"file_path":"/repo/doc.md","content":"run shaper clearance record"}}`, 0},
		{"malformed input denied", `{"tool_name":`, 2},
		{"empty input denied", ``, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, msg := RunGuardHook([]byte(tc.input))
			if code != tc.wantCode {
				t.Fatalf("RunGuardHook code = %d (%q), want %d", code, msg, tc.wantCode)
			}
			if code == 0 && msg != "" {
				t.Errorf("allow returned message %q", msg)
			}
			if code == 2 {
				for _, want := range []string{"speed bump", "same OS user"} {
					if !strings.Contains(msg, want) {
						t.Errorf("deny message %q does not state %q", msg, want)
					}
				}
			}
		})
	}
}
