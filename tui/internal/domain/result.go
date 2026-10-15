package domain

// NoExitStatus is the exit code of an invocation that has no exit status of the
// backend's to report: it was refused and never made, the backend could not be
// started, or it did not end by exiting.
const NoExitStatus = -1

// Invocation is one call of the backend that an action made, or was stopped
// from making, and what came of it. The result of an action records them in
// the order they were made; how they read on a screen is the screen's.
type Invocation struct {
	// Args are the arguments of the call, the subcommand first.
	Args []string
	// Refusal is why the call was never made (the runner could not verify that
	// `--target all` still means the catalog the operator was shown); nil when
	// it was made. A refused call has no Run.
	Refusal error
	// Run is what the backend left behind. The zero Execution when the call
	// was refused.
	Run Execution
}

// Err is why the invocation failed: its refusal, or the error of its run; nil
// when the call was made and the backend exited with status 0.
func (i Invocation) Err() error {
	if i.Refusal != nil {
		return i.Refusal
	}
	return i.Run.Err
}

// ExitCode is the exit status the invocation failed with, which ranks a hard
// failure against a degraded run (InvocationSeverity). A refused call has none
// of the backend's, so it is NoExitStatus.
func (i Invocation) ExitCode() int {
	if i.Refusal != nil {
		return NoExitStatus
	}
	return i.Run.ExitCode
}

// CommandResult is the outcome of running the backend for one or more targets.
type CommandResult struct {
	Action  Action
	Targets []Target
	// Invocations are the calls the action made, in order, each with what the
	// backend left behind. The result holds no text of its own: the screen
	// renders its transcript from them.
	Invocations []Invocation
	// Verdicts are the per-target verdicts of the sync-check report, for an
	// action that declares its result a report (ResultSyncDashboard).
	Verdicts []TargetVerdict
	// Err is the worst failure among the invocations (see InvocationSeverity),
	// and the first of equals; nil when none failed.
	Err error
	// ExitCode captures the process exit code when Err is non-nil. It
	// distinguishes a hard failure (code 1) from a degraded/warning result
	// (code 2, e.g. 'engine status' DEGRADED). Zero when err is nil.
	ExitCode int
}
