package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// verifiedStateMarker is what `workflow verify` prints once the workflow has been started (event 1)
// and verified (event 2): the sequence number of the last verified event.
const verifiedStateMarker = `"last_verified_seq": 2`

// Drift is advisory (the owner's decision: nothing is refused), so a catalog that fails when it is
// asked for the profile, with an error that is not "there is no such profile", must cost a status or
// a verify nothing but one warning line: the state is printed as it would be, and the exit code is
// the same. Status is the read-only diagnostic; it is the last command to lose the state of a workflow.
func TestAFailingCatalogCostsStatusAndVerifyOneWarningAndNothingElse(t *testing.T) {
	broken := errors.New("the catalog cannot be read")
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "odd")
	e.step(t, "proj-1", "wf-1", "start")
	goalFile := e.goalPath("proj-1", "wf-1")
	failing := e.deps
	failing.profileCatalog = func() workflow.ProfileCatalog {
		return workflow.ProfileCatalogFunc(func(string) (workflowprofile.WorkflowProfile, error) {
			return workflowprofile.WorkflowProfile{}, broken
		})
	}
	flags := []string{"--project", "proj-1", "--workflow", "wf-1"}

	t.Run("status", func(t *testing.T) {
		args := append([]string{"status"}, flags...)
		// The state status prints when the catalog answers is the state it prints when it does not.
		want := runWorkflowTestWith(e.deps, args, e.dir)
		got := runWorkflowTestWith(failing, args, e.dir)
		if got.code != 0 || got.stdout != want.stdout {
			t.Fatalf("status over a failing catalog = exit %d, stdout %q, want exit 0 and the state %q (stderr %q)", got.code, got.stdout, want.stdout, got.stderr)
		}
		assertDriftCheckWarning(t, got.stderr, "status", broken)
	})

	t.Run("verify", func(t *testing.T) {
		args := append([]string{"verify", "--goal", goalFile}, flags...)
		got := runWorkflowTestWith(failing, args, e.dir)
		if got.code != 0 || !strings.Contains(got.stdout, verifiedStateMarker) {
			t.Fatalf("verify over a failing catalog = exit %d, stdout %q (stderr %q), want exit 0 and the verified state", got.code, got.stdout, got.stderr)
		}
		assertDriftCheckWarning(t, got.stderr, "verify", broken)
	})
}

// assertDriftCheckWarning requires stderr to be the one line that says why drift could not be checked.
func assertDriftCheckWarning(t *testing.T, stderr, verb string, cause error) {
	t.Helper()
	prefix := "warning: workflow " + verb + ": could not check whether the profile drifted: "
	if !strings.HasPrefix(stderr, prefix) || !strings.HasSuffix(stderr, cause.Error()+"\n") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr = %q, want one line that begins %q and ends with the catalog's error %q", stderr, prefix, cause)
	}
}
