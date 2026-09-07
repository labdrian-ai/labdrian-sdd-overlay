package promote

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestPlan_WritesNothingAndPredictsWhatSyncThenDoes is the whole point of a
// dry run, and both halves are load-bearing.
//
// The first half: a plan must leave the vault byte-identical. A first sync
// on a real project promotes hundreds of pages into a vault whose index.md
// and log.md may be hand-authored, and an operator currently has no way to
// see that coming before it lands.
//
// The second half is what stops the preview from becoming a lie. The plan
// is asserted against what Sync ACTUALLY promotes on the same fixture,
// because a preview computed by its own second copy of the eligibility
// walk is a record that can stop describing what it records the moment
// either copy changes -- which is exactly the failure this codebase keeps
// re-finding.
func TestPlan_WritesNothingAndPredictsWhatSyncThenDoes(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	writeAllocateScript(t, vaultRoot, uniqueAllocateAddressFixture)

	store, ids := newFixtureEngramStore(t, []fixtureObs{
		{title: "First Decision", content: "Body one.", project: "p", obsType: "decision", revisionCount: 1, syncID: "s-1"},
		{title: "Second Decision", content: "Body two.", project: "p", obsType: "decision", revisionCount: 1, syncID: "s-2"},
		{title: "Already Current", content: "Body three.", project: "p", obsType: "decision", revisionCount: 3, syncID: "s-3"},
	}, nil)

	precedence := PrecedenceStore{}
	seedPromotedPage(t, vaultRoot, precedence, engram.Observation{
		ID: ids[2], Type: "decision", Title: "Already Current", Content: "Body three.", Project: "p", RevisionCount: 3,
	}, "c-000900")

	w := &Writer{VaultRoot: vaultRoot, Store: precedence}
	deps := Deps{Engram: store, Writer: w}

	before := vaultSnapshot(t, vaultRoot)

	plan, err := Plan(context.Background(), deps, "p")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if after := vaultSnapshot(t, vaultRoot); !equalSnapshots(before, after) {
		t.Fatalf("Plan changed the vault.\nbefore: %v\nafter:  %v", before, after)
	}
	if _, err := os.Stat(filepath.Join(vaultRoot, syncStateRelPath)); err == nil {
		t.Fatalf("Plan wrote the sync-state record; a dry run must not claim a sync happened")
	}

	report, err := Sync(context.Background(), deps, "p")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if plan.WouldPromote != len(report.Promoted) {
		t.Fatalf("plan predicted %d promotions, Sync then made %d -- the preview does not describe the run", plan.WouldPromote, len(report.Promoted))
	}
	if plan.WouldPromote != 2 {
		t.Fatalf("plan.WouldPromote = %d, want 2 (two stale, one already current)", plan.WouldPromote)
	}
	if plan.Skipped != report.Skipped {
		t.Fatalf("plan predicted %d skipped, Sync then skipped %d", plan.Skipped, report.Skipped)
	}
	if len(plan.Titles) != plan.WouldPromote {
		t.Fatalf("plan names %d observations but predicts %d promotions", len(plan.Titles), plan.WouldPromote)
	}
}

// vaultSnapshot records every file under root and its bytes, so a dry run
// that touches anything at all is visible -- not just one file a test
// thought to check.
func vaultSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		snap[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot vault: %v", err)
	}
	return snap
}

func equalSnapshots(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
