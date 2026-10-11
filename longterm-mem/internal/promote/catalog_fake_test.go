package promote

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"time"
)

// memFile is the catalog and the log kept in memory: an IndexRepository and a LogRepository whose file holds a
// string, reads as the file system says a file that is not there does, can fail each operation on demand, and
// tells what it was asked. The file system adapter has its own tests (internal/vaultfs); the registration that
// has to be seen to go through the port, and the failures a disk will not produce on demand, are tested here
// against this fake.
type memFile struct {
	// content is the file; exists says whether there is one.
	content string
	exists  bool
	// location is what the file is named in the messages.
	location string
	// readErr and writeErr, when set, are what the matching call answers.
	readErr, writeErr error

	// reads counts the reads asked for, failed ones included; writes lists every content WritePage was asked to
	// write, failed ones included.
	reads  int
	writes []string
}

// newMemFile is a file that is there and holds content.
func newMemFile(content string) *memFile {
	return &memFile{content: content, exists: true, location: filepath.Join("memory-vault", "wiki", "file.md")}
}

// newAbsentMemFile is a file that is not there yet.
func newAbsentMemFile() *memFile {
	return &memFile{location: filepath.Join("memory-vault", "wiki", "file.md")}
}

func (m *memFile) read() (string, error) {
	m.reads++
	if m.readErr != nil {
		return "", m.readErr
	}
	if !m.exists {
		return "", fmt.Errorf("open %s: %w", m.location, fs.ErrNotExist)
	}
	return m.content, nil
}

func (m *memFile) write(content string) error {
	m.writes = append(m.writes, content)
	if m.writeErr != nil {
		return m.writeErr
	}
	m.content, m.exists = content, true
	return nil
}

func (m *memFile) ReadIndex() (string, error) { return m.read() }
func (m *memFile) IndexLocation() string      { return m.location }
func (m *memFile) WriteIndex(c string) error  { return m.write(c) }
func (m *memFile) ReadLog() (string, error)   { return m.read() }
func (m *memFile) LogLocation() string        { return m.location }
func (m *memFile) WriteLog(c string) error    { return m.write(c) }

// memSyncState is a SyncStateRecorder that remembers the instants it was asked to record, and can fail on demand.
type memSyncState struct {
	err      error
	recorded []time.Time
}

func (m *memSyncState) RecordSyncCompleted(at time.Time) error {
	m.recorded = append(m.recorded, at)
	return m.err
}
