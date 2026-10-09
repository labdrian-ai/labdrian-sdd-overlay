// Package gitfs finds the git repository a directory belongs to by reading the files of the
// repository, without running the git binary or any other subprocess. It is the adapter of the
// projection domain's RepoLocator port (Phase 9 unit H29), and it also tells the workflow what
// worktree and HEAD a command was run in, which the workflow records as provenance.
//
// It deliberately does not share engine/gitprov's way of asking: gitprov shells out to the real
// git binary (by design, to get git's own answer, and it fails closed on every ambiguity), which
// the workflow lifecycle commands and the hooks must never do (no subprocesses). The two share
// what a pointer file names (gitprov.PointerTarget) and nothing else: this reader is lenient on
// purpose, because provenance is audit-only and never blocks an operation, and because the key of
// a binding must not change for a repository that has one.
//
// Every step fails soft: anything missing, unreadable, or unexpected leaves the corresponding
// answer empty rather than failing the caller's operation (Decision 6 of the workflow design:
// provenance is audit-only, never a lookup key, and never blocks a lifecycle operation).
package gitfs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// hexObjectID matches a full SHA-1 (40 hex) or SHA-256 (64 hex) git object
// id, the same shape workflow.Provenance.GitHead requires.
var hexObjectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Locator reads repositories from the file system. The zero value is ready to use. It answers
// projection.RepoLocator.
type Locator struct{}

var _ projection.RepoLocator = Locator{}

// Provenance returns the best-effort workflow.Provenance for dir: the worktree root that holds it
// and what HEAD resolves to. dir should be absolute (os.Getwd()'s result); a relative or empty
// dir yields an empty Provenance, since WorktreeRoot must be absolute or empty.
func (Locator) Provenance(dir string) workflow.Provenance {
	if dir == "" || !filepath.IsAbs(dir) {
		return workflow.Provenance{}
	}
	root, gitDir, ok := findGitDir(filepath.Clean(dir))
	if !ok {
		return workflow.Provenance{}
	}
	return workflow.Provenance{WorktreeRoot: root, GitHead: resolveHead(gitDir)}
}

// RepoKey returns the key that identifies the repository containing dir, and whether there is
// one: projection.RepoKeyOf the absolute git common directory, with its symbolic links resolved.
// That is the .git directory itself for a normal checkout and the directory a linked worktree's
// commondir file names, so every worktree of one repository, and every symlinked spelling of its
// path, yields the same key. If the symlinks cannot be resolved for any reason (the directory
// does not exist, a permission error, a symlink loop), the path as it is gets the key. That is
// deterministic for a given spelling of the path, but the key then depends on that spelling: two
// paths to the same repository, one of which cannot be resolved, would get different keys and so
// different bindings.
//
// It needs an absolute dir; a relative or empty dir, no repository above dir, or an unusable
// .git entry yields ("", false).
func (Locator) RepoKey(dir string) (string, bool) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", false
	}
	_, gitDir, ok := findGitDir(filepath.Clean(dir))
	if !ok {
		return "", false
	}
	common := commonDir(gitDir)
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	return projection.RepoKeyOf(common), true
}

// findGitDir walks up from start looking for a .git entry: a directory (a
// normal repository) or a regular file naming another directory (a linked
// worktree or submodule's gitdir pointer). It returns the directory that
// held .git (the worktree root observed, which for a linked worktree is
// this worktree's own root, not the main repository's) and the resolved
// git directory. ok is false when no .git is found before reaching the
// filesystem root, or when a .git file cannot be read and resolved.
//
// Decision: a symlinked .git IS followed (classifyGitEntry below resolves
// it with os.Stat), matching real git, which never distinguishes a plain
// .git directory or gitdir-pointer file from a symlink to one. Nothing
// about that final target's own reading changes: a resolved directory is
// returned as gitDir exactly like a plain one (later os.ReadFile calls
// under it already follow symlinks transparently), and a resolved regular
// file still goes through readGitdirPointer. A symlink that cannot be
// resolved (broken, or pointing at neither a directory nor a regular file)
// fails soft, the same as any other unusable .git entry.
func findGitDir(start string) (worktreeRoot, gitDir string, ok bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".git")
		if info, err := os.Lstat(candidate); err == nil {
			// A .git entry that exists but does not classify to a usable
			// git directory (a malformed gitdir-pointer file, an unusable
			// special file, or a symlink resolving to either) is never
			// something to keep walking past: .git here is not ours to
			// interpret, and a parent directory's .git would name a
			// different, unrelated repository.
			target, ok := classifyGitEntry(candidate, dir, info)
			if !ok {
				return "", "", false
			}
			return dir, target, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// classifyGitEntry resolves one .git entry (already os.Lstat'd as info) to
// the git directory it names: a plain directory or a symlink to one is
// returned as-is (candidate itself, since later reads under it follow
// symlinks transparently); a plain regular file or a symlink to one is
// resolved via readGitdirPointer; anything else (a broken symlink, or some
// other special file) is rejected.
func classifyGitEntry(candidate, base string, info os.FileInfo) (gitDir string, ok bool) {
	switch {
	case info.IsDir():
		return candidate, true
	case info.Mode().IsRegular():
		return readGitdirPointer(candidate, base)
	case info.Mode()&os.ModeSymlink != 0:
		resolved, err := os.Stat(candidate)
		if err != nil {
			return "", false
		}
		switch {
		case resolved.IsDir():
			return candidate, true
		case resolved.Mode().IsRegular():
			return readGitdirPointer(candidate, base)
		default:
			return "", false
		}
	default:
		// Some other special file (device, socket, ...) named .git is not
		// a shape this helper trusts.
		return "", false
	}
}

// readGitdirPointer reads a ".git" file's single "gitdir: <path>" line and
// resolves it (relative to base when not already absolute) to an existing
// directory. The whitespace around the line and around the path is not part of
// either; what the path means is gitprov.PointerTarget's.
func readGitdirPointer(path, base string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	rest, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return "", false
	}
	target, ok := gitprov.PointerTarget(strings.TrimSpace(rest), "", base)
	if !ok {
		return "", false
	}
	target = filepath.Clean(target)
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		return "", false
	}
	return target, true
}

// resolveHead reads gitDir/HEAD and resolves it to a full object id,
// without running git: a detached HEAD already names one directly; a
// symbolic ref ("ref: refs/heads/<name>") is resolved first against a loose
// ref file under gitDir, then against gitDir/packed-refs. Anything missing,
// unreadable, or not a full 40- or 64-hex id yields "" rather than an
// error.
func resolveHead(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if hexObjectID.MatchString(line) {
		return line
	}
	ref, found := strings.CutPrefix(line, "ref:")
	if !found {
		return ""
	}
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "..") || filepath.IsAbs(ref) {
		return ""
	}

	// A linked worktree's gitdir holds its own HEAD but shares refs with the
	// main repository, named by the commondir file; look in both.
	for _, dir := range refDirs(gitDir) {
		if id, ok := lookupRef(dir, ref); ok {
			return id
		}
	}
	return ""
}

// refDirs returns gitDir followed by its common directory when gitDir has a
// commondir file (as `git worktree add` creates), relative paths resolved
// against gitDir. A git directory that is its own common directory yields just
// itself.
func refDirs(gitDir string) []string {
	dirs := []string{gitDir}
	if common := commonDir(gitDir); common != gitDir {
		dirs = append(dirs, common)
	}
	return dirs
}

// commonDir returns the git common directory of gitDir, the directory that
// holds what all worktrees of a repository share: the one gitDir's commondir
// file names (a relative path is resolved against gitDir, and the result is
// cleaned), as `git worktree add` writes for a linked worktree. A git
// directory with no commondir file, or an empty one, is its own common
// directory, so commonDir returns gitDir itself, as `git rev-parse
// --git-common-dir` does.
func commonDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}
	common := strings.TrimSpace(string(data))
	if common == "" {
		return gitDir
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return filepath.Clean(common)
}

// lookupRef resolves ref to an object id from dir's loose ref file or, when
// no loose file exists, from dir's packed-refs.
func lookupRef(dir, ref string) (string, bool) {
	if data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ref))); err == nil {
		candidate := strings.TrimSpace(string(data))
		return candidate, hexObjectID.MatchString(candidate)
	}
	packed, err := os.ReadFile(filepath.Join(dir, "packed-refs"))
	if err != nil {
		return "", false
	}
	for _, l := range strings.Split(string(packed), "\n") {
		if l == "" || l[0] == '#' || l[0] == '^' {
			continue
		}
		id, name, found := strings.Cut(l, " ")
		if found && name == ref && hexObjectID.MatchString(id) {
			return id, true
		}
	}
	return "", false
}
