package main

import (
	"bytes"
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

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

// A zero deps has no environment: every command that reads one reads nothing, none panics, and
// each ends the way a command with nothing to read ends: with its own exit code and the line that
// says what was missing, so a change in what a nil environment means is seen here.
func TestACommandRunOverAZeroDepsReadsNoEnvironmentAndEndsAsItShould(t *testing.T) {
	root := t.TempDir()
	// The expected lines are the program's own words for what was missing (the phrase that names it,
	// not the whole message), so they change when what a command says about a nil environment
	// changes, which is what this test is for; the paths in a message are not part of them.
	for _, tc := range []struct {
		name string
		run  func(p *capturedProcess)
		// wantExits is every exit the command called, in order. None (nil) means it returned
		// without calling exit at all; {0} means it called exit(0), a deliberate clean exit (the
		// sync-trigger usage error and the probe report are reported with exit 0 by design).
		wantExits []int
		stream    func(p *capturedProcess) string // the stream the line is on
		want      string
	}{
		{
			name:      "status",
			run:       func(p *capturedProcess) { runStatus(p.process, deps{}) },
			wantExits: []int{1},
			stream:    stdoutOf,
			want:      "[FAIL] binary: .claude/bin/gentle-ai-overlay — not found",
		},
		{
			name:      "gadu-generate",
			run:       func(p *capturedProcess) { runGaduGenerate(p.process, deps{}, nil) },
			wantExits: []int{1},
			stream:    stderrOf,
			want:      "OVERLAY_DIR is not set",
		},
		{
			name:      "sync-trigger",
			run:       func(p *capturedProcess) { runSyncTriggerCore(deps{}, []string{"--event", "bogus"}, &p.err, p.exit) },
			wantExits: []int{0},
			stream:    stderrOf,
			want:      `sync-trigger: error:usage event="bogus" cwd=""`,
		},
		{
			name:      "runtime probe",
			run:       func(p *capturedProcess) { runRuntimeProbe(deps{}, nil, &p.out, &p.err, p.exit) },
			wantExits: []int{0},
			stream:    stdoutOf,
			want:      "the home directory is unknown, so the Engram database file was not looked for",
		},
		{
			name: "runtime status",
			run: func(p *capturedProcess) {
				runRuntimeCore(deps{}, noPi(), noGit(), []string{"status", "--target", "claude", "--config-root", root}, &p.out, &p.err, p.exit)
			},
			wantExits: []int{1},
			stream:    stdoutOf,
			want:      "[claude] status: unsupported — Claude settings file not found",
		},
		{
			name: "pipkg",
			run: func(p *capturedProcess) {
				runPipkgCore(deps{}, noGit(), []string{"build", "--overlay-root", root, "--registry", root + "/r.yaml", "--dest-dir", root + "/d"}, &p.out, &p.err, p.exit)
			},
			wantExits: []int{1},
			stream:    stderrOf,
			want:      "overlaps overlay root",
		},
		{
			name: "runtime wrapper",
			run: func(p *capturedProcess) {
				runRuntime(p.process, deps{}, []string{"status", "--target", "claude", "--config-root", root})
			},
			wantExits: []int{1},
			stream:    stdoutOf,
			want:      "[claude] status: unsupported — Claude settings file not found",
		},
		{
			name:      "pipkg wrapper",
			run:       func(p *capturedProcess) { runPipkg(p.process, deps{}, []string{"bogus"}) },
			wantExits: []int{1},
			stream:    stderrOf,
			want:      `unknown pipkg verb "bogus"`,
		},
		{
			name: "skills list",
			run: func(p *capturedProcess) {
				runSkillsCore(deps{}, "list", []string{"list", "--registry", root + "/absent.yaml"}, &p.out, &p.err, p.exit)
			},
			wantExits: []int{1},
			stream:    stderrOf,
			want:      "reading registry",
		},
		{
			name:      "review-receipt",
			run:       func(p *capturedProcess) { runReviewReceiptCapture(p.process, deps{}, []string{"--cwd", root}) },
			wantExits: nil,
			stream:    stdoutOf,
			want:      "review-receipt capture: no active change; nothing to capture",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newCapturedProcess()
			tc.run(p)
			if !slices.Equal(p.exits, tc.wantExits) {
				t.Errorf("exits %v, want %v", p.exits, tc.wantExits)
			}
			if got := tc.stream(p); !strings.Contains(got, tc.want) {
				t.Errorf("output %q does not say %q", got, tc.want)
			}
		})
	}
}

func stdoutOf(p *capturedProcess) string { return p.out.String() }
func stderrOf(p *capturedProcess) string { return p.err.String() }

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

// The bound of a probe is one number: the deps' own when it is positive, and otherwise the one the
// workflow lifecycle bounds its own probes with, so the two ways a probe is bounded cannot drift.
func TestTheBoundOfAProbeIsTheDepsOwnOrTheLifecyclesDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  time.Duration
		want time.Duration
	}{
		{"unset", 0, workflow.DefaultDependencyProbeTimeout},
		{"negative", -time.Second, workflow.DefaultDependencyProbeTimeout},
		{"its own", 3 * time.Second, 3 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (deps{probeTimeout: tc.set}).probeBound(); got != tc.want {
				t.Errorf("probeBound = %s, want %s", got, tc.want)
			}
		})
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
