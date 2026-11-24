//go:build unix

package execrunner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// X1: with the group option, the kill at the deadline reaches every process the program started, so the call comes back
// within the deadline and the grandchild that held the pipes (the stub forks one inside a command substitution) is gone.
func TestProcessGroupKillsTheGrandchildAtTheDeadline(t *testing.T) {
	g := overlaytest.NewGrandchild(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()

	_, _, err := New().WithProcessGroup().Output(ctx, []string{"PATH="}, "/bin/sh", "-c", g.Body)

	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("the call took %v, want it back within 1.5s", took)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	g.Started(t)
	g.RequireGone(t, time.Second)
}

// X2: the group is an option nobody has by default, and it does not touch the output bound.
func TestProcessGroupIsOptIn(t *testing.T) {
	if New().ProcessGroup() {
		t.Error("New().ProcessGroup() is true, want false")
	}
	r := New().WithProcessGroup().WithMaxOutput(8)
	if !r.ProcessGroup() || r.MaxOutput() != 8 {
		t.Errorf("ProcessGroup() = %v, MaxOutput() = %d; want true and 8", r.ProcessGroup(), r.MaxOutput())
	}
}

// X3: the kill of a group that has already ended is not an error of the run: Cancel reports os.ErrProcessDone, which is
// what exec.Cmd takes for "nothing left to kill".
func TestOwnGroupCancelOfAnEndedProgramIsProcessDone(t *testing.T) {
	if _, err := os.Stat("/bin/true"); err != nil {
		t.Skipf("no /bin/true: %v", err)
	}
	cmd := exec.CommandContext(context.Background(), "/bin/true")
	ownGroup(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Cancel(); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("Cancel of an ended program = %v, want os.ErrProcessDone", err)
	}
}
