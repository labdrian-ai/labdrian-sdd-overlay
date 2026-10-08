// Package fsresolve is the file system adapter of pathguard: it resolves the symlinks in the
// existing ancestry of a path against the real file system, and it binds pathguard's resolved
// containment proof to that resolver.
//
// pathguard owns the rules (what "within a root" means, and what a proof owes when its resolver
// fails or answers nothing); this package only answers the question the rules ask a resolver:
// what does this path name once every link in it is followed. The dependency points from the
// adapter to the domain, never back.
package fsresolve

import (
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
)

// KeepingMissing returns p with every symlink in its EXISTING ancestry
// resolved, keeping components that do not exist literal, and errors only when
// resolution genuinely failed (a symlink loop, a permission denial, a
// dangling symlink).
//
// Bare filepath.EvalSymlinks does not satisfy this contract: it fails on a
// missing final component, which would turn an ordinary absent target into a
// resolution failure instead of a "missing" verdict callers may owe their own
// caller. So this walks up to the deepest existing ancestor, resolves that,
// and re-appends the literal tail.
//
// It is a pathguard.Resolver, and ResolvedWithinRoot below uses it, so a write
// path's containment proof and a read-side proof can never resolve a path
// differently.
func KeepingMissing(p string) (string, error) {
	cur := filepath.Clean(p)
	var tail []string

	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if len(tail) == 0 {
				return resolved, nil
			}
			return filepath.Join(append([]string{resolved}, tail...)...), nil
		}
		if !os.IsNotExist(err) {
			// A genuine failure: a symlink loop, a permission denial, a
			// non-directory component. Never silently degraded into a
			// literal-tail answer.
			return "", err
		}
		// ENOENT is ambiguous: the component may not exist at all, or it may
		// exist as a symlink whose TARGET does not exist yet. Keeping a
		// dangling symlink literal un-follows it, and every containment proof
		// built on the result is then decided on a path that only looks
		// contained. Lstat does not follow the link, so it tells the two
		// cases apart.
		if fi, lerr := os.Lstat(cur); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			return "", err
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			// Walked to the topmost component without finding anything that
			// exists; there is nothing left to resolve against.
			//
			// Defence in depth, shadowed by the EvalSymlinks call at the top
			// of the loop: the topmost component is "/" for an absolute path
			// and "." for a relative one, and both always resolve, so this is
			// unreachable on any filesystem that has a root. It is kept
			// because it is the loop's termination proof: without it the walk
			// would spin forever rather than answer, and that is not a
			// failure mode worth trading for a covered line.
			return "", err
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// ResolvedWithinRoot is pathguard.ResolvedWithinRootUsing bound to the file
// system: the non-lexical half of a containment proof. It refuses a
// destination reached through a symlinked directory pointing outside the
// project root — the case a lexical-only guard admits, because such a path
// names no "..".
//
// It returns an error only when resolution genuinely failed; a destination
// that does not exist yet is the NORMAL state on a first registration and
// resolves fine, because the missing components stay literal.
//
// Callers owe themselves BOTH steps: the lexical guard (pathguard.WithinRoot)
// first, then this one. This check is also a point-in-time proof; a
// check-then-act writer must still re-establish containment at creation time,
// since a component can become a symlink in between.
func ResolvedWithinRoot(root, p string) (bool, error) {
	return pathguard.ResolvedWithinRootUsing(KeepingMissing, root, p)
}
