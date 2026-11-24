//go:build unix

package evidencesh_test

import (
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/evidencesh"
)

// E1 (t): over a runner that kills the group, a script whose shell forks a grandchild that holds the pipes is stopped at
// the adapter's timeout with every process it started: the error says no answer within the timeout, the call is back
// within the bound and the grandchild is gone (T4x, `4b9c3789c2e0-3` F1).
func TestClassified_AGrandchildDoesNotOutliveTheTimeoutOverAGroupRunner(t *testing.T) {
	w := newWorld(t)
	g := overlaytest.NewGrandchild(t)
	script := w.stub("grandchild.sh", g.Body)
	adapter := evidencesh.New(execrunner.New().WithProcessGroup(), evidencesh.Options{Script: script, Environ: w.environ, Timeout: 200 * time.Millisecond})
	start := time.Now()

	_, err := adapter.Classified(w.root, []string{"f/aaaa-1"})

	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("the call took %v, want it back within 1.5s", took)
	}
	wantError(t, err, "gave no answer within 200ms")
	g.Started(t)
	g.RequireGone(t, time.Second)
}
