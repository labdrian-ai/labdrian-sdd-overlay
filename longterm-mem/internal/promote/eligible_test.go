package promote

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// TestEligible covers R-007's rewritten predicate: pinned, explicit, or a
// non-empty topic_key whose first path segment is outside the excluded
// {sdd, review, delivery, odd, session} set. Type and revision count are retired as
// automatic eligibility criteria (R-003).
func TestEligible(t *testing.T) {
	tests := []struct {
		name     string
		obs      memory.Observation
		explicit bool
		want     bool
	}{
		{
			name: "pinned observation is eligible",
			obs:  memory.Observation{Type: "discovery", Pinned: true, RevisionCount: 1},
			want: true,
		},
		{
			name:     "explicit promote call overrides the automatic criteria",
			obs:      memory.Observation{Type: "discovery", RevisionCount: 1, Pinned: false},
			explicit: true,
			want:     true,
		},
		{
			name: "untopiced observation is not eligible",
			obs:  memory.Observation{Type: "decision", RevisionCount: 1, Pinned: false, TopicKey: ""},
			want: false,
		},
		{
			name: "curated topic_key is eligible regardless of type or revision count",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "longterm-mem/promotion-eligibility-policy"},
			want: true,
		},
		{
			name: "sdd/-prefixed topic_key is excluded",
			obs:  memory.Observation{Type: "decision", RevisionCount: 5, Pinned: false, TopicKey: "sdd/longterm-mem-promotion-scoping/tasks"},
			want: false,
		},
		{
			name: "a goal's sdd/-prefixed topic_key is excluded: the goal prefix is not the first segment",
			obs:  memory.Observation{Type: "decision", RevisionCount: 5, Pinned: false, TopicKey: "goal:g-42/sdd/some-change/tasks"},
			want: false,
		},
		{
			name: "a goal's curated topic_key is eligible",
			obs:  memory.Observation{Type: "discovery", Pinned: false, TopicKey: "goal:g-42/longterm-mem/promotion-eligibility-policy"},
			want: true,
		},
		{
			name: "a goal's topic with nothing under the prefix is not eligible",
			obs:  memory.Observation{Type: "decision", Pinned: false, TopicKey: "goal:g-42/"},
			want: false,
		},
		{
			name: "review/-prefixed topic_key is excluded",
			obs:  memory.Observation{Type: "decision", RevisionCount: 5, Pinned: false, TopicKey: "review/some-change/verdict"},
			want: false,
		},
		{
			name: "delivery/-prefixed topic_key is excluded",
			obs:  memory.Observation{Type: "decision", RevisionCount: 5, Pinned: false, TopicKey: "delivery/some-change/receipt"},
			want: false,
		},
		{
			name: "odd/-prefixed topic_key (a feature-doc mirror) is excluded",
			obs:  memory.Observation{Type: "architecture", RevisionCount: 5, Pinned: false, TopicKey: "odd/phase9-c22fu-memorycard-compat/tasks"},
			want: false,
		},
		{
			name: "session/-prefixed topic_key (a session summary) is excluded",
			obs:  memory.Observation{Type: "session_summary", RevisionCount: 2, Pinned: false, TopicKey: "session/07a89cf6-d9a4-4988-917a-9f99a013181a"},
			want: false,
		},
		{
			name: "a goal's odd/-prefixed topic_key is excluded: the goal prefix is not the first segment",
			obs:  memory.Observation{Type: "architecture", Pinned: false, TopicKey: "goal:g-42/odd/some-feature/tasks"},
			want: false,
		},
		{
			name: "a goal's session/-prefixed topic_key is excluded",
			obs:  memory.Observation{Type: "session_summary", Pinned: false, TopicKey: "goal:g-42/session/abc"},
			want: false,
		},
		{
			name: "a pinned odd/-prefixed memory stays eligible: the pin overrides the exclusion",
			obs:  memory.Observation{Type: "architecture", Pinned: true, TopicKey: "odd/some-feature/tasks"},
			want: true,
		},
		{
			name: "odd-notes/ is not excluded (first segment odd-notes != odd)",
			obs:  memory.Observation{Type: "discovery", Pinned: false, TopicKey: "odd-notes/x"},
			want: true,
		},
		{
			name: "sdd-init/ is not excluded (first segment sdd-init != sdd)",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "sdd-init/onboarding"},
			want: true,
		},
		{
			name: "sddx/ is not excluded (first segment sddx != sdd)",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "sddx/whatever"},
			want: true,
		},
		{
			name: "high-revision, decision-typed, unpinned, untopiced observation is not eligible",
			obs:  memory.Observation{Type: "decision", RevisionCount: 5, Pinned: false, TopicKey: ""},
			want: false,
		},
		{
			name: "pinned observation overrides both the untopiced and prefix exclusions",
			obs:  memory.Observation{Type: "decision", RevisionCount: 0, Pinned: true, TopicKey: "review/some-change/verdict"},
			want: true,
		},
		{
			name: "bare sdd topic_key (no slash) is excluded",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "sdd"},
			want: false,
		},
		{
			name: "bare non-excluded topic_key (no slash) is eligible",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "foo"},
			want: true,
		},
		{
			name: "whitespace-only topic_key is not eligible",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "   "},
			want: false,
		},
		{
			name: "leading-slash topic_key has an empty first segment and is not eligible",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "/sdd/auth"},
			want: false,
		},
		{
			name: "leading-slash before a non-excluded segment still has an empty first segment and is not eligible",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "/sdd/x"},
			want: false,
		},
		{
			name: "a single slash has an empty first segment and is not eligible",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "/"},
			want: false,
		},
		{
			name: "uppercase SDD/ segment is a different string than sdd and is eligible (case-sensitive match)",
			obs:  memory.Observation{Type: "discovery", RevisionCount: 0, Pinned: false, TopicKey: "SDD/x"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Eligible(obsMemory(tt.obs), tt.explicit); got != tt.want {
				t.Fatalf("Eligible(%+v, explicit=%v) = %v, want %v", tt.obs, tt.explicit, got, tt.want)
			}
		})
	}
}

// TestPromote_AnIneligibleObservationReportsNoActionAndWritesNothing: an
// observation that is not eligible is a normal skip for a scanning caller,
// and the Result says so in its own words (ActionNone, no page) rather than
// by being an unset struct. Nothing is written to the vault.
func TestPromote_AnIneligibleObservationReportsNoActionAndWritesNothing(t *testing.T) {
	vaultRoot := t.TempDir()
	w := &Writer{Index: indexIn(vaultRoot), Log: logIn(vaultRoot), SyncState: syncStateIn(vaultRoot), Pages: pagesIn(vaultRoot), Store: PrecedenceStore{}, Precedence: &memPrecedence{}, Clock: &fakeClock{at: testInstant}}
	obs := memory.Observation{ID: 602, Type: "discovery", Title: "Not Curated", Content: "Never automatically eligible.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	result, err := w.Promote(t.Context(), obsMemory(obs), false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionNone || result.Page.Address != "" {
		t.Fatalf("Promote = %+v, want ActionNone and no page for an ineligible observation", result)
	}
	entries, err := os.ReadDir(vaultRoot)
	if err != nil {
		t.Fatalf("read the vault: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("an ineligible observation wrote %d entries into the vault", len(entries))
	}
}

// A page names an observation only by a positive engram_id: zero is no observation, so no page is joined
// to an observation it does not name.
func TestPageObservationIDNamesOnlyAPositiveID(t *testing.T) {
	for _, tc := range []struct {
		frontmatter string
		wantID      int64
		wantNamed   bool
	}{
		{"---\nengram_id: 12\n---\n", 12, true},
		{"---\nengram_id: 0\n---\n", 0, false},
		{"---\ntitle: x\n---\n", 0, false},
	} {
		id, named := PageObservationID(Page{Frontmatter: tc.frontmatter})
		if id != tc.wantID || named != tc.wantNamed {
			t.Errorf("PageObservationID(%q) = %d, %v; want %d, %v", tc.frontmatter, id, named, tc.wantID, tc.wantNamed)
		}
	}
}
