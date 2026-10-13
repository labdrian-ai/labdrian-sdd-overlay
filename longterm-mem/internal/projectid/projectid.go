// Package projectid answers one question: for a given directory, what is
// the canonical identity of the project it belongs to?
//
// Why it exists. longterm-mem keys everything on a project NAME -- the
// vault registry is a map keyed by that name, and Engram is queried
// `WHERE project = ?` with whatever string it was handed. Nothing else in
// the module derives that string, so the module invents no fragmentation
// of its own; it also has no defence against it. One repository addressed
// under several names -- one per git worktree, say -- becomes several
// memories that know nothing about each other, which is the exact opposite
// of long-term memory. There is precedent: an observation was once bound
// to the wrong project because identity was resolved by cwd.
//
// The rule is a chain, first match wins:
//
//  1. DECLARED -- a .longterm-mem-project file at the repository root
//     naming the project. It wins over everything: it is the only rule a
//     human controls, and the only escape hatch when the two derived rules
//     answer something the operator does not want.
//  2. NORMALIZED REMOTE -- origin's URL reduced to "host/path", so that
//     git@host:owner/name.git, https://host/owner/name and
//     https://host/owner/name.git all collapse to host/owner/name.
//  3. REALPATH OF THE GIT COMMON DIR -- always available, even with no
//     remote, and shared by a repository's main checkout and every linked
//     worktree.
//
// THE PROPERTY: for any two directories inside the same repository -- the
// main checkout and any number of linked worktrees -- Resolve returns the
// IDENTICAL identity, at every level of the chain independently.
//
// Rule 3 carries a measured trap. `git rev-parse --git-common-dir` answers
// ".git" from a main checkout and "/abs/path/repo/.git" from a linked
// worktree, and the on-disk metadata has the same shape: a worktree's
// .git file points at <repo>/.git/worktrees/<name>, whose commondir file
// holds the relative "../..". Using either answer raw as a key fragments a
// repository exactly the way this package exists to prevent, so the common
// dir is always made absolute, cleaned, and symlink-resolved -- by the
// inspector that locates the repository, which hands the rules a Repository
// whose paths are already so.
//
// The rules read the repository through RepositoryInspector, a port they
// own: where the repository is, the declaration file of its main checkout,
// and its git configuration. They walk no directory, read no file and run no
// process; internal/projectidfs answers the port from the metadata git keeps
// on a disk (it never shells out, R-021), and the composition root hands the
// rules that adapter.
package projectid

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/identity"
)

// DeclaredFileName is the file a repository carries to name its project
// explicitly. It sits at the root of the working tree, holds exactly one
// non-empty line, and beats both derived rules.
const DeclaredFileName = ".longterm-mem-project"

// Rule names which link of the chain produced an identity. A caller that
// cannot tell a declared name from a path-derived fallback cannot warn
// about the fallback being fragile, so this is part of the result.
type Rule string

const (
	// RuleDeclared: read verbatim from DeclaredFileName.
	RuleDeclared Rule = "declared"
	// RuleRemote: origin's URL normalized to "host/path".
	RuleRemote Rule = "remote"
	// RuleCommonDir: the absolute, symlink-resolved git common directory.
	// This one is a fallback: it is stable across worktrees but tied to
	// where the repository happens to live, so it does not survive the
	// repository being moved or re-cloned elsewhere.
	RuleCommonDir Rule = "common_dir"
	// RuleRemembered: a name this repository was known by before, recovered
	// from a caller's ledger rather than derived from the repository as it
	// looks now. It is what reunites a repository with memory it stored
	// under a name nothing derives any more.
	RuleRemembered Rule = "remembered"
)

var (
	// ErrNotARepository is returned when the directory is not inside a git
	// repository. It is a named failure on purpose: an empty string would
	// later become a project called "".
	ErrNotARepository = errors.New("projectid: directory is not inside a git repository")

	// ErrDeclaredInvalid is returned when DeclaredFileName exists but does
	// not hold exactly one non-empty line. A broken declaration is
	// reported rather than skipped: silently falling through to a derived
	// rule would hide the very thing the human wrote the file to control.
	ErrDeclaredInvalid = errors.New("projectid: declared project file is not a single non-empty line")
)

// Identity is a canonical project identity plus the rule that produced it.
type Identity struct {
	// Project is the canonical identity string. Its shape depends on Rule:
	// a bare name for RuleDeclared, "host/owner/name" for RuleRemote, an
	// absolute path for RuleCommonDir.
	Project string `json:"project"`
	// Rule names the chain link that produced Project.
	Rule Rule `json:"rule"`
	// WorktreeRoot is the root of the working tree dir belongs to, and
	// differs between a main checkout and each linked worktree. It is
	// reported for diagnostics and is deliberately NOT part of the
	// identity -- it is the value that fragments.
	WorktreeRoot string `json:"worktree_root"`
	// CommonDir is the absolute, symlink-resolved git common directory,
	// shared by the main checkout and every linked worktree.
	CommonDir string `json:"common_dir"`
}

// Resolve returns the canonical identity of the project containing dir.
func (r Resolver) Resolve(dir string) (Identity, error) {
	repo, err := r.repositories.Locate(dir)
	if err != nil {
		return Identity{}, err
	}

	if name, ok, err := r.declared(repo); err != nil {
		return Identity{}, err
	} else if ok {
		return identityOf(repo, name, RuleDeclared), nil
	}

	if name, ok := r.remote(repo.CommonDir); ok {
		return identityOf(repo, name, RuleRemote), nil
	}

	return identityOf(repo, repo.CommonDir, RuleCommonDir), nil
}

func identityOf(repo Repository, project string, rule Rule) Identity {
	return Identity{Project: project, Rule: rule, WorktreeRoot: repo.WorktreeRoot, CommonDir: repo.CommonDir}
}

// declared reads DeclaredFileName from EXACTLY ONE location: the main
// checkout's working-tree root. Not the caller's worktree, and not "the
// caller's worktree, falling back to the main one".
//
// That fallback was the first implementation, and it fragmented. A file
// present on a linked worktree's branch but absent on the main checkout's
// made the worktree resolve by the declared rule while the main checkout
// resolved by the remote — two identities for one repository, which is the
// exact defect this package exists to prevent, arriving through its own
// convenience. Searching both roots can only ever be one-directional; the
// direction it misses always fragments.
//
// So the declaration is a property of the REPOSITORY, and the main checkout
// is the repository's canonical working tree. A per-worktree declaration is
// not a convenience this package can offer, because a per-worktree
// declaration IS fragmentation. When the main checkout's root cannot be
// located or carries no file, the declaration simply does not exist for this
// repository, whatever any linked worktree holds, and the chain moves on.
func (r Resolver) declared(repo Repository) (string, bool, error) {
	root := repo.MainWorktreeRoot
	if root == "" {
		root = repo.WorktreeRoot
	}

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

// remote reads origin's URL out of the common dir's config and normalizes
// it. The text is the inspector's, because this module may not shell out
// (R-021); what the text means is the identity module's rule, shared with
// the engine.
func (r Resolver) remote(commonDir string) (string, bool) {
	return identity.OriginRemote(r.repositories.GitConfig(commonDir))
}

// Correspondence is the result of checking a caller-supplied project name
// against the canonical identity of the directory it is being run from.
type Correspondence struct {
	// Given is the project name the caller supplied.
	Given string
	// Canonical is the identity resolved from the directory.
	Canonical Identity
	// Match is true when Given corresponds to Canonical.
	Match bool
}

// CheckCorrespondence resolves dir's canonical identity and reports whether
// given corresponds to it. Outside a git repository it returns
// ErrNotARepository and the caller has nothing to check.
//
// Correspondence is deliberately looser than string equality. Two of the
// three rules produce an identity no operator would ever type -- a
// "host/owner/name" remote form, or an absolute path -- so the repository's
// own directory name and the remote's last segment count as corresponding
// too. Without that, every command run in a repository with no declared
// file would warn, and a warning that fires on every correct call teaches
// operators to stop reading warnings, which costs more than it catches.
func (r Resolver) CheckCorrespondence(dir, given string) (Correspondence, error) {
	id, err := r.Resolve(dir)
	if err != nil {
		return Correspondence{}, err
	}

	c := Correspondence{Given: strings.TrimSpace(given), Canonical: id}
	for _, candidate := range correspondingNames(id) {
		if candidate != "" && candidate == c.Given {
			c.Match = true
			break
		}
	}
	return c, nil
}

// correspondingNames lists the strings that count as naming id.
func correspondingNames(id Identity) []string {
	names := []string{id.Project}
	switch id.Rule {
	case RuleRemote:
		names = append(names, lastSegment(id.Project))
	case RuleCommonDir:
		// The repository's own directory name: the main checkout's
		// directory, i.e. the parent of the common dir.
		names = append(names, filepath.Base(filepath.Dir(id.Project)))
	}
	return names
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// Warning renders the operator-facing warning for a mismatch, and "" when
// the given project corresponds.
//
// It WARNS rather than refuses, and the reason is consequence, not taste.
// The failure this whole package exists to prevent is a SILENT one -- an
// observation bound to the wrong project with nobody told -- and a warning
// removes the silence, which is the actual defect. Refusing would break
// work that is legitimate and routine: an operator standing in one
// repository and deliberately promoting into another project's vault, a CI
// job running from a checkout that is not the project being addressed, or
// an overlay repository operating on every project it manages. Refusing
// would also be unsound as a rule, because two of the three chain rules
// produce an identity no operator would ever type, so "differs from
// canonical" is not the same claim as "wrong". An explicit --project stays
// the operator's decision; it just stops being an invisible one.
func (c Correspondence) Warning() string {
	if c.Match {
		return ""
	}
	return fmt.Sprintf(
		"--project %s does not correspond to this directory's project, which resolves to %q (via the %s rule); "+
			"proceeding as asked, but if this was not deliberate the memory you write lands in a project that will never be searched back",
		c.Given, c.Canonical.Project, c.Canonical.Rule)
}
