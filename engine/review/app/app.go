// Package app holds the use cases of `engine review`: Bind computes the candidate of a review from git and
// makes the detached worktree the reviewer reads, Record holds the reply of the reviewer to the candidate
// it recomputes and stores it, with that binding, in the results directory of a feature, and Verify holds
// the reply of the verifier to a stored result, decides whether the review blocks and stores the reply beside
// the result. Exempt holds the risk the agent declares and the report of the verifier for a head whose work takes
// no review, and stores the exemption beside the results. CheckEvidence holds the evidence of the record that
// closes a review to the files stored, Check holds a head being delivered to the results stored for it, and CheckChain
// holds the commits from a base to it to an unbroken chain of them, and NextUnreviewed names the next commit of a branch
// that no stored review covers, which the stop hook asks for. CountLenses, which needs no git, counts the lens
// rows the stored results of a feature record. They answer with typed values and typed refusals
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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
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

// Standing is where HEAD stands in its branch, as the stop hook asks about it (own-rdd T6a, S6).
type Standing struct {
	// Branch is the short name of the branch HEAD is on, and "" when HEAD is detached, which is all that is told then.
	Branch string
	// Default is the short name of the default branch of the repository, and "" when the repository does not tell
	// which it is, which is all that is told then besides Branch.
	Default string
	// Commits are the commits of HEAD that the default branch does not have, whether it is the local branch or the
	// one the remote keeps: the first-parent line of HEAD down to the commit it was made on, oldest first, each a full
	// object name. None when HEAD is on the default branch or behind it.
	Commits []string
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
	// Parent returns the full object name of the first parent of commit, a full object name, and "" when the commit
	// has none. For a merge it is the parent the merge was made on, the one a diff of the merge is taken from.
	Parent(repo Repository, commit string) (string, error)
	// Parents returns the full object names of every parent of commit, a full object name, in the order git records them
	// (the first is the one a merge was made on), and none for a commit that has no parent (own-rdd T7a, S7 g).
	Parents(repo Repository, commit string) ([]string, error)
	// MergeBases returns the full object names of the best common ancestors of a and b, both full object names: none
	// for histories that share no commit, and no error then.
	MergeBases(repo Repository, a, b string) ([]string, error)
	// Tree returns the full object name of the tree of commit, a full object name.
	Tree(repo Repository, commit string) (string, error)
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
	// Standing says which branch HEAD is on, which branch is the default one (the branch origin/HEAD names, else main,
	// else master, whichever the repository has) and the commits of HEAD the default branch does not have. It asks
	// no more than it needs: a detached HEAD, and a repository with no default branch, are told without the commits.
	Standing(repo Repository) (Standing, error)
	// Message returns the raw message of commit, a full object name, as git stores it, with no trailing newline
	// added or removed beyond the one git ends a message with (own-rdd T6b1, S6 a).
	Message(repo Repository, commit string) (string, error)
	// Trailers returns the trailers git parses from the end of message, in order, each unfolded to one value: what
	// `git interpret-trailers --parse` prints. A message with no trailer block gives none.
	Trailers(repo Repository, message string) ([]review.Trailer, error)
	// DeletedOrRenamed returns the paths the diff from base to head removes, once each, in the order git prints them: with
	// renames not detected, the old path of a rename is a removal (own-rdd T6b3, S6 b, T1).
	DeletedOrRenamed(repo Repository, base, head string) ([]string, error)
	// ChangedLines returns the lines the diff from base to head removes and the lines it adds, of the .go and .sh files it
	// changes, as review.RemovedNames reads them: per path as git prints it, in the order of the paths, each with its
	// lines without the leading '-' or '+'.
	ChangedLines(repo Repository, base, head string) (removed, added []review.ChangedLines, err error)
	// Modules returns the directories, relative to the root of the tree and written with '/', that hold a go.mod at
	// commit, a full object name; the root is ".". Sorted.
	Modules(repo Repository, commit string) ([]string, error)
	// TreeFiles returns every path of the tree of commit, a full object name, written with '/', in the order git prints
	// them (own-rdd T12c, S15 c).
	TreeFiles(repo Repository, commit string) ([]string, error)
	// Mentions returns, for each of names, the paths of the tree of commit, a full object name, that hold it as a whole
	// word, matched as a fixed string, once each, in the order git prints them. A name nothing mentions is not a key and
	// no names is no answer and no error; a search that did not run, or a name that is empty or has a line break, is an error.
	Mentions(repo Repository, commit string, names []string) (map[string][]string, error)
	// IsCommit says whether name, a full object name, is an object git has and that object is a commit. An object git
	// does not have, or one of another kind, is false and no error; git failing to answer is an error.
	IsCommit(repo Repository, name string) (bool, error)
	// Worktrees returns the working trees of the repository, the main one first, as `git worktree list` tells them. A
	// record of another shape is an error (own-rdd T12e, S15 g).
	Worktrees(repo Repository) ([]review.Worktree, error)
	// MergedBranches returns the short names of the local branches merged into into, a commit or a revision, sorted.
	MergedBranches(repo Repository, into string) ([]string, error)
	// IsAncestor says whether a is an ancestor of b, or b itself, both full object names. An object git does not have is
	// an error.
	IsAncestor(repo Repository, a, b string) (bool, error)
	// Status returns the lines of `git status --porcelain` of the working tree of repo, with every untracked file listed on
	// its own line. A clean tree gives none.
	Status(repo Repository) ([]string, error)
	// Archive fills dir, an absolute path to a directory that is there and empty, with the exact bytes of the tree of
	// commit, a full object name: no attribute, filter or end-of-line setting changes a byte of it. Files keep their
	// mode, links their target, and submodules are skipped.
	Archive(repo Repository, commit, dir string) error
}

// The two copies a sweep runs the analyzers on (own-rdd T6b3, S6 b): the tree of the base and the tree of the head.
const (
	CopyBase = "base"
	CopyHead = "head"
)

// Analyzers are the programs of the second check of a sweep, run over a copy of a tree: root is the absolute path of the
// copy and module the directory in it, relative and written with '/', where the program runs. Each returns the lines the
// program printed in its finding format and nothing else, in the order it printed them. A program that is not found, that
// stopped at its deadline, or that exited non-zero without printing a line in its finding format did not run, and the error
// wraps ErrToolDidNotRun and names the program. The adapter is review/analyzers.
type Analyzers interface {
	// Deadcode runs `deadcode -test ./...` over the whole module. A module the copy does not have gives none.
	Deadcode(root, module string) ([]string, error)
	// Staticcheck runs staticcheck over packages, patterns like "./a" or "." of the module, leaving out the ones the copy
	// does not have. None left gives none, and no run. A module the copy does not have gives none.
	Staticcheck(root, module string, packages []string) ([]string, error)
	// Tidy runs `go mod tidy -diff` in the module and gives the requirements it would remove from go.mod, as
	// `go.mod: require <path> <version>`. A module the copy does not have gives none.
	Tidy(root, module string) ([]string, error)
	// Shellcheck runs shellcheck over files, paths relative to root, leaving out the ones the copy does not have.
	Shellcheck(root string, files []string) ([]string, error)
}

// Copies is where a sweep keeps the two copies and the lock that guards them. Their paths are stable between runs, so the
// cache of an analyzer keyed by path is reused. The adapter keeps them under the state home of the user, outside every
// working tree.
type Copies interface {
	// Lock takes the lock of the sweep of repo without waiting. A lock another sweep holds gives ErrSweepBusy. Calling the
	// function it returns gives the lock back.
	Lock(repo Repository) (unlock func(), err error)
	// Reset empties the copy called side (CopyBase or CopyHead) of repo, makes it there, and returns its absolute path.
	Reset(repo Repository, side string) (string, error)
}

var (
	// ErrSweepBusy is what Copies.Lock returns, wrapped or not, for a repository whose sweep is running.
	ErrSweepBusy = errors.New("another review sweep of this repository is running")
	// ErrToolDidNotRun is what an analyzer returns, wrapped, for a check that did not run: a sweep that cannot run a check
	// stores nothing (own-rdd S1: a check that cannot run refuses, never passes).
	ErrToolDidNotRun = errors.New("review: a check of the sweep did not run")
)

// Features is what a working tree says of the features whose reviews it keeps, as the stop hook asks (own-rdd T6a, S6).
type Features struct {
	// Tracked says that the tree keeps features at all: odd/tasks or odd/review-results is a directory in it. The
	// stop hook is installed for every repository of the machine and has nothing to say in one that is not.
	Tracked bool
	// Names are the features that have a results directory, sorted: every one of them holds reviews of the branch.
	Names []string
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
	// Features says whether the working tree under root keeps features and which of them have a results directory.
	// It makes nothing, follows no link, and lists only the directories whose names are a feature
	// (review.CheckFeature); a link or a file where odd or odd/review-results should be a directory gives an error.
	Features(root string) (Features, error)
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
