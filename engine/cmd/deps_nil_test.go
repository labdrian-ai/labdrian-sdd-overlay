package main

import (
	"bytes"
	"context"
	"io/fs"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// absentEverywhereFS is a stat access under which nothing exists.
type absentEverywhereFS struct{}

func (absentEverywhereFS) Lstat(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
func (absentEverywhereFS) Stat(string) (fs.FileInfo, error)  { return nil, fs.ErrNotExist }

// A deps that leaves a field unset is a deps with the program's own choice for it (a nil store,
// gate or prober) or with nothing to offer (no environment, no home, no working directory), and
// never a nil function call. The program sets every field, but a test, or a caller that comes
// later, may build a deps with fewer.

// A zero deps has no environment: every command that reads one reads nothing, and none panics.
func TestACommandRunOverAZeroDepsReadsNoEnvironmentAndDoesNotPanic(t *testing.T) {
	root := t.TempDir()
	for name, run := range map[string]func(p *capturedProcess){
		"status":        func(p *capturedProcess) { runStatus(p.process, deps{}) },
		"gadu-generate": func(p *capturedProcess) { runGaduGenerate(p.process, deps{}, nil) },
		"sync-trigger":  func(p *capturedProcess) { runSyncTriggerCore(deps{}, []string{"--event", "bogus"}, p.exit) },
		"runtime probe": func(p *capturedProcess) { runRuntimeProbe(deps{}, nil, &p.out, &p.err, p.exit) },
		"runtime status": func(p *capturedProcess) {
			runRuntimeCore(deps{}, noPi(), noGit(), []string{"status", "--target", "claude", "--config-root", root}, &p.out, &p.err, p.exit)
		},
		"pipkg": func(p *capturedProcess) {
			runPipkgCore(deps{}, noGit(), []string{"build", "--overlay-root", root, "--registry", root + "/r.yaml", "--dest-dir", root + "/d"}, &p.out, &p.err, p.exit)
		},
		"runtime wrapper": func(p *capturedProcess) {
			runRuntime(p.process, deps{}, []string{"status", "--target", "claude", "--config-root", root})
		},
		"pipkg wrapper": func(p *capturedProcess) { runPipkg(p.process, deps{}, []string{"bogus"}) },
		"skills list": func(p *capturedProcess) {
			runSkillsCore(deps{}, "list", []string{"list", "--registry", root + "/absent.yaml"}, &p.out, &p.err, p.exit)
		},
		"review-receipt": func(p *capturedProcess) { runReviewReceiptCapture(p.process, deps{}, []string{"--cwd", root}) },
	} {
		t.Run(name, func(t *testing.T) {
			p := newCapturedProcess()
			run(p)
		})
	}
}

func TestAZeroDepsHasNoHomeAndNoWorkingDirectory(t *testing.T) {
	if home, err := (deps{}).homeDir(); err == nil || home != "" {
		t.Errorf("homeDir = %q, %v; want no home and an error", home, err)
	}
	if cwd, err := (deps{}).workingDir(); err == nil || cwd != "" {
		t.Errorf("workingDir = %q, %v; want no directory and an error", cwd, err)
	}
	if got := (deps{}).env("HOME"); got != "" {
		t.Errorf("env(HOME) = %q, want empty", got)
	}
	if got := (deps{}).environment(); len(got) != 0 {
		t.Errorf("environment = %q, want none", got)
	}
}

// A probe with no timeout set is bound by the default, not by a deadline that has already passed.
func TestTheProbeOfADepsWithNoTimeoutIsBoundByTheDefault(t *testing.T) {
	home, path := probeFixture(t)
	d := probeDeps(home, path)
	d.probeTimeout = 0

	rep := decodeProbe(t, runProbeTestWith(d))

	for _, o := range rep.Observations {
		if o.Capability == "memory:engram" && o.Status != workflow.ObservationAvailable {
			t.Errorf("memory:engram = %+v, want available: a zero timeout must not mean an expired one", o)
		}
	}
}

// A deps with no prober has the presence prober over the environment of that deps, read when the
// verb asks: it is neither a nil call nor the prober of another environment.
func TestAWorkflowVerbOverADepsWithNoProberProbesThePresenceOfThatDeps(t *testing.T) {
	_, stateHome := phase6IsolatedHome(t) // the process's own home holds nothing
	home, path := probeFixture(t)
	d := probeDeps(home, path)
	d.workflowProber = nil

	createOddWorkflowWith(t, d)

	events := phase6LoadOwned(t, stateHome, "proj-1", "wf-1").Events
	if len(events) == 0 {
		t.Fatal("no events recorded")
	}
	available := map[string]bool{}
	for _, o := range events[0].Observations {
		available[o.Capability] = o.Status == workflow.ObservationAvailable
	}
	if !available["memory:engram"] || !available["gentle-ai-review"] {
		t.Errorf("observations %v, want memory:engram and gentle-ai-review available from the home and PATH of the deps", available)
	}
}

// The default prober is as testable as any other: the stat access of the deps reaches it.
func TestTheDefaultProberOfADepsUsesItsStatAccess(t *testing.T) {
	home, path := probeFixture(t)
	d := probeDeps(home, path)
	d.workflowProber = nil
	d.probeFS = absentEverywhereFS{}

	got, err := d.dependencyProber().Probe(context.Background(), []string{"memory:engram"})

	if err != nil || len(got) != 1 || got[0].Status != workflow.ObservationUnavailable {
		t.Errorf("Probe = %+v, %v; want memory:engram unavailable through the stat access of the deps", got, err)
	}
}

// A deps with no binding store opener has the program's store, under the state home.
func TestABindingVerbOverADepsWithNoStoreOpenerUsesTheProgramsStore(t *testing.T) {
	e := newBindEnv(t)
	d := e.deps
	d.openBindings = nil
	var out, errBuf bytes.Buffer
	var codes []int

	runWorkflowCore(d, []string{"binding"}, e.repo, &out, &errBuf, func(c int) { codes = append(codes, c) })

	if (len(codes) != 0 && codes[0] != 0) || errBuf.Len() != 0 {
		t.Errorf("exits %v, stderr %q; want the binding of the repository (none) reported without a failure", codes, errBuf.String())
	}
}

// A deps with no gate decides with the domain's.
func TestAGateHookOverADepsWithNoGateDecidesWithTheDomains(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	d := e.deps
	d.gate = nil

	assertDenied(t, "a paused workflow", e.gateWith(d, t, e.repo, "Edit", editInput), "paused")
}
