package app

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// fakeProbe is a RepoProbe that answers with a fixed report.
type fakeProbe struct {
	report string
	asked  int
}

func (f *fakeProbe) Report() string {
	f.asked++
	return f.report
}

func report(origin, release string) string {
	return "\n=== sync-check: claude ===\n" +
		"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=" + origin + " REPO_BEHIND_RELEASE=" + release + "\n" +
		"ACTION:claude: run 'git pull' to update your local clone\n"
}

// One report feeds both numbers (D2): the probe is asked once, and every way
// the report can fail to say degrades to "unknown", never to "up to date".
func TestRepoProber_BehindReadsBothNumbersFromOneReport(t *testing.T) {
	tests := []struct {
		name        string
		report      string
		wantOrigin  int
		wantRelease int
	}{
		{"both counts", report("7", "4"), 7, 4},
		{"both up to date", report("0", "0"), 0, 0},
		{"origin known, release not yet tagged", report("3", "NA"), 3, domain.RepoBehindOriginNA},
		{"neither known", report("NA", "NA"), domain.RepoBehindOriginNA, domain.RepoBehindOriginNA},
		{"no verdict at all", "SYNC_CHECK:claude: target dir not found -- skipping\n", domain.RepoBehindOriginNA, domain.RepoBehindOriginNA},
		{"nothing printed", "", domain.RepoBehindOriginNA, domain.RepoBehindOriginNA},
		{"garbage", "not even close to a sync-check line\n\x00\xff binary noise", domain.RepoBehindOriginNA, domain.RepoBehindOriginNA},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := &fakeProbe{report: tc.report}

			origin, release := NewRepoProber(probe).Behind()

			if origin != tc.wantOrigin || release != tc.wantRelease {
				t.Errorf("Behind() = (%d, %d), want (%d, %d)", origin, release, tc.wantOrigin, tc.wantRelease)
			}
			if probe.asked != 1 {
				t.Errorf("the probe was run %d times, want once: one report feeds both numbers", probe.asked)
			}
		})
	}
}
