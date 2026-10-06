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
//   - ReceiptSink: openspec/changes/<change>/review-receipts/ under the openspec root, where
//     the project versions what it captured.
//   - ChangeCatalog: the directories under <openspec root>/openspec/changes.
//
// The project root is where git is asked to start: it may be the working tree's toplevel or
// any directory inside it. The hook's working directory is whatever the runner gives it
// ($CLAUDE_PROJECT_DIR, or the shell's), and it is not always the toplevel, so git is asked
// which working tree holds the directory (gitprov's Locate, which finds the toplevel from a
// subdirectory and judges it as strictly as the rest of the engine does) and the stores are
// found from there. openspec/ is found from that toplevel too: a session started in a
// subdirectory of the project captures into the project's change, instead of passing the
// acknowledgement through and losing the receipt. Two cases keep the directory as it was
// given: when it is the toplevel itself, so the paths printed keep the form they always had,
// and when the toplevel has no openspec/changes (or git cannot say where it is) but the
// directory has its own, which is served as it always was, because it is the only place the
// receipt can go. A toplevel whose openspec/changes cannot be looked at for another reason than
// that it is absent (no permission, an I/O error) is not the second case: the change may be there,
// so the ports that can say so return the error, and the hook, which is fail-closed, denies. One
// question is asked of git for this, the first time a port needs it.
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
	"sync"

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
	// place is where openspec/ is found, decided once and shared by every copy of the value.
	place *placement
}

// placement is the directory openspec/ is looked for under, found the first time it is needed, or
// the reason it could not be found.
type placement struct {
	once sync.Once
	dir  string
	err  error
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
	return Store{root: root, git: git, place: &placement{}}, nil
}

// openspecRoot is the directory openspec/ is looked for under: the toplevel of the working tree
// that holds the root, when that is another directory than the root and has openspec/changes,
// and the root as it was given otherwise (see the package comment). It is an error when the
// toplevel's openspec/changes cannot be looked at for a reason other than its absence: the
// receipts could be there, so the directory below is not served in its place.
//
// A Store that was not built by New (the zero value) has no placement to share, and decides on
// every use.
func (s Store) openspecRoot() (string, error) {
	if s.place == nil {
		return s.locateOpenspecRoot()
	}
	s.place.once.Do(func() { s.place.dir, s.place.err = s.locateOpenspecRoot() })
	return s.place.dir, s.place.err
}

func (s Store) locateOpenspecRoot() (string, error) {
	abs, err := filepath.Abs(s.root)
	if err != nil || s.git == nil {
		return s.root, nil
	}
	obs, err := s.git.Locate(abs)
	if err != nil || obs.Toplevel == "" {
		return s.root, nil
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved == obs.Toplevel {
		return s.root, nil
	}
	changes := filepath.Join(obs.Toplevel, "openspec", "changes")
	info, err := os.Stat(changes)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s.root, nil
	case err != nil:
		return "", fmt.Errorf("stat %s: %w", changes, unwrapPathError(err))
	case !info.IsDir():
		return s.root, nil
	}
	return obs.Toplevel, nil
}

// unwrapPathError is the error of the file system behind the path an *fs.PathError names, for a
// message that names the path itself.
func unwrapPathError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
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
				return nil, fmt.Errorf("read %s: %w", path, unwrapPathError(err))
			}
			documents = append(documents, reviewreceipt.Document{Shape: file.shape, Data: data, Origin: path})
		}
	}
	return documents, nil
}

// folder is the review-receipts folder of a change.
func (s Store) folder(change string) (string, error) {
	root, err := s.openspecRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "openspec", "changes", change, receiptsFolder), nil
}

// Location is reviewreceipt.ReceiptSink.Location. It only says where a receipt is, for a person to
// read: when the toplevel's openspec cannot be looked at it says the directory the store was
// given, and nothing is read from or written to it (Read and Write refuse).
func (s Store) Location(change, name string) string {
	folder, err := s.folder(change)
	if err != nil {
		folder = filepath.Join(s.root, "openspec", "changes", change, receiptsFolder)
	}
	return filepath.Join(folder, name)
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
	folder, err := s.folder(change)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(folder, name)
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
	folder, err := s.folder(change)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", folder, err)
	}
	return atomicfile.WriteFile(filepath.Join(folder, name), data, atomicfile.Options{Perm: 0o600, Sync: true, TempPattern: tempPattern})
}

// Changes is reviewreceipt.ChangeCatalog. Only a directory is a change, so a stray file or a
// link is not one.
func (s Store) Changes() ([]string, error) {
	root, err := s.openspecRoot()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "openspec", "changes")
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
// Only the absence of the name is "no": any other answer of the file system (no permission to look
// inside the change, an I/O error) is an error that names the path, as it is for the other ports.
func (s Store) HasArtifact(change, name string) (bool, error) {
	root, err := s.openspecRoot()
	if err != nil {
		return false, err
	}
	path := filepath.Join(root, "openspec", "changes", change, name)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", path, unwrapPathError(err))
	}
	return true, nil
}
