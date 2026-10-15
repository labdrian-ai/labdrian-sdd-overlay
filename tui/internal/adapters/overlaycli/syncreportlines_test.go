package overlaycli

// The adapter reads the lines of a sync-check report one output at a time; the
// domain adds them up across the invocations of an action. Together they have
// to be what the program did before, when the domain read the whole transcript
// of an action as one text: syncreport_oracle_test.go keeps that reader as it
// was, and these tests hold the two against each other.

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// transcriptOf is the text the old action runner built from the outputs of an
// action's invocations, which is what the old reader read: each output after
// the "$" line of its invocation, a blank line between invocations.
func transcriptOf(outputs []string) string {
	var sb strings.Builder
	for i, out := range outputs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("$ bin/labdrian-overlay sync-check --target claude\n")
		sb.WriteString(out)
	}
	return sb.String()
}

// addedUp is what the program makes of the same outputs now: the lines read
// from each, added up in order as one report.
func addedUp(outputs []string) []domain.TargetVerdict {
	var report domain.SyncReport
	for _, out := range outputs {
		report.Add(readSyncLines(out))
	}
	return report.Verdicts()
}

func TestSyncLines_AddUpLikeTheOneReaderTheyReplaced(t *testing.T) {
	const full = `=== sync-check: claude (/stub/.claude) ===
  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)
  IN_SYNC: skills/foo
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=2 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=no
ACTION:claude: run 'labdrian apply --target claude'
`
	tests := []struct {
		name    string
		outputs []string
	}{
		{"one report", []string{full}},
		{"nothing", []string{""}},
		{"no output at all", nil},
		{"the same target in two outputs", []string{full, strings.ReplaceAll(full, "UPSTREAM_CHANGED=0", "UPSTREAM_CHANGED=3")}},
		{"a target, another, and the first again", []string{
			"=== sync-check: claude ===\nVERDICT:claude:UPSTREAM_CHANGED=1\n=== sync-check: codex ===\nVERDICT:codex:OVERLAY_NOT_DEPLOYED=1\n",
			"=== sync-check: claude ===\nVERDICT:claude:OVERLAY_NOT_DEPLOYED=2\nACTION:claude: again\n",
		}},
		{"a later line that names fewer fields", []string{
			"VERDICT:claude:UPSTREAM_CHANGED=5 REPO_BEHIND_ORIGIN=4 REPO_BEHIND_RELEASE=3 RECORDED_VERSION=v1 DIGEST_MATCH=no\n",
			"VERDICT:claude:OVERLAY_NOT_DEPLOYED=1\n",
		}},
		{"a file line whose section was opened in an earlier output", []string{
			"=== sync-check: claude ===\nVERDICT:claude:UPSTREAM_CHANGED=0\n",
			"  OVERLAY_NOT_DEPLOYED: agents/CARRIED.md (live missing)\n=== sync-check: codex ===\nVERDICT:codex:UPSTREAM_CHANGED=0\n",
		}},
		{"a file line with no section open anywhere", []string{"  IN_SYNC: agents/A.md\n", "VERDICT:claude:UPSTREAM_CHANGED=0\n"}},
		{"a section with no name closes the one before it", []string{"=== sync-check: claude ===\nVERDICT:claude:\n", "=== sync-check: \n  IN_SYNC: agents/A.md\n"}},
		{"values that cannot be read", []string{
			"VERDICT:claude:UPSTREAM_CHANGED=x OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=x REPO_BEHIND_RELEASE=NA\n",
			"VERDICT:claude:UPSTREAM_CHANGED=2 REPO_BEHIND_ORIGIN=7 REPO_BEHIND_RELEASE=y OVERLAY_NOT_DEPLOYED\n",
		}},
		{"a key given twice on one line", []string{"VERDICT:claude:REPO_BEHIND_ORIGIN=5 REPO_BEHIND_ORIGIN=x UPSTREAM_CHANGED=1 UPSTREAM_CHANGED=y\n"}},
		{"lines that are not about anything", []string{"noise\n\nVERDICT:nocolon\nACTION:nocolon\n$ bin/labdrian-overlay x\n"}},
		{"an action before its verdict, with colons in its text", []string{"ACTION:claude: run: this: now\nVERDICT:claude:UPSTREAM_CHANGED=1\n"}},
		{"output with no final newline, then another", []string{"VERDICT:claude:UPSTREAM_CHANGED=1", "ACTION:claude: after"}},
		{"carriage returns", []string{"=== sync-check: claude ===\r\nVERDICT:claude:UPSTREAM_CHANGED=1\r\n"}},
		{"three outputs", []string{full, "=== sync-check: codex ===\nVERDICT:codex:UPSTREAM_CHANGED=1\n", "  UPSTREAM_CHANGED: agents/X.md\nVERDICT:claude:OVERLAY_NOT_DEPLOYED=0\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := oldParseSyncCheck(transcriptOf(tc.outputs))
			if got := addedUp(tc.outputs); !reflect.DeepEqual(got, want) {
				t.Errorf("read output by output and added up:\n%+v\nthe old reader of the whole transcript:\n%+v", got, want)
			}
		})
	}
}

// The same check over a few hundred reports put together from a vocabulary of
// lines that includes the awkward ones, from a fixed seed.
func TestSyncLines_AddUpLikeTheOneReaderTheyReplacedOverGeneratedReports(t *testing.T) {
	vocabulary := []string{
		"=== sync-check: claude (/p) ===", "=== sync-check: codex ===", "=== sync-check: ", "=== sync-check: pi (/q) ===",
		"VERDICT:claude:UPSTREAM_CHANGED=1 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=2 REPO_BEHIND_RELEASE=3 RECORDED_VERSION=v1 DIGEST_MATCH=no",
		"VERDICT:claude:OVERLAY_NOT_DEPLOYED=2", "VERDICT:codex:UPSTREAM_CHANGED=x REPO_BEHIND_ORIGIN=NA",
		"VERDICT:codex:REPO_BEHIND_RELEASE=7 DIGEST_MATCH=yes", "VERDICT:pi:", "VERDICT:nocolon", "VERDICT:pi:REPO_BEHIND_ORIGIN=NA RECORDED_VERSION=NA",
		"ACTION:claude: run a:b", "ACTION:codex: x", "ACTION:pi", "ACTION:opencode:  spaced  ",
		"  IN_SYNC: agents/A.md (same)", "  OVERLAY_NOT_DEPLOYED: agents/B.md", "  UPSTREAM_CHANGED: agents/C.md (x)",
		"  IN_SYNC: skills/foo", "  UPSTREAM_CHANGED: agents/", "", "noise", "$ bin/labdrian-overlay sync-check",
	}
	rng := rand.New(rand.NewSource(44))
	for i := 0; i < 400; i++ {
		outputs := make([]string, 1+rng.Intn(4))
		for j := range outputs {
			lines := make([]string, rng.Intn(7))
			for k := range lines {
				lines[k] = vocabulary[rng.Intn(len(vocabulary))]
			}
			outputs[j] = strings.Join(lines, "\n")
			if rng.Intn(4) > 0 { // most outputs end in a newline, some do not
				outputs[j] += "\n"
			}
		}
		want := oldParseSyncCheck(transcriptOf(outputs))
		if got := addedUp(outputs); !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d, outputs %q:\nread output by output and added up:\n%+v\nthe old reader of the whole transcript:\n%+v", i, outputs, got, want)
		}
	}
}

// A VERDICT line gives only the fields it names and could read: a count that is
// no number is zero for the two file counts (as it always was) and not given for
// the two behind counts, whose NA is a value of its own.
func TestReadSyncLines_GivesOnlyTheFieldsAVerdictLineCouldRead(t *testing.T) {
	lines := readSyncLines("VERDICT:claude:UPSTREAM_CHANGED=x OVERLAY_NOT_DEPLOYED=2 REPO_BEHIND_ORIGIN=x REPO_BEHIND_RELEASE=NA RECORDED_VERSION=v1 DIGEST_MATCH=\n")

	if len(lines) != 1 || lines[0].Kind != domain.SyncVerdictLine || lines[0].Target != "claude" {
		t.Fatalf("lines = %+v, want the one verdict line of claude", lines)
	}
	f := lines[0].Fields
	if f.UpstreamChanged == nil || *f.UpstreamChanged != 0 {
		t.Errorf("UpstreamChanged = %v, want 0: a count that is no number reads as zero", f.UpstreamChanged)
	}
	if f.OverlayNotDeployed == nil || *f.OverlayNotDeployed != 2 {
		t.Errorf("OverlayNotDeployed = %v, want 2", f.OverlayNotDeployed)
	}
	if f.RepoBehindOrigin != nil {
		t.Errorf("RepoBehindOrigin = %v, want it not given: an origin count that is no number says nothing", *f.RepoBehindOrigin)
	}
	if f.RepoBehindRelease == nil || *f.RepoBehindRelease != domain.RepoBehindOriginNA {
		t.Errorf("RepoBehindRelease = %v, want the NA sentinel", f.RepoBehindRelease)
	}
	if f.RecordedVersion == nil || *f.RecordedVersion != "v1" {
		t.Errorf("RecordedVersion = %v, want v1", f.RecordedVersion)
	}
	if f.DigestMatch == nil || *f.DigestMatch != "" {
		t.Errorf("DigestMatch = %v, want it given, empty", f.DigestMatch)
	}
}

// What the reader makes of each kind of line, in the order it read them.
func TestReadSyncLines_TypesEachKindOfLineInOrder(t *testing.T) {
	lines := readSyncLines("=== sync-check: claude (/p) ===\n  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)\n  IN_SYNC: skills/foo\nVERDICT:claude:UPSTREAM_CHANGED=1\nACTION:claude:  run it  \nnoise\n")

	want := []domain.SyncLineKind{domain.SyncSectionLine, domain.SyncAgentFileLine, domain.SyncVerdictLine, domain.SyncActionLine}
	if len(lines) != len(want) {
		t.Fatalf("lines = %+v, want %d (the skills file and the noise say nothing)", lines, len(want))
	}
	for i, k := range want {
		if lines[i].Kind != k {
			t.Errorf("line %d is kind %d, want %d", i, lines[i].Kind, k)
		}
	}
	if lines[0].Target != "claude" || lines[1].File != (domain.AgentFileEntry{Path: "agents/GADU.md", Status: "OVERLAY_NOT_DEPLOYED"}) || lines[3].Remedy != "run it" {
		t.Errorf("lines = %+v, want the section's target, the file's path and status, and the trimmed remedy", lines)
	}
}
