// Package app holds the use cases of `engine review`: Bind computes the candidate of a review from git and
// makes the detached worktree the reviewer reads, Record holds the reply of the reviewer to the candidate
// it recomputes and stores it, with that binding, in the results directory of a feature, and Verify holds
// the reply of the verifier to a stored result, decides whether the review blocks and stores the reply beside
// the result. Exempt holds the risk the agent declares and the report of the verifier for a head whose work takes
// no review, and stores the exemption beside the results. CheckEvidence holds the evidence of the record that
// closes a review to the files stored, and Check holds a head being delivered to the results stored for it. They
// answer with typed values and typed refusals
// over two ports this package owns: Git, the repository as the use cases ask about it, and Results, the
// directory where results are stored. The adapters are review/gitexec and review/fsstore, and the words a
// person reads, the flags and the exit codes are the command's, in cmd.
//
// The package is pure: it reads no file, runs no program, prints nothing and exits nowhere. The vocabulary
// (the candidate, the names of files and worktrees, the stored file, the skeleton of the record of the role
// chain) is review's, the rules a reply of the reviewer is held to are reviewresult's, and the rules a reply
// of the verifier is held to, and the decision, are reviewverdict's.
//
// Two rules the use cases keep, because the commands are what the parent of the role chain trusts:
//
//   - the candidate of Bind, Record and Exempt is computed from git on every call and never taken from a reply or
//     from an earlier call, so what a reply is held to is the repository as it is; Verify takes it from the
//     stored result, which names the commits in full; Check computes it again for the commits a stored result
//     names, in the same way, and holds the stored binding to it;
//   - a refused reply stores nothing: validation is the last step before the first write, and the write is
//     one atomic creation of a file that did not exist.
package app

import (
	"errors"
	"fmt"
)

// Repository is the repository a command runs in, as the Git port locates it from a directory.
type Repository struct {
	// Root is the root of the working tree that holds the directory: the working tree the command ran in,
	// whether it is the main one or a linked one. The results of a feature are stored under it.
	Root string
	// Main is the root of the main working tree of the repository, beside which the worktrees of the
	// reviews are made. It equals Root when the command runs in the main working tree.
	Main string
	// CommonDir is the git directory every working tree of the repository shares: what makes two
	// directories worktrees of one repository.
	CommonDir string
}

// WorktreeState is what is at the path where the worktree of a review goes.
type WorktreeState struct {
	// Present says that something is at the path: a directory, a file, a link.
	Present bool
	// OfRepository says that it is a linked worktree of the repository.
	OfRepository bool
	// Head is the full name of the commit the worktree has checked out, when OfRepository.
	Head string
}

// Git is the repository as the use cases ask about it. Every revision it is given has passed
// review.CheckRev, and every commit it is given after Commit is a full object name. An implementation runs
// git with an environment of its own and never with a command line a caller composed.
type Git interface {
	// Locate finds the repository that holds dir, which is absolute and may be any directory inside a
	// working tree.
	Locate(dir string) (Repository, error)
	// Commit resolves rev to the full object name of the commit it names.
	Commit(repo Repository, rev string) (string, error)
	// Diff returns the bytes of the canonical diff from base to head, both full names: the bytes whose
	// SHA-256 is the diff_sha256 of the binding. They are the same for the same two commits whichever
	// commit is checked out, whichever user configuration git reads and whatever the environment says.
	Diff(repo Repository, base, head string) ([]byte, error)
	// Paths returns the paths the diff from base to head changes, once each, in the order git prints
	// them, byte for byte, with a rename counted as the removal of the old path and the creation of the
	// new one.
	Paths(repo Repository, base, head string) ([]string, error)
	// Worktree says what is at path.
	Worktree(repo Repository, path string) (WorktreeState, error)
	// AddWorktree makes path a detached worktree of the repository at the commit head.
	AddWorktree(repo Repository, path, head string) error
}

// Results is the directory where the results of the reviews of a feature are stored, odd/review-results/
// <feature>/, under the root of a working tree.
type Results interface {
	// Names lists the names in the directory of the feature, none when there is none. It refuses a path that
	// leads outside the results directory.
	Names(root, feature string) ([]string, error)
	// Create stores data under name in the directory of the feature, making the directory when it is not
	// there. It is atomic (a reader sees the whole file or no file) and never replaces a file: a name that
	// is taken gives ErrExists, whether it was taken before the call or by a writer that raced it.
	Create(root, feature, name string, data []byte) error
	// Read returns the bytes of the file called name in the directory of the feature, and makes nothing. A
	// file that is not there gives ErrNotFound. It refuses a path that leads outside the results directory,
	// a name that is not a regular file and a file above review.MaxStoredBytes.
	Read(root, feature, name string) ([]byte, error)
	// Lock takes the lock of the directory of the feature, which is there, and waits while another holder has
	// it; calling the function it returns gives the lock back. It makes nothing: a directory that is not there
	// gives ErrNotFound. Where the store cannot hold a lock, it says so in its documentation and holds none.
	Lock(root, feature string) (unlock func(), err error)
}

// ErrExists is what Results.Create returns, wrapped or not, for a name that is already taken.
var ErrExists = errors.New("review: a result is already stored under that name")

// ErrNotFound is what Results.Read returns, wrapped or not, for a name that is not stored.
var ErrNotFound = errors.New("review: no result is stored under that name")

var (
	// ErrWorktreeElsewhere is a worktree at the path of the review that has another commit checked out.
	ErrWorktreeElsewhere = errors.New("review: the worktree of the review is at another commit")
	// ErrWorktreeForeign is something at the path of the review that is not a worktree of the repository.
	ErrWorktreeForeign = errors.New("review: the path of the worktree of the review is taken")
	// ErrNoFreeNumber is a results directory in which no number stayed free for the head over a few tries.
	ErrNoFreeNumber = errors.New("review: no result number stayed free")
	// ErrNotFlushed is what Results.Create returns, wrapped, when the result is stored under its name but the
	// directory could not be flushed after it: the result is there, and may not survive a crash of the machine.
	ErrNotFlushed = errors.New("review: the result is stored but its directory was not flushed")
)

// ReplyRefusedError is the refusal of a reply: it did not hold to the binding or to the result it answers,
// was not the object of the contract, or was too large. The review or its verification did not run, and
// nothing was stored. Err is the named error of reviewresult or reviewverdict (or review.ErrReplyTooLarge),
// which errors.Is reads through it.
type ReplyRefusedError struct{ Err error }

func (e *ReplyRefusedError) Error() string {
	return fmt.Sprintf("the reply is refused and nothing was stored: %v", e.Err)
}

func (e *ReplyRefusedError) Unwrap() error { return e.Err }
