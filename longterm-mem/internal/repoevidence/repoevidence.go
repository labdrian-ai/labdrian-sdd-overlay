// Package repoevidence is the adapter that answers what staleness asks of a repository (staleness.RepoEvidence)
// on a file system and a git repository: the files of the working tree, listed by walking it, and what became of
// a path, told by internal/repohistory, which asks git. It only reads. Only the composition root names it.
package repoevidence

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repohistory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// Repository is the working tree and the repository at root.
type Repository struct {
	root string
}

var _ staleness.RepoEvidence = Repository{}

// At names the repository whose working tree is root.
func At(root string) Repository {
	return Repository{root: root}
}

// Files lists the files of the working tree, walking it once: every entry that is not a directory (a link is a
// file here, and is not followed), as a slash-separated path relative to the root, in the order the walk meets
// them. A directory named .git is not entered, wherever it is: it is the repository's own metadata.
//
// A root that is not there, and a walk that fails, are told in the words the report has always used for them,
// which is why they name staleness, the rule they are told for.
func (r Repository) Files() ([]string, error) {
	var files []string
	err := filepath.WalkDir(r.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(r.root, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("staleness: %s does not exist", r.root)
		}
		return nil, fmt.Errorf("staleness: walking %s: %w", r.root, err)
	}
	return files, nil
}

// History asks the repository's history what became of each path, in the rule's own words.
func (r Repository) History(paths []string) (map[string]staleness.PathFact, error) {
	history, err := repohistory.Inspect(r.root, paths)
	if err != nil {
		return nil, err
	}
	facts := make(map[string]staleness.PathFact, len(history))
	for asked, fact := range history {
		facts[asked] = staleness.PathFact{
			Path:    fact.Path,
			State:   stateOf(fact.State),
			NewPath: fact.NewPath,
			Commit:  fact.Commit,
			At:      fact.At,
		}
	}
	return facts, nil
}

// stateOf is the rule's word for what the history says. A state it has no word for is the one the rule reports
// nothing for.
func stateOf(state repohistory.State) staleness.PathState {
	switch state {
	case repohistory.StatePresent:
		return staleness.PathPresent
	case repohistory.StateDeleted:
		return staleness.PathDeleted
	case repohistory.StateRenamed:
		return staleness.PathRenamed
	default:
		return staleness.PathUnknown
	}
}
