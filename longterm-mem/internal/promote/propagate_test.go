package promote

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestPropagate: R-033's four scenarios, table-driven.
func TestPropagate(t *testing.T) {
	t.Run("Supersession updates status and related, body untouched", func(t *testing.T) {
		vaultRoot := t.TempDir()
		fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		store, ids := newFixtureEngramStore(t, []fixtureObs{
			{title: "Old Decision", content: "Old body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-old", createdAt: "2026-08-01 00:00:00"},
			{title: "New Decision", content: "New body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-new", createdAt: "2026-08-15 00:00:00"},
		}, []fixtureRelation{
			// Direction deliberately reversed from "source is superseded,
			// target is successor" -- D11 resolves the successor purely by
			// created_at, independent of which side the edge names source
			// or target.
			{syncID: "rel-1", sourceSyncID: "sync-new", targetSyncID: "sync-old", relation: "supersedes"},
		})
		oldID, newID := ids[0], ids[1]

		precedence := PrecedenceStore{}
		oldObs := engram.Observation{ID: oldID, Type: "decision", Title: "Old Decision", Content: "Old body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
		newObs := engram.Observation{ID: newID, Type: "decision", Title: "New Decision", Content: "New body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
		oldPage := seedPromotedPage(t, vaultRoot, precedence, oldObs, "c-000001")
		seedPromotedPage(t, vaultRoot, precedence, newObs, "c-000002")

		w := &Writer{VaultRoot: vaultRoot, Store: precedence}
		report, err := Propagate(context.Background(), Deps{Engram: store, Writer: w}, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Propagate: %v", err)
		}
		if len(report.Patched) != 1 || report.Patched[0] != "c-000001" {
			t.Fatalf("Patched = %+v, want exactly [c-000001] (only the older side)", report.Patched)
		}

		oldData, err := os.ReadFile(filepath.Join(vaultRoot, oldPage.Path))
		if err != nil {
			t.Fatalf("read old page: %v", err)
		}
		oldContent := string(oldData)
		if !strings.Contains(oldContent, "status: superseded") {
			t.Fatalf("old page status not patched to superseded; got:\n%s", oldContent)
		}
		if !strings.Contains(oldContent, "[[c-000002|New Decision]]") {
			t.Fatalf("old page related field does not point to the successor; got:\n%s", oldContent)
		}
		if !strings.Contains(oldContent, "Old body.") {
			t.Fatalf("old page body was rewritten; want it byte-identical, got:\n%s", oldContent)
		}

		newData, err := os.ReadFile(filepath.Join(vaultRoot, pagePathPrefix, "c-000002.md"))
		if err != nil {
			t.Fatalf("read new (successor) page: %v", err)
		}
		if strings.Contains(string(newData), "superseded") {
			t.Fatalf("successor page must be left untouched; got:\n%s", newData)
		}
	})

	t.Run("Soft-delete with no successor archives the page", func(t *testing.T) {
		vaultRoot := t.TempDir()
		fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		store, ids := newFixtureEngramStore(t, []fixtureObs{
			{title: "Deleted Decision", content: "Body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-del", deletedAt: "2026-08-20T00:00:00Z"},
		}, nil)

		precedence := PrecedenceStore{}
		obs := engram.Observation{ID: ids[0], Type: "decision", Title: "Deleted Decision", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
		page := seedPromotedPage(t, vaultRoot, precedence, obs, "c-000003")

		w := &Writer{VaultRoot: vaultRoot, Store: precedence}
		report, err := Propagate(context.Background(), Deps{Engram: store, Writer: w}, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Propagate: %v", err)
		}
		if len(report.Patched) != 1 || report.Patched[0] != "c-000003" {
			t.Fatalf("Patched = %+v, want exactly [c-000003]", report.Patched)
		}

		data, err := os.ReadFile(filepath.Join(vaultRoot, page.Path))
		if err != nil {
			t.Fatalf("read page: %v", err)
		}
		if !strings.Contains(string(data), "status: archived") {
			t.Fatalf("page status not patched to archived; got:\n%s", data)
		}
	})

	t.Run("Untouched observation keeps its status", func(t *testing.T) {
		vaultRoot := t.TempDir()
		fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		store, ids := newFixtureEngramStore(t, []fixtureObs{
			{title: "Stable Decision", content: "Body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-stable"},
		}, nil)

		precedence := PrecedenceStore{}
		obs := engram.Observation{ID: ids[0], Type: "decision", Title: "Stable Decision", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
		page := seedPromotedPage(t, vaultRoot, precedence, obs, "c-000004")
		before, err := os.ReadFile(filepath.Join(vaultRoot, page.Path))
		if err != nil {
			t.Fatalf("read page (before): %v", err)
		}

		w := &Writer{VaultRoot: vaultRoot, Store: precedence}
		report, err := Propagate(context.Background(), Deps{Engram: store, Writer: w}, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Propagate: %v", err)
		}
		if len(report.Patched) != 0 {
			t.Fatalf("Patched = %+v, want none (untouched observation)", report.Patched)
		}

		after, err := os.ReadFile(filepath.Join(vaultRoot, page.Path))
		if err != nil {
			t.Fatalf("read page (after): %v", err)
		}
		if string(before) != string(after) {
			t.Fatalf("page changed despite being untouched; before:\n%s\nafter:\n%s", before, after)
		}
	})

	t.Run("Status patch on a locally edited page still lands (canon wins)", func(t *testing.T) {
		vaultRoot := t.TempDir()
		fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		store, ids := newFixtureEngramStore(t, []fixtureObs{
			{title: "Edited Decision", content: "Original body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-edited", deletedAt: "2026-08-20T00:00:00Z"},
		}, nil)

		precedence := PrecedenceStore{}
		obs := engram.Observation{ID: ids[0], Type: "decision", Title: "Edited Decision", Content: "Original body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
		page := seedPromotedPage(t, vaultRoot, precedence, obs, "c-000005")

		full := filepath.Join(vaultRoot, page.Path)
		raw, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read page: %v", err)
		}
		locallyEdited := string(raw) + "\nHand-added text after promotion.\n"
		if err := os.WriteFile(full, []byte(locallyEdited), 0o644); err != nil {
			t.Fatalf("write local edit: %v", err)
		}
		// The precedence store's entry still reflects the ORIGINAL page
		// (seedPromotedPage's fingerprint), diverging from the local
		// edit's real on-disk hash -- this is a genuine "local edit" state
		// per UpdateInPlace's own detection, deliberately not re-seeded.

		w := &Writer{VaultRoot: vaultRoot, Store: precedence}
		report, err := Propagate(context.Background(), Deps{Engram: store, Writer: w}, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Propagate: %v", err)
		}
		if len(report.Patched) != 1 || report.Patched[0] != "c-000005" {
			t.Fatalf("Patched = %+v, want exactly [c-000005] -- the patch must land even on a locally edited page", report.Patched)
		}

		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read page after patch: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "status: archived") {
			t.Fatalf("locally edited page's status was not patched; got:\n%s", content)
		}
		if !strings.Contains(content, "Hand-added text after promotion.") {
			t.Fatalf("the local edit was lost; the body must never be rewritten by a status-only patch; got:\n%s", content)
		}

		entry, ok := precedence.Get("c-000005")
		if !ok {
			t.Fatal("precedence store has no entry for c-000005 after the patch")
		}
		fmBlock, ok := frontmatterBlock(content)
		if !ok {
			t.Fatalf("patched page has no parseable frontmatter block")
		}
		body := content[len(fmBlock):]
		if entry.FrontmatterHash != hashText(fmBlock) {
			t.Fatalf("frontmatter_hash was not updated to the newly patched block")
		}
		// longterm-mem wrote the frontmatter, so its hash moves. It did
		// NOT write the body: stamping the human's edited body as our own
		// last write would erase the very divergence signal R-030 relies
		// on, and the next Sync would overwrite the edit in silence
		// (review finding R3-precedence-blesses-local-edit).
		if entry.BodyHash == hashText(body) {
			t.Fatalf("body_hash was rewritten to the human's edited body; the pre-edit hash must survive so divergence stays detectable")
		}
		if entry.BodyHash != hashText(page.Body) {
			t.Fatalf("body_hash = %q, want the pre-edit hash %q recorded when longterm-mem last wrote this body", entry.BodyHash, hashText(page.Body))
		}

		// The property that actually matters: a later promotion of the
		// same page still refuses to touch the human's body.
		fixedNow(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
		obs.RevisionCount = 2
		obs.Content = "Freshly rendered body."
		next, err := EmitPage(obs, "c-000005", nil)
		if err != nil {
			t.Fatalf("EmitPage (later sync): %v", err)
		}
		action, err := UpdateInPlace(precedence, next, full)
		if err != nil {
			t.Fatalf("UpdateInPlace after propagate: %v", err)
		}
		if action.Kind != ActionSkippedLocalEdit {
			t.Fatalf("action.Kind = %v, want ActionSkippedLocalEdit -- the status patch must not have blessed the human's edit", action.Kind)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read page after the later sync: %v", err)
		}
		if !strings.Contains(string(after), "Hand-added text after promotion.") {
			t.Fatalf("the human's edit was overwritten by a later sync; got:\n%s", after)
		}
	})
}

// TestPropagate_OneBrokenPageDoesNotWedgeTheRun: Propagate walks the same
// per-observation shape Sync does, and carries the same availability
// hazard its review named (R4-poison-pill-abort). ObservationsIncludingDeleted
// returns the same set on every run, so returning from inside the loop
// makes one broken promoted page permanent: every later observation's
// canonical status -- a supersession, an archival -- never lands, and each
// retry fails identically. A broken page must be recorded and stepped over.
func TestPropagate_OneBrokenPageDoesNotWedgeTheRun(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))

	store, ids := newFixtureEngramStore(t, []fixtureObs{
		{title: "Broken Page", content: "Body one.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, deletedAt: "2026-08-20T00:00:00Z"},
		{title: "Archivable", content: "Body two.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, deletedAt: "2026-08-20T00:00:00Z"},
	}, nil)

	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	// A promoted page whose engram_revision cannot be parsed: deciding
	// anything about it fails on every run.
	broken := "---\ntype: concept\ntitle: \"Broken Page\"\naddress: c-000700\nstatus: seed\nengram_id: " +
		strconv.FormatInt(ids[0], 10) + "\nengram_revision: not-a-number\nproject: labdrian-sdd-overlay\n---\n\nBody one.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000700.md"), []byte(broken), 0o644); err != nil {
		t.Fatalf("write broken page: %v", err)
	}

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	healthy := engram.Observation{ID: ids[1], Type: "decision", Title: "Archivable", Content: "Body two.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	seedPromotedPage(t, vaultRoot, w.Store, healthy, "c-000701")

	report, err := Propagate(context.Background(), Deps{Engram: store, Writer: w}, "labdrian-sdd-overlay")
	if err == nil {
		t.Fatal("Propagate = nil error, want the broken page surfaced so a partial run cannot pass as clean")
	}
	if len(report.Failed) != 1 || report.Failed[0].ObservationID != ids[0] {
		t.Fatalf("Failed = %+v, want exactly observation %d", report.Failed, ids[0])
	}
	if len(report.Patched) != 1 || report.Patched[0] != "c-000701" {
		t.Fatalf("Patched = %+v, want the healthy page c-000701 patched despite the broken one", report.Patched)
	}
}
