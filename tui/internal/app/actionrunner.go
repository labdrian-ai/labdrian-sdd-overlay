// Package app holds the use cases of the TUI: what it does when the operator
// asks for something, over the ports the domain owns. It imports the domain and
// the pure standard library, never a process, a file or a terminal.
package app

import (
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
// The result records each invocation as a value, in order, with what the
// backend left behind or why the call was refused; the screen renders the
// transcript from them. The result's failure is the worst of the invocations',
// and an action that declares its result a sync-check report carries the
// verdicts that the report lines of all its executions add up to, read as one
// report.
func (r ActionRunner) Run(action domain.Action, selected, shown []domain.Target) domain.CommandResult {
	res := domain.CommandResult{Action: action, Targets: selected}
	scope := domain.TargetScope{Shown: shown, Catalog: r.catalog}

	argSets := domain.AllArgSets(action, selected, scope.CoversAll(selected))

	// An action that declares its result a sync-check report shows one report
	// for all of its invocations, in the order they ran; any other action shows
	// its transcript only, whatever its output looked like.
	reported := action.ResultKind == domain.ResultSyncDashboard
	var report domain.SyncReport

	severity := 0
	for _, args := range argSets {
		inv := r.invoke(scope, args)
		res.Invocations = append(res.Invocations, inv)
		if reported {
			report.Add(inv.Run.Report)
		}

		// Aggregate by worst severity seen across all invocations in this
		// CommandResult, not "last failing invocation wins": a later degraded
		// (exit 2) invocation must never mask an earlier hard failure (or vice
		// versa) once the Also merge concatenates invocations from different
		// backend subcommands into this loop. The exit code lets callers tell a
		// hard failure (exit 1) from a degraded/warning result (exit 2).
		if sev := domain.InvocationSeverity(inv.Err(), inv.ExitCode()); sev > severity {
			severity = sev
			res.Err = inv.Err()
			res.ExitCode = inv.ExitCode()
		}
	}
	if reported {
		res.Verdicts = report.Verdicts()
	}
	return res
}

// invoke makes one invocation: it runs args through the backend, unless it is a
// `--target all` that cannot be verified. `all` is defined by the backend when
// it runs, so verify it still means what the operator saw; if it cannot be
// verified, do not run, and say why.
func (r ActionRunner) invoke(scope domain.TargetScope, args []string) domain.Invocation {
	if domain.UsesTargetAll(args) {
		if err := scope.ConfirmUnchanged(); err != nil {
			return domain.Invocation{Args: args, Refusal: err}
		}
	}
	return domain.Invocation{Args: args, Run: r.backend.Run(args)}
}
