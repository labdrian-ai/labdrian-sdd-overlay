package projectid

import (
	"path/filepath"
)

// fakeRepositories is a RepositoryInspector over a table: it answers for the repositories a test describes, from
// no disk, and records every question so that a test can say what was asked and what was not.
type fakeRepositories struct {
	// repos are the repositories by the directory that locates them.
	repos map[string]Repository
	// locateErr, when set, is what every Locate answers.
	locateErr error
	// declarations are the texts of the declaration file by the root they are read from.
	declarations map[string]string
	// declarationErr, when set, is what every DeclaredProject answers.
	declarationErr error
	// configs are the git configuration texts by common directory.
	configs map[string]string

	located    []string
	declared   []string
	configRead []string
}

func (f *fakeRepositories) Locate(dir string) (Repository, error) {
	f.located = append(f.located, dir)
	if f.locateErr != nil {
		return Repository{}, f.locateErr
	}
	repo, ok := f.repos[dir]
	if !ok {
		return Repository{}, ErrNotARepository
	}
	return repo, nil
}

func (f *fakeRepositories) DeclaredProject(root string) (string, bool, error) {
	f.declared = append(f.declared, root)
	if f.declarationErr != nil {
		return "", false, f.declarationErr
	}
	text, ok := f.declarations[root]
	return text, ok, nil
}

func (f *fakeRepositories) GitConfig(commonDir string) string {
	f.configRead = append(f.configRead, commonDir)
	return f.configs[commonDir]
}

// originConfig is the text of a git configuration that names url as origin.
func originConfig(url string) string {
	return "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + url + "\n"
}

// widgets is the repository most cases ask about: a main checkout at /src/widgets and a linked worktree of it at
// /src/widgets-feature, both of the common directory /src/widgets/.git.
func widgets(config string) *fakeRepositories {
	common := filepath.FromSlash("/src/widgets/.git")
	main := Repository{WorktreeRoot: filepath.FromSlash("/src/widgets"), MainWorktreeRoot: filepath.FromSlash("/src/widgets"), CommonDir: common}
	linked := Repository{WorktreeRoot: filepath.FromSlash("/src/widgets-feature"), MainWorktreeRoot: filepath.FromSlash("/src/widgets"), CommonDir: common}
	f := &fakeRepositories{
		repos:        map[string]Repository{main.WorktreeRoot: main, linked.WorktreeRoot: linked},
		declarations: map[string]string{},
		configs:      map[string]string{},
	}
	if config != "" {
		f.configs[common] = config
	}
	return f
}

// mainRoot and linkedRoot are the directories widgets() locates.
var (
	mainRoot   = filepath.FromSlash("/src/widgets")
	linkedRoot = filepath.FromSlash("/src/widgets-feature")
	commonDir  = filepath.FromSlash("/src/widgets/.git")
)
