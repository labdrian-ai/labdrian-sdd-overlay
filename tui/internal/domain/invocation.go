package domain

// TargetAll is the backend's `--target` value that stands for every target in
// its catalog. BuildArgSets is the only producer and UsesTargetAll the only
// reader, so the TUI has one definition of what "an all invocation" is.
const TargetAll = "all"

// prefix returns the invariant head of every invocation this action produces:
// its subcommand followed by its positional Args.
//
// It allocates a new slice per call, which is what makes appending --target
// to the result safe -- but note that this is a property of the CALL SITE,
// not of the exact capacity requested: BuildArgSets calls prefix inside its
// per-target loop, so each target already gets its own array whatever
// capacity is asked for. An earlier version of this comment claimed the
// zero-spare-capacity sizing was itself load-bearing against one target
// overwriting another's flag. It is not, nothing observes capacity, and no
// test could have caught the claim being wrong -- which is exactly the kind
// of confident-but-unverifiable record this repository has spent an audit
// removing. Reuse prefix per target; do not hoist the call out of the loop.
func prefix(action Action) []string {
	out := make([]string, 0, len(action.Args)+1)
	out = append(out, action.Command)
	return append(out, action.Args...)
}

// BuildArgSets constructs the argument sets to pass to the backend binary.
//
// Positional Args survive every route, not just the TargetAgnostic one: a
// subcommand like `longterm-mem status` is positional and is validated by the
// backend BEFORE it parses --target, so dropping it on the target-using routes
// would send the backend a bare `longterm-mem --target all` that it refuses.
//
// Routing priority:
//  1. TargetAgnostic: single invocation with NO --target. The three hooks
//     subcommands (status-hooks, install-hooks, uninstall-hooks) operate on
//     ~/.claude/settings.json globally and do not accept --target. Verified
//     against bin/labdrian-overlay: cmd_status_hooks, cmd_install_hooks, and
//     cmd_uninstall_hooks receive "$@" from the dispatcher but parse no
//     arguments; extra flags are silently ignored. We omit --target anyway
//     because it is semantically incorrect and future-proofs against stricter
//     argument parsing.
//  2. SupportsAll + allSelected: single `--target all` invocation.
//     allSelected must mean "the selection is the whole catalog the operator
//     was shown" (TargetScope.CoversAll), never a count compared with a list
//     kept here: `all` is whatever the backend says it is.
//  3. Default: one invocation per selected target.
func BuildArgSets(action Action, selected []Target, allSelected bool) [][]string {
	switch {
	case action.TargetAgnostic:
		return [][]string{prefix(action)}
	case action.SupportsAll && allSelected:
		return [][]string{append(prefix(action), "--target", TargetAll)}
	default:
		var sets [][]string
		for _, t := range selected {
			sets = append(sets, append(prefix(action), "--target", t.Name))
		}
		return sets
	}
}

// AllArgSets flattens the primary action's arg sets with those of every
// nested Also action, each routed through BuildArgSets independently.
func AllArgSets(action Action, selected []Target, allSelected bool) [][]string {
	sets := BuildArgSets(action, selected, allSelected)
	for _, sub := range action.Also {
		sets = append(sets, BuildArgSets(sub, selected, allSelected)...)
	}
	return sets
}

// UsesTargetAll reports whether args is an `--target all` invocation, the one
// kind the action runner verifies against the backend's catalog before running.
func UsesTargetAll(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--target" && args[i+1] == TargetAll {
			return true
		}
	}
	return false
}

// InvocationSeverity ranks a single invocation's outcome so the action runner can
// aggregate multiple invocations (primary action plus any merged Also
// actions, which may be structurally different backend subcommands with
// different exit-code contracts) by worst outcome rather than by which one
// ran last. Exit code 2 ("degraded") ranks below any other non-nil error
// ("hard failure"); a nil err ranks lowest of all.
func InvocationSeverity(err error, exitCode int) int {
	switch {
	case err == nil:
		return 0
	case exitCode == 2:
		return 1 // degraded
	default:
		return 2 // hard failure
	}
}
