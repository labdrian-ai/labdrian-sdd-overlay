// Package app holds the use cases of the TUI: what it does when the operator
// asks for something, over the ports the domain owns. It imports the domain and
// the pure standard library, never a process, a file or a terminal.
package app

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// ActionRunner runs an action the operator chose, against the targets the
// operator selected, through the Backend port.
type ActionRunner struct {
	backend domain.Backend
	catalog domain.TargetCatalog
}

// NewActionRunner returns the runner over the backend that carries out the
// invocations and the catalog that says what `--target all` means right now.
// They are two ports because they answer two questions, though one adapter
// answers both.
func NewActionRunner(backend domain.Backend, catalog domain.TargetCatalog) ActionRunner {
	return ActionRunner{backend: backend, catalog: catalog}
}

// Run executes the action for the selected targets. shown is the catalog the
// operator was shown, in the backend's order: it is what `--target all` is
// allowed to stand for.
//
// When the action supports `all` and the selection is the whole catalog the
// operator was shown (TargetScope.CoversAll), it issues a single `--target all`
// invocation, after confirming the backend still lists that same catalog.
// Otherwise it iterates per target (this is required for `capture`, which
// rejects `--target all`, and it is what keeps a partial selection from ever
// widening into `all`). For TargetAgnostic actions (hooks lifecycle), no
// --target is emitted at all. Nested Also actions contribute their own
// invocations, routed and appended after the primary action's.
//
// The result carries a text transcript of the invocations (the "$ ..." line of
// each, what it printed, and how it failed) because that is what the screen
// shows; the structured form of an invocation and its outcome is the next
// slice of T3.
func (r ActionRunner) Run(action domain.Action, selected, shown []domain.Target) domain.CommandResult {
	res := domain.CommandResult{Action: action, Targets: selected}
	scope := domain.TargetScope{Shown: shown, Catalog: r.catalog}

	argSets := domain.AllArgSets(action, selected, scope.CoversAll(selected))

	var sb strings.Builder
	severity := 0
	// Aggregate by worst severity seen across all invocations in this
	// CommandResult, not "last failing invocation wins": a later degraded
	// (exit 2) invocation must never mask an earlier hard failure (or vice
	// versa) once the Also merge concatenates invocations from different
	// backend subcommands into this loop.
	fail := func(err error, exitCode int) {
		if sev := domain.InvocationSeverity(err, exitCode); sev > severity {
			severity = sev
			res.Err = err
			res.ExitCode = exitCode
		}
	}
	for i, args := range argSets {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("$ bin/labdrian-overlay %s\n", strings.Join(args, " ")))

		// `all` is defined by the backend when it runs, so verify it still
		// means what the operator saw; if it cannot be verified, do not run.
		if domain.UsesTargetAll(args) {
			if err := scope.ConfirmUnchanged(); err != nil {
				sb.WriteString(fmt.Sprintf("[omitido: %v]\n", err))
				fail(err, -1)
				continue
			}
		}

		run := r.backend.Run(args)
		sb.WriteString(run.Output)
		if run.Err != nil {
			sb.WriteString(fmt.Sprintf("\n[exit error: %v]\n", run.Err))
			// The exit code lets callers tell a hard failure (exit 1) from a
			// degraded/warning result (exit 2).
			fail(run.Err, run.ExitCode)
		}
	}

	res.Output = sb.String()
	if action.ResultKind == domain.ResultSyncDashboard {
		res.Verdicts = domain.ParseSyncCheck(res.Output)
	}
	return res
}
