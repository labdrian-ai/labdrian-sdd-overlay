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
