package staleness

import "time"

// PathState is what the repository says about a path a memory or a skill names.
type PathState string

const (
	// PathPresent: the path is in the working tree.
	PathPresent PathState = "present"
	// PathDeleted: the path was in the history and a commit removed it.
	PathDeleted PathState = "deleted"
	// PathRenamed: the path's content moved to NewPath. It is absent from where it was and NOT gone, so a memory
	// naming it is out of date, not stale.
	PathRenamed PathState = "renamed"
	// PathUnknown: the repository has nothing to say about the path (it never held it, or the evidence does not
	// decide). Most often it is another project's file, and manufacturing a finding out of it would be inventing
	// evidence.
	PathUnknown PathState = "unknown"
)

// PathFact is what the repository's evidence says about one path.
type PathFact struct {
	Path  string
	State PathState
	// NewPath is where the content went, for PathRenamed.
	NewPath string
	// Commit is the commit that deleted or renamed the path: the evidence, without which a claim of removal is
	// only an assertion.
	Commit string
	// At is that commit's committer date, so that a caller can ask the question that actually decides
	// staleness: was this removed AFTER the memory was written?
	At time.Time
}

// RepoEvidence is what the rule asks of the repository it reads: which files the working tree holds, and what
// the history says of the paths it does not. Both are read-only questions in the words of the rule; where the
// answers come from (a directory and git, a table in a test) is the adapter's, which the program's composition
// root chooses. The rule itself touches no file and runs no process.
type RepoEvidence interface {
	// Files lists every file of the working tree as a slash-separated path relative to its root, leaving out
	// the repository's own metadata. The error says why the tree could not be listed; nothing is returned
	// with it.
	Files() ([]string, error)
	// History says what became of each of paths, which are the ones the listing of Files does not account for:
	// one fact for every distinct path asked about, under the text it was asked by. (A path can still come back
	// PathPresent: the evidence may know of a file the listing does not name, by another spelling of it.) The
	// error says why the history could not answer; it is the answer for all of paths at once, and no fact is
	// returned with it.
	History(paths []string) (map[string]PathFact, error)
}
