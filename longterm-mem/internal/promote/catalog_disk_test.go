package promote

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// The modes this double gives what it creates. They are the double's own and no test of this package reads a
// mode (`rg 'Perm\(\)|\.Mode\(\)' internal/promote --glob '*_test.go'` finds none), so nothing here depends on
// them matching the real adapter; what the adapter creates (a new file owner-only, a new directory 0o755) is
// held where the adapter is, by internal/vaultfs's own tests (TestWritingTheCatalogAndTheLog), which this
// package cannot import. They are named so that the double does not repeat a bare number.
const (
	diskFileMode = 0o600
	diskDirMode  = 0o755
)

// diskFile is one file of a vault, as the catalog and the log are: an IndexRepository and a LogRepository over a
// path of the test's own, with the file system semantics the real adapter has (internal/vaultfs, which has its own
// tests, and which this package cannot import): a file is read whole through a link, a failure is the file
// system's own, and a write is the module's atomic replace, which keeps the mode of a file that exists. The
// promotion tests that put the catalog and the log on disk with os and look at them there use it, as the ones
// that care only for what is asked of the port use memFile.
type diskFile struct{ path string }

// indexIn is the catalog of the vault at root, where vaultlayout says it is.
func indexIn(root string) diskFile { return diskFile{path: filepath.Join(root, vaultlayout.IndexFile)} }

// logIn is the log of the vault at root.
func logIn(root string) diskFile { return diskFile{path: filepath.Join(root, vaultlayout.LogFile)} }

func (f diskFile) read() (string, error) {
	data, err := os.ReadFile(f.path)
	return string(data), err
}

func (f diskFile) write(content string) error {
	if err := os.MkdirAll(filepath.Dir(f.path), diskDirMode); err != nil {
		return fmt.Errorf("promote: create directory for %s: %w", f.path, err)
	}
	if err := durable.WriteFile(f.path, []byte(content), diskFileMode); err != nil {
		return fmt.Errorf("promote: %w", err)
	}
	return nil
}

func (f diskFile) ReadIndex() (string, error) { return f.read() }
func (f diskFile) IndexLocation() string      { return f.path }
func (f diskFile) WriteIndex(c string) error  { return f.write(c) }
func (f diskFile) ReadLog() (string, error)   { return f.read() }
func (f diskFile) LogLocation() string        { return f.path }
func (f diskFile) WriteLog(c string) error    { return f.write(c) }

// diskSyncState is a SyncStateRecorder over the sync-state file of a vault of the test's own, writing the record
// the real adapter writes.
type diskSyncState struct{ path string }

// syncStateIn is the sync state of the vault at root, where vaultlayout says it is.
func syncStateIn(root string) diskSyncState {
	return diskSyncState{path: filepath.Join(root, vaultlayout.SyncStateFile)}
}

func (d diskSyncState) RecordSyncCompleted(at time.Time) error {
	record := struct {
		Schema              int    `json:"schema"`
		LastSyncCompletedAt string `json:"last_sync_completed_at"`
	}{Schema: 1, LastSyncCompletedAt: at.Format(time.RFC3339)}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return diskFile(d).write(string(append(data, '\n')))
}
