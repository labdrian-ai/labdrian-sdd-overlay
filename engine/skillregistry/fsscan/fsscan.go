// Package fsscan is the adapter of the skill source of the registry refresh
// (engine/skillregistry/app): the skills of a source as the directories and SKILL.md files of the
// machine, and the test of a project root as a stat of .git and .atl.
package fsscan

import (
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillregistry"
)

// skillFile is the file that makes a directory a skill. The name is matched in this case only.
const skillFile = "SKILL.md"

// Scanner reads the skills of the machine. It holds no state; the zero value is the adapter.
type Scanner struct{}

// IsProject says whether dir holds .git or .atl, as a file (a worktree's .git is one) or a
// directory.
func (Scanner) IsProject(dir string) bool {
	for _, marker := range []string{".git", ".atl"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// Scan returns the skills kept in the source: one for each entry of the directory, in the order
// os.ReadDir gives (by name, in byte order), that holds a SKILL.md it can read. A link to a
// directory is followed and keeps its own path. Hidden directories count; what is not a skill
// (a file, a directory with no SKILL.md, a skill one level deeper) is passed over. A source that is
// not a directory, or cannot be read, does not exist.
func (Scanner) Scan(source skillregistry.Source) ([]skillregistry.Candidate, bool) {
	if info, err := os.Stat(source.Dir); err != nil || !info.IsDir() {
		return nil, false
	}
	entries, err := os.ReadDir(source.Dir)
	if err != nil {
		return nil, false
	}
	var found []skillregistry.Candidate
	for _, entry := range entries {
		path := filepath.Join(source.Dir, entry.Name(), skillFile)
		text, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		found = append(found, skillregistry.Candidate{Scope: source.Scope, DirName: entry.Name(), Path: path, Text: string(text)})
	}
	return found, true
}
