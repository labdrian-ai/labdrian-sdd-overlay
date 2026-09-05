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
// dir is always made absolute, cleaned, and symlink-resolved.
//
// No subprocess. Production code in this module may not import os/exec
// (see exec_allowlist_test.go, R-021), so every fact above is read from
// the repository's own on-disk metadata rather than from git itself.
package projectid

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
func Resolve(dir string) (Identity, error) {
	repo, err := discover(dir)
	if err != nil {
		return Identity{}, err
	}

	if name, ok, err := declared(repo); err != nil {
		return Identity{}, err
	} else if ok {
		return repo.identity(name, RuleDeclared), nil
	}

	if name, ok := remote(repo.commonDir); ok {
		return repo.identity(name, RuleRemote), nil
	}

	return repo.identity(repo.commonDir, RuleCommonDir), nil
}

// repository is the resolved on-disk shape of one working tree.
type repository struct {
	// worktreeRoot is the root of the working tree containing the queried
	// directory: the main checkout, or one linked worktree.
	worktreeRoot string
	// mainWorktreeRoot is the main checkout's root, derived from the
	// common dir. Empty for a bare repository.
	mainWorktreeRoot string
	// commonDir is absolute, cleaned and symlink-resolved.
	commonDir string
}

func (r repository) identity(project string, rule Rule) Identity {
	return Identity{Project: project, Rule: rule, WorktreeRoot: r.worktreeRoot, CommonDir: r.commonDir}
}

// discover walks up from dir to the nearest .git entry and resolves the
// working tree root and the git common directory.
func discover(dir string) (repository, error) {
	start, err := realDir(dir)
	if err != nil {
		return repository{}, err
	}

	for cur := start; ; {
		dotGit := filepath.Join(cur, ".git")
		// os.Stat, not os.Lstat: a `.git` that is a SYMLINK to the real
		// git directory is a layout git supports, and lstat reports it as
		// neither a directory nor a regular file. Lstat sent it down the
		// linked-worktree branch, which parsed a directory for a
		// "gitdir:" line and failed the repository outright. Following
		// the link costs nothing here -- the common dir is realpathed
		// either way -- and a dangling link still falls through to the
		// IsNotExist walk, exactly as no .git at all does.
		info, err := os.Stat(dotGit)
		switch {
		case err == nil && info.IsDir():
			// Main checkout: .git is the common dir itself.
			common, err := canonicalDir(dotGit)
			if err != nil {
				return repository{}, err
			}
			// The main checkout's root is cur, which is KNOWN here --
			// not filepath.Dir(common), which merely happens to equal it
			// while the git directory sits inside the working tree. With
			// a symlinked or relocated .git it does not, and deriving it
			// pointed the declared-file read at a directory that is not
			// the repository at all.
			return repository{worktreeRoot: cur, mainWorktreeRoot: cur, commonDir: common}, nil
		case err == nil:
			// Linked worktree (or a submodule): .git is a file naming the
			// worktree's own git dir, whose commondir file points back at
			// the shared one -- relatively, which is the trap.
			//
			// Here the main checkout's root can only be DERIVED, as the
			// parent of the common dir, which is how git presents it too.
			// A repository whose git directory has been relocated out of
			// its working tree therefore has no main root a linked
			// worktree can find; the declared rule then finds no file and
			// the chain falls through to a derived rule, which still
			// answers identically from every worktree. Wrong-but-shared
			// beats right-but-fragmented, and only the declared rule is
			// lost.
			common, err := commonDirFromGitFile(dotGit, cur)
			if err != nil {
				return repository{}, err
			}
			return repository{worktreeRoot: cur, mainWorktreeRoot: filepath.Dir(common), commonDir: common}, nil
		case !os.IsNotExist(err):
			return repository{}, fmt.Errorf("projectid: inspecting %s: %w", dotGit, err)
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return repository{}, fmt.Errorf("%w: %s", ErrNotARepository, start)
		}
		cur = parent
	}
}

// commonDirFromGitFile reads a linked worktree's .git file and returns the
// canonical common dir it ultimately points at.
func commonDirFromGitFile(gitFile, worktreeRoot string) (string, error) {
	raw, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("projectid: reading %s: %w", gitFile, err)
	}
	line := strings.TrimSpace(string(raw))
	rest, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("projectid: %s does not name a git directory: %w", gitFile, ErrNotARepository)
	}
	gitDir := strings.TrimSpace(rest)
	if gitDir == "" {
		return "", fmt.Errorf("projectid: %s names an empty git directory: %w", gitFile, ErrNotARepository)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreeRoot, gitDir)
	}

	// commondir is git's own pointer back to the shared directory, and it
	// is written relative to gitDir ("../.."). Joining it and leaving the
	// ".." segments in place is exactly the fragmenting mistake; canonical
	// resolution below cleans and realpaths it.
	commonPointer := filepath.Join(gitDir, "commondir")
	if raw, err := os.ReadFile(commonPointer); err == nil {
		target := strings.TrimSpace(string(raw))
		if target != "" {
			if !filepath.IsAbs(target) {
				target = filepath.Join(gitDir, target)
			}
			return canonicalDir(target)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("projectid: reading %s: %w", commonPointer, err)
	}

	// No commondir file: this git dir IS the common dir (a submodule, or a
	// plain worktree-less .git file).
	return canonicalDir(gitDir)
}

// canonicalDir makes p absolute, cleaned and symlink-free. Skipping any one
// of the three reproduces the measured fragmentation.
func canonicalDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("projectid: resolving %s: %w", p, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("projectid: resolving %s: %w", abs, err)
	}
	return filepath.Clean(real), nil
}

// realDir canonicalizes the queried directory itself, so a repository
// reached through a symlinked parent resolves like the real path.
func realDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	return canonicalDir(dir)
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
func declared(repo repository) (string, bool, error) {
	root := repo.mainWorktreeRoot
	if root == "" {
		root = repo.worktreeRoot
	}

	path := filepath.Join(root, DeclaredFileName)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("projectid: reading %s: %w", path, err)
	}

	name := strings.TrimSpace(string(raw))
	if name == "" || strings.ContainsAny(name, "\n\r") {
		return "", false, fmt.Errorf("%w: %s", ErrDeclaredInvalid, path)
	}
	return name, true, nil
}

// remote reads origin's URL out of the common dir's config and normalizes
// it. It parses the config file directly because this module may not shell
// out (R-021); the grammar it needs is one section header and one key.
func remote(commonDir string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(commonDir, "config"))
	if err != nil {
		return "", false
	}

	inOrigin := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inOrigin = sectionIsOrigin(line)
			continue
		}
		if !inOrigin {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.ToLower(strings.TrimSpace(key)) != "url" {
			continue
		}
		if normalized := NormalizeRemote(value); normalized != "" {
			return normalized, true
		}
	}
	return "", false
}

// sectionIsOrigin recognizes `[remote "origin"]` and the equivalent
// `[remote.origin]` spelling, case-insensitively on the section name as
// git itself is.
func sectionIsOrigin(header string) bool {
	inner := strings.TrimSpace(strings.Trim(header, "[]"))
	inner = strings.ReplaceAll(inner, "\"", "")
	inner = strings.ReplaceAll(inner, ".", " ")
	fields := strings.Fields(inner)
	return len(fields) == 2 && strings.EqualFold(fields[0], "remote") && fields[1] == "origin"
}

// NormalizeRemote reduces a remote URL to its normal form, "host/path":
// the host lowercased, the path stripped of its leading slash, any
// trailing slash and any ".git" suffix. All of
//
//	git@github.com:acme/widgets.git
//	https://github.com/acme/widgets
//	https://github.com/acme/widgets.git
//	ssh://git@github.com/acme/widgets.git
//
// collapse to "github.com/acme/widgets". The path's case is preserved --
// forge hosts differ on whether it is significant, and folding it would
// merge two repositories that a case-sensitive host keeps apart, which is
// the one mistake this package must never make.
//
// A local-filesystem remote ("/srv/git/widgets.git", "../widgets") has no
// host to key on; it returns "" so the chain falls through to the common
// dir, which for a local remote is the more stable answer anyway.
//
// Two normalizations are deliberately NOT done, because each would merge
// what might be two repositories, and this package's one unforgivable
// mistake is merging: a port is kept on the host ("host:2222/x" stays
// distinct from "host/x", since a second daemon on one machine is a
// different forge), and a ".GIT" suffix is left in place, since the path's
// case is preserved for the same reason. Both cost at worst a second
// identity for one repository -- visible, and fixable with a declared
// file -- where folding them costs one identity for two repositories,
// which is silent.
func NormalizeRemote(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}

	var host, path string
	if scheme, rest, ok := strings.Cut(url, "://"); ok {
		if strings.EqualFold(scheme, "file") {
			return ""
		}
		hostPart, p, _ := strings.Cut(rest, "/")
		host, path = hostPart, p
	} else if before, after, ok := strings.Cut(url, ":"); ok && !strings.Contains(before, "/") {
		// scp-like: [user@]host:path
		host, path = before, after
	} else {
		return ""
	}

	if _, h, ok := strings.Cut(host, "@"); ok {
		host = h
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}

	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	return host + "/" + path
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
func CheckCorrespondence(dir, given string) (Correspondence, error) {
	id, err := Resolve(dir)
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
