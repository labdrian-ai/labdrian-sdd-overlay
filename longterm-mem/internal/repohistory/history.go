// Package repohistory asks the repository what became of a path.
//
// Why it exists. longterm-mem can see that a path a memory points at is
// absent from the working tree, and absence is NOT evidence that anything
// was removed. Measured against a real database, the absent paths split
// four ways: files genuinely extirpated, files recorded under a different
// root, files RENAMED, and files belonging to another repository entirely.
// Only the first is staleness. A detector built on "does this file exist"
// would mark the other three as removed, and a memory wrongly marked as
// describing something gone is a memory somebody deletes -- the silent,
// destructive failure this module refuses everywhere else.
//
// Git can tell the four apart, and it is the only thing that can: a path
// never in the history is not this repository's; a path whose last change
// was a delete is extirpated, and the deleting commit is the EVIDENCE; a
// path whose last change was a rename went somewhere, and is not gone at
// all.
//
// This is the second and last file in longterm-mem permitted to import
// "os/exec" (R-021), and the allowlist test names it. Every invocation here
// runs the git binary directly with literal argv -- never a shell, never
// Engram's CLI -- reads only, writes nothing, and is bounded to the one
// repository root it was given.
package repohistory

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// State is what the repository says about a path.
type State string

const (
	// StatePresent: the path is in the working tree.
	StatePresent State = "present"
	// StateDeleted: the path was in the history and a commit removed it.
	StateDeleted State = "deleted"
	// StateRenamed: the path's content moved to NewPath. It is absent from
	// where it was and NOT gone, so a memory naming it is out of date, not
	// stale.
	StateRenamed State = "renamed"
	// StateUnknown: the path never appeared in this repository's history,
	// so this repository has nothing to say about it. Most often it is
	// another project's file, and manufacturing a finding out of it would
	// be inventing evidence.
	StateUnknown State = "unknown"
)

// PathFact is what the history says about one path.
type PathFact struct {
	Path  string
	State State
	// NewPath is where the content went, for StateRenamed.
	NewPath string
	// Commit is the commit that deleted or renamed the path: the evidence,
	// without which a claim of removal is only an assertion.
	Commit string
	// At is that commit's committer date, so a caller can ask the question
	// that actually decides staleness -- was this removed AFTER the memory
	// was written?
	At time.Time
}

// Inspect reports what became of each path, relative to repoRoot.
func Inspect(repoRoot string, paths []string) (map[string]PathFact, error) {
	if err := verifyRepository(repoRoot); err != nil {
		return nil, err
	}

	facts := make(map[string]PathFact, len(paths))
	for _, p := range paths {
		if _, seen := facts[p]; seen {
			continue
		}
		fact, err := inspectOne(repoRoot, p)
		if err != nil {
			return nil, err
		}
		facts[p] = fact
	}
	return facts, nil
}

// verifyRepository refuses a directory that is not a repository, rather
// than letting every path come back "nothing was ever deleted" -- the
// answer that silently hides every finding.
func verifyRepository(repoRoot string) error {
	out, err := run(repoRoot, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("repohistory: %s is not a readable git repository: %w", repoRoot, err)
	}
	if strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("repohistory: %s is not inside a git work tree", repoRoot)
	}
	return nil
}

func inspectOne(repoRoot, path string) (PathFact, error) {
	// The working tree first: it is the cheapest answer and by far the
	// commonest, so most paths never reach a subprocess at all.
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(path))); err == nil {
		return PathFact{Path: path, State: StatePresent}, nil
	}

	// Two steps, and the reason is measured rather than stylistic. A
	// path-limited log restricts the diff to that path BEFORE rename
	// detection runs, so git has no destination left to match against and
	// reports a rename as a plain delete -- verified against a real
	// repository: `git log -1 -M --name-status -- pkg/moved.go` prints
	// "D pkg/moved.go", while `git show --name-status -M <same commit>`
	// prints "R100 pkg/moved.go pkg/renamed.go". Asking the pathspec for
	// the commit and the UNRESTRICTED diff for the classification is what
	// keeps every moved file from being reported as extirpated.
	out, err := run(repoRoot, "log", "-1", "--format=%H%x00%cI", "--", path)
	if err != nil {
		return PathFact{}, fmt.Errorf("repohistory: reading the history of %s: %w", path, err)
	}
	header := strings.TrimSpace(string(out))
	if header == "" {
		return PathFact{Path: path, State: StateUnknown}, nil
	}

	commit, isoDate, _ := strings.Cut(header, "\x00")
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(isoDate))
	if err != nil {
		return PathFact{}, fmt.Errorf("repohistory: %s: unparseable commit date %q: %w", path, isoDate, err)
	}

	status, err := run(repoRoot, "show", "--name-status", "-M", "--format=", commit)
	if err != nil {
		return PathFact{}, fmt.Errorf("repohistory: reading commit %s: %w", commit, err)
	}
	return classify(path, commit, at, string(status)), nil
}

// classify reads one commit's unrestricted `--name-status -M` diff and
// says what that commit did to path.
func classify(path, commit string, at time.Time, status string) PathFact {
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 2 || fields[1] != path {
			continue
		}
		switch st := fields[0]; {
		case st == "D":
			return PathFact{Path: path, State: StateDeleted, Commit: commit, At: at}
		case strings.HasPrefix(st, "R") && len(fields) >= 3 && isRenameScore(st[1:]):
			return PathFact{Path: path, State: StateRenamed, NewPath: fields[2], Commit: commit, At: at}
		}
	}

	// The commit touched the path but neither deleted nor renamed it, and
	// it is not in the tree -- a merge commit, whose diff `git show` omits
	// by default, is the ordinary way to land here. Reporting unknown keeps
	// this from inventing a deletion it never observed.
	return PathFact{Path: path, State: StateUnknown, Commit: commit, At: at}
}

// isRenameScore guards against reading a status like "RENAMED" or a future
// letter pair as a rename: git writes R followed by a similarity score.
func isRenameScore(s string) bool {
	if s == "" {
		return true // bare "R", which git emits without --find-renames scoring
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

// run executes git in repoRoot with literal argv -- never a shell -- and
// returns stdout. Nothing here writes to the repository.
func run(repoRoot string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	// A read of history must not be steered by the caller's own git
	// configuration: an alias, a pager, or a rename-detection default could
	// change the answer this package reports as evidence.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_PAGER=cat",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
