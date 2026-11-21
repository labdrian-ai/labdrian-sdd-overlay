package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitfs"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/evidencelog"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	receiptfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles/filechain"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper/fsadapter"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow/filelog"
)

// This file is where the file-backed stores of the domain are built: the workflow
// event log and its memory-read evidence, the role handoff chain, the session binding store and the shaper's
// clearance store, and the finder of the repository a directory belongs to. Each adapter is
// handed the state home and reads no environment variable, so the composition root is the
// one place that learns where the state lives
// ($XDG_STATE_HOME, or $HOME/.local/state; see statestore.Home). A home that cannot be
// resolved is reported in the words each store has always used for it, with the store's
// name in front. The shaper's contained source is built here too: it keeps no state, so it
// has no home to be handed. So is the review receipt capture, which keeps its state in the
// project it is started for, not in the state home: it is handed the project root and the
// git it asks through, whose ambient environment is read here and nowhere else.

// resolveStateHome resolves the state home from the environment for the store named
// store, which prefixes the error.
func resolveStateHome(store string) (string, error) {
	stateHome, err := statestore.Home()
	if err != nil {
		return "", fmt.Errorf("%s: %w", store, err)
	}
	return stateHome, nil
}

// newWorkflowStore builds the workflow's event log: the file-backed adapter over the
// state home.
func newWorkflowStore() (workflow.EventLog, error) {
	stateHome, err := resolveStateHome("workflow store")
	if err != nil {
		return nil, err
	}
	store, err := filelog.NewStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newEvidenceLog builds the memory-read evidence log (C23 M7): the file-backed adapter over the
// state home.
func newEvidenceLog() (projection.ReadEvidenceLog, error) {
	stateHome, err := resolveStateHome("memory-read evidence")
	if err != nil {
		return nil, err
	}
	log, err := evidencelog.New(stateHome)
	if err != nil {
		return nil, err
	}
	return log, nil
}

// newRoleChainStore builds the role handoff chain log: the file-backed adapter over
// the state home.
func newRoleChainStore() (roles.ChainLog, error) {
	stateHome, err := resolveStateHome("role chain store")
	if err != nil {
		return nil, err
	}
	store, err := filechain.NewStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newBindingStore builds the binding store of the projection domain: the file-backed
// adapter over the state home.
func newBindingStore() (projection.BindingStore, error) {
	stateHome, err := resolveStateHome("projection store")
	if err != nil {
		return nil, err
	}
	store, err := fsstore.NewStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// promptStore keeps, for each session, the SHA-256 of the last block the UserPromptSubmit projection hook added to
// it (S19), so that the hook adds a block only when it differs from the last one. It is shaped as the review
// stop-hook's store (review/fsstore.Asked): <state home>/labdrian/projection-prompt/<sha256 of the session>/last,
// the session hashed because it is a name Claude Code (or the OpenCode bridge) gives and not a path, the
// directories made 0700 and refused when one is a link (statestore.EnsureDirs, CheckDirs), the file opened
// without following a link (statestore.AppendLine, LastLine). The file is append-only, because
// statestore.Publish never replaces a record: one line for each change, and the last line is the last block. A
// line is the 64 hex digits of a hash, or "reset", which says the session's context was cut (start, resume,
// clear, compact) and the block must be added again. A last line that is neither is no block, and the next one
// is added. No cleanup: one small directory for each session, as the review stop-hook keeps.
type promptStore struct{ home string }

const (
	promptStoreName   = "projection prompt store"
	promptStoreFile   = "last"
	promptResetLine   = "reset"
	promptHashLen     = 2 * sha256.Size
	promptStoreWindow = 2 * (promptHashLen + 1) // a little over the longest two lines the file holds
)

var errPromptStoreUnsupported = errors.New(promptStoreName + ": unsupported platform")

// newPromptStore builds the store over the state home, on a platform that has a no-follow open (linux, darwin).
func newPromptStore() (promptStore, error) {
	stateHome, err := resolveStateHome(promptStoreName)
	if err != nil {
		return promptStore{}, err
	}
	if err := statestore.RequireStore(promptStoreName, errPromptStoreUnsupported, stateHome); err != nil {
		return promptStore{}, err
	}
	return promptStore{home: stateHome}, nil
}

func (s promptStore) directory(session string) []string {
	sum := sha256.Sum256([]byte(session))
	return []string{s.home, "labdrian", "projection-prompt", hex.EncodeToString(sum[:])}
}

// last is the hash of the last block added to the session, or "" when none was, the session was reset since, or
// the last line is not a hash. It makes nothing: a store nobody wrote to is absent, not broken.
func (s promptStore) last(session string) (string, error) {
	parts := s.directory(session)
	if err := statestore.CheckDirs(parts); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("%s: %w", promptStoreName, err)
	}
	line, err := statestore.LastLine(filepath.Join(filepath.Join(parts...), promptStoreFile), promptStoreWindow)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", promptStoreName, err)
	}
	if !isHexHash(line) {
		return "", nil
	}
	return line, nil
}

// record says the block whose hash is hash was added to the session. Recording the last block again changes
// nothing.
func (s promptStore) record(session, hash string) error {
	last, err := s.last(session)
	if err != nil {
		return err
	}
	if last == hash {
		return nil
	}
	return s.append(session, hash)
}

// reset says the session's context was cut, so the next block is added whatever it is. A session with no block
// to forget is left as it is.
func (s promptStore) reset(session string) error {
	last, err := s.last(session)
	if err != nil {
		return err
	}
	if last == "" {
		return nil
	}
	return s.append(session, promptResetLine)
}

func (s promptStore) append(session, line string) error {
	if err := statestore.AppendLine(s.directory(session), promptStoreFile, line); err != nil {
		return fmt.Errorf("%s: %w", promptStoreName, err)
	}
	return nil
}

func isHexHash(s string) bool {
	if len(s) != promptHashLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// newClearanceStore builds the shaper's clearance store: the file-backed adapter over the
// state home.
func newClearanceStore() (shaper.ClearanceStore, error) {
	stateHome, err := resolveStateHome("clearance store")
	if err != nil {
		return nil, err
	}
	store, err := fsadapter.NewClearanceStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newRepoLocator builds the adapter that finds the git repository a directory belongs to by
// reading its files, with no subprocess: the answer of the projection domain's RepoLocator port,
// and the worktree and HEAD a workflow records as provenance.
func newRepoLocator() gitfs.Locator { return gitfs.Locator{} }

// newContainedSource builds the shaper's source of the handoff and Goal files: the
// file-backed adapter, with the production behavior of the read (no test hook).
func newContainedSource() shaper.ContainedSource {
	return fsadapter.ContainedSource{}
}

// newReviewReceiptService builds the review receipt capture for the project at root, which
// is the directory the verb was given: the file-backed adapter, finding the review
// transaction stores through gitprov with the real git binary, asked in environ (the environment
// of the process, which main hands over).
func newReviewReceiptService(root string, environ []string) (*reviewreceipt.Service, error) {
	store, err := receiptfs.New(root, gitprov.Observer{Run: gitprov.ExecRunner, Environ: environ, FS: gitprov.OSFS{}})
	if err != nil {
		return nil, err
	}
	return reviewreceipt.NewService(reviewreceipt.Ports{Stores: store, Source: store, Sink: store, Changes: store}), nil
}
