package skills

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"

// withinRoot reports whether the cleaned absolute path p sits STRICTLY below
// the cleaned root: p equal to root is not within it. It is the single
// definition of lexical containment for this package — PlanInstall's R-055
// guard (install.go), the lexical guard in resolveTarget, the resolved-path
// check in EvaluateOwnership and resolvedWithinRoot below all call it, so no
// two of them can disagree about what containment means.
//
// An empty or uncleaned argument is refused (false) rather than trusted; see
// pathguard.WithinRoot.
//
// The algorithm lives in the shared package engine/pathguard
// (pathguard.WithinRoot) so containment is never implemented twice; this is a
// thin wrapper kept so every existing call site in this package compiles
// unchanged.
func withinRoot(cleanRoot, p string) bool {
	return pathguard.WithinRoot(cleanRoot, p)
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
//
// The algorithm lives in engine/pathguard (pathguard.ResolvePathKeepingMissing);
// this is a thin wrapper kept so every existing call site in this package
// compiles unchanged.
func resolvePathKeepingMissing(p string) (string, error) {
	return pathguard.ResolvePathKeepingMissing(p)
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
//
// The algorithm lives in engine/pathguard (pathguard.ResolvedWithinRoot); this
// is a thin wrapper kept so every existing call site in this package compiles
// unchanged.
func resolvedWithinRoot(root, p string) (bool, error) {
	return pathguard.ResolvedWithinRoot(root, p)
}

// resolvedWithinRootUsing is resolvedWithinRoot with the resolver injected,
// for callers that must perform no filesystem access of their own —
// PlanProjectRegister, which is pure by contract, and any test that needs to
// control every path the guard sees. resolvedWithinRoot is the production
// binding of the same single implementation.
//
// Both failure modes here are refused explicitly rather than allowed to fall
// through to withinRoot: a root that resolves to nothing — because the
// resolver errored and its zero value was used, or because it handed back an
// empty string with no error at all — yields an error, not a verdict (review
// round 2, COV-4). withinRoot("", p) used to be true for EVERY absolute path;
// it now refuses an empty root itself, so these checks are defence in depth
// that keep the failure visible as an error instead of a bare false.
//
// The algorithm lives in engine/pathguard (pathguard.ResolvedWithinRootUsing);
// this is a thin wrapper kept so every existing call site in this package
// compiles unchanged.
func resolvedWithinRootUsing(resolve func(string) (string, error), root, p string) (bool, error) {
	return pathguard.ResolvedWithinRootUsing(resolve, root, p)
}
