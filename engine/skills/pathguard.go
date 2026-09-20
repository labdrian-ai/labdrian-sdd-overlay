package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// withinRoot reports whether the cleaned absolute path p sits STRICTLY below
// the cleaned root: p equal to root is not within it. It is the single
// definition of lexical containment for this package — PlanInstall's R-055
// guard (install.go), the lexical guard in resolveTarget, the resolved-path
// check in EvaluateOwnership and resolvedWithinRoot below all call it, so no
// two of them can disagree about what containment means.
//
// Both arguments must already be cleaned; feeding it a raw root changes the
// semantics.
//
// Extraction note (3b-i.2/3b-i.3): PlanInstall's inline check was
// prefix-on-path-plus-separator only, which ADMITTED the path equal to the
// root (an entry whose id or path cleans to "."). This helper is
// strictly-below, so the extraction deliberately tightens that one case. It
// is a tightening, not a no-op, and it is recorded here because
// install_test.go never exercised the equality case and therefore cannot
// witness it.
func withinRoot(cleanRoot, p string) bool {
	return strings.HasPrefix(p+string(filepath.Separator), cleanRoot+string(filepath.Separator)) && p != cleanRoot
}

// resolvePathKeepingMissing returns p with every symlink in its EXISTING
// ancestry resolved, keeping components that do not exist literal, and errors
// only when resolution genuinely failed (a symlink loop, a permission
// denial).
//
// It is the implementation of the resolvePath contract EvaluateOwnership
// documents and tasks.md 3b-i.5b requires. Bare filepath.EvalSymlinks does
// not satisfy that contract: it fails on a missing final component, which
// would turn an ordinary absent target into a resolution failure instead of
// the "missing <path>" verdict ownership owes its caller. So this walks up to
// the deepest existing ancestor, resolves that, and re-appends the literal
// tail.
//
// resolvedWithinRoot below uses this same implementation, so the write path's
// containment proof and ownership's read-side proof can never resolve a path
// differently (tasks.md 3b-i.5b: "share one implementation").
func resolvePathKeepingMissing(p string) (string, error) {
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

		parent := filepath.Dir(cur)
		if parent == cur {
			// Walked to the filesystem root without finding anything that
			// exists; there is nothing left to resolve against.
			return "", err
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// resolvedWithinRoot is the non-lexical half of the write-path containment
// proof: it resolves both root and p through resolvePathKeepingMissing and
// re-applies withinRoot between the RESOLVED paths. It refuses a destination
// reached through a symlinked `.claude` or `.agents` pointing outside the
// project root — the case a lexical-only guard admits, because such a path
// names no ".." (review-slice-3a-ii-round-2, SEC-2).
//
// It returns an error only when resolution genuinely failed; a destination
// that does not exist yet is the NORMAL state on a first registration and
// resolves fine, because the missing components stay literal.
//
// Callers owe themselves BOTH steps: the lexical guard (resolveTarget or
// withinRoot) first, then this one. This check is also a point-in-time proof;
// a check-then-act writer must still re-establish containment at creation
// time, since a component can become a symlink in between.
func resolvedWithinRoot(root, p string) (bool, error) {
	return resolvedWithinRootUsing(resolvePathKeepingMissing, root, p)
}

// resolvedWithinRootUsing is resolvedWithinRoot with the resolver injected,
// for callers that must perform no filesystem access of their own —
// PlanProjectRegister, which is pure by contract, and any test that needs to
// control every path the guard sees. resolvedWithinRoot is the production
// binding of the same single implementation.
func resolvedWithinRootUsing(resolve func(string) (string, error), root, p string) (bool, error) {
	resolvedRoot, err := resolve(root)
	if err != nil {
		return false, err
	}
	resolvedPath, err := resolve(p)
	if err != nil {
		return false, err
	}
	return withinRoot(filepath.Clean(resolvedRoot), filepath.Clean(resolvedPath)), nil
}
