package app

// The invariant behind T1: the TUI never acts on a target the operator did
// not see. `--target all` is the one argument that can name more than the
// selection, so it is sent only when the selection is the whole catalog the
// operator was shown, and only while the backend still answers with that same
// catalog. The runner is tested over a backend that records what it receives.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// The helper that stands for a backend that failed must say so truthfully for
// any exit status, since the tests that rank outcomes read the code it gives.
func TestFailing_SaysTheExitStatusItWasGiven(t *testing.T) {
	for _, code := range []int{1, 2, 9, 10, 42, 127, 255} {
		got := failing("output", code)
		if want := fmt.Sprintf("exit status %d", code); got.Err == nil || got.Err.Error() != want {
			t.Errorf("failing(_, %d).Err = %v, want %q", code, got.Err, want)
		}
		if got.ExitCode != code || got.Output != "output" {
			t.Errorf("failing(%q, %d) = %+v, want that output and that code", "output", code, got)
		}
	}
}

func statusAction() domain.Action {
	return domain.Action{Name: "Estado", Command: "status", SupportsAll: true}
}

// run has the runner carry out action over selected, with shown as the
// catalog the operator saw and catalog as what the backend lists now.
func run(action domain.Action, selected, shown []domain.Target, catalog *fakeCatalog, backend *fakeBackend) domain.CommandResult {
	return NewActionRunner(backend, catalog).Run(action, selected, shown)
}

func TestActionRunner_AllStandsForTheSelectionOnlyWhenItIsTheWholeCatalog(t *testing.T) {
	t.Run("every target of the catalog selected sends all", func(t *testing.T) {
		shown := fourTargets()
		backend := &fakeBackend{}
		run(statusAction(), shown, shown, &fakeCatalog{targets: shown}, backend)
		if got := strings.Join(backend.ran, "|"); got != "status --target all" {
			t.Errorf("invocations = %q, want exactly [status --target all]", got)
		}
	})

	t.Run("the three targets the TUI used to call all no longer do", func(t *testing.T) {
		// This is the bug: a four-target catalog, three of them selected,
		// used to be sent as `--target all`, which also acts on pi.
		shown := fourTargets()
		catalog := &fakeCatalog{targets: shown}
		backend := &fakeBackend{}
		run(statusAction(), threeCopyTargets(), shown, catalog, backend)
		want := "status --target claude|status --target opencode|status --target codex"
		if got := strings.Join(backend.ran, "|"); got != want {
			t.Errorf("invocations = %q, want one explicit call per selected target %q", got, want)
		}
		if catalog.asked != 0 {
			t.Errorf("explicit per-target calls asked the catalog %d times, want 0: only `all` needs re-checking", catalog.asked)
		}
	})

	t.Run("a one-target catalog fully selected is all", func(t *testing.T) {
		shown := []domain.Target{{Name: "claude", Kind: domain.KindCopy}}
		backend := &fakeBackend{}
		run(statusAction(), shown, shown, &fakeCatalog{targets: shown}, backend)
		if got := strings.Join(backend.ran, "|"); got != "status --target all" {
			t.Errorf("invocations = %q, want [status --target all]", got)
		}
	})

	t.Run("an empty selection of an empty catalog is not all", func(t *testing.T) {
		// Two empty sets are equal, and `all` would then act on every target
		// the backend has. Nothing selected means nothing runs.
		backend := &fakeBackend{}
		res := run(statusAction(), nil, nil, &fakeCatalog{}, backend)
		if len(backend.ran) != 0 || len(res.Invocations) != 0 {
			t.Errorf("invocations = %q, recorded %+v, want none", backend.ran, res.Invocations)
		}
	})

	t.Run("a non-all action never sends all, even for the whole catalog", func(t *testing.T) {
		shown := fourTargets()
		capture := domain.Action{Name: "Capturar", Command: "capture", SupportsAll: false}
		backend := &fakeBackend{}
		run(capture, shown, shown, &fakeCatalog{targets: shown}, backend)
		for _, args := range backend.ran {
			if strings.Contains(args, "--target all") {
				t.Errorf("capture was sent %q; the backend refuses --target all for it", args)
			}
		}
		if len(backend.ran) != 4 {
			t.Errorf("invocations = %q, want one per target", backend.ran)
		}
	})
}

func TestActionRunner_AllIsRefusedUnlessTheBackendStillListsTheCatalogShown(t *testing.T) {
	shown := threeCopyTargets()

	t.Run("the backend now lists another target", func(t *testing.T) {
		// What an update that adds a target looks like from the TUI: it showed
		// three, the backend now has four, and `all` would reach the fourth.
		catalog := &fakeCatalog{targets: fourTargets()}
		backend := &fakeBackend{}
		res := run(statusAction(), shown, shown, catalog, backend)

		if len(backend.ran) != 0 {
			t.Errorf("the backend was invoked with %q; `--target all` must not be sent", backend.ran)
		}
		if res.Err == nil {
			t.Error("a refused invocation must fail the result, not pass as a success")
		}
		inv := onlyInvocation(t, res)
		if got := strings.Join(inv.Args, " "); got != "status --target all" {
			t.Errorf("the refused invocation = %q, want [status --target all]", got)
		}
		if inv.Refusal == nil || !strings.Contains(inv.Refusal.Error(), "catálogo") {
			t.Errorf("the refusal should say the catalog changed, got %v", inv.Refusal)
		}
		if inv.Run.Output != "" || inv.Run.Err != nil {
			t.Errorf("a refused invocation was never run, yet it carries %+v", inv.Run)
		}
		if catalog.asked != 1 {
			t.Errorf("the catalog was asked %d times, want once before the all invocation", catalog.asked)
		}
	})

	t.Run("the backend cannot be asked", func(t *testing.T) {
		backend := &fakeBackend{}
		res := run(statusAction(), shown, shown, &fakeCatalog{err: errors.New("catalog unreadable")}, backend)

		if len(backend.ran) != 0 {
			t.Errorf("the backend was invoked with %q; an unverified `all` must not be sent", backend.ran)
		}
		if refusal := onlyInvocation(t, res).Refusal; res.Err == nil || refusal == nil || !strings.Contains(refusal.Error(), "catalog unreadable") {
			t.Errorf("want the failure and its cause in the result, got err=%v refusal=%v", res.Err, refusal)
		}
	})

	t.Run("the backend lists one target fewer", func(t *testing.T) {
		backend := &fakeBackend{}
		res := run(statusAction(), shown, shown, &fakeCatalog{targets: shown[:2]}, backend)
		if len(backend.ran) != 0 || res.Err == nil {
			t.Errorf("invocations = %q, err = %v; a changed catalog must refuse `all`", backend.ran, res.Err)
		}
	})

	t.Run("the same set in another order is still the catalog shown", func(t *testing.T) {
		reordered := []domain.Target{shown[2], shown[0], shown[1]}
		backend := &fakeBackend{}
		res := run(statusAction(), shown, shown, &fakeCatalog{targets: reordered}, backend)
		if got := strings.Join(backend.ran, "|"); res.Err != nil || got != "status --target all" {
			t.Errorf("invocations = %q, err = %v; want [status --target all]", got, res.Err)
		}
	})

	t.Run("a refused all does not stop the invocations around it", func(t *testing.T) {
		// "Actualizar repositorio": self-update, then the chained apply. The
		// first runs; only the target-using second one is refused.
		chain := domain.Action{
			Name: "Actualizar repositorio", Command: "self-update", TargetAgnostic: true,
			Also: []domain.Action{{Command: "apply", SupportsAll: true}},
		}
		backend := &fakeBackend{}
		res := run(chain, shown, shown, &fakeCatalog{targets: fourTargets()}, backend)

		if got := strings.Join(backend.ran, "|"); got != "self-update" {
			t.Errorf("invocations = %q, want only [self-update]", got)
		}
		if res.Err == nil {
			t.Error("the refused apply must fail the result")
		}
		// The result tells both, in order: the one that ran, then the one that was not made.
		if len(res.Invocations) != 2 || res.Invocations[0].Refusal != nil || res.Invocations[1].Refusal == nil {
			t.Errorf("invocations = %+v, want self-update run and then the apply refused", res.Invocations)
		}
	})
}

// onlyInvocation is the one invocation a result records.
func onlyInvocation(t *testing.T, res domain.CommandResult) domain.Invocation {
	t.Helper()
	if len(res.Invocations) != 1 {
		t.Fatalf("the result records %d invocations, want exactly one: %+v", len(res.Invocations), res.Invocations)
	}
	return res.Invocations[0]
}

// A refused `all` is recorded as a failure with exit code -1: the invocation was
// never started, so there is no status of the backend's to carry. That is a hard
// failure, and it must stay the result when it meets a degraded run (exit 2) in
// either order: a refusal that a later degraded run could replace would turn
// "nothing was applied" into a warning.
func TestActionRunner_ARefusedAllIsAHardFailureThatOutranksADegradedRun(t *testing.T) {
	shown := threeCopyTargets()
	changed := &fakeCatalog{targets: fourTargets()} // what the backend lists now: another target, so `all` is refused

	t.Run("alone, its code is -1", func(t *testing.T) {
		res := run(statusAction(), shown, shown, changed, &fakeBackend{})
		if res.Err == nil || res.ExitCode != -1 {
			t.Errorf("result = (%v, exit %d), want a failure with exit -1", res.Err, res.ExitCode)
		}
	})

	chain := func(first, second domain.Action) domain.Action {
		first.Also = []domain.Action{second}
		return first
	}
	degraded := domain.Action{Command: "status-hooks", TargetAgnostic: true}
	refused := domain.Action{Command: "apply", SupportsAll: true}
	backend := func() *fakeBackend {
		return &fakeBackend{script: map[string]domain.Execution{"status-hooks": failing("engine: DEGRADED\n", 2)}}
	}

	t.Run("after a degraded run, the refusal is the result", func(t *testing.T) {
		b := backend()
		res := run(chain(degraded, refused), shown, shown, changed, b)
		if res.Err == nil || res.ExitCode != -1 {
			t.Errorf("result = (%v, exit %d), want the refusal (exit -1) to outrank the degraded run", res.Err, res.ExitCode)
		}
		if got := strings.Join(b.ran, "|"); got != "status-hooks" {
			t.Errorf("invocations = %q, want only the degraded one: the refused all is never sent", got)
		}
	})

	t.Run("before a degraded run, the refusal stays the result", func(t *testing.T) {
		res := run(chain(refused, degraded), shown, shown, changed, backend())
		if res.Err == nil || res.ExitCode != -1 {
			t.Errorf("result = (%v, exit %d), want the refusal (exit -1) not to be replaced by the degraded run", res.Err, res.ExitCode)
		}
	})
}

func TestActionRunner_RoutesEveryInvocationOfAnAction(t *testing.T) {
	shown := fourTargets()
	tests := []struct {
		name     string
		action   domain.Action
		selected []domain.Target
		want     string
	}{
		{"a target-agnostic action sends no target", domain.Action{Command: "install-hooks", TargetAgnostic: true}, shown, "install-hooks"},
		{"positional arguments survive the target-using routes", domain.Action{Command: "longterm-mem", Args: []string{"status"}, SupportsAll: true}, shown[:2],
			"longterm-mem status --target claude|longterm-mem status --target opencode"},
		{"the merged actions follow the primary one", domain.Action{Command: "status", SupportsAll: true,
			Also: []domain.Action{{Command: "status-hooks", TargetAgnostic: true}, {Command: "version", TargetAgnostic: true}}},
			shown[:1], "status --target claude|status-hooks|version"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeBackend{}
			run(tc.action, tc.selected, shown, &fakeCatalog{targets: shown}, backend)
			if got := strings.Join(backend.ran, "|"); got != tc.want {
				t.Errorf("invocations = %q, want %q", got, tc.want)
			}
		})
	}
}

// The result records each invocation as a value, in the order it was made: the
// arguments, and what the backend left behind (its output and how it failed).
// How that reads on the screen is the screen's, not the runner's: the result
// holds no text of its own.
func TestActionRunner_TheResultRecordsEachInvocationAndWhatItLeftBehind(t *testing.T) {
	shown := threeCopyTargets()
	backend := &fakeBackend{script: map[string]domain.Execution{
		"status": {Output: "claude: deployed\n"},
		"apply":  failing("boom\n", 1),
	}}
	action := domain.Action{Command: "status", Also: []domain.Action{{Command: "apply", TargetAgnostic: true}}}

	res := run(action, shown[:1], shown, &fakeCatalog{targets: shown}, backend)

	if len(res.Invocations) != 2 {
		t.Fatalf("the result records %d invocations, want 2: %+v", len(res.Invocations), res.Invocations)
	}
	first, second := res.Invocations[0], res.Invocations[1]
	if got := strings.Join(first.Args, " "); got != "status --target claude" || first.Run.Output != "claude: deployed\n" || first.Err() != nil || first.Refusal != nil {
		t.Errorf("first invocation = %+v, want status --target claude, its output, no failure", first)
	}
	if got := strings.Join(second.Args, " "); got != "apply" || second.Run.Output != "boom\n" || second.Err() == nil || second.ExitCode() != 1 || second.Refusal != nil {
		t.Errorf("second invocation = %+v, want apply, its output, exit status 1", second)
	}
	if res.Action.Command != "status" || targetNames(res.Targets) != "claude" {
		t.Errorf("the result names %q on %q, want the action run and the targets it ran against", res.Action.Command, targetNames(res.Targets))
	}
}

// Several invocations may fail, with different exit-code contracts; the result
// is the worst of them, not the last: a hard failure (any status but 2) outranks
// a degraded run (status 2), whichever came first, and the first of equals wins.
func TestActionRunner_TheResultIsTheWorstOutcomeNotTheLastOne(t *testing.T) {
	tests := []struct {
		name     string
		first    domain.Execution
		second   domain.Execution
		wantCode int
	}{
		{"degraded then hard", failing("", 2), failing("", 1), 1},
		{"hard then degraded", failing("", 1), failing("", 2), 1},
		{"hard then another hard keeps the first", failing("", 1), failing("", 3), 1},
		{"degraded then clean", failing("", 2), domain.Execution{}, 2},
		{"clean then degraded", domain.Execution{}, failing("", 2), 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeBackend{script: map[string]domain.Execution{"status": tc.first, "version": tc.second}}
			action := domain.Action{Command: "status", TargetAgnostic: true, Also: []domain.Action{{Command: "version", TargetAgnostic: true}}}
			res := run(action, nil, nil, &fakeCatalog{}, backend)
			if res.Err == nil || res.ExitCode != tc.wantCode {
				t.Errorf("result = (%v, exit %d), want a failure with exit %d", res.Err, res.ExitCode, tc.wantCode)
			}
		})
	}

	t.Run("clean runs leave no failure", func(t *testing.T) {
		res := run(statusAction(), nil, nil, &fakeCatalog{}, &fakeBackend{})
		if res.Err != nil || res.ExitCode != 0 {
			t.Errorf("result = (%v, exit %d), want no failure", res.Err, res.ExitCode)
		}
	})

	t.Run("an invocation that could not be started carries the code the backend gave", func(t *testing.T) {
		backend := &fakeBackend{script: map[string]domain.Execution{"status": {Err: errors.New("fork/exec: no such file"), ExitCode: -1}}}
		res := run(domain.Action{Command: "status", TargetAgnostic: true}, nil, nil, &fakeCatalog{}, backend)
		if res.Err == nil || res.ExitCode != -1 {
			t.Errorf("result = (%v, exit %d), want a failure with exit -1", res.Err, res.ExitCode)
		}
	})
}

// An action shows the verdicts of a sync-check report when it declares that its
// result is one (ResultSyncDashboard), and only then: not because its command
// is called sync-check, and not because the backend's output happens to hold a
// report. The verdicts come from the executions, read by the adapter; the
// runner reads no text.
func TestActionRunner_OnlyAnActionThatDeclaresAReportShowsItsVerdicts(t *testing.T) {
	needsApply := domain.TargetVerdict{Target: "claude", OverlayNotDeployed: 1, Status: domain.SyncNeedsApply}
	reports := domain.Execution{Output: "a report", Verdicts: []domain.TargetVerdict{needsApply}}
	shown := threeCopyTargets()
	backend := &fakeBackend{script: map[string]domain.Execution{
		"sync-check": reports, "status": reports, "compare": reports,
	}}
	catalog := &fakeCatalog{targets: shown}

	t.Run("an action that declares a report", func(t *testing.T) {
		action := domain.Action{Command: "sync-check", SupportsAll: true, ResultKind: domain.ResultSyncDashboard}
		res := run(action, shown, shown, catalog, backend)
		if len(res.Verdicts) != 1 || res.Verdicts[0].Target != "claude" || res.Verdicts[0].Status != domain.SyncNeedsApply {
			t.Errorf("verdicts = %+v, want claude needing apply", res.Verdicts)
		}
	})

	t.Run("it need not be called sync-check", func(t *testing.T) {
		action := domain.Action{Command: "compare", SupportsAll: true, ResultKind: domain.ResultSyncDashboard}
		if res := run(action, shown, shown, catalog, backend); len(res.Verdicts) != 1 {
			t.Errorf("verdicts = %+v, want the report shown: the kind is declared", res.Verdicts)
		}
	})

	t.Run("an action that shows its transcript shows no verdicts, whatever its execution carried", func(t *testing.T) {
		if res := run(statusAction(), shown, shown, catalog, backend); len(res.Verdicts) != 0 {
			t.Errorf("verdicts = %+v, want none: only a declared report is shown", res.Verdicts)
		}
	})

	t.Run("a command called sync-check that declares nothing shows none", func(t *testing.T) {
		action := domain.Action{Command: "sync-check", SupportsAll: true}
		if res := run(action, shown, shown, catalog, backend); len(res.Verdicts) != 0 {
			t.Errorf("verdicts = %+v, want none: the command string is being read", res.Verdicts)
		}
	})

	t.Run("a declared report that held no verdict shows none", func(t *testing.T) {
		quiet := &fakeBackend{script: map[string]domain.Execution{"sync-check": {Output: "nothing to report\n"}}}
		action := domain.Action{Command: "sync-check", SupportsAll: true, ResultKind: domain.ResultSyncDashboard}
		if res := run(action, shown, shown, catalog, quiet); len(res.Verdicts) != 0 {
			t.Errorf("verdicts = %+v, want none", res.Verdicts)
		}
	})

	t.Run("the verdicts of every invocation are shown, in the order they ran", func(t *testing.T) {
		// A partial selection is one sync-check per target, each reporting its own.
		perTarget := &fakeBackend{script: map[string]domain.Execution{
			"sync-check --target claude": {Verdicts: []domain.TargetVerdict{{Target: "claude"}}},
			"sync-check --target codex":  {Verdicts: []domain.TargetVerdict{{Target: "codex"}}},
		}}
		action := domain.Action{Command: "sync-check", SupportsAll: true, ResultKind: domain.ResultSyncDashboard}
		selected := []domain.Target{shown[0], shown[2]}

		res := run(action, selected, shown, catalog, perTarget)

		var got []string
		for _, v := range res.Verdicts {
			got = append(got, v.Target)
		}
		if strings.Join(got, ",") != "claude,codex" {
			t.Errorf("verdicts of %v, want claude then codex, one per invocation", got)
		}
	})
}
