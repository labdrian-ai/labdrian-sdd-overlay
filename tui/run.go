package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// targetAll is the backend's `--target` value that stands for every target in
// its catalog. buildArgSets is the only producer and usesTargetAll the only
// reader, so the TUI has one definition of what "an all invocation" is.
const targetAll = "all"

// probeBehindOriginCmd returns a tea.Cmd that runs a cached-only sync-check
// (no --fetch/--check-origin, per R-001) against root and delivers a
// probeDoneMsg. bubbletea runs the returned func() tea.Msg off the UI
// goroutine, so this never blocks Init()'s first render. CombinedOutput is
// fed to probeBehind/probeBehindRelease even when the process exits
// non-zero — the probe reads cached refs, it does not require a clean exit.
// One sync-check invocation feeds both fields (D2): no second exec is
// spent just to pick up the release-behind count.
func probeBehindOriginCmd(root string) tea.Cmd {
	return func() tea.Msg {
		bin := filepath.Join(root, "bin", "labdrian-overlay")
		cmd := exec.Command(bin, "sync-check")
		cmd.Dir = root
		out, _ := cmd.CombinedOutput()
		output := string(out)
		return probeDoneMsg{
			behind:        probeBehind(output),
			behindRelease: probeBehindRelease(output),
		}
	}
}

// RepoRoot resolves the overlay repo root.
//
// Resolution order:
//  1. OVERLAY_DIR env var (set by the bash wrapper when launching the TUI)
//  2. walk up from the executable's directory looking for bin/labdrian-overlay
//  3. walk up from the current working directory looking for bin/labdrian-overlay
func RepoRoot() (string, error) {
	if dir := os.Getenv("OVERLAY_DIR"); dir != "" {
		if hasBackend(dir) {
			return dir, nil
		}
	}

	if exe, err := os.Executable(); err == nil {
		if root, ok := walkUpForBackend(filepath.Dir(exe)); ok {
			return root, nil
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if root, ok := walkUpForBackend(wd); ok {
			return root, nil
		}
	}

	return "", fmt.Errorf("could not locate overlay repo root (bin/labdrian-overlay not found); set OVERLAY_DIR")
}

func hasBackend(root string) bool {
	info, err := os.Stat(filepath.Join(root, "bin", "labdrian-overlay"))
	return err == nil && !info.IsDir()
}

func walkUpForBackend(start string) (string, bool) {
	dir := start
	for {
		if hasBackend(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// commandResult is the outcome of running the backend for one or more targets.
type commandResult struct {
	action   Action
	targets  []Target
	output   string // combined stdout+stderr across invocations
	verdicts []TargetVerdict
	err      error
	// exitCode captures the process exit code when err is non-nil. It
	// distinguishes a hard failure (code 1) from a degraded/warning result
	// (code 2, e.g. 'engine status' DEGRADED). Zero when err is nil.
	exitCode int
}

// prefix returns the invariant head of every invocation this action produces:
// its subcommand followed by its positional Args.
//
// It allocates a new slice per call, which is what makes appending --target
// to the result safe -- but note that this is a property of the CALL SITE,
// not of the exact capacity requested: buildArgSets calls prefix inside its
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

// buildArgSets constructs the argument sets to pass to the backend binary.
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
//     was shown" (targetScope.coversAll), never a count compared with a list
//     kept here: `all` is whatever the backend says it is.
//  3. Default: one invocation per selected target.
func buildArgSets(action Action, selected []Target, allSelected bool) [][]string {
	switch {
	case action.TargetAgnostic:
		return [][]string{prefix(action)}
	case action.SupportsAll && allSelected:
		return [][]string{append(prefix(action), "--target", targetAll)}
	default:
		var sets [][]string
		for _, t := range selected {
			sets = append(sets, append(prefix(action), "--target", t.Name))
		}
		return sets
	}
}

// allArgSets flattens the primary action's arg sets with those of every
// nested Also action, each routed through buildArgSets independently.
func allArgSets(action Action, selected []Target, allSelected bool) [][]string {
	sets := buildArgSets(action, selected, allSelected)
	for _, sub := range action.Also {
		sets = append(sets, buildArgSets(sub, selected, allSelected)...)
	}
	return sets
}

// usesTargetAll reports whether args is an `--target all` invocation, the one
// kind runBackend verifies against the backend's catalog before running.
func usesTargetAll(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--target" && args[i+1] == targetAll {
			return true
		}
	}
	return false
}

// invocationSeverity ranks a single invocation's outcome so runBackend can
// aggregate multiple invocations (primary action plus any merged Also
// actions, which may be structurally different backend subcommands with
// different exit-code contracts) by worst outcome rather than by which one
// ran last. Exit code 2 ("degraded") ranks below any other non-nil error
// ("hard failure"); a nil err ranks lowest of all.
func invocationSeverity(err error, exitCode int) int {
	switch {
	case err == nil:
		return 0
	case exitCode == 2:
		return 1 // degraded
	default:
		return 2 // hard failure
	}
}

// runBackend executes the backend action for the selected targets.
//
// When the action supports `all` and the selection is the whole catalog the
// operator was shown (scope.coversAll), it issues a single `--target all`
// invocation, after confirming the backend still lists that same catalog.
// Otherwise it iterates per target (this is required for `capture`, which
// rejects `--target all`, and it is what keeps a partial selection from ever
// widening into `all`). For TargetAgnostic actions (hooks lifecycle), no
// --target is emitted at all. Nested Also actions contribute their own
// invocations, routed and appended after the primary action's.
func runBackend(root string, action Action, selected []Target, scope targetScope) commandResult {
	res := commandResult{action: action, targets: selected}
	bin := filepath.Join(root, "bin", "labdrian-overlay")

	argSets := allArgSets(action, selected, scope.coversAll(selected))

	var sb strings.Builder
	severity := 0
	// Aggregate by worst severity seen across all invocations in this
	// commandResult, not "last failing invocation wins": a later degraded
	// (exit 2) invocation must never mask an earlier hard failure (or vice
	// versa) once the Also merge concatenates invocations from different
	// backend subcommands into this loop.
	fail := func(err error, exitCode int) {
		if sev := invocationSeverity(err, exitCode); sev > severity {
			severity = sev
			res.err = err
			res.exitCode = exitCode
		}
	}
	for i, args := range argSets {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("$ bin/labdrian-overlay %s\n", strings.Join(args, " ")))

		// `all` is defined by the backend when it runs, so verify it still
		// means what the operator saw; if it cannot be verified, do not run.
		if usesTargetAll(args) {
			if err := scope.confirmUnchanged(); err != nil {
				sb.WriteString(fmt.Sprintf("[omitido: %v]\n", err))
				fail(err, -1)
				continue
			}
		}

		cmd := exec.Command(bin, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		sb.Write(out)
		if err != nil {
			sb.WriteString(fmt.Sprintf("\n[exit error: %v]\n", err))
			// Extract the exit code so callers can distinguish a hard failure
			// (exit 1) from a degraded/warning result (exit 2).
			exitCode := -1
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			}
			fail(err, exitCode)
		}
	}

	res.output = sb.String()
	if action.Command == "sync-check" {
		res.verdicts = ParseSyncCheck(res.output)
	}
	return res
}
