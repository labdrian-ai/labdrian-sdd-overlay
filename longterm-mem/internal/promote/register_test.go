package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRegister_NewPageDiscoverableAndLogged: R-029 scenario.
func TestRegister_NewPageDiscoverableAndLogged(t *testing.T) {
	vaultRoot := t.TempDir()
	indexPath := filepath.Join(vaultRoot, "wiki", "index.md")
	logPath := filepath.Join(vaultRoot, "wiki", "log.md")

	if err := RegisterIndex(indexPath, "c-000042", "Widget Decision"); err != nil {
		t.Fatalf("RegisterIndex: %v", err)
	}
	at := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if err := RegisterLog(logPath, "c-000042", "Widget Decision", at); err != nil {
		t.Fatalf("RegisterLog: %v", err)
	}

	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	if !strings.Contains(string(indexData), "[[c-000042|Widget Decision]]") {
		t.Fatalf("index.md does not list the new page (master catalog); got:\n%s", indexData)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log.md: %v", err)
	}
	if !strings.Contains(string(logData), "## [2026-08-31] promote | Widget Decision") {
		t.Fatalf("log.md does not record the promotion event; got:\n%s", logData)
	}
}

// TestRegisterIndex_IdempotentAndSortedByAddress triangulates D7's marker
// block contract: entries render sorted by address, and re-registering an
// existing address replaces its entry in place rather than duplicating it.
func TestRegisterIndex_IdempotentAndSortedByAddress(t *testing.T) {
	vaultRoot := t.TempDir()
	indexPath := filepath.Join(vaultRoot, "wiki", "index.md")

	if err := RegisterIndex(indexPath, "c-000099", "Second"); err != nil {
		t.Fatalf("RegisterIndex (c-000099): %v", err)
	}
	if err := RegisterIndex(indexPath, "c-000042", "First"); err != nil {
		t.Fatalf("RegisterIndex (c-000042): %v", err)
	}
	if err := RegisterIndex(indexPath, "c-000099", "Second Retitled"); err != nil {
		t.Fatalf("RegisterIndex (c-000099 retitle): %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	content := string(data)

	if strings.Count(content, "c-000099") != 1 {
		t.Fatalf("c-000099 appears %d times, want exactly 1 (idempotent replace, not append); got:\n%s", strings.Count(content, "c-000099"), content)
	}
	if !strings.Contains(content, "[[c-000099|Second Retitled]]") {
		t.Fatalf("index.md was not updated to the new title; got:\n%s", content)
	}
	if strings.Contains(content, "|Second]]") {
		t.Fatalf("stale title still present after replace; got:\n%s", content)
	}

	firstPos := strings.Index(content, "c-000042")
	secondPos := strings.Index(content, "c-000099")
	if firstPos == -1 || secondPos == -1 || firstPos > secondPos {
		t.Fatalf("entries are not sorted by address (c-000042 must precede c-000099); got:\n%s", content)
	}
}

// TestRegisterLog_NewestEntryInsertedBeforeExisting triangulates D7's
// newest-first log contract: a later RegisterLog call inserts its entry
// before an earlier one, not after.
func TestRegisterLog_NewestEntryInsertedBeforeExisting(t *testing.T) {
	vaultRoot := t.TempDir()
	logPath := filepath.Join(vaultRoot, "wiki", "log.md")

	older := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	if err := RegisterLog(logPath, "c-000001", "Older Entry", older); err != nil {
		t.Fatalf("RegisterLog (older): %v", err)
	}
	if err := RegisterLog(logPath, "c-000002", "Newer Entry", newer); err != nil {
		t.Fatalf("RegisterLog (newer): %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log.md: %v", err)
	}
	content := string(data)

	newerPos := strings.Index(content, "Newer Entry")
	olderPos := strings.Index(content, "Older Entry")
	if newerPos == -1 || olderPos == -1 || newerPos > olderPos {
		t.Fatalf("newest entry is not inserted before the existing one (newest-first contract); got:\n%s", content)
	}
}

// TestRegisterLog_OutOfOrderCallsStaySortedByTimestamp hardens RegisterLog
// against trusting call order instead of the at timestamp: registering the
// newer entry FIRST, then the older one SECOND, must still leave the file
// sorted by timestamp, not by call order. A writer that always inserts at
// the very top (ignoring at) would put Older Entry above Newer Entry here
// simply because it was registered second.
func TestRegisterLog_OutOfOrderCallsStaySortedByTimestamp(t *testing.T) {
	vaultRoot := t.TempDir()
	logPath := filepath.Join(vaultRoot, "wiki", "log.md")

	newer := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)

	if err := RegisterLog(logPath, "c-000002", "Newer Entry", newer); err != nil {
		t.Fatalf("RegisterLog (newer, first call): %v", err)
	}
	if err := RegisterLog(logPath, "c-000001", "Older Entry", older); err != nil {
		t.Fatalf("RegisterLog (older, second call): %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log.md: %v", err)
	}
	content := string(data)

	newerPos := strings.Index(content, "Newer Entry")
	olderPos := strings.Index(content, "Older Entry")
	if newerPos == -1 || olderPos == -1 || newerPos > olderPos {
		t.Fatalf("out-of-order calls did not stay sorted by timestamp (want Newer Entry before Older Entry regardless of call order); got:\n%s", content)
	}
}

// TestRegisterIndex_MalformedMarkerBlockRefusesToDropEntries hardens
// RegisterIndex against a corrupted marker block (begin marker present,
// end marker lost -- a bad hand-edit or a partial prior write): the old
// behavior silently treated this as "no block yet" and appended a fresh
// block containing only the new entry, discarding whatever entries lived
// under the orphaned begin marker. RegisterIndex must refuse instead.
func TestRegisterIndex_MalformedMarkerBlockRefusesToDropEntries(t *testing.T) {
	vaultRoot := t.TempDir()
	indexPath := filepath.Join(vaultRoot, "wiki", "index.md")

	corrupted := "# Vault Index\n\n<!-- longterm-mem:begin -->\n- [[c-000001|First]]\n- [[c-000002|Second]]\n"
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte(corrupted), 0o644); err != nil {
		t.Fatalf("write corrupted index.md: %v", err)
	}

	if err := RegisterIndex(indexPath, "c-000003", "Third"); err == nil {
		t.Fatal("RegisterIndex on a malformed marker block = nil error, want a refusal that names the risk of dropping entries")
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.md after refused write: %v", err)
	}
	if string(data) != corrupted {
		t.Fatalf("index.md was modified despite RegisterIndex refusing to write; got:\n%s", data)
	}
}

// TestRegisterIndex_PreservesHandWrittenContentOutsideBlock closes a
// coverage gap: nothing previously asserted that hand-authored prose
// outside the marker block survives a rewrite.
func TestRegisterIndex_PreservesHandWrittenContentOutsideBlock(t *testing.T) {
	vaultRoot := t.TempDir()
	indexPath := filepath.Join(vaultRoot, "wiki", "index.md")

	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	preamble := "# Vault Index\n\nHand-written introduction. Do not remove.\n\n"
	if err := os.WriteFile(indexPath, []byte(preamble), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}

	if err := RegisterIndex(indexPath, "c-000001", "First"); err != nil {
		t.Fatalf("RegisterIndex (first): %v", err)
	}
	if err := RegisterIndex(indexPath, "c-000002", "Second"); err != nil {
		t.Fatalf("RegisterIndex (second): %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Hand-written introduction. Do not remove.") {
		t.Fatalf("hand-written content outside the marker block was lost across two rewrites; got:\n%s", content)
	}
	if !strings.Contains(content, "[[c-000001|First]]") || !strings.Contains(content, "[[c-000002|Second]]") {
		t.Fatalf("both registered entries must survive alongside the hand-written preamble; got:\n%s", content)
	}
}

// TestRegisterIndex_TitleWithSpecialCharsRoundTrips hardens the wikilink
// entry parser against titles carrying "|" and "]]": the old
// character-class-based pattern ([^\]]*) stops at the first "]", so a
// title containing "]]" was silently truncated on the NEXT registration's
// re-parse, permanently losing the tail past the first "]]".
func TestRegisterIndex_TitleWithSpecialCharsRoundTrips(t *testing.T) {
	vaultRoot := t.TempDir()
	indexPath := filepath.Join(vaultRoot, "wiki", "index.md")

	if err := RegisterIndex(indexPath, "c-000001", "Weird | Pipe]]Bracket"); err != nil {
		t.Fatalf("RegisterIndex: %v", err)
	}
	// The second call forces a re-parse of the first entry's rendered
	// line -- exactly where a truncating parser loses the tail.
	if err := RegisterIndex(indexPath, "c-000002", "Second"); err != nil {
		t.Fatalf("RegisterIndex (second, forces a re-parse of the first entry): %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[[c-000001|Weird | Pipe]]Bracket]]") {
		t.Fatalf("title containing | and ]] did not round-trip intact; got:\n%s", content)
	}
	if strings.Count(content, "c-000001") != 1 {
		t.Fatalf("c-000001 appears %d times, want exactly 1 (title truncation must not fragment the entry); got:\n%s", strings.Count(content, "c-000001"), content)
	}
}
