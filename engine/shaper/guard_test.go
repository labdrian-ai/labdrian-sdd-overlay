package shaper

import (
	"strings"
	"testing"
)

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

// DecideGuard decides on the command a shell tool is about to run and the paths the file tools
// are about to write: how a hook input is read is engine/hookwire's, and the clearance guard's
// own words for an input it cannot read are in GuardUnreadable.
func TestDecideGuard(t *testing.T) {
	for _, tc := range []struct {
		name     string
		call     GuardCall
		wantDeny bool
	}{
		{"bash record verb denied", GuardCall{Command: "gentle-ai-overlay shaper clearance record --stdin"}, true},
		{"bash store path denied", GuardCall{Command: "cat ~/.local/state/labdrian/shaper-clearance/p/g/x.json"}, true},
		{"write into store denied", GuardCall{FilePath: "/home/u/.local/state/labdrian/shaper-clearance/p/g/x.json"}, true},
		{"edit into store denied", GuardCall{FilePath: "/s/labdrian/shaper-clearance/p/g/x.json"}, true},
		{"notebook into store denied", GuardCall{NotebookPath: "/s/labdrian/shaper-clearance/n.ipynb"}, true},
		{"a command and a path, the command alone denies", GuardCall{Command: "gentle-ai-overlay shaper clearance record", FilePath: "/repo/doc.md"}, true},
		{"a command and a path, the path alone denies", GuardCall{Command: "ls", FilePath: "/s/labdrian/shaper-clearance/x"}, true},
		{"bash unrelated allowed", GuardCall{Command: "go test ./..."}, false},
		{"a file outside the store allowed (what is written is not read)", GuardCall{FilePath: "/repo/doc.md"}, false},
		{"a path that only starts like the store is not the store", GuardCall{FilePath: "/repo/labdrian/shaper-clearance-notes.md"}, true},
		{"the zero call allowed", GuardCall{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := DecideGuard(tc.call)
			if v.Deny != tc.wantDeny {
				t.Fatalf("DecideGuard(%+v) = %+v, want deny=%v", tc.call, v, tc.wantDeny)
			}
			if !v.Deny && v.Reason != "" {
				t.Errorf("allow carries the reason %q", v.Reason)
			}
			if v.Deny {
				if v.Reason != guardDenyMessage {
					t.Errorf("deny reason %q, want the guard's own message", v.Reason)
				}
				for _, want := range []string{"speed bump", "same OS user"} {
					if !strings.Contains(v.Reason, want) {
						t.Errorf("deny reason %q does not state %q", v.Reason, want)
					}
				}
			}
		})
	}
}

// The guard fails closed: input the hook adapter could not read is denied, because a guard
// that cannot see the call cannot vouch for it. The denial is one fixed sentence.
func TestGuardUnreadableDeniesWithOneStableSentence(t *testing.T) {
	v := GuardUnreadable()
	want := guardDenyMessage + " (this tool call could not be read as a command or a file path, so the guard denied it; send it again as a well-formed tool call)"
	if !v.Deny || v.Reason != want {
		t.Errorf("GuardUnreadable() = %+v, want a denial with reason %q", v, want)
	}
}

// A call over the bound is denied too, and the denial names the bound.
func TestGuardTooLargeDeniesNamingTheBound(t *testing.T) {
	v := GuardTooLarge(8388608)
	want := guardDenyMessage + " (this tool call is larger than the 8388608 bytes the guard reads, so the guard denied it; send a smaller call)"
	if !v.Deny || v.Reason != want {
		t.Errorf("GuardTooLarge(8388608) = %+v, want a denial with reason %q", v, want)
	}
}
