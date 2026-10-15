package overlaycli

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// TestParseSyncCheckDashboard verifies a sample VERDICT/ACTION block parses
// into the correct colored statuses for each target.
func TestParseSyncCheckDashboard(t *testing.T) {
	sample := `
=== sync-check: claude ===
  IN_SYNC: skills/foo
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0
ACTION:claude: in sync with gentle-ai (healthy)

=== sync-check: opencode ===
  OVERLAY_NOT_DEPLOYED: skills/bar (live missing)
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=2 REPO_BEHIND_ORIGIN=NA
ACTION:opencode: run 'labdrian apply --target opencode'

=== sync-check: codex ===
  UPSTREAM_CHANGED: skills/baz
VERDICT:codex:UPSTREAM_CHANGED=3 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=5
ACTION:codex: gentle-ai sync detected: run 'labdrian capture --target codex' then 'labdrian apply'
`

	verdicts := parseSyncCheck(sample)
	if len(verdicts) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(verdicts))
	}

	want := []struct {
		target       string
		status       domain.SyncStatus
		uc, ond, rbo int
	}{
		{"claude", domain.SyncHealthy, 0, 0, 0},
		{"opencode", domain.SyncNeedsApply, 0, 2, domain.RepoBehindOriginNA},
		{"codex", domain.SyncNeedsCapture, 3, 1, 5},
	}

	for i, w := range want {
		v := verdicts[i]
		if v.Target != w.target {
			t.Errorf("verdict %d: target = %q, want %q", i, v.Target, w.target)
		}
		if v.Status != w.status {
			t.Errorf("%s: status = %d, want %d", w.target, v.Status, w.status)
		}
		if v.UpstreamChanged != w.uc || v.OverlayNotDeployed != w.ond {
			t.Errorf("%s: counts = (%d,%d), want (%d,%d)",
				w.target, v.UpstreamChanged, v.OverlayNotDeployed, w.uc, w.ond)
		}
		if v.RepoBehindOrigin != w.rbo {
			t.Errorf("%s: RepoBehindOrigin = %d, want %d (R-002/R-004 NA->-1 parsing)", w.target, v.RepoBehindOrigin, w.rbo)
		}
		if v.Action == "" {
			t.Errorf("%s: action should not be empty", w.target)
		}
	}
}

// TestParseSyncCheckAgentFiles verifies that per-file agent lines under a target
// section are captured into AgentFiles on the corresponding TargetVerdict.
func TestParseSyncCheckAgentFiles(t *testing.T) {
	sample := `
=== sync-check: claude (/home/user/.claude) ===
  IN_SYNC: skills/foo
  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1
ACTION:claude: run 'labdrian apply --target claude'

=== sync-check: opencode (/home/user/.config/opencode) ===
  IN_SYNC: skills/bar
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0
ACTION:opencode: in sync with gentle-ai (healthy)
`
	verdicts := parseSyncCheck(sample)
	if len(verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(verdicts))
	}

	claudeV := verdicts[0]
	if claudeV.Target != "claude" {
		t.Fatalf("first verdict target = %q, want claude", claudeV.Target)
	}
	if len(claudeV.AgentFiles) != 1 {
		t.Fatalf("claude: expected 1 agent file entry, got %d: %+v", len(claudeV.AgentFiles), claudeV.AgentFiles)
	}
	af := claudeV.AgentFiles[0]
	if af.Path != "agents/GADU.md" {
		t.Errorf("claude agent file path = %q, want %q", af.Path, "agents/GADU.md")
	}
	if af.Status != "OVERLAY_NOT_DEPLOYED" {
		t.Errorf("claude agent file status = %q, want OVERLAY_NOT_DEPLOYED", af.Status)
	}

	// skills/foo must NOT appear in AgentFiles (not an agents/ path).
	opencodeV := verdicts[1]
	if len(opencodeV.AgentFiles) != 0 {
		t.Errorf("opencode: expected 0 agent file entries, got %d: %+v", len(opencodeV.AgentFiles), opencodeV.AgentFiles)
	}
}

// TestParseSyncCheckAgentFilesAllStatuses verifies all three per-file statuses
// (IN_SYNC, OVERLAY_NOT_DEPLOYED, UPSTREAM_CHANGED) are captured correctly.
func TestParseSyncCheckAgentFilesAllStatuses(t *testing.T) {
	cases := []struct {
		line   string
		status string
	}{
		{"  IN_SYNC: agents/GADU.md", "IN_SYNC"},
		{"  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)", "OVERLAY_NOT_DEPLOYED"},
		{"  UPSTREAM_CHANGED: agents/GADU.md", "UPSTREAM_CHANGED"},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			sample := "=== sync-check: claude (/home/user/.claude) ===\n" + tc.line + "\nVERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0\n"
			verdicts := parseSyncCheck(sample)
			if len(verdicts) != 1 {
				t.Fatalf("expected 1 verdict, got %d", len(verdicts))
			}
			if len(verdicts[0].AgentFiles) != 1 {
				t.Fatalf("expected 1 agent file entry, got %d", len(verdicts[0].AgentFiles))
			}
			if verdicts[0].AgentFiles[0].Status != tc.status {
				t.Errorf("status = %q, want %q", verdicts[0].AgentFiles[0].Status, tc.status)
			}
			if verdicts[0].AgentFiles[0].Path != "agents/GADU.md" {
				t.Errorf("path = %q, want agents/GADU.md", verdicts[0].AgentFiles[0].Path)
			}
		})
	}
}

// TestParseSyncCheck_ReleaseFields verifies the three new VERDICT keys parse
// onto TargetVerdict, including the REPO_BEHIND_RELEASE NA sentinel and the
// resulting classification (digest mismatch -> needs-apply here, since
// OVERLAY_NOT_DEPLOYED=0 but DIGEST_MATCH=no).
func TestParseSyncCheck_ReleaseFields(t *testing.T) {
	sample := `
=== sync-check: claude ===
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=2 RECORDED_VERSION=v1.3.0 DIGEST_MATCH=no
ACTION:claude: run 'labdrian apply --target claude' (release v1.4.0 available)

=== sync-check: opencode ===
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.4.0 DIGEST_MATCH=yes
ACTION:opencode: in sync with gentle-ai at v1.4.0

=== sync-check: codex ===
VERDICT:codex:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=NA REPO_BEHIND_RELEASE=NA RECORDED_VERSION=NA DIGEST_MATCH=NA
ACTION:codex: run 'labdrian apply --target codex'
`
	verdicts := parseSyncCheck(sample)
	if len(verdicts) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(verdicts))
	}

	claude := verdicts[0]
	if claude.RecordedVersion != "v1.3.0" {
		t.Errorf("claude.RecordedVersion = %q, want v1.3.0", claude.RecordedVersion)
	}
	if claude.DigestMatch != "no" {
		t.Errorf("claude.DigestMatch = %q, want no", claude.DigestMatch)
	}
	if claude.RepoBehindRelease != 2 {
		t.Errorf("claude.RepoBehindRelease = %d, want 2", claude.RepoBehindRelease)
	}
	if claude.Status != domain.SyncNeedsApply {
		t.Errorf("claude.Status = %d, want SyncNeedsApply (digest mismatch alone must classify as needs-apply, D2)", claude.Status)
	}

	opencode := verdicts[1]
	if opencode.RepoBehindRelease != 0 {
		t.Errorf("opencode.RepoBehindRelease = %d, want 0", opencode.RepoBehindRelease)
	}
	if opencode.Status != domain.SyncHealthy {
		t.Errorf("opencode.Status = %d, want SyncHealthy", opencode.Status)
	}

	codex := verdicts[2]
	if codex.RepoBehindRelease != domain.RepoBehindOriginNA {
		t.Errorf("codex.RepoBehindRelease = %d, want RepoBehindOriginNA (NA parsing)", codex.RepoBehindRelease)
	}
	if codex.RecordedVersion != "NA" || codex.DigestMatch != "NA" {
		t.Errorf("codex RecordedVersion/DigestMatch = %q/%q, want NA/NA (never deployed)", codex.RecordedVersion, codex.DigestMatch)
	}
}

// TestParseSyncCheck_ReleaseFieldsAbsentDefaultsBackwardsCompatible verifies
// a legacy VERDICT line without the new keys (pre-Slice-3b sync-check
// output, or a hand-built verdict in an older test) leaves RepoBehindRelease
// at the NA sentinel and RecordedVersion/DigestMatch empty, never a
// fabricated zero/healthy value.
func TestParseSyncCheck_ReleaseFieldsAbsentDefaultsBackwardsCompatible(t *testing.T) {
	sample := "\n=== sync-check: claude ===\nVERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0\nACTION:claude: in sync with gentle-ai (healthy)\n"
	verdicts := parseSyncCheck(sample)
	if len(verdicts) != 1 {
		t.Fatalf("expected 1 verdict, got %d", len(verdicts))
	}
	v := verdicts[0]
	if v.RepoBehindRelease != domain.RepoBehindOriginNA {
		t.Errorf("RepoBehindRelease = %d, want RepoBehindOriginNA when the key is absent", v.RepoBehindRelease)
	}
	if v.RecordedVersion != "" || v.DigestMatch != "" {
		t.Errorf("RecordedVersion/DigestMatch = %q/%q, want empty when the keys are absent", v.RecordedVersion, v.DigestMatch)
	}
	if v.Status != domain.SyncHealthy {
		t.Errorf("Status = %d, want SyncHealthy for a legacy all-zero verdict", v.Status)
	}
}

// TestParseSyncCheck_TheStatusOfAReport follows the VERDICT line of a report to
// the status the domain classifies it as: each line earns exactly the one
// status the operator's remedy depends on, so a parser that misreads a field
// (a release count ignored, a digest mismatch not counted, an NA read as zero)
// fails here and not only on the screen. The labels that paint each status are
// the screen's, tested in package ui.
func TestParseSyncCheck_TheStatusOfAReport(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   domain.SyncStatus
	}{
		{"nothing to do", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0", domain.SyncHealthy},
		{"files not deployed", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=2 REPO_BEHIND_ORIGIN=0", domain.SyncNeedsApply},
		{"a digest that no longer matches", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 DIGEST_MATCH=no", domain.SyncNeedsApply},
		{"upstream changed", "UPSTREAM_CHANGED=3 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0", domain.SyncNeedsCapture},
		{"upstream changed outranks files not deployed", "UPSTREAM_CHANGED=1 OVERLAY_NOT_DEPLOYED=4 REPO_BEHIND_ORIGIN=0", domain.SyncNeedsCapture},
		{"behind the release", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=2", domain.SyncBehindRelease},
		{"behind the release outranks behind origin", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=5 REPO_BEHIND_RELEASE=2", domain.SyncBehindRelease},
		{"behind origin", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=3", domain.SyncBehindOrigin},
		{"files not deployed outrank being behind", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=3 REPO_BEHIND_RELEASE=2", domain.SyncNeedsApply},
		{"an origin that could not be read adds nothing to the status", "UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=NA", domain.SyncHealthy},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verdicts := parseSyncCheck("=== sync-check: claude ===\nVERDICT:claude:" + tc.fields + "\nACTION:claude: next step\n")

			if len(verdicts) != 1 || verdicts[0].Status != tc.want {
				t.Errorf("the report of %q = %+v, want one verdict with status %d", tc.fields, verdicts, tc.want)
			}
		})
	}
}

// Output that is no report holds no verdict: not an empty one, which a reader
// could take for a target that is fine.
func TestParseSyncCheck_OutputThatIsNoReportHoldsNoVerdict(t *testing.T) {
	for name, output := range map[string]string{
		"nothing":                         "",
		"a target skipped":                "\nSYNC_CHECK:claude: target dir not found -- skipping\n",
		"the transcript of another verb":  "$ bin/labdrian-overlay status --target claude\nclaude: deployed\n",
		"garbage":                         "not even close to a sync-check line\n\x00\xff binary noise",
		"a verdict line with no counts":   "VERDICT:claude",
		"a section header and no verdict": "=== sync-check: claude ===\n  IN_SYNC: skills/foo\n",
	} {
		t.Run(name, func(t *testing.T) {
			if verdicts := parseSyncCheck(output); len(verdicts) != 0 {
				t.Errorf("parseSyncCheck(%q) = %+v, want no verdict", output, verdicts)
			}
		})
	}
}
