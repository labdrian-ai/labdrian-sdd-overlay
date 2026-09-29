package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// Classification is the closed vocabulary Load reports for the on-disk state
// of one workflow. Only ClassificationAbsent and ClassificationOwned accept
// further writes through Append; every other value is preserved exactly as
// found and reported through Append's named refusal errors.
type Classification string

const (
	// ClassificationAbsent means no file exists yet at the workflow's path.
	// The next Append must be the workflow's created event at seq 0.
	ClassificationAbsent Classification = "absent"
	// ClassificationOwned means every line parsed as a WorkflowEvent v1 for
	// this exact project_id/workflow_id, the hash chain and seq sequence
	// verify (VerifyEvents), and the lifecycle transitions replay cleanly
	// (Replay). Loaded.Events and Loaded.State are populated.
	ClassificationOwned Classification = "owned"
	// ClassificationForeign means the file exists and is not a workflow log
	// of ours: it may be valid JSON lacking our version/shape, or a
	// correctly shaped WorkflowEvent log for a different workflow_id or
	// project_id than requested. A foreign file is never overwritten.
	ClassificationForeign Classification = "foreign"
	// ClassificationMalformed means the file cannot be parsed as a sequence
	// of our JSONL records at all: not valid UTF-8, oversized beyond
	// maxWorkflowLogBytes, missing its final trailing newline, containing a
	// blank line, or containing a line that is not even syntactically valid
	// JSON. A malformed file is never overwritten.
	ClassificationMalformed Classification = "malformed"
	// ClassificationDrifted means every line parses as one of our events for
	// the right ids, but the hash chain, seq sequence, or lifecycle
	// transitions do not verify. A drifted file is never overwritten.
	ClassificationDrifted Classification = "drifted"
	// ClassificationUnavailable means the state root or the file itself
	// could not be read (for example, a permission error, or a symlink at a
	// store path component). Nothing about the file's content is known, so
	// it is never overwritten.
	ClassificationUnavailable Classification = "unavailable"
)

// Loaded is the result of Load: the classification, and, only when
// Classification is ClassificationOwned, the parsed event log and its
// replayed State. Detail carries a human-readable explanation for every
// classification other than ClassificationAbsent and ClassificationOwned.
type Loaded struct {
	Classification Classification
	Events         []WorkflowEvent
	State          State
	Detail         string
}

// Sentinel errors Append returns. Wrap with %w so callers can distinguish
// the failure with errors.Is.
var (
	// ErrStoreNotInitialized is returned by every Store method when called
	// on the zero Store value instead of one built with NewStore.
	ErrStoreNotInitialized = errors.New("workflow store: store is not initialized; use NewStore")
	// ErrRefuseForeignState is returned by Append when the current on-disk
	// state classifies as foreign; the file is left byte-for-byte unchanged.
	ErrRefuseForeignState = errors.New("workflow store: refusing to write: on-disk state is foreign")
	// ErrRefuseMalformedState is returned by Append when the current
	// on-disk state classifies as malformed; the file is left
	// byte-for-byte unchanged.
	ErrRefuseMalformedState = errors.New("workflow store: refusing to write: on-disk state is malformed")
	// ErrRefuseDriftedState is returned by Append when the current on-disk
	// state classifies as drifted; the file is left byte-for-byte
	// unchanged.
	ErrRefuseDriftedState = errors.New("workflow store: refusing to write: on-disk state is drifted")
	// ErrStateUnavailable is returned by Append when the current on-disk
	// state could not be read (classification unavailable); nothing is
	// written.
	ErrStateUnavailable = errors.New("workflow store: refusing to write: on-disk state is unavailable")
	// ErrAppendConflict is returned by Append when another Append for the
	// same project_id/workflow_id holds the append lock. Append does not
	// retry or wait: the caller decides whether to retry.
	ErrAppendConflict = errors.New("workflow store: append: a concurrent append is in progress for this workflow")
)

// workflowStoreComponents are the fixed directories under the state home.
var workflowStoreComponents = []string{"labdrian", "workflows"}

// maxWorkflowLogBytes bounds the total size of one workflow's on-disk JSONL
// log that Load will read and classify. It exists to give Load a documented,
// finite worst case; a file beyond this size is classified malformed rather
// than read in full. It is far larger than any workflow this phase's
// profiles produce (each event is bounded well under 64 KiB by
// MaxEventBytes), so it is not expected to be reached by normal use: even a
// workflow that recorded MaxStages (256) stages plus every other event kind
// would use a small fraction of this ceiling. 16 MiB also keeps Append's
// O(n) full-log rewrite (see Store's doc comment) a fast, bounded, in-memory
// operation on every supported platform.
const maxWorkflowLogBytes = 16 * 1024 * 1024

// Store holds one workflow's append-only event log at
// <state home>/labdrian/workflows/<project_id>/<workflow_id>.jsonl, outside
// every worktree. The state home is $XDG_STATE_HOME, or $HOME/.local/state
// when XDG_STATE_HOME is unset. It follows the same resolution, directory
// permissions (0700), and file permissions (0600) as engine/roles's
// ChainStore and engine/shaper's FileStore.
//
// Unlike those two stores, which hold one immutable record per file (keyed
// by seq or content hash, published via hardlink so a concurrent write can
// never silently replace an existing record), a workflow's log is one
// mutable, growing file: each Append rewrites the whole file with the new
// event appended. That rewrite-and-replace shape is why Store needs an
// explicit append lock (see acquireLock) where ChainStore and FileStore do
// not: two of their writers race for a name that at most one can ever claim,
// while two of this store's writers would otherwise both read the same
// prefix and each publish a "next" event, silently discarding one of them.
//
// Each Append therefore reads the whole existing log and rewrites it plus
// the new event to a temporary file before renaming it into place (see
// appendWorkflowLog): that is what makes the publish atomic (a single
// rename can never leave a reader with a half-written file), at the cost of
// making one Append's I/O cost O(n) in the number of events already
// recorded. This is bounded and acceptable: maxWorkflowLogBytes caps a
// workflow's whole log at 16 MiB, so the worst-case rewrite is a bounded,
// fast, in-memory copy, never an unbounded scan.
//
// Store supports linux and darwin only (see checkPlatform).
type Store struct {
	stateHome string
}

// ErrUnsupportedPlatform: the running platform has no store support.
var ErrUnsupportedPlatform = errors.New("workflow store: unsupported platform")

// checkPlatform accepts only linux and darwin (no-follow read + reclaimable lock).
func checkPlatform(goos string) error {
	if goos == "linux" || goos == "darwin" {
		return nil
	}
	return fmt.Errorf("%w: %s (supported: linux, darwin)", ErrUnsupportedPlatform, goos)
}

// NewStore resolves the store from the environment, exactly as
// roles.NewChainStore and shaper.NewFileStore do. A set but relative
// XDG_STATE_HOME, and an unset, empty, or relative HOME fallback, are
// refused, as is an unsupported platform (see checkPlatform).
func NewStore() (Store, error) {
	if err := checkPlatform(runtime.GOOS); err != nil {
		return Store{}, err
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return Store{}, fmt.Errorf("workflow store: XDG_STATE_HOME %q is not absolute", xdg)
		}
		return Store{stateHome: filepath.Clean(xdg)}, nil
	}
	home := os.Getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return Store{}, fmt.Errorf("workflow store: XDG_STATE_HOME is unset and HOME %q is not an absolute path", home)
	}
	return Store{stateHome: filepath.Join(home, ".local", "state")}, nil
}

// dirParts returns the state home followed by every store directory
// component up to and including projectID, validating projectID as a safe
// single path component.
func (s Store) dirParts(projectID string) ([]string, error) {
	if s.stateHome == "" {
		return nil, ErrStoreNotInitialized
	}
	if err := ValidateIdentifier("project_id", projectID); err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	parts := append([]string{s.stateHome}, workflowStoreComponents...)
	return append(parts, projectID), nil
}

// path returns the workflow log's path without touching the filesystem,
// validating both identifiers.
func (s Store) path(projectID, workflowID string) (string, error) {
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return "", err
	}
	if err := ValidateIdentifier("workflow_id", workflowID); err != nil {
		return "", fmt.Errorf("workflow store: %w", err)
	}
	return filepath.Join(append(dirParts, workflowID+".jsonl")...), nil
}

// lockPath returns the append lock's path for one workflow.
func (s Store) lockPath(projectID, workflowID string) (string, error) {
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return "", err
	}
	if err := ValidateIdentifier("workflow_id", workflowID); err != nil {
		return "", fmt.Errorf("workflow store: %w", err)
	}
	return filepath.Join(append(dirParts, workflowID+".lock")...), nil
}

// Load classifies the on-disk state of one workflow (see Classification)
// and, for ClassificationOwned, returns its parsed events and replayed
// State. Load never writes.
func (s Store) Load(projectID, workflowID string) (Loaded, error) {
	path, err := s.path(projectID, workflowID)
	if err != nil {
		return Loaded{}, err
	}
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return Loaded{}, err
	}

	current := dirParts[0]
	for _, part := range dirParts[1:] {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return Loaded{Classification: ClassificationAbsent}, nil
		}
		if statErr != nil {
			return Loaded{Classification: ClassificationUnavailable, Detail: statErr.Error()}, nil
		}
		if err := requireWorkflowPlainDir(current, info); err != nil {
			return Loaded{Classification: ClassificationUnavailable, Detail: err.Error()}, nil
		}
	}

	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Loaded{Classification: ClassificationAbsent}, nil
	}
	if err != nil {
		return Loaded{Classification: ClassificationUnavailable, Detail: err.Error()}, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Loaded{Classification: ClassificationUnavailable, Detail: fmt.Sprintf("refusing symlinked workflow log %q", path)}, nil
	}

	data, err := readWorkflowLogFile(path)
	if err != nil {
		return Loaded{Classification: ClassificationUnavailable, Detail: err.Error()}, nil
	}
	return classifyWorkflowLog(projectID, workflowID, data), nil
}

// classifyWorkflowLog classifies raw JSONL bytes against the requested
// project_id/workflow_id. See Classification for the exact rules.
func classifyWorkflowLog(projectID, workflowID string, data []byte) Loaded {
	if len(data) == 0 {
		return Loaded{Classification: ClassificationMalformed, Detail: "workflow log is empty"}
	}
	if len(data) > maxWorkflowLogBytes {
		return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("workflow log is %d bytes, exceeding the maximum of %d", len(data), maxWorkflowLogBytes)}
	}
	if !utf8.Valid(data) {
		return Loaded{Classification: ClassificationMalformed, Detail: "workflow log is not valid UTF-8"}
	}
	if data[len(data)-1] != '\n' {
		return Loaded{Classification: ClassificationMalformed, Detail: "workflow log does not end with a trailing newline"}
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")

	events := make([]WorkflowEvent, 0, len(lines))
	for i, line := range lines {
		// Detail messages report 1-based line numbers: a person reading the
		// raw file (or an editor's line gutter) counts lines from 1, not 0.
		lineNumber := i + 1
		if line == "" {
			return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("line %d is blank", lineNumber)}
		}
		if !json.Valid([]byte(line)) {
			return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("line %d is not valid JSON", lineNumber)}
		}
		e, err := ParseWorkflowEvent([]byte(line))
		if err != nil {
			return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("line %d is not a workflow event we recognize: %v", lineNumber, err)}
		}
		if e.ProjectID != projectID || e.WorkflowID != workflowID {
			return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("line %d declares project_id=%q workflow_id=%q, want %q/%q", lineNumber, e.ProjectID, e.WorkflowID, projectID, workflowID)}
		}
		events = append(events, e)
	}

	if err := VerifyEvents(events); err != nil {
		return Loaded{Classification: ClassificationDrifted, Detail: err.Error()}
	}
	state, err := Replay(events)
	if err != nil {
		return Loaded{Classification: ClassificationDrifted, Detail: err.Error()}
	}
	return Loaded{Classification: ClassificationOwned, Events: events, State: state}
}

// Append validates next against the current on-disk state and, if legal,
// appends it atomically. It is allowed only when Load classifies the
// current state as ClassificationAbsent (next must be the seq-0 created
// event) or ClassificationOwned (next must be the next seq, chain to the
// last stored event's digest, and pass CheckTransition against the replayed
// State). Every other classification is refused with a named error and the
// file, if any, is left byte-for-byte unchanged. Append serializes
// concurrent writers for the same workflow with a lock file; see
// acquireLock.
func (s Store) Append(projectID, workflowID string, next WorkflowEvent) error {
	path, err := s.path(projectID, workflowID)
	if err != nil {
		return err
	}
	lockPath, err := s.lockPath(projectID, workflowID)
	if err != nil {
		return err
	}
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return err
	}
	if err := ensureWorkflowStoreDirs(dirParts); err != nil {
		return err
	}

	release, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer release()

	if next.ProjectID != projectID || next.WorkflowID != workflowID {
		return fmt.Errorf("workflow store: append: event project_id/workflow_id (%q/%q) does not match the requested workflow (%q/%q)", next.ProjectID, next.WorkflowID, projectID, workflowID)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("workflow store: append: %w", err)
	}

	loaded, err := s.Load(projectID, workflowID)
	if err != nil {
		return fmt.Errorf("workflow store: append: %w", err)
	}

	switch loaded.Classification {
	case ClassificationAbsent:
		// An absent workflow has no prior event, so its first append must
		// be the created event at seq 0: checked explicitly and locally
		// here (not only through CheckTransition(State{}, next) below,
		// which enforces the same rule against the zero State as a second,
		// independent line of defense; a future change to CheckTransition
		// cannot silently drop this invariant without also failing here).
		// The zero State also has no last stored digest, so next.PrevDigest
		// must be empty, mirroring the explicit prev_digest check the
		// ClassificationOwned branch below performs against its own last
		// stored event's digest.
		if next.Seq != 0 || next.Kind != KindCreated {
			return fmt.Errorf("workflow store: append: the first event must be a created event at seq 0, got kind %q at seq %d", next.Kind, next.Seq)
		}
		if next.PrevDigest != "" {
			return fmt.Errorf("workflow store: append: prev_digest must be empty for the first event, got %q", next.PrevDigest)
		}
		if err := CheckTransition(State{}, next); err != nil {
			return fmt.Errorf("workflow store: append: %w", err)
		}
	case ClassificationOwned:
		last := loaded.Events[len(loaded.Events)-1]
		lastDigest, err := EventDigest(last)
		if err != nil {
			return fmt.Errorf("workflow store: append: %w", err)
		}
		if next.Seq != len(loaded.Events) {
			return fmt.Errorf("workflow store: append: seq %d is not the next seq (%d)", next.Seq, len(loaded.Events))
		}
		if next.PrevDigest != lastDigest {
			return fmt.Errorf("workflow store: append: prev_digest %q does not match the last stored event's digest %q", next.PrevDigest, lastDigest)
		}
		if err := CheckTransition(loaded.State, next); err != nil {
			return fmt.Errorf("workflow store: append: %w", err)
		}
	case ClassificationForeign:
		return fmt.Errorf("%w: %s", ErrRefuseForeignState, loaded.Detail)
	case ClassificationMalformed:
		return fmt.Errorf("%w: %s", ErrRefuseMalformedState, loaded.Detail)
	case ClassificationDrifted:
		return fmt.Errorf("%w: %s", ErrRefuseDriftedState, loaded.Detail)
	case ClassificationUnavailable:
		return fmt.Errorf("%w: %s", ErrStateUnavailable, loaded.Detail)
	default:
		return fmt.Errorf("workflow store: append: unknown classification %q", loaded.Classification)
	}

	line, err := next.MarshalLine()
	if err != nil {
		return fmt.Errorf("workflow store: append: %w", err)
	}
	return appendWorkflowLog(path, line)
}

// acquireLock serializes concurrent Append calls for one workflow, failing
// every other caller immediately with ErrAppendConflict; the returned
// release func must be called exactly once (via defer). Platform-specific:
// see store_lock_unix.go and store_lock_other.go.

// appendWorkflowLog rewrites the whole workflow log with line appended to
// its current content (empty if the file does not exist yet), publishing
// the result atomically: written and synced to a temporary file in the
// target directory, then renamed into place (unlike engine/roles's and
// engine/shaper's per-record hardlink publish, a rename is correct here
// because this file is a single mutable, growing log rather than one
// immutable record per name; see Store's doc comment).
func appendWorkflowLog(path string, line []byte) error {
	var existing []byte
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workflow store: refusing symlinked workflow log %q", path)
		}
		existing, err = readWorkflowLogFile(path)
		if err != nil {
			return fmt.Errorf("workflow store: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("workflow store: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".workflow-*.tmp")
	if err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(existing); err != nil {
		tmp.Close()
		return fmt.Errorf("workflow store: %w", err)
	}
	if _, err := tmp.Write(line); err != nil {
		tmp.Close()
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("workflow store: publish: %w", err)
	}
	if err := syncWorkflowDir(dir); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	return nil
}

// ensureWorkflowStoreDirs makes sure the state home exists, then walks every
// store component below it, creating missing ones with mode 0700 and
// refusing any that is a symlink or not a directory. It mirrors
// engine/roles's ensureChainDirs and engine/shaper's ensureStoreDirs.
func ensureWorkflowStoreDirs(parts []string) error {
	home := parts[0]
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return fmt.Errorf("workflow store: create state home: %w", err)
		}
	}
	if info, err := os.Stat(home); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("workflow store: state home %q is not a directory", home)
	}

	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err == nil {
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("workflow store: %w", err)
			}
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("workflow store: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("workflow store: %w", err)
		}
		if err := requireWorkflowPlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

func requireWorkflowPlainDir(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workflow store: refusing symlinked store component %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("workflow store: store component %q is not a directory", path)
	}
	return nil
}

// readWorkflowLogFile reads path without following a final symlink and
// requires the opened descriptor to be a regular file.
func readWorkflowLogFile(path string) ([]byte, error) {
	f, err := openNoFollow(path)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("workflow store: refusing symlinked workflow log %q", path)
		}
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("workflow store: %q is not a regular file", path)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	return data, nil
}

func syncWorkflowDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
