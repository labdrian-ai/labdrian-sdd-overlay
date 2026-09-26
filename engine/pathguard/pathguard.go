// Package pathguard provides shared filesystem-containment primitives: a
// strict lexical "below root" check and a symlink-aware resolver used to
// prove containment against the resolved filesystem, not just the literal
// path text.
package pathguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WithinRoot reports whether the cleaned absolute path p sits STRICTLY below
// the cleaned root: p equal to root is not within it. It is the single
// definition of lexical containment for callers that need it.
//
// It fails closed on its own precondition: an empty argument, or one that is
// not already cleaned (filepath.Clean(x) != x), is refused (false) rather than
// trusted, so a caller that forgets to clean cannot turn "/a/../etc" into a
// child of "/a" or an empty root into a container of every absolute path. A
// filesystem root ("/") is still never a containing root; that existing
// fail-closed behaviour is deliberately unchanged.
func WithinRoot(cleanRoot, p string) bool {
	if cleanRoot == "" || p == "" || filepath.Clean(cleanRoot) != cleanRoot || filepath.Clean(p) != p {
		return false
	}
	return strings.HasPrefix(p+string(filepath.Separator), cleanRoot+string(filepath.Separator)) && p != cleanRoot
}

// ResolvePathKeepingMissing returns p with every symlink in its EXISTING
// ancestry resolved, keeping components that do not exist literal, and errors
// only when resolution genuinely failed (a symlink loop, a permission
// denial).
//
// Bare filepath.EvalSymlinks does not satisfy this contract: it fails on a
// missing final component, which would turn an ordinary absent target into a
// resolution failure instead of a "missing" verdict callers may owe their own
// caller. So this walks up to the deepest existing ancestor, resolves that,
// and re-appends the literal tail.
//
// ResolvedWithinRoot below uses this same implementation, so a write path's
// containment proof and a read-side proof can never resolve a path
// differently.
func ResolvePathKeepingMissing(p string) (string, error) {
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

// ResolvedWithinRoot is the non-lexical half of a containment proof: it
// resolves both root and p through ResolvePathKeepingMissing and re-applies
// WithinRoot between the RESOLVED paths. It refuses a destination reached
// through a symlinked directory pointing outside the project root — the case
// a lexical-only guard admits, because such a path names no "..".
//
// It returns an error only when resolution genuinely failed; a destination
// that does not exist yet is the NORMAL state on a first registration and
// resolves fine, because the missing components stay literal.
//
// Callers owe themselves BOTH steps: the lexical guard (WithinRoot) first,
// then this one. This check is also a point-in-time proof; a check-then-act
// writer must still re-establish containment at creation time, since a
// component can become a symlink in between.
func ResolvedWithinRoot(root, p string) (bool, error) {
	return ResolvedWithinRootUsing(ResolvePathKeepingMissing, root, p)
}

// ResolvedWithinRootUsing is ResolvedWithinRoot with the resolver injected,
// for callers that must perform no filesystem access of their own, and any
// test that needs to control every path the guard sees. ResolvedWithinRoot is
// the production binding of the same single implementation.
//
// Both failure modes here are refused explicitly rather than allowed to fall
// through to WithinRoot: a root that resolves to nothing — because the
// resolver errored and its zero value was used, or because it handed back an
// empty string with no error at all — yields an error, not a verdict.
// WithinRoot itself refuses an empty root, so these checks are defence in
// depth: they no longer stand alone between the caller and a fail-open
// containment answer, but they keep the failure visible as an error instead
// of a bare false.
func ResolvedWithinRootUsing(resolve func(string) (string, error), root, p string) (bool, error) {
	resolvedRoot, err := resolve(root)
	if err != nil {
		return false, err
	}
	resolvedPath, err := resolve(p)
	if err != nil {
		return false, err
	}
	if resolvedRoot == "" || resolvedPath == "" {
		return false, fmt.Errorf("resolver returned an empty path for root %q / path %q", root, p)
	}
	return WithinRoot(filepath.Clean(resolvedRoot), filepath.Clean(resolvedPath)), nil
}
