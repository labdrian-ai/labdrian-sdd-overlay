package main

// Tests for the presence prober wired into the workflow verbs: the observations a
// workflow records now come from stat-level checks of the process's home and
// PATH, and the old all-unavailable behavior stays available through the seam.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// presenceWorld makes a home holding the Engram database and the longterm-mem
// registration record (and no credentials), and a PATH directory holding
// gentle-ai, and points the process at them.
func presenceWorld(t *testing.T) (stateHome string) {
	t.Helper()
	home, stateHome := phase6IsolatedHome(t)
	for _, rel := range [][]string{{".engram", "engram.db"}, {".labdrian-overlay", "longterm-mem-registration.json"}} {
		p := filepath.Join(append([]string{home}, rel...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	name := "gentle-ai"
	if runtime.GOOS == "windows" {
		name = "gentle-ai.exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return stateHome
}

// useUnavailableProber installs workflow.UnavailableProber through the seam for
// one test, so a test that asserts "every dependency is unavailable" does not
// depend on the gentle-ai binary or the memory files of the machine it runs on.
func useUnavailableProber(t *testing.T) {
	t.Helper()
	saved := workflowProber
	workflowProber = func() workflow.DependencyProber { return workflow.UnavailableProber{} }
	t.Cleanup(func() { workflowProber = saved })
}

func createOddWorkflow(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	phase6MustExitZero(t, "create", []string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd"}, dir)
}

func TestWorkflowRecordsWhatThePresenceProberFindsInTheHomeAndPath(t *testing.T) {
	stateHome := presenceWorld(t)
	createOddWorkflow(t)

	events := phase6LoadOwned(t, stateHome, "proj-1", "wf-1").Events
	if len(events) == 0 {
		t.Fatal("no events recorded")
	}
	want := map[string]string{
		"memory:engram":            workflow.ObservationAvailable,
		"memory:longterm-mem":      workflow.ObservationAvailable,
		"memory:procedural-skills": workflow.ObservationUnavailable,
		"gentle-ai-review":         workflow.ObservationAvailable,
	}
	seen := map[string]bool{}
	for _, o := range events[0].Observations {
		wantStatus, known := want[o.Capability]
		if !known {
			t.Errorf("unexpected observation %+v", o)
			continue
		}
		seen[o.Capability] = true
		if o.Status != wantStatus {
			t.Errorf("%s = %s, want %s (detail %q)", o.Capability, o.Status, wantStatus, o.Detail)
		}
		if o.Status == workflow.ObservationAvailable && !strings.Contains(o.Detail, "not opened") && !strings.Contains(o.Detail, "not executed") {
			t.Errorf("%s detail %q does not state the stat-level limit", o.Capability, o.Detail)
		}
	}
	for _, capName := range []string{"gentle-ai-review", "memory:engram"} {
		if !seen[capName] {
			t.Errorf("the odd profile recorded no %s observation", capName)
		}
	}
}

// TestWorkflowSeamKeepsTheUnavailableProberAvailable: the default prober reads
// the home and PATH, but the constructor takes its prober from a seam, so the
// old behavior (nothing is ever confirmed) stays covered in the same world
// where the presence prober confirms everything.
func TestWorkflowSeamKeepsTheUnavailableProberAvailable(t *testing.T) {
	stateHome := presenceWorld(t)
	useUnavailableProber(t)
	createOddWorkflow(t)

	for _, e := range phase6LoadOwned(t, stateHome, "proj-1", "wf-1").Events {
		for _, o := range e.Observations {
			if o.Status != workflow.ObservationUnavailable {
				t.Errorf("event %s observation %+v, want unavailable through the seam", e.Kind, o)
			}
		}
	}
}

func TestWorkflowDefaultProberUsesTheProcessHomeAndPath(t *testing.T) {
	presenceWorld(t)
	got, err := workflowProber().Probe(context.Background(), []string{"memory:engram", "gentle-ai-review", "credentials:codex"})
	if err != nil || len(got) != 3 {
		t.Fatalf("Probe() = %v, %v", got, err)
	}
	wantStatus := []string{workflow.ObservationAvailable, workflow.ObservationAvailable, workflow.ObservationUnavailable}
	for i, o := range got {
		if o.Status != wantStatus[i] {
			t.Errorf("%+v, want %s", o, wantStatus[i])
		}
	}
}

// TestProjectedContextListsOnlyTheDependenciesThePresenceProberDidNotFind: what the
// prober confirms is not reported as unavailable to the session, and what it did
// not find still is.
func TestProjectedContextListsOnlyTheDependenciesThePresenceProberDidNotFind(t *testing.T) {
	e := newHookEnv(t)
	bin := t.TempDir()
	name := "gentle-ai"
	if runtime.GOOS == "windows" {
		name = "gentle-ai.exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	e.running(t, "proj-1", "wf-1", "odd")

	line := contextLineWithPrefix(decodeHookOutput(t, e.hook(t, e.repo)).Context, "unavailable dependencies at the last recorded event:")
	if strings.Contains(line, "gentle-ai-review") {
		t.Errorf("line %q lists gentle-ai-review, which is on PATH", line)
	}
	if !strings.Contains(line, "memory:engram") || !strings.Contains(line, "memory:procedural-skills") {
		t.Errorf("line %q lost the dependencies that were not found", line)
	}
}
