package main

// observeProvenance best-effort discovers the workflow.Provenance for a
// working directory by walking up the filesystem looking for a .git entry
// and reading HEAD, without running the git binary or any other
// subprocess. Every step fails soft: anything missing, unreadable, or
// unexpected leaves the corresponding field empty rather than failing the
// caller's operation, matching Decision 6 (provenance is audit-only, never
// a lookup key, and never blocks a lifecycle operation).
//
// This deliberately duplicates none of engine/gitprov's logic: gitprov
// shells out to the real git binary (by design, to get git's own answer),
// which the workflow lifecycle commands must never do (no subprocesses).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// hexObjectID matches a full SHA-1 (40 hex) or SHA-256 (64 hex) git object
// id, the same shape workflow.Provenance.GitHead requires.
var hexObjectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// observeProvenance returns the best-effort Provenance for cwd. cwd should
// be absolute (os.Getwd()'s result); a relative or empty cwd yields an
// empty Provenance, since WorktreeRoot must be absolute or empty.
func observeProvenance(cwd string) workflow.Provenance {
	if cwd == "" || !filepath.IsAbs(cwd) {
		return workflow.Provenance{}
	}
	root, gitDir, ok := findGitDir(filepath.Clean(cwd))
	if !ok {
		return workflow.Provenance{}
	}
	return workflow.Provenance{WorktreeRoot: root, GitHead: resolveHead(gitDir)}
}

// findGitDir walks up from start looking for a .git entry: a directory (a
// normal repository) or a regular file naming another directory (a linked
// worktree or submodule's gitdir pointer). It returns the directory that
// held .git (the worktree root observed, which for a linked worktree is
// this worktree's own root, not the main repository's) and the resolved
// git directory. ok is false when no .git is found before reaching the
// filesystem root, or when a .git file cannot be read and resolved.
func findGitDir(start string) (worktreeRoot, gitDir string, ok bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Lstat(candidate)
		if err == nil {
			switch {
			case info.IsDir():
				return dir, candidate, true
			case info.Mode().IsRegular():
				target, ok := readGitdirPointer(candidate, dir)
				if !ok {
					return "", "", false
				}
				return dir, target, true
			default:
				// A symlink or other special file named .git is not a shape
				// this helper trusts; fail soft rather than following it.
				return "", "", false
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// readGitdirPointer reads a ".git" file's single "gitdir: <path>" line and
// resolves it (relative to base when not already absolute) to an existing
// directory.
func readGitdirPointer(path, base string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	target, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
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

	if data, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(ref))); err == nil {
		candidate := strings.TrimSpace(string(data))
		if hexObjectID.MatchString(candidate) {
			return candidate
		}
		return ""
	}

	packed, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(packed), "\n") {
		if l == "" || l[0] == '#' || l[0] == '^' {
			continue
		}
		id, name, found := strings.Cut(l, " ")
		if !found || name != ref {
			continue
		}
		if hexObjectID.MatchString(id) {
			return id
		}
	}
	return ""
}
