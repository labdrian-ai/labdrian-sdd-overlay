// Package evidencelog is the file-backed projection.ReadEvidenceLog (C23 M7,
// docs/architecture/adr-c23-m7-read-evidence.md): the memory-read evidence of each workflow, kept
// in an append-only JSONL sidecar beside its workflow log,
// <state home>/labdrian/workflows/<project_id>/<workflow_id>.memory.jsonl (0600), serialized by its
// own lock, <workflow_id>.memory.lock. It never reads or touches the workflow log itself.
//
// Unlike that log, which is rewritten whole on every append (engine/workflow/filelog), the sidecar
// is only ever appended to: a read is one line written with O_APPEND under the lock, so its cost
// does not grow with the file.
package evidencelog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// Log is a projection.ReadEvidenceLog.
var _ projection.ReadEvidenceLog = Log{}

// ErrUnsupportedPlatform: the running platform has no store support.
var ErrUnsupportedPlatform = errors.New("memory-read evidence: unsupported platform")

// recordVersion is the version of the line's shape.
const recordVersion = 1

// Log appends to the sidecars under one state home.
type Log struct {
	stateHome string
}

// New builds the log over stateHome, which must be absolute; a platform without the no-follow open
// and the lock the log needs is refused with ErrUnsupportedPlatform.
func New(stateHome string) (Log, error) {
	if err := statestore.RequireStore("memory-read evidence", ErrUnsupportedPlatform, stateHome); err != nil {
		return Log{}, err
	}
	return Log{stateHome: stateHome}, nil
}

// line is the shape of one record on disk.
type line struct {
	Version        int      `json:"version"`
	At             string   `json:"at"`
	Tool           string   `json:"tool"`
	Source         string   `json:"source"`
	Project        string   `json:"project"`
	Goal           string   `json:"goal"`
	IDs            []string `json:"ids"`
	Skill          string   `json:"skill"`
	CallID         string   `json:"call_id"`
	Session        string   `json:"session"`
	ResponseSHA256 string   `json:"response_sha256"`
	ResponseBytes  int64    `json:"response_bytes"`
}

// Append writes rec as one line at the end of its workflow's sidecar, creating the directories
// (0700) and the file (0600) when they are missing. It waits for the lock up to filelock's default
// bound. A symlink at the sidecar, or at a directory on its way, is refused, never followed.
func (l Log) Append(rec projection.ReadRecord) error {
	if l.stateHome == "" {
		return errors.New("memory-read evidence: log is not initialized; use New")
	}
	ev := rec.Evidence
	if err := workflow.ValidateIdentifier("project_id", ev.ProjectID); err != nil {
		return fmt.Errorf("memory-read evidence: %w", err)
	}
	if err := workflow.ValidateIdentifier("workflow_id", ev.WorkflowID); err != nil {
		return fmt.Errorf("memory-read evidence: %w", err)
	}
	ids := ev.IDs
	if ids == nil {
		ids = []string{}
	}
	data, err := json.Marshal(line{
		Version: recordVersion, At: rec.At.UTC().Format(time.RFC3339Nano), Tool: ev.Tool, Source: string(ev.Source),
		Project: ev.Project, Goal: ev.Goal, IDs: ids, Skill: ev.Skill, CallID: rec.CallID, Session: rec.SessionID,
		ResponseSHA256: rec.ResponseSHA256, ResponseBytes: rec.ResponseBytes,
	})
	if err != nil {
		return fmt.Errorf("memory-read evidence: encode: %w", err)
	}
	dirParts := []string{l.stateHome, "labdrian", "workflows", ev.ProjectID}
	if err := statestore.EnsureDirs(dirParts); err != nil {
		return fmt.Errorf("memory-read evidence: %w", err)
	}
	dir := filepath.Join(dirParts...)
	unlock, err := filelock.Acquire(filepath.Join(dir, ev.WorkflowID+".memory.lock"), filelock.Options{Perm: 0o600})
	if err != nil {
		return fmt.Errorf("memory-read evidence: lock: %w", err)
	}
	defer unlock()
	path := filepath.Join(dir, ev.WorkflowID+".memory.jsonl")
	f, err := statestore.OpenAppendNoFollow(path, 0o600)
	if err != nil {
		if statestore.IsSymlinkRefusal(err) {
			return fmt.Errorf("memory-read evidence: refusing symlinked sidecar %q", path)
		}
		return fmt.Errorf("memory-read evidence: open: %w", err)
	}
	data = append(data, '\n')
	// A line torn by an earlier append that failed or was cut short (a crash, a full disk) has no
	// newline at its end; this one starts a line of its own instead of being glued onto it, so one
	// broken record never costs a second.
	torn, err := endsTorn(f)
	if err != nil {
		f.Close()
		return fmt.Errorf("memory-read evidence: %w", err)
	}
	if torn {
		data = append([]byte{'\n'}, data...)
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		return fmt.Errorf("memory-read evidence: write: %w", err)
	}
	return nil
}

// endsTorn reports whether f, the sidecar opened under its lock, ends in a line that has no newline.
// It reads the last byte through the same descriptor, never through the path.
func endsTorn(f *os.File) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, info.Size()-1); err != nil {
		return false, err
	}
	return last[0] != '\n', nil
}
