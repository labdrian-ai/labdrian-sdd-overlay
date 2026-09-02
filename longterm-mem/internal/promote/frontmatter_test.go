package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPatchStatusFields_ReplacesExistingFieldInPlace: task 7.10 Gap 2
// coverage. frontmatter.go's PatchStatusFields (7.8 REFACTOR) was until
// now only exercised indirectly through propagate_test.go; this proves
// the direct contract: an existing status:/related: line is rewritten
// where it already sits, not appended a second time elsewhere.
func TestPatchStatusFields_ReplacesExistingFieldInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c-000601.md")
	fm := frontmatter{
		Title: "Test Page", Address: "c-000601", Aliases: []string{"Test Page"},
		Created: "2026-08-01", Updated: "2026-08-01",
		Tags: []string{"memory", "decision"}, Status: "developing",
		Related:        []string{"[[c-000001|Old]]"},
		EngramID:       601,
		EngramSyncID:   "sync-601",
		EngramType:     "decision",
		EngramRevision: 1,
		Project:        "labdrian-sdd-overlay",
	}
	block := fm.Render()
	body := "# Test Page\n\nOriginal content.\n\n---\nPromoted from Engram observation 601, revision 1.\n"
	if err := os.WriteFile(path, []byte(block+body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, _, err := PatchStatusFields(path, "archived", []string{"[[c-000002|New]]"}); err != nil {
		t.Fatalf("PatchStatusFields: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "status: archived") {
		t.Fatalf("status line not replaced; got:\n%s", content)
	}
	if strings.Contains(content, "status: developing") {
		t.Fatalf("stale status line still present; got:\n%s", content)
	}
	if !strings.Contains(content, `"[[c-000002|New]]"`) {
		t.Fatalf("related line not replaced with the new link; got:\n%s", content)
	}
	if strings.Contains(content, "[[c-000001|Old]]") {
		t.Fatalf("stale related entry still present; got:\n%s", content)
	}
}

// TestPatchStatusFields_AddsAbsentFieldIntoBlock: task 7.10 Gap 2
// coverage. A hand-authored or legacy page whose frontmatter has no
// related: line at all (unlike every page EmitPage/Render ever produces,
// which always emits both status: and related:) must still gain a
// related: entry inside the block when Propagate resolves a successor
// for it, rather than silently leaving the field missing forever.
func TestPatchStatusFields_AddsAbsentFieldIntoBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c-000602.md")
	raw := "---\ntype: concept\ntitle: \"Legacy Page\"\naddress: c-000602\nstatus: developing\nengram_id: 602\nproject: labdrian-sdd-overlay\n---\n\nBody.\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, _, err := PatchStatusFields(path, "archived", []string{"[[c-000003|Successor]]"}); err != nil {
		t.Fatalf("PatchStatusFields: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	content := string(data)
	fmBlock, ok := frontmatterBlock(content)
	if !ok {
		t.Fatalf("patched file has no parseable frontmatter block; got:\n%s", content)
	}
	if !strings.Contains(fmBlock, "related:\n  - \"[[c-000003|Successor]]\"") {
		t.Fatalf("related: field was not added inside the frontmatter block; got block:\n%s", fmBlock)
	}
	if !strings.Contains(fmBlock, "status: archived") {
		t.Fatalf("status line not replaced; got block:\n%s", fmBlock)
	}
	if !strings.Contains(content, "Body.") {
		t.Fatalf("body was lost; got:\n%s", content)
	}
}

// TestPatchStatusFields_AddsAbsentScalarFieldIntoBlock: the scalar setter
// must insert an absent field for the same reason the list setter does.
// Silently returning the block unchanged makes a patch report success
// while having written nothing -- and `status:` is exactly the field a
// caller patches to record supersession or archival, so a page that
// predates the field (or any future scalar the vault schema gains) would
// be reported as patched while staying stale on disk.
func TestPatchStatusFields_AddsAbsentScalarFieldIntoBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c-000605.md")
	raw := "---\ntype: concept\ntitle: \"No Status Yet\"\naddress: c-000605\nengram_id: 605\nproject: labdrian-sdd-overlay\n---\n\nBody.\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, _, err := PatchStatusFields(path, "superseded", nil); err != nil {
		t.Fatalf("PatchStatusFields: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	content := string(data)
	fmBlock, ok := frontmatterBlock(content)
	if !ok {
		t.Fatalf("patched file has no parseable frontmatter block; got:\n%s", content)
	}
	if !strings.Contains(fmBlock, "status: superseded") {
		t.Fatalf("status: field was not added inside the frontmatter block; got block:\n%s", fmBlock)
	}
	if !strings.Contains(content, "Body.") {
		t.Fatalf("body was lost; got:\n%s", content)
	}
}

// TestPatchStatusFields_BodyNeverTouched: task 7.10 Gap 2 coverage
// (R-033's own promise, restated by frontmatter.go's doc comment: "the
// entire body -- byte-identical"). Also checks the returned bodyHash
// matches the untouched on-disk body, so a caller (propagate.go) never
// records a stale fingerprint for content it never rewrote.
func TestPatchStatusFields_BodyNeverTouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c-000603.md")
	fm := frontmatter{
		Title: "Body Guard", Address: "c-000603", Aliases: []string{"Body Guard"},
		Created: "2026-08-01", Updated: "2026-08-01",
		Tags: []string{"memory", "decision"}, Status: "developing",
		Related:        []string{},
		EngramID:       603,
		EngramSyncID:   "sync-603",
		EngramType:     "decision",
		EngramRevision: 1,
		Project:        "labdrian-sdd-overlay",
	}
	block := fm.Render()
	body := "# Body Guard\n\nA human edited this body by hand after promotion, adding\nan extra paragraph the patch must never rewrite.\n\n---\nPromoted from Engram observation 603, revision 1.\n"
	if err := os.WriteFile(path, []byte(block+body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, bodyHash, err := PatchStatusFields(path, "archived", nil)
	if err != nil {
		t.Fatalf("PatchStatusFields: %v", err)
	}
	if bodyHash != hashText(body) {
		t.Fatalf("returned bodyHash does not match the untouched body's own hash")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	patchedBlock, ok := frontmatterBlock(string(data))
	if !ok {
		t.Fatalf("patched file has no parseable frontmatter block")
	}
	gotBody := string(data)[len(patchedBlock):]
	if gotBody != body {
		t.Fatalf("body was rewritten by a status-only patch; got:\n%s\nwant:\n%s", gotBody, body)
	}
}

// TestPatchStatusFields_PreservesContentOutsideBlockByteForByte: task
// 7.10 Gap 2 coverage. Uses a body containing trailing whitespace, a
// whitespace-only blank line, and a non-ASCII rune -- bytes a naive
// split/rejoin reconstruction of the file could silently alter even
// while looking "unchanged" under a substring check.
func TestPatchStatusFields_PreservesContentOutsideBlockByteForByte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c-000604.md")
	fm := frontmatter{
		Title: "Byte Guard", Address: "c-000604", Aliases: []string{"Byte Guard"},
		Created: "2026-08-01", Updated: "2026-08-01",
		Tags: []string{"memory", "decision"}, Status: "developing",
		Related:        []string{},
		EngramID:       604,
		EngramSyncID:   "sync-604",
		EngramType:     "decision",
		EngramRevision: 1,
		Project:        "labdrian-sdd-overlay",
	}
	block := fm.Render()
	body := "# Byte Guard\n\nLine with trailing spaces.   \n  \nUnicode: café ☃.\n\n---\nPromoted from Engram observation 604, revision 1.\n"
	if err := os.WriteFile(path, []byte(block+body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, _, err := PatchStatusFields(path, "mature", []string{"[[c-000005|Related]]"}); err != nil {
		t.Fatalf("PatchStatusFields: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	patchedBlock, ok := frontmatterBlock(string(data))
	if !ok {
		t.Fatalf("patched file has no parseable frontmatter block")
	}
	gotTail := string(data)[len(patchedBlock):]
	if gotTail != body {
		t.Fatalf("content outside the frontmatter block was not preserved byte-for-byte;\ngot:  %q\nwant: %q", gotTail, body)
	}
}
