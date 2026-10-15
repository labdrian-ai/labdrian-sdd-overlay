package domain

// CommandResult is the outcome of running the backend for one or more targets.
type CommandResult struct {
	Action   Action
	Targets  []Target
	Output   string // combined stdout+stderr across invocations
	Verdicts []TargetVerdict
	Err      error
	// ExitCode captures the process exit code when Err is non-nil. It
	// distinguishes a hard failure (code 1) from a degraded/warning result
	// (code 2, e.g. 'engine status' DEGRADED). Zero when err is nil.
	ExitCode int
}
