package app_test

// OnSessionStart (C23 M8, docs/architecture/adr-c23-m8-session-availability.md): the memory
// instructions of OnInstructions with the session-time probe of the plan's sources. Only the
// SessionStart hook probes; OnInstructions, which the gate-task rewrite and 'memory plan
// --instructions' use, never does.

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// recordingProber answers every source it is asked about as unavailable, and records what it saw.
type recordingProber struct {
	calls   int
	dir     string
	sources []memoryscope.Source
}

// oddRunning is a running workflow of the odd profile, whose plan has sources to probe
// (standalone-minimal permits none).
func oddRunning() workflow.Loaded {
	l := owned(workflow.StatusRunning)
	l.State.Profile = "odd"
	return l
}

func (p *recordingProber) Probe(dir string, sources []memoryscope.Source) []projection.SourceAvailability {
	p.calls++
	p.dir, p.sources = dir, sources
	out := make([]projection.SourceAvailability, len(sources))
	for i, s := range sources {
		out[i] = projection.SourceAvailability{Source: s, Detail: "down in the test"}
	}
	return out
}

func TestOnSessionStartGivesTheDomainsBlockWithTheProbeOfTheSessionsDirectory(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.workflows.put("proj-1", "wf-1", oddRunning())
	w.service.EngramName = engramName
	prober := &recordingProber{}
	w.service.Availability = prober

	got := w.service.OnSessionStart(prompt(repoDir, ""))

	if prober.calls != 1 || prober.dir != repoDir || len(prober.sources) == 0 {
		t.Fatalf("the prober saw %d calls, dir %q, sources %v; want one call for %q about the plan's sources", prober.calls, prober.dir, prober.sources, repoDir)
	}
	want := projection.Instructions(projection.ProjectionInput{
		Binding:  projection.Loaded{Classification: projection.ClassificationOwned, Binding: w.bindings.bound(repoKey)},
		Workflow: ptr(oddRunning()),
	}, engramName, func(sources []memoryscope.Source) []projection.SourceAvailability {
		return (&recordingProber{}).Probe(repoDir, sources)
	})
	if got.Kind != app.PromptProjection || got.Result != want || !strings.Contains(want.Block, "Do not read") {
		t.Errorf("OnSessionStart = %+v, want the domain's block with a degradation line %+v", got, want)
	}
	if again := w.service.OnSessionStart(prompt("", repoDir)); again.Result != want || prober.dir != repoDir {
		t.Errorf("without a directory in the input the process's is used: %+v, probed %q", again, prober.dir)
	}
}

func TestOnInstructionsNeverProbes(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.workflows.put("proj-1", "wf-1", oddRunning())
	prober := &recordingProber{}
	w.service.Availability = prober

	got := w.service.OnInstructions(prompt(repoDir, ""))

	if prober.calls != 0 || strings.Contains(got.Result.Block, "Do not read") {
		t.Errorf("OnInstructions probed %d times and gave %q; the gate-task rewrite runs on every Agent call and must not", prober.calls, got.Result.Block)
	}
}

func TestOnSessionStartWithoutAProberIsOnInstructions(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.service.EngramName = engramName
	if got, want := w.service.OnSessionStart(prompt(repoDir, "")), w.service.OnInstructions(prompt(repoDir, "")); got != want {
		t.Errorf("OnSessionStart with no prober = %+v, want OnInstructions's %+v", got, want)
	}
}

func TestOnSessionStartProbesNothingWhenThereIsNoBlockToGive(t *testing.T) {
	w := newHookWorld(workflow.StatusClosed)
	prober := &recordingProber{}
	w.service.Availability = prober
	w.service.OnSessionStart(prompt(repoDir, ""))
	empty := newHookWorld(workflow.StatusRunning)
	empty.bindings = newBindings()
	empty.service.Bindings = empty.bindings
	empty.service.Availability = prober
	empty.service.OnSessionStart(prompt(repoDir, ""))
	if prober.calls != 0 {
		t.Errorf("the prober ran %d times for a closed workflow or an unbound repository", prober.calls)
	}
}
