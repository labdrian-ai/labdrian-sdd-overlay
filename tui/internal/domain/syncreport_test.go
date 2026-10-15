package domain

import (
	"testing"
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

	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(verdicts))
	}

	want := []struct {
		target       string
		status       SyncStatus
		uc, ond, rbo int
	}{
		{"claude", SyncHealthy, 0, 0, 0},
		{"opencode", SyncNeedsApply, 0, 2, RepoBehindOriginNA},
		{"codex", SyncNeedsCapture, 3, 1, 5},
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

// TestClassifyPrecedence verifies UPSTREAM_CHANGED wins over OVERLAY_NOT_DEPLOYED
// (and digest mismatch, D2's addition to that same tier), which in turn wins
// over REPO_BEHIND_RELEASE (D2), which wins over REPO_BEHIND_ORIGIN (R-006/D2
// full precedence: capture > apply/digest-mismatch > behind-release >
// behind-origin).
func TestClassifyPrecedence(t *testing.T) {
	if classify(2, 5, 0, 0, false) != SyncNeedsCapture {
		t.Fatal("upstream_changed>0 must classify as needs-capture (RED)")
	}
	if classify(0, 1, 0, 0, false) != SyncNeedsApply {
		t.Fatal("overlay_not_deployed>0 must classify as needs-apply (YELLOW)")
	}
	if classify(0, 0, 0, 0, true) != SyncNeedsApply {
		t.Fatal("digest mismatch alone (D2) must classify as needs-apply (YELLOW), same tier as overlay_not_deployed>0")
	}
	if classify(0, 0, 0, 3, false) != SyncBehindRelease {
		t.Fatal("repo_behind_release>0 alone (D2) must classify as SyncBehindRelease")
	}
	if classify(0, 0, 3, 2, false) != SyncBehindRelease {
		t.Fatal("repo_behind_release>0 must outrank repo_behind_origin>0 (D2 precedence)")
	}
	if classify(0, 0, 3, 0, false) != SyncBehindOrigin {
		t.Fatal("repo_behind_origin>0 alone must classify as SyncBehindOrigin (R-006: never silently healthy)")
	}
	if classify(0, 0, 0, 0, false) != SyncHealthy {
		t.Fatal("zero counts and no digest mismatch must classify as healthy (GREEN)")
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
	verdicts := ParseSyncCheck(sample)
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
			verdicts := ParseSyncCheck(sample)
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

// TestBehindOf covers what the launch-time probe makes of a report: the two
// counts come from the same verdict, either may be unknown on its own, and a
// report with no verdict says nothing -- which is "unknown", never "up to
// date" (Go's zero value would collapse into "confirmed 0 behind", the R-006
// bug class the parser of a report already guards against).
func TestBehindOf(t *testing.T) {
	verdict := func(target string, origin, release int) TargetVerdict {
		return TargetVerdict{Target: target, RepoBehindOrigin: origin, RepoBehindRelease: release}
	}
	unknown := RepoBehind{Origin: RepoBehindOriginNA, Release: RepoBehindOriginNA}

	cases := []struct {
		name     string
		verdicts []TargetVerdict
		want     RepoBehind
	}{
		{"both counts", []TargetVerdict{verdict("claude", 4, 3)}, RepoBehind{Origin: 4, Release: 3}},
		{"both up to date", []TargetVerdict{verdict("claude", 0, 0)}, RepoBehind{}},
		{"origin known, release not yet tagged", []TargetVerdict{verdict("claude", 3, RepoBehindOriginNA)}, RepoBehind{Origin: 3, Release: RepoBehindOriginNA}},
		{"release known, origin not", []TargetVerdict{verdict("claude", RepoBehindOriginNA, 2)}, RepoBehind{Origin: RepoBehindOriginNA, Release: 2}},
		{"explicit NA for both", []TargetVerdict{verdict("claude", RepoBehindOriginNA, RepoBehindOriginNA)}, unknown},
		{"zero verdicts (e.g. every target dir missing)", []TargetVerdict{}, unknown},
		{"nothing at all", nil, unknown},
		{"the first verdict stands for the repository", []TargetVerdict{verdict("claude", 7, 4), verdict("codex", 1, 0)}, RepoBehind{Origin: 7, Release: 4}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BehindOf(tc.verdicts); got != tc.want {
				t.Errorf("BehindOf(%+v) = %+v, want %+v", tc.verdicts, got, tc.want)
			}
		})
	}
}

// TestParseSyncCheck_ReleaseFields verifies the three new VERDICT keys parse
// onto TargetVerdict, including the REPO_BEHIND_RELEASE NA sentinel and the
// resulting classify() precedence (digest mismatch -> needs-apply here,
// since OVERLAY_NOT_DEPLOYED=0 but DIGEST_MATCH=no).
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
	verdicts := ParseSyncCheck(sample)
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
	if claude.Status != SyncNeedsApply {
		t.Errorf("claude.Status = %d, want SyncNeedsApply (digest mismatch alone must classify as needs-apply, D2)", claude.Status)
	}

	opencode := verdicts[1]
	if opencode.RepoBehindRelease != 0 {
		t.Errorf("opencode.RepoBehindRelease = %d, want 0", opencode.RepoBehindRelease)
	}
	if opencode.Status != SyncHealthy {
		t.Errorf("opencode.Status = %d, want SyncHealthy", opencode.Status)
	}

	codex := verdicts[2]
	if codex.RepoBehindRelease != RepoBehindOriginNA {
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
	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 1 {
		t.Fatalf("expected 1 verdict, got %d", len(verdicts))
	}
	v := verdicts[0]
	if v.RepoBehindRelease != RepoBehindOriginNA {
		t.Errorf("RepoBehindRelease = %d, want RepoBehindOriginNA when the key is absent", v.RepoBehindRelease)
	}
	if v.RecordedVersion != "" || v.DigestMatch != "" {
		t.Errorf("RecordedVersion/DigestMatch = %q/%q, want empty when the keys are absent", v.RecordedVersion, v.DigestMatch)
	}
	if v.Status != SyncHealthy {
		t.Errorf("Status = %d, want SyncHealthy for a legacy all-zero verdict", v.Status)
	}
}
