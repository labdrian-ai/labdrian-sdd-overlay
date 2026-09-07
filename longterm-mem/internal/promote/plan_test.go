package promote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

// TestPlan_CountsThePagesPropagateWouldPatch (PD-1): `sync` runs TWO
// passes and prints both counts -- promoted and patched. A preview that
// covers only the first tells an operator "nothing will be rewritten" on a
// re-sync that is about to rewrite existing pages, which is the opposite of
// what a dry run is for. The count is asserted against what Propagate then
// actually patches on the same fixture, for the same reason Plan's
// promotion count is: a prediction from a second copy of the walk is a
// prediction that can quietly stop being true.
func TestPlan_CountsThePagesPropagateWouldPatch(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	writeAllocateScript(t, vaultRoot, uniqueAllocateAddressFixture)

	store, ids := newFixtureEngramStore(t, []fixtureObs{
		{title: "Old Decision", content: "Old body.", project: "p", obsType: "decision", revisionCount: 1, syncID: "sync-old", createdAt: "2026-08-01 00:00:00"},
		{title: "New Decision", content: "New body.", project: "p", obsType: "decision", revisionCount: 1, syncID: "sync-new", createdAt: "2026-08-15 00:00:00"},
	}, []fixtureRelation{
		{syncID: "rel-1", sourceSyncID: "sync-new", targetSyncID: "sync-old", relation: "supersedes"},
	})

	precedence := PrecedenceStore{}
	seedPromotedPage(t, vaultRoot, precedence, engram.Observation{
		ID: ids[0], Type: "decision", Title: "Old Decision", Content: "Old body.", Project: "p", RevisionCount: 1,
	}, "c-000001")
	seedPromotedPage(t, vaultRoot, precedence, engram.Observation{
		ID: ids[1], Type: "decision", Title: "New Decision", Content: "New body.", Project: "p", RevisionCount: 1,
	}, "c-000002")

	deps := Deps{Engram: store, Writer: &Writer{VaultRoot: vaultRoot, Store: precedence}}

	before := vaultSnapshot(t, vaultRoot)

	plan, err := Plan(context.Background(), deps, "p")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if after := vaultSnapshot(t, vaultRoot); !equalSnapshots(before, after) {
		t.Fatalf("Plan changed the vault while previewing the patch pass.\nbefore: %v\nafter:  %v", before, after)
	}

	report, err := Propagate(context.Background(), deps, "p")
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}

	if plan.WouldPatch != len(report.Patched) {
		t.Fatalf("plan predicted %d patched page(s), Propagate then patched %d -- the preview does not describe the second pass", plan.WouldPatch, len(report.Patched))
	}
	if plan.WouldPatch != 1 {
		t.Fatalf("plan.WouldPatch = %d, want 1 (the superseded page)", plan.WouldPatch)
	}
	if len(plan.PatchAddresses) != plan.WouldPatch {
		t.Fatalf("plan names %d addresses but predicts %d patches", len(plan.PatchAddresses), plan.WouldPatch)
	}
	if plan.PatchAddresses[0] != report.Patched[0] {
		t.Fatalf("plan named %q, Propagate patched %q", plan.PatchAddresses[0], report.Patched[0])
	}
}

// TestPlan_OneBrokenObservationIsReportedOnce (round 2): Plan walks the
// project twice -- once to decide promotions, once to decide patches -- and
// both walks call findPromotedPage on the same observation. A page whose
// frontmatter cannot be parsed fails both, so one broken observation
// arrived in plan.Failed twice and the CLI printed the same line twice.
//
// A failure list that counts one problem as two is the same defect this
// preview was built to remove, one layer in: the record stops describing
// what it records. An operator reading "2 failures" goes looking for a
// second broken page that does not exist.
func TestPlan_OneBrokenObservationIsReportedOnce(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	writeAllocateScript(t, vaultRoot, uniqueAllocateAddressFixture)

	store, ids := newFixtureEngramStore(t, []fixtureObs{
		{title: "Broken", content: "Body.", project: "p", obsType: "decision", revisionCount: 1, syncID: "s-b"},
	}, nil)

	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	broken := "---\ntype: concept\ntitle: \"Broken\"\naddress: c-000900\nstatus: seed\nengram_id: " +
		strconv.FormatInt(ids[0], 10) + "\nengram_revision: not-a-number\nproject: p\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000900.md"), []byte(broken), 0o644); err != nil {
		t.Fatalf("write broken page: %v", err)
	}

	deps := Deps{Engram: store, Writer: &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}}
	plan, err := Plan(context.Background(), deps, "p")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.Failed) != 1 {
		t.Fatalf("one broken observation produced %d failure entries: %+v", len(plan.Failed), plan.Failed)
	}
	if plan.Failed[0].ObservationID != ids[0] {
		t.Fatalf("Failed[0].ObservationID = %d, want %d", plan.Failed[0].ObservationID, ids[0])
	}
}

// TestMergeFailures_DropsOnlyExactRepeats pins the claim mergeFailures'
// own doc comment makes. Deduping by observation ID alone passes the
// one-broken-page test just as well, and silently hides the second of two
// genuinely different failures for the same observation -- the same lie as
// the double count, pointing the other way. Nothing enforced that until
// this test: collapsing the key to the ID left the suite green.
func TestMergeFailures_DropsOnlyExactRepeats(t *testing.T) {
	sameCause := errors.New("check promoted state: unparseable")
	otherCause := errors.New("resolve status: edge unreadable")

	t.Run("an exact repeat is dropped", func(t *testing.T) {
		got := mergeFailures(
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
		)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1: %+v", len(got), got)
		}
	})

	t.Run("two different causes for one observation both survive", func(t *testing.T) {
		got := mergeFailures(
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
			[]SyncFailure{{ObservationID: 7, Err: otherCause}},
		)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: the patch pass's own distinct failure for observation 7 was dropped, hiding a real problem: %+v", len(got), got)
		}
	})

	t.Run("different observations always survive", func(t *testing.T) {
		got := mergeFailures(
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
			[]SyncFailure{{ObservationID: 8, Err: sameCause}},
		)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %+v", len(got), got)
		}
	})
}
