// Package fsstore is the file-backed adapter of the review receipt capture: what
// engine/reviewreceipt asks of the world through its ports (ports.go there), answered from
// git and files.
//
// One Store, built over a project root, is the four ports of the domain at once:
//
//   - TransactionStores: the review-transaction stores of the repository the project root is
//     in, <git dir>/gentle-ai/review-transactions/v2 for the working tree's own git
//     directory and for the one every working tree of the repository shares (the same
//     directory in a plain repository, two in a linked working tree, where an active review
//     lives under the working tree's own).
//   - ReceiptSource: the documents of those stores, one directory per lineage holding the
//     legacy review-receipt.json and the lifecycle review-state.json.
//   - ReceiptSink: openspec/changes/<change>/review-receipts/ under the project root, where
//     the project versions what it captured.
//   - ChangeCatalog: the directories under <project root>/openspec/changes.
//
// The project root is where openspec/ is looked for, and where git is asked to start: it may
// be the working tree's toplevel or any directory inside it. The hook's working directory
// is whatever the runner gives it ($CLAUDE_PROJECT_DIR, or the shell's), and it is not
// always the toplevel, so git is asked which working tree holds the directory (gitprov's
// Locate, which finds the toplevel from a subdirectory and judges it as strictly as the rest
// of the engine does) and the stores are found from there. openspec/ stays under the
// directory as it was given: a project whose openspec lives below its toplevel is served as
// it always was, and one started from a subdirectory that has none is passed through, as it
// always was.
//
// Git is reached only through the Locator the composition root hands in, which is gitprov in
// production; the package starts no process. Files are written with engine/atomicfile.
//
// The layout and the on-disk bytes are a contract with every earlier version of the program
// and with the tool that writes the transactions, and they do not change here: a receipt is
// persisted byte for byte, mode 0600, in a folder of mode 0755 (subject to the umask), under
// <lineage>.json or <lineage>.review-state.json. The words of a failure are the ones the
// domain's package printed before the move, except that a failure to write now carries
// engine/atomicfile's words ("atomicfile: create temporary file: ..."), a git failure
// carries the words of the git question gitprov asks first, and a store is named by the
// path gitprov resolved (symlinks followed). A repository that gitprov refuses (no working
// tree, a forged gitfile, GIT_DIR in the environment) is refused here as everywhere else;
// one with no commit yet is served.
package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

// storeRelPath is the transaction store location relative to a git directory.
const storeRelPath = "gentle-ai/review-transactions/v2"

// The two files a lineage directory of a store holds, one per shape.
const (
	receiptFileName = "review-receipt.json"
	stateFileName   = "review-state.json"
)

// receiptsFolder is the folder of a change that holds the persisted receipts.
const receiptsFolder = "review-receipts"

// tempPattern names the temporary files of a write of either shape of document the package
// persists (a lineage's legacy receipt, <lineage>.json, or its lifecycle state,
// <lineage>.review-state.json): hidden, and the name tells a person that a leftover is a
// review receipt that was being put in place, not which of the two it was.
const tempPattern = ".review-receipt-*.json.tmp"

// Locator finds the git working tree that holds a directory. gitprov.Observer is the
// production one.
type Locator interface {
	Locate(dir string) (gitprov.Observation, error)
}

var _ Locator = gitprov.Observer{}

// Store is the four ports of the review receipt capture over one project root.
type Store struct {
	root string
	git  Locator
}

// New builds the store over the project root, which is where openspec/ is looked for and
// where git is asked to start (see the package comment), and the locator git is asked
// through. Nothing is read until the store is used.
func New(root string, git Locator) (Store, error) {
	if root == "" {
		return Store{}, errors.New("review receipt store: the project root is required")
	}
	if git == nil {
		return Store{}, errors.New("review receipt store: a git locator is required")
	}
	return Store{root: root, git: git}, nil
}

// Stores is reviewreceipt.TransactionStores.
func (s Store) Stores() ([]reviewreceipt.Store, error) {
	abs, err := filepath.Abs(s.root)
	if err != nil {
		return nil, err
	}
	obs, err := s.git.Locate(abs)
	if err != nil {
		return nil, err
	}
	rel := filepath.FromSlash(storeRelPath)
	stores := []reviewreceipt.Store{reviewreceipt.Store(filepath.Join(obs.GitDir, rel))}
	if obs.CommonDir != obs.GitDir {
		stores = append(stores, reviewreceipt.Store(filepath.Join(obs.CommonDir, rel)))
	}
	return stores, nil
}

// Documents is reviewreceipt.ReceiptSource. A store that does not exist holds nothing, and a
// document that does not exist is absent: the stores belong to another tool, which may leave
// a lineage half written. A store or a document that is there and cannot be read is an error
// that names it, never an absence, because the hook counts the approved receipts it reads:
// one dropped in silence would let an acknowledgement proceed and lose that receipt.
func (s Store) Documents(store reviewreceipt.Store) ([]reviewreceipt.Document, error) {
	dir := string(store)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var documents []reviewreceipt.Document
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, file := range []struct {
			shape reviewreceipt.Shape
			name  string
		}{{reviewreceipt.ShapeReceipt, receiptFileName}, {reviewreceipt.ShapeState, stateFileName}} {
			path := filepath.Join(dir, e.Name(), file.name)
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				// The path is named once: the cause of a *fs.PathError, not its own text.
				var pathErr *fs.PathError
				if errors.As(err, &pathErr) {
					err = pathErr.Err
				}
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			documents = append(documents, reviewreceipt.Document{Shape: file.shape, Data: data})
		}
	}
	return documents, nil
}

// folder is the review-receipts folder of a change.
func (s Store) folder(change string) string {
	return filepath.Join(s.root, "openspec", "changes", change, receiptsFolder)
}

// Location is reviewreceipt.ReceiptSink.Location.
func (s Store) Location(change, name string) string {
	return filepath.Join(s.folder(change), name)
}

// notPersisted is the error for a receipt with no file. Its text is the file system's own
// behind the path, and it is reviewreceipt.ErrNotPersisted as well as fs.ErrNotExist.
type notPersisted struct {
	path string
	err  error
}

func (e notPersisted) Error() string   { return fmt.Sprintf("read %s: %v", e.path, e.err) }
func (e notPersisted) Unwrap() []error { return []error{reviewreceipt.ErrNotPersisted, e.err} }

// Read is reviewreceipt.ReceiptSink.Read.
func (s Store) Read(change, name string) ([]byte, error) {
	path := s.Location(change, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notPersisted{path, err}
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// Write is reviewreceipt.ReceiptSink.Write: a temporary file in the folder, flushed, then
// put in place by rename, so a reader sees all of the receipt or none and a failure leaves
// no file behind.
func (s Store) Write(change, name string, data []byte) error {
	folder := s.folder(change)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", folder, err)
	}
	return atomicfile.WriteFile(filepath.Join(folder, name), data, atomicfile.Options{Perm: 0o600, Sync: true, TempPattern: tempPattern})
}

// Changes is reviewreceipt.ChangeCatalog. Only a directory is a change, so a stray file or a
// link is not one.
func (s Store) Changes() ([]string, error) {
	dir := filepath.Join(s.root, "openspec", "changes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// HasArtifact is reviewreceipt.ChangeCatalog.HasArtifact: whether the change holds a file or
// a directory by that name, followed through a link as a person looking at the folder would.
func (s Store) HasArtifact(change, name string) bool {
	_, err := os.Stat(filepath.Join(s.root, "openspec", "changes", change, name))
	return err == nil
}
