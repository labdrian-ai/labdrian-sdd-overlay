package app

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// fakeProbe is a RepoProbe that answers with a fixed report.
type fakeProbe struct {
	verdicts []domain.TargetVerdict
	asked    int
}

func (f *fakeProbe) Report() []domain.TargetVerdict {
	f.asked++
	return f.verdicts
}

func verdictBehind(origin, release int) domain.TargetVerdict {
	return domain.TargetVerdict{Target: "claude", RepoBehindOrigin: origin, RepoBehindRelease: release}
}

// One report feeds both numbers (D2): the probe is asked once, and every way
// the report can fail to say degrades to "unknown", never to "up to date".
func TestRepoProber_BehindReadsBothNumbersFromOneReport(t *testing.T) {
	unknown := domain.RepoBehind{Origin: domain.RepoBehindOriginNA, Release: domain.RepoBehindOriginNA}
	tests := []struct {
		name     string
		verdicts []domain.TargetVerdict
		want     domain.RepoBehind
	}{
		{"both counts", []domain.TargetVerdict{verdictBehind(7, 4)}, domain.RepoBehind{Origin: 7, Release: 4}},
		{"both up to date", []domain.TargetVerdict{verdictBehind(0, 0)}, domain.RepoBehind{Origin: 0, Release: 0}},
		{"origin known, release not yet tagged", []domain.TargetVerdict{verdictBehind(3, domain.RepoBehindOriginNA)}, domain.RepoBehind{Origin: 3, Release: domain.RepoBehindOriginNA}},
		{"neither known", []domain.TargetVerdict{verdictBehind(domain.RepoBehindOriginNA, domain.RepoBehindOriginNA)}, unknown},
		{"no verdict at all", []domain.TargetVerdict{}, unknown},
		{"nothing read", nil, unknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := &fakeProbe{verdicts: tc.verdicts}

			got := NewRepoProber(probe).Behind()

			if got != tc.want {
				t.Errorf("Behind() = %+v, want %+v", got, tc.want)
			}
			if probe.asked != 1 {
				t.Errorf("the probe was run %d times, want once: one report feeds both numbers", probe.asked)
			}
		})
	}
}
