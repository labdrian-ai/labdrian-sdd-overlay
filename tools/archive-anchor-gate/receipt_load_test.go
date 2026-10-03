package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadApprovedTreeFromReceipts decides which persisted receipt an archive may be anchored to.
// It reads each candidate file itself and hands the bytes to reviewreceipt.Parse, and what it
// does with a file that cannot be read, that does not parse, that is not approved or that has
// no tree is to move on to the next one: never to stop, and never to accept it. These cases
// pin that wiring on the folders a change can really hold.
func TestLoadApprovedTreeFromReceipts(t *testing.T) {
	put := func(t *testing.T, dir, name, content string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const receiptJSON = `{"schema":"gentle-ai.review-receipt/v2","lineage_id":"review-inside","final_candidate_tree":"treeinside","selected_lenses":["review-risk"],"terminal_state":"approved"}`

	for _, tc := range []struct {
		name      string
		arrange   func(t *testing.T, dir string)
		wantTree  string
		wantLine  string
		wantFound bool
		wantErr   string // a substring, empty for no error
	}{
		{name: "a folder that does not exist holds no receipt",
			arrange: func(t *testing.T, dir string) { os.RemoveAll(dir) }},
		{name: "an empty folder holds no receipt",
			arrange: func(t *testing.T, dir string) {}},
		{name: "a folder that is a file cannot be read",
			arrange: func(t *testing.T, dir string) {
				os.RemoveAll(dir)
				if err := os.WriteFile(dir, []byte("not a folder"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "reading"},
		{name: "an approved legacy receipt",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-a", "tree-a", "approved")
			},
			wantTree: "tree-a", wantLine: "review-a", wantFound: true},
		{name: "an approved lifecycle state",
			arrange: func(t *testing.T, dir string) {
				writeReviewStateFile(t, dir, "review-a", "tree-a", "approved")
			},
			wantTree: "tree-a", wantLine: "review-a", wantFound: true},
		{name: "the lineage is the one the document names, not the file's",
			arrange:  func(t *testing.T, dir string) { put(t, dir, "renamed.json", receiptJSON) },
			wantTree: "treeinside", wantLine: "review-inside", wantFound: true},
		{name: "a directory named like a receipt is not a receipt",
			arrange: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "review-a.json"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(dir, "review-b.review-state.json"), 0o755); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "a file that is not .json is not a candidate",
			arrange: func(t *testing.T, dir string) { put(t, dir, "review-a.txt", receiptJSON) }},
		{name: "override.json is never a candidate",
			arrange: func(t *testing.T, dir string) { put(t, dir, "override.json", receiptJSON) }},
		{name: "a declined receipt is skipped",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-a", "tree-a", "declined")
				writeReviewStateFile(t, dir, "review-b", "tree-b", "reviewing")
			}},
		{name: "a file that does not parse is skipped, and the next one is tried",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-a", "tree-a", "approved")
				put(t, dir, "review-b.json", "this is not json")
			},
			wantTree: "tree-a", wantLine: "review-a", wantFound: true},
		{name: "a receipt with no tree is skipped, and the next one is tried",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-a", "tree-a", "approved")
				writeReceiptFile(t, dir, "review-b", "  ", "approved")
			},
			wantTree: "tree-a", wantLine: "review-a", wantFound: true},
		{name: "a newer lineage that is not approved does not hide an older one that is",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-a", "tree-a", "approved")
				writeReviewStateFile(t, dir, "review-b", "tree-b", "escalated")
			},
			wantTree: "tree-a", wantLine: "review-a", wantFound: true},
		{name: "of several approved receipts the file name that sorts last wins, across both shapes",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-a", "tree-a", "approved")
				writeReviewStateFile(t, dir, "review-b", "tree-b", "approved")
				writeReceiptFile(t, dir, "review-0", "tree-0", "approved")
			},
			wantTree: "tree-b", wantLine: "review-b", wantFound: true},
		{name: "the same lineage in both shapes: the lifecycle state sorts after the legacy receipt",
			arrange: func(t *testing.T, dir string) {
				writeReceiptFile(t, dir, "review-x", "tree-legacy", "approved")
				writeReviewStateFile(t, dir, "review-x", "tree-state", "approved")
			},
			wantTree: "tree-state", wantLine: "review-x", wantFound: true},
		{name: "a file that cannot be read is skipped, and the next one is tried",
			arrange: func(t *testing.T, dir string) {
				if os.Geteuid() == 0 {
					t.Skip("a file without permissions does not stop root")
				}
				writeReceiptFile(t, dir, "review-a", "tree-a", "approved")
				writeReceiptFile(t, dir, "review-b", "tree-b", "approved")
				if err := os.Chmod(filepath.Join(dir, "review-b.json"), 0); err != nil {
					t.Fatal(err)
				}
			},
			wantTree: "tree-a", wantLine: "review-a", wantFound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "review-receipts")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			tc.arrange(t, dir)

			tree, lineage, found, err := loadApprovedTreeFromReceipts(dir)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want none", err)
			}
			if tree != tc.wantTree || lineage != tc.wantLine || found != tc.wantFound {
				t.Errorf("loadApprovedTreeFromReceipts = (%q, %q, %v), want (%q, %q, %v)", tree, lineage, found, tc.wantTree, tc.wantLine, tc.wantFound)
			}
		})
	}
}
