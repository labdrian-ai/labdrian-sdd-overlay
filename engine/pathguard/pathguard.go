// Package pathguard is the domain of path containment: a strict lexical "below
// root" check, and the proof that a path is below a root once both are
// resolved, so that containment is decided on what a path names and not on the
// literal text of it.
//
// It touches no file system. The resolution of links is a port, Resolver, that
// the caller supplies; pathguard/fsresolve is the adapter that answers it
// against a real one.
package pathguard

import (
	"fmt"
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

// Resolver is the port a resolved containment proof asks the file system through:
// it returns what path names once every link in it is followed, or an error
// when that cannot be told. The domain owns the question; an adapter
// (pathguard/fsresolve) answers it against a real file system.
type Resolver func(path string) (string, error)

// ResolvedWithinRootUsing is the non-lexical half of a containment proof: it
// resolves both root and p through the resolver and re-applies WithinRoot
// between the RESOLVED paths. With a resolver that follows links it refuses a
// destination reached through a symlinked directory pointing outside the
// root, the case a lexical-only guard admits because such a path names no
// "..". The resolver is injected so the caller performs no file system access
// of its own, and a test controls every path the guard sees;
// fsresolve.ResolvedWithinRoot is the production binding.
//
// Callers owe themselves BOTH steps: the lexical guard (WithinRoot) first,
// then this one. This check is also a point-in-time proof; a check-then-act
// writer must still re-establish containment at creation time, since a
// component can become a symlink in between.
//
// Both failure modes here are refused explicitly rather than allowed to fall
// through to WithinRoot: a root that resolves to nothing — because the
// resolver errored and its zero value was used, or because it handed back an
// empty string with no error at all — yields an error, not a verdict.
// WithinRoot itself refuses an empty root, so these checks are defence in
// depth: they no longer stand alone between the caller and a fail-open
// containment answer, but they keep the failure visible as an error instead
// of a bare false.
func ResolvedWithinRootUsing(resolve Resolver, root, p string) (bool, error) {
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
