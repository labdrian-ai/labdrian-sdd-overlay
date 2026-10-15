package domain

import (
	"errors"
	"testing"
)

// An invocation fails in one of two ways, and the result ranks them by what
// they say: a run the backend ended (its own error, its own exit status) and a
// call that was never made. The refusal has no status of the backend's to
// carry, so it reports NoExitStatus, and it is a hard failure by that code
// (InvocationSeverity ranks only status 2 as a degraded run).
func TestInvocation_SaysHowItFailedAndWithWhatStatus(t *testing.T) {
	boom := errors.New("exit status 1")
	degraded := errors.New("exit status 2")
	refused := errors.New("the catalog changed")
	notStarted := errors.New("fork/exec: no such file")

	tests := []struct {
		name         string
		invocation   Invocation
		wantErr      error
		wantExitCode int
	}{
		{"a clean run", Invocation{Args: []string{"status"}, Run: Execution{Output: "ok\n"}}, nil, 0},
		{"a hard failure", Invocation{Args: []string{"apply"}, Run: Execution{Err: boom, ExitCode: 1}}, boom, 1},
		{"a degraded run", Invocation{Args: []string{"status"}, Run: Execution{Err: degraded, ExitCode: 2}}, degraded, 2},
		{"a backend that could not be started", Invocation{Args: []string{"apply"}, Run: Execution{Err: notStarted, ExitCode: NoExitStatus}}, notStarted, NoExitStatus},
		{"a call that was refused", Invocation{Args: []string{"apply", "--target", TargetAll}, Refusal: refused}, refused, NoExitStatus},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.invocation.Err(); got != tc.wantErr {
				t.Errorf("Err() = %v, want %v", got, tc.wantErr)
			}
			if got := tc.invocation.ExitCode(); got != tc.wantExitCode {
				t.Errorf("ExitCode() = %d, want %d", got, tc.wantExitCode)
			}
		})
	}
}

// A refused call was never run, so whatever an execution would have said is not
// said: a zero run behind a refusal reads as no failure of its own, and the
// refusal is what the invocation reports even if a run were attached.
func TestInvocation_ARefusalIsWhatItReportsEvenOverARun(t *testing.T) {
	refused := errors.New("the catalog changed")
	inv := Invocation{Args: []string{"apply"}, Refusal: refused, Run: Execution{Err: errors.New("exit status 2"), ExitCode: 2}}

	if inv.Err() != refused || inv.ExitCode() != NoExitStatus {
		t.Errorf("(Err, ExitCode) = (%v, %d), want the refusal and NoExitStatus", inv.Err(), inv.ExitCode())
	}
}
