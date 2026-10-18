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
// The rule is the one the engine uses too (identity/resolve, C23 M2), a chain
// where the first match wins:
//
//  1. DECLARED -- a .longterm-mem-project file at the root of the main
//     checkout naming the project. It wins over every derived rule: it is the
//     only rule a human controls, and the only escape hatch when a derived
//     rule answers something the operator does not want.
//  2. NORMALIZED REMOTE -- origin's URL reduced to "host/path", so that
//     git@host:owner/name.git, https://host/owner/name and
//     https://host/owner/name.git all collapse to host/owner/name.
//  3. DIRECTORY -- the name of the main checkout's root, shared by the main
//     checkout, every linked worktree and every directory below them. Outside
//     a git repository it is the name of the directory asked about, so the
//     module works there too (decision 4 of M2).
//
// (The engine's chain starts with its --project-id; this module's explicit
// name is --project, which never reaches Resolve.)
//
// THE PROPERTY: for any two directories inside the same repository -- the
// main checkout and any number of linked worktrees -- Resolve returns the
// IDENTICAL identity, at every level of the chain independently.
//
// Until M2 the third rule was the realpath of the git common directory. It is
// no longer derived as the identity, but a repository may hold memory under
// it, so adoption still offers it (RuleCommonDir): a repository is never cut
// off from what it stored under its former name.
//
// The rules read the repository through RepositoryInspector, the shared
// rules' port: where the repository is, the declaration file of its main
// checkout, and its git configuration. They walk no directory, read no file
// and run no process; internal/projectidfs names the shared disk adapter
// (identity/gitfs, which never shells out, R-021), and the composition root
// hands the rules that adapter.
package projectid

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// DeclaredFileName is the file a repository carries to name its project
// explicitly. It sits at the root of the main checkout, holds exactly one
// non-empty line, and beats every derived rule.
const DeclaredFileName = resolve.DeclaredFileName

// Rule names which link of the chain produced an identity. A caller that
// cannot tell a declared name from a path-derived fallback cannot warn
// about the fallback being fragile, so this is part of the result.
type Rule string

const (
	// RuleDeclared: read verbatim from DeclaredFileName.
	RuleDeclared Rule = "declared"
	// RuleRemote: origin's URL normalized to "host/path".
	RuleRemote Rule = "remote"
	// RuleDirectory: the name of the main checkout's root, or, outside a git
	// repository, of the directory itself. It is the chain's last rule.
	RuleDirectory Rule = "directory"
	// RuleCommonDir: the absolute, symlink-resolved git common directory.
	// It was the chain's last rule until M2 and is no longer derived as the
	// identity; adoption still offers it, so memory stored under it stays
	// reachable. It is tied to where the repository happens to live, so it
	// does not survive the repository being moved or re-cloned elsewhere.
	RuleCommonDir Rule = "common_dir"
	// RuleRemembered: a name this repository was known by before, recovered
	// from a caller's ledger rather than derived from the repository as it
	// looks now. It is what reunites a repository with memory it stored
	// under a name nothing derives any more.
	RuleRemembered Rule = "remembered"
)

var (
	// ErrNotARepository says that a directory is not inside a git repository.
	// Resolve no longer returns it (outside a repository the directory's name
	// is the identity); CheckCorrespondence and CommonDir still do, because
	// outside a repository there is nothing to check and no ledger to keep.
	ErrNotARepository = resolve.ErrNotARepository

	// ErrDeclaredInvalid is returned when DeclaredFileName exists but does
	// not hold exactly one non-empty line. A broken declaration is
	// reported rather than skipped: silently falling through to a derived
	// rule would hide the very thing the human wrote the file to control.
	ErrDeclaredInvalid = resolve.ErrDeclaredInvalid
)

// Identity is a canonical project identity plus the rule that produced it.
type Identity struct {
	// Project is the canonical identity string. Its shape depends on Rule:
	// a bare name for RuleDeclared and RuleDirectory, "host/owner/name" for
	// RuleRemote, an absolute path for an adopted RuleCommonDir.
	Project string `json:"project"`
	// Rule names the chain link that produced Project.
	Rule Rule `json:"rule"`
	// WorktreeRoot is the root of the working tree dir belongs to, and
	// differs between a main checkout and each linked worktree. It is
	// reported for diagnostics and is deliberately NOT part of the
	// identity -- it is the value that fragments. It is empty outside a
	// repository.
	WorktreeRoot string `json:"worktree_root"`
	// CommonDir is the absolute, symlink-resolved git common directory,
	// shared by the main checkout and every linked worktree. It is empty
	// outside a repository.
	CommonDir string `json:"common_dir"`
}

// Resolve returns the canonical identity of the project containing dir,
// by the shared chain (identity/resolve). The declaration is read from
// EXACTLY ONE location, the main checkout's root: a file that only a linked
// worktree's branch carries would give that worktree a second name, which is
// the fragmentation this package exists to prevent.
func (r Resolver) Resolve(dir string) (Identity, error) {
	id, err := r.rules.Resolve(dir, "")
	if err != nil {
		return Identity{}, err
	}
	return identityOf(id.Repository, id.Project, ruleOf(id.Source)), nil
}

// ruleOf is the rule of this module that names a source of the shared chain.
// The chain is asked with no explicit id, so it never answers explicit.
func ruleOf(source resolve.Source) Rule {
	switch source {
	case resolve.SourceDeclared:
		return RuleDeclared
	case resolve.SourceRemote:
		return RuleRemote
	default:
		return RuleDirectory
	}
}

func identityOf(repo Repository, project string, rule Rule) Identity {
	return Identity{Project: project, Rule: rule, WorktreeRoot: repo.WorktreeRoot, CommonDir: repo.CommonDir}
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
// ErrNotARepository and the caller has nothing to check: there the identity
// is only the name of the directory the command happens to run in, and an
// operator naming a project from such a place (a CI job, a scratch
// directory) fragments no repository.
//
// Correspondence is deliberately looser than string equality. The remote
// rule produces an identity no operator would ever type -- a
// "host/owner/name" form -- so the remote's last segment counts as
// corresponding too, as the directory name of an adopted common-dir path
// does. Without that, every command run in a repository with a remote and no
// declared file would warn, and a warning that fires on every correct call
// teaches operators to stop reading warnings, which costs more than it
// catches.
func (r Resolver) CheckCorrespondence(dir, given string) (Correspondence, error) {
	resolved, err := r.rules.Resolve(dir, "")
	if err != nil {
		return Correspondence{}, err
	}
	if !resolved.InRepository {
		return Correspondence{}, fmt.Errorf("%w: %s", ErrNotARepository, dir)
	}
	id := identityOf(resolved.Repository, resolved.Project, ruleOf(resolved.Source))

	c := Correspondence{Given: strings.TrimSpace(given), Canonical: id}
	c.Match = corresponds(id, c.Given)
	return c, nil
}

// corresponds reports whether given names id. An empty name names nothing, not even an identity that is empty
// itself: a project called "" is the failure the named errors of this package exist to prevent.
func corresponds(id Identity, given string) bool {
	for _, candidate := range correspondingNames(id) {
		if candidate != "" && candidate == given {
			return true
		}
	}
	return false
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
// would also be unsound as a rule, because the remote rule produces an
// identity no operator would ever type, so "differs from
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
