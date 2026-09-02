package promote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestPromote_ExplicitCallOverridesAutomaticEligibility (task 8b.4, R-032):
// an observation that is not pinned, not of an eligible type, and below
// the revision-count threshold -- Eligible(obs, false) reports false, per
// TestEligible above -- must still be promoted through the same
// page-emission, addressing, and registration path any other eligible
// observation uses once an explicit promote call names it. This exercises
// ExplicitPromote (explicit.go), the id-lookup entrypoint the CLI promote
// subcommand (8b.6) and the MCP promote tool (8b.7) both call, proving
// design.md's directive that R-032 flows through Writer.Promote's existing
// explicit-bool parameter rather than a second bypass: the page actually
// lands under wiki/memory/ and is registered in wiki/index.md/wiki/log.md,
// not merely accepted without writing anything.
func TestPromote_ExplicitCallOverridesAutomaticEligibility(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 601, Type: "discovery", Title: "Below Threshold", Content: "Never automatically eligible.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: false}

	var lookedUp int64
	result, err := ExplicitPromote(w, func(id int64) (engram.Observation, bool, error) {
		lookedUp = id
		return obs, true, nil
	}, obs.ID)
	if err != nil {
		t.Fatalf("ExplicitPromote: %v", err)
	}
	if lookedUp != obs.ID {
		t.Fatalf("lookup called with id %d, want %d", lookedUp, obs.ID)
	}
	if result.Action.Kind != ActionCreated {
		t.Fatalf("Action.Kind = %v, want ActionCreated: an explicit call on a below-threshold observation must still write a page", result.Action.Kind)
	}

	full := filepath.Join(vaultRoot, result.Page.Path)
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", full, err)
	}
	if !strings.Contains(string(got), "Never automatically eligible.") {
		t.Fatalf("written page missing observation content; got:\n%s", got)
	}

	indexData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("read wiki/index.md: %v", err)
	}
	if !strings.Contains(string(indexData), "[["+result.Page.Address+"|Below Threshold]]") {
		t.Fatalf("wiki/index.md does not list the explicitly-promoted page (registration path skipped); got:\n%s", indexData)
	}

	logData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "log.md"))
	if err != nil {
		t.Fatalf("read wiki/log.md: %v", err)
	}
	if !strings.Contains(string(logData), "promote | Below Threshold") {
		t.Fatalf("wiki/log.md does not record the explicit promotion (registration path skipped); got:\n%s", logData)
	}
}

// TestPromote_InvalidObservationIdRejected (task 8b.5, R-032): an invalid
// or nonexistent observation id must be rejected with a clear,
// distinguishable error, never a silent no-op that leaves the caller
// unable to tell "nothing to do" from "the id was wrong".
func TestPromote_InvalidObservationIdRejected(t *testing.T) {
	vaultRoot := t.TempDir()
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}

	_, err := ExplicitPromote(w, func(int64) (engram.Observation, bool, error) {
		return engram.Observation{}, false, nil
	}, 999)
	if err == nil {
		t.Fatal("ExplicitPromote = nil error, want a rejection for an invalid observation id")
	}
	if !errors.Is(err, ErrObservationNotFound) {
		t.Fatalf("ExplicitPromote error = %v, want errors.Is(err, ErrObservationNotFound)", err)
	}

	if _, statErr := os.Stat(filepath.Join(vaultRoot, "wiki", "index.md")); !os.IsNotExist(statErr) {
		t.Fatalf("wiki/index.md exists after a rejected promote call (stat err = %v); an invalid id must do nothing, not partially register", statErr)
	}
}
