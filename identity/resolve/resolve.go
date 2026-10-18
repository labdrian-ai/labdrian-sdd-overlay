// Package resolve is the one resolver of a project's identity, shared by the engine and
// longterm-mem (C23, M2): the chain of rules that names the project a directory belongs to, and the
// port it reads a repository through. The workflow, the skills verbs, the memory gate and
// longterm-mem all name a project with it, so that one repository has one name everywhere.
//
// The chain, first match wins (the owner's decision 1 of M2, 2026-10-18):
//
//  1. EXPLICIT -- the id the person gave (the engine's --project-id). Nothing is looked at.
//  2. DECLARED -- the file .longterm-mem-project at the root of the main checkout, holding one
//     non-empty line. Only the main checkout's root is read: a declaration is a property of the
//     repository, and one that a linked worktree's branch carries alone would give the worktree a
//     second name.
//  3. REMOTE -- the origin remote, normalized by identity.NormalizeRemote ("host/path").
//  4. DIRECTORY -- the name of the main checkout's root, so a linked worktree and a directory below
//     the root answer like the main checkout. Outside a git repository it is the name of the
//     directory asked about (decision 4 of M2).
//
// The rules are pure: they read the repository through Repositories, a port this package owns, and
// touch no file and run no process. identity/gitfs answers the port from the metadata git keeps on
// a disk; a module may hand the rules another adapter of its own.
package resolve

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/identity"
)

// DeclaredFileName is the file a repository carries at the root of its main checkout to name its
// project. It holds exactly one non-empty line and beats every derived rule.
const DeclaredFileName = ".longterm-mem-project"

// The errors keep the "projectid:" prefix longterm-mem has always printed: they are what an
// operator reads, and longterm-mem's recorded transcripts hold them.
var (
	// ErrNotARepository says that a directory is in no git repository. Repositories.Locate wraps it;
	// the chain then names the project by the directory.
	ErrNotARepository = errors.New("projectid: directory is not inside a git repository")

	// ErrDeclaredInvalid says that the declaration file is there and is not a single non-empty line.
	// A broken declaration is refused, not skipped: falling through to a derived rule would hide the
	// one thing a person wrote the file to control.
	ErrDeclaredInvalid = errors.New("projectid: declared project file is not a single non-empty line")
)

// Source names the rule of the chain that gave an identity.
type Source string

const (
	// SourceExplicit is the id the person gave.
	SourceExplicit Source = "explicit"
	// SourceDeclared is the text of DeclaredFileName.
	SourceDeclared Source = "declared"
	// SourceRemote is the origin remote, normalized.
	SourceRemote Source = "remote"
	// SourceDirectory is the name of the main checkout's root, or of the directory outside a
	// repository.
	SourceDirectory Source = "directory"
)

// Repository is where the repository a directory belongs to is. The adapter that answers for it
// has made its paths absolute and clean.
type Repository struct {
	// WorktreeRoot is the root of the working tree the directory is in: the main checkout or one
	// linked worktree.
	WorktreeRoot string
	// MainWorktreeRoot is the root of the main checkout, empty when there is none the adapter can
	// name (a bare repository).
	MainWorktreeRoot string
	// CommonDir is the git directory a main checkout and every linked worktree share; its config
	// names the remotes.
	CommonDir string
}

// Repositories is what the rules ask of the repository a directory belongs to.
type Repositories interface {
	// Locate finds the repository dir belongs to. An error that satisfies
	// errors.Is(err, ErrNotARepository) says dir is in none; any other error says the repository
	// could not be read, and stops the chain.
	Locate(dir string) (Repository, error)
	// DeclaredProject returns the text of DeclaredFileName in root and whether it is there. The
	// error says why a file that is there could not be read.
	DeclaredProject(root string) (text string, found bool, err error)
	// GitConfig returns the text of the config file in commonDir, "" when there is none. The error
	// says why a file that is there could not be read.
	GitConfig(commonDir string) (string, error)
}

// Identity is a project's name and how it was found.
type Identity struct {
	// Project is the name.
	Project string
	// Source is the rule that gave Project.
	Source Source
	// InRepository says that the directory is in a git repository, which Repository describes. It
	// is false for an explicit id, which looks at nothing, and outside a repository.
	InRepository bool
	// Repository is where the repository is, when InRepository.
	Repository Repository
}

// Resolver applies the chain to the repositories its port answers for.
type Resolver struct {
	repositories Repositories
}

// New returns the rules reading repositories through r.
func New(r Repositories) Resolver {
	return Resolver{repositories: r}
}

// Resolve names the project dir belongs to. explicit is the id the person gave, "" for none. An
// error is a repository or a declaration that could not be read, or a declaration that is broken;
// the chain never goes on past one to name the project by something less than what was asked.
func (r Resolver) Resolve(dir, explicit string) (Identity, error) {
	if explicit != "" {
		return Identity{Project: explicit, Source: SourceExplicit}, nil
	}
	repo, err := r.repositories.Locate(dir)
	switch {
	case errors.Is(err, ErrNotARepository):
		return Identity{Project: filepath.Base(dir), Source: SourceDirectory}, nil
	case err != nil:
		return Identity{}, err
	}
	in := func(project string, source Source) Identity {
		return Identity{Project: project, Source: source, InRepository: true, Repository: repo}
	}

	if name, ok, err := r.Declared(repo); err != nil {
		return Identity{}, err
	} else if ok {
		return in(name, SourceDeclared), nil
	}
	if name, ok, err := r.Remote(repo); err != nil {
		return Identity{}, err
	} else if ok {
		return in(name, SourceRemote), nil
	}
	return in(MainCheckoutName(repo), SourceDirectory), nil
}

// Declared is the name repo's declaration file gives, read at the root of its main checkout (or
// at the worktree's root when the repository has no main checkout). It reports false when there is
// no file, and ErrDeclaredInvalid, naming the file, when it is not one non-empty line.
func (r Resolver) Declared(repo Repository) (string, bool, error) {
	root := mainRoot(repo)
	text, found, err := r.repositories.DeclaredProject(root)
	if err != nil || !found {
		return "", false, err
	}
	name := strings.TrimSpace(text)
	if name == "" || strings.ContainsAny(name, "\n\r") {
		return "", false, fmt.Errorf("%w: %s", ErrDeclaredInvalid, filepath.Join(root, DeclaredFileName))
	}
	return name, true, nil
}

// Remote is repo's origin remote, normalized by identity.OriginRemote. It reports false when the
// config names no origin with a host.
func (r Resolver) Remote(repo Repository) (string, bool, error) {
	config, err := r.repositories.GitConfig(repo.CommonDir)
	if err != nil {
		return "", false, err
	}
	name, ok := identity.OriginRemote(config)
	return name, ok, nil
}

// MainCheckoutName is the name of repo's main checkout: the base of its root, or of the
// worktree's root when it has no main checkout.
func MainCheckoutName(repo Repository) string {
	return filepath.Base(mainRoot(repo))
}

func mainRoot(repo Repository) string {
	if repo.MainWorktreeRoot != "" {
		return repo.MainWorktreeRoot
	}
	return repo.WorktreeRoot
}
