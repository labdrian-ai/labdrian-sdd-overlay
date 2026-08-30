package promote

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestEligible covers R-007's four scenarios plus one extra case proving
// the eligible-type membership branch (decision/architecture/pattern) is
// real, not merely reachable by the pin/revision branches the four named
// scenarios already exercise.
func TestEligible(t *testing.T) {
	tests := []struct {
		name     string
		obs      engram.Observation
		explicit bool
		want     bool
	}{
		{
			name: "pinned observation is eligible",
			obs:  engram.Observation{Type: "discovery", Pinned: true, RevisionCount: 1},
			want: true,
		},
		{
			name: "high-revision, untyped, unpinned observation is eligible",
			obs:  engram.Observation{Type: "discovery", RevisionCount: 4, Pinned: false},
			want: true,
		},
		{
			name: "low-revision, untyped, unpinned observation is not eligible",
			obs:  engram.Observation{Type: "discovery", RevisionCount: 1, Pinned: false},
			want: false,
		},
		{
			name:     "explicit promote call overrides the automatic criteria",
			obs:      engram.Observation{Type: "discovery", RevisionCount: 1, Pinned: false},
			explicit: true,
			want:     true,
		},
		{
			name: "eligible-type observation is eligible without pin or revision (type-membership branch)",
			obs:  engram.Observation{Type: "architecture", RevisionCount: 1, Pinned: false},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Eligible(tt.obs, tt.explicit); got != tt.want {
				t.Fatalf("Eligible(%+v, explicit=%v) = %v, want %v", tt.obs, tt.explicit, got, tt.want)
			}
		})
	}
}
