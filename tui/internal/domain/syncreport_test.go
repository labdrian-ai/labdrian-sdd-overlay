package domain

import (
	"testing"
)

// TestClassifyPrecedence verifies UPSTREAM_CHANGED wins over OVERLAY_NOT_DEPLOYED
// (and digest mismatch, D2's addition to that same tier), which in turn wins
// over REPO_BEHIND_RELEASE (D2), which wins over REPO_BEHIND_ORIGIN (R-006/D2
// full precedence: capture > apply/digest-mismatch > behind-release >
// behind-origin).
func TestClassifyPrecedence(t *testing.T) {
	if Classify(2, 5, 0, 0, false) != SyncNeedsCapture {
		t.Fatal("upstream_changed>0 must classify as needs-capture (RED)")
	}
	if Classify(0, 1, 0, 0, false) != SyncNeedsApply {
		t.Fatal("overlay_not_deployed>0 must classify as needs-apply (YELLOW)")
	}
	if Classify(0, 0, 0, 0, true) != SyncNeedsApply {
		t.Fatal("digest mismatch alone (D2) must classify as needs-apply (YELLOW), same tier as overlay_not_deployed>0")
	}
	if Classify(0, 0, 0, 3, false) != SyncBehindRelease {
		t.Fatal("repo_behind_release>0 alone (D2) must classify as SyncBehindRelease")
	}
	if Classify(0, 0, 3, 2, false) != SyncBehindRelease {
		t.Fatal("repo_behind_release>0 must outrank repo_behind_origin>0 (D2 precedence)")
	}
	if Classify(0, 0, 3, 0, false) != SyncBehindOrigin {
		t.Fatal("repo_behind_origin>0 alone must classify as SyncBehindOrigin (R-006: never silently healthy)")
	}
	if Classify(0, 0, 0, 0, false) != SyncHealthy {
		t.Fatal("zero counts and no digest mismatch must classify as healthy (GREEN)")
	}
}

// TestBehindOf covers what the launch-time probe makes of a report: the two
// counts come from the same verdict, either may be unknown on its own, and a
// report with no verdict says nothing -- which is "unknown", never "up to
// date" (Go's zero value would collapse into "confirmed 0 behind", the R-006
// bug class the reader of a report already guards against).
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
