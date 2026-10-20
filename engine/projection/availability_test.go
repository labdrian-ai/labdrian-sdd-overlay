package projection_test

// The degradation lines of C23 M8 (docs/architecture/adr-c23-m8-session-availability.md): the
// instructions block says, for each source of the plan that the session-time probe found unavailable,
// not to read it and why. It is information only: the plan and the gate are not changed by a probe.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

const projectSourcesLine = "- Read memory only from: engram, longterm-mem, procedural-skills. A read from any other source is denied."

// probeAnswering is a probe that answers with av, and records the sources it was asked about.
func probeAnswering(asked *[]memoryscope.Source, av ...projection.SourceAvailability) projection.SourceProber {
	return func(sources []memoryscope.Source) []projection.SourceAvailability {
		*asked = append(*asked, sources...)
		return av
	}
}

func blockWith(w *workflow.Loaded, probe projection.SourceProber) string {
	return projection.InstructionsBlock(validBinding(), w.State, lastSegment, probe)
}

func lines(block string) []string { return strings.Split(block, "\n") }

// An unavailable source of the plan gets one line right after the sources line, with the reason;
// an available one gets none.
func TestAnUnavailableSourceOfThePlanGetsOneDegradationLine(t *testing.T) {
	w := withIdentity(loadedWorkflow("odd", workflow.StatusRunning), "github.com/acme/proj-1")
	var asked []memoryscope.Source
	probe := probeAnswering(&asked,
		projection.SourceAvailability{Source: memoryscope.SourceEngram, Available: true},
		projection.SourceAvailability{Source: memoryscope.SourceLongtermMem, Available: false, Detail: "the longterm-mem binary did not answer"},
		projection.SourceAvailability{Source: memoryscope.SourceProceduralSkills, Available: false, Detail: "no procedural skills lock: none installed"},
	)
	got := lines(blockWith(w, probe))
	want := []string{
		"labdrian memory instructions (workflow wf-1, scope project):",
		projectSourcesLine,
		"- Do not read longterm-mem: it is unavailable in this session (the longterm-mem binary did not answer).",
		"- Do not read procedural-skills: it is unavailable in this session (no procedural skills lock: none installed).",
		`- Engram reads (mem_search, mem_context) name project "proj-1"; longterm-mem query and get name project "github.com/acme/proj-1".`,
		instructionsWrites,
		instructionsProcedural,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InstructionsBlock() lines =\n%q\nwant\n%q", got, want)
	}
	wantAsked := []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceLongtermMem, memoryscope.SourceProceduralSkills}
	if !reflect.DeepEqual(asked, wantAsked) {
		t.Errorf("the probe was asked about %v, want the sources of the plan %v", asked, wantAsked)
	}
}

// With every source available, or with no probe at all, the block is the one it was before M8.
func TestTheBlockIsUnchangedWhenEverySourceIsAvailableOrThereIsNoProbe(t *testing.T) {
	w := withIdentity(loadedWorkflow("odd", workflow.StatusRunning), "github.com/acme/proj-1")
	plain := blockWith(w, nil)
	var asked []memoryscope.Source
	allUp := probeAnswering(&asked,
		projection.SourceAvailability{Source: memoryscope.SourceEngram, Available: true},
		projection.SourceAvailability{Source: memoryscope.SourceLongtermMem, Available: true},
		projection.SourceAvailability{Source: memoryscope.SourceProceduralSkills, Available: true},
	)
	if got := blockWith(w, allUp); got != plain {
		t.Errorf("a block with every source available =\n%s\nwant the block without a probe\n%s", got, plain)
	}
	if strings.Contains(plain, "Do not read") {
		t.Errorf("a block with no probe says not to read a source:\n%s", plain)
	}
}

// A source the plan does not include is never reported, even if the probe answers for it, and a
// source the probe says nothing about is not reported either.
func TestOnlyTheSourcesOfThePlanAreReported(t *testing.T) {
	eng := memoryscope.SourceEngram
	w := withIdentity(narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, eng), nil, nil), "github.com/acme/proj-1")
	var asked []memoryscope.Source
	probe := probeAnswering(&asked,
		projection.SourceAvailability{Source: memoryscope.SourceLongtermMem, Available: false, Detail: "not in the plan"},
	)
	got := blockWith(w, probe)
	if strings.Contains(got, "Do not read") {
		t.Errorf("the block reports a source the plan excludes, or one the probe did not answer for:\n%s", got)
	}
	if want := []memoryscope.Source{eng}; !reflect.DeepEqual(asked, want) {
		t.Errorf("the probe was asked about %v, want only %v", asked, want)
	}
}

// Under scope none there is nothing to read, so nothing is probed and nothing is reported.
func TestUnderScopeNoneNothingIsProbed(t *testing.T) {
	w := withIdentity(narrowedWorkflow(narrowerDirective(memoryscope.ScopeNone), nil, nil), "github.com/acme/proj-1")
	called := false
	probe := func([]memoryscope.Source) []projection.SourceAvailability {
		called = true
		return nil
	}
	got := blockWith(w, probe)
	if called {
		t.Error("the probe ran under scope none")
	}
	if got != blockWith(w, nil) {
		t.Errorf("a block under scope none changed with a probe:\n%s", got)
	}
}

// A reason from outside is one line, bounded, with no control characters.
func TestADegradationReasonIsSanitizedAndBounded(t *testing.T) {
	w := withIdentity(loadedWorkflow("odd", workflow.StatusRunning), "github.com/acme/proj-1")
	var asked []memoryscope.Source
	probe := probeAnswering(&asked, projection.SourceAvailability{
		Source:    memoryscope.SourceEngram,
		Available: false,
		Detail:    "line one\nline two\x1b[31m" + strings.Repeat("x", 4000),
	})
	for _, line := range lines(blockWith(w, probe)) {
		if strings.HasPrefix(line, "- Do not read engram") {
			if strings.ContainsAny(line, "\x1b\r") || len([]rune(line)) > 600 {
				t.Errorf("the degradation line is not one bounded line: %q", line)
			}
			return
		}
	}
	t.Error("the unavailable source has no degradation line")
}

// A probe that panics costs the lines, never the block.
func TestAProbeThatPanicsCostsTheLinesNotTheBlock(t *testing.T) {
	w := withIdentity(loadedWorkflow("odd", workflow.StatusRunning), "github.com/acme/proj-1")
	probe := func([]memoryscope.Source) []projection.SourceAvailability { panic("probe failed") }
	if got, want := blockWith(w, probe), blockWith(w, nil); got != want {
		t.Errorf("a panicking probe changed the block:\n%s\nwant\n%s", got, want)
	}
}
