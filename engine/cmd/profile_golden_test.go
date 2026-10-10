package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The golden files under testdata/profile-golden and the profile cases of testdata/hook-golden
// record what each of the five built-in workflow profiles does through the program: the memory
// plan it grants, the dependencies a workflow of it records on creation (with nothing present and
// with everything present), the stage it admits next and the ones it refuses, and what the
// projection hooks say and gate for a workflow of it. They were recorded from the program as it was
// before the profile's memory default and review dependency became typed data of the profile
// (Phase 9 unit H32), so a profile that behaves differently after that move fails here. Rewrite
// them deliberately with
//
//	go test ./cmd -run TestProfileGolden -update-profile-golden
//
// (the hook cases with -update-hook-golden), and read the diff before committing it.
var updateProfileGolden = flag.Bool("update-profile-golden", false, "rewrite the golden files of the workflow profiles")

// profileStages is the declared stage order of each built-in profile, written out here so that a
// change to a profile's stages is a change to this test.
var profileStages = []struct {
	profile string
	stages  []string
}{
	{"odd", []string{"authorize", "explore", "resolve-uncertainty", "classify", "track-if-substantial", "implement-task-by-task", "close"}},
	{"sdd", []string{"dispatcher-selected-planning-phases", "tasks", "apply", "verify", "archive"}},
	{"standalone-minimal", []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report"}},
	{"maintenance", []string{"inspect", "isolate-smallest-change", "repair", "validate", "report"}},
	{"incident-recovery", []string{"preserve-evidence", "classify", "identify-supported-recovery", "obtain-required-authorization", "recover", "verify", "report"}},
}

// profileHookGoldenCases are the cases of the projection hooks that run over every profile.
func profileHookGoldenCases() []hookGoldenCase {
	return []hookGoldenCase{
		{"prompt-projects-every-profile-through-its-stages", func(w *hookWorld) {
			for _, p := range profileStages {
				e := w.env()
				e.running(w.t, "proj-1", "wf-1", p.profile)
				w.promptHook(p.profile+", running, no stage recorded", e, e.repo)
				for _, stage := range p.stages {
					e.step(w.t, "proj-1", "wf-1", "stage", "--stage", stage)
				}
				w.promptHook(p.profile+", every declared stage recorded", e, e.repo)
			}
		}},
		// The hook worlds confirm nothing by default; here every dependency is present, so what the
		// projection says of a workflow whose dependencies are available is pinned too.
		{"prompt-projects-every-profile-with-its-dependencies-present", func(w *hookWorld) {
			for _, p := range profileStages {
				e := w.envWithEverythingPresent()
				e.running(w.t, "proj-1", "wf-1", p.profile)
				w.promptHook(p.profile+", running, every dependency present", e, e.repo)
			}
		}},
		// A workflow whose log was written before the log recorded a snapshot of the profile (version
		// 1, with two stages recorded) is projected and gated by the profile its name resolves to.
		{"prompt-and-gate-follow-a-version-1-log-of-every-profile", func(w *hookWorld) {
			for _, p := range profileStages {
				e := w.env()
				installV1Log(w.t, e.state, p.profile, "running")
				mustBindOK(w.t, e.repo, "proj-1", "wf-1")
				w.promptHook(p.profile+", a version 1 log with two stages recorded", e, e.repo)
				w.toolHook(p.profile+", the plan's project", e, e.repo, queryTool, `{"query":"q","project":"proj-1"}`)
				w.toolHook(p.profile+", another project", e, e.repo, queryTool, `{"query":"q","project":"proj-2"}`)
			}
		}},
		{"pretooluse-gates-every-profile-by-its-memory-ceiling", func(w *hookWorld) {
			for _, p := range profileStages {
				e := w.env()
				e.running(w.t, "proj-1", "wf-1", p.profile)
				w.toolHook(p.profile+", the plan's project", e, e.repo, queryTool, `{"query":"q","project":"proj-1"}`)
				w.toolHook(p.profile+", another project", e, e.repo, queryTool, `{"query":"q","project":"proj-2"}`)
				w.toolHook(p.profile+", no project", e, e.repo, queryTool, `{"query":"q"}`)
			}
		}},
	}
}

// everythingPresentProber finds every capability it is asked about available.
type everythingPresentProber struct{}

func (everythingPresentProber) Probe(_ context.Context, capabilities []string) ([]workflow.Observation, error) {
	observed := make([]workflow.Observation, len(capabilities))
	for i, name := range capabilities {
		observed[i] = workflow.Observation{Capability: name, Status: workflow.ObservationAvailable, Detail: "present (test)"}
	}
	return observed, nil
}

// envWithEverythingPresent is env, but with a prober that finds every dependency available, both
// for the workflow the case creates and for the hooks it then runs. env resets the world's prober to
// the one that confirms nothing each time it is called, so the prober is set after it, once per
// environment made; only the prober differs from env's.
func (w *hookWorld) envWithEverythingPresent() hookEnv {
	w.t.Helper()
	e := w.env()
	w.deps.workflowProber = func() workflow.DependencyProber { return everythingPresentProber{} }
	e.deps = w.deps
	return e
}

// profileWorld is the scratch space of one profile transcript: the program, a state home, and the
// transcript. Every path of it is written as a placeholder.
type profileWorld struct {
	t      *testing.T
	bin    string
	home   string
	state  string
	path   string
	dir    string
	b      strings.Builder
	places map[string]string
}

// newProfileWorld makes a world whose home holds the presence signals when equipped is set (the
// files the presence prober looks for, and a gentle-ai binary on PATH), and holds nothing
// otherwise. The directories are made outside the test's own, whose name the program must not see.
func newProfileWorld(t *testing.T, bin string, equipped bool) *profileWorld {
	t.Helper()
	root, err := os.MkdirTemp("", "pgw-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	w := &profileWorld{t: t, bin: bin, home: filepath.Join(root, "home"), state: filepath.Join(root, "state"), path: filepath.Join(root, "bin"), dir: filepath.Join(root, "work"), places: map[string]string{}}
	for _, d := range []string{w.home, w.state, w.path, w.dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w.places[w.home], w.places[w.state], w.places[w.path], w.places[w.dir] = "<HOME>", "<STATE>", "<BIN>", "<DIR>"
	if equipped {
		for _, rel := range [][]string{{".engram", "engram.db"}, {".labdrian-overlay", "longterm-mem-registration.json"}} {
			w.put(filepath.Join(append([]string{w.home}, rel...)...), "x", 0o600)
		}
		name := "gentle-ai"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		w.put(filepath.Join(w.path, name), "x", 0o755)
	}
	return w
}

func (w *profileWorld) put(path, content string, mode os.FileMode) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		w.t.Fatal(err)
	}
}

// run records one invocation of the program: its arguments, its exit code and both streams.
func (w *profileWorld) run(args ...string) { w.t.Helper(); w.record(false, args...) }

// runBrief is run for a verb that prints the state of the workflow, of which it records only the
// status and the stages: the state is recorded whole by the verbs that matter for it.
func (w *profileWorld) runBrief(args ...string) { w.t.Helper(); w.record(true, args...) }

// exec runs the program with the arguments in the world and returns its exit code and both streams,
// as they are (record writes them into the transcript).
func (w *profileWorld) exec(args ...string) (int, string, string) {
	w.t.Helper()
	cmd := exec.Command(w.bin, args...)
	cmd.Dir = w.dir
	cmd.Env = append(goldenEnvironment(), "HOME="+w.home, "XDG_STATE_HOME="+w.state, "PATH="+w.path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			w.t.Fatalf("run %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

func (w *profileWorld) record(brief bool, args ...string) {
	w.t.Helper()
	code, out, errOut := w.exec(args...)
	if brief && out != "" {
		var state struct {
			Status string   `json:"status"`
			Stages []string `json:"stages"`
		}
		if err := json.Unmarshal([]byte(out), &state); err != nil {
			w.t.Fatalf("the state printed by %v is not JSON: %v", args, err)
		}
		out = fmt.Sprintf("status %s, stages %v\n", state.Status, state.Stages)
	}
	w.write("$ %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s\n", strings.Join(args, " "), code, ensureNewline(out), ensureNewline(errOut))
}

func (w *profileWorld) write(format string, args ...any) {
	fmt.Fprintf(&w.b, format, args...)
}

// observations records what the created event of a workflow holds as its dependency observations.
func (w *profileWorld) observations(project, workflow string) {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.state, "labdrian", "workflows", project, workflow+".jsonl"))
	if err != nil {
		w.t.Fatal(err)
	}
	var first struct {
		Kind         string `json:"kind"`
		Observations []struct {
			Capability string `json:"capability"`
			Status     string `json:"status"`
			Detail     string `json:"detail"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(bytes.SplitN(data, []byte("\n"), 2)[0], &first); err != nil {
		w.t.Fatal(err)
	}
	w.write("observations of the %s event:\n", first.Kind)
	for _, o := range first.Observations {
		w.write("  %s: %s (%s)\n", o.Capability, o.Status, o.Detail)
	}
	w.write("\n")
}

// text is the transcript with the places written as their placeholders.
func (w *profileWorld) text() string {
	text := w.b.String()
	places := make([]string, 0, len(w.places))
	for place := range w.places {
		places = append(places, place)
	}
	// Longest first, so a path under another is replaced as itself.
	sort.Slice(places, func(i, j int) bool { return len(places[i]) > len(places[j]) })
	for _, place := range places {
		text = strings.ReplaceAll(text, place, w.places[place])
	}
	return text
}

// profileTranscript runs the verbs of a workflow of the profile through its whole life: the memory
// plan, the creation (and the dependencies it records), a stage out of order, every declared stage,
// one stage too many, the verification and the close.
func profileTranscript(t *testing.T, bin, profile string, stages []string, equipped bool) string {
	w := newProfileWorld(t, bin, equipped)
	goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	w.places[goal] = "<GOAL>"
	flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
	with := func(verb string, more ...string) []string {
		return append(append([]string{"workflow", verb}, flags...), more...)
	}
	w.run("memory", "plan", "--profile", profile)
	w.run("memory", "plan", "--profile", profile, "--goal", goal)
	w.run(with("create", "--goal", goal, "--profile", profile)...)
	w.observations("proj-1", "wf-1")
	w.runBrief(with("start")...)
	w.runBrief(with("stage", "--stage", stages[len(stages)-1])...)
	w.runBrief(with("stage", "--stage", "no-such-stage")...)
	for i, stage := range stages {
		if i == 1 {
			// In the middle of the sequence the next stage is the second one: the one just recorded
			// again, one that is not declared and one declared far ahead are all refused.
			w.runBrief(with("stage", "--stage", stages[0])...)
			w.runBrief(with("stage", "--stage", "no-such-stage")...)
			w.runBrief(with("stage", "--stage", stages[len(stages)-1])...)
		}
		w.runBrief(with("stage", "--stage", stage)...)
	}
	w.runBrief(with("stage", "--stage", stages[0])...)
	w.run(with("status")...)
	w.run(with("verify", "--goal", goal)...)
	// A completed workflow takes no reason (a reason is for an abandoned one), so the close is
	// given none and is the successful one; the state after it is recorded too.
	w.run(with("close", "--outcome", "completed")...)
	w.run(with("status")...)
	return w.text()
}

// unknownProfileTranscript runs the two verbs that take a profile by name with a name that is none
// of the built-in ones.
func unknownProfileTranscript(t *testing.T, bin string) string {
	w := newProfileWorld(t, bin, false)
	goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	w.places[goal] = "<GOAL>"
	w.run("memory", "plan", "--profile", "no-such-profile")
	w.run("workflow", "create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goal, "--profile", "no-such-profile")
	return w.text()
}

// notAGoldenFileNameCharacter matches a run of characters a golden file name may not hold: a name
// is acceptable when it does NOT match (checkProfileGolden refuses one that does).
var notAGoldenFileNameCharacter = regexp.MustCompile(`[^a-z0-9-]+`)

// profileGoldenCaseName is the name of the case, and of the golden file, of one profile in one world.
func profileGoldenCaseName(profile string, equipped bool) string {
	if equipped {
		return "workflow-" + profile + "-with-everything-present"
	}
	return "workflow-" + profile + "-with-nothing-present"
}

// checkProfileGolden compares the transcript got with the golden file name under testdata/dir, or
// rewrites that file when the update flag is given.
func checkProfileGolden(t *testing.T, dir, name, got string) {
	t.Helper()
	if notAGoldenFileNameCharacter.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", dir, name+".golden")
	if *updateProfileGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-profile-golden)", err)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}

// TestProfileGolden runs the verbs of a workflow of every profile and compares the transcript with
// its golden file.
func TestProfileGolden(t *testing.T) {
	check := func(t *testing.T, name, got string) {
		t.Helper()
		checkProfileGolden(t, "profile-golden", name, got)
	}
	for _, p := range profileStages {
		for _, equipped := range []bool{false, true} {
			p, equipped := p, equipped
			name := profileGoldenCaseName(p.profile, equipped)
			t.Run(name, func(t *testing.T) {
				check(t, name, profileTranscript(t, engineBinary(t), p.profile, p.stages, equipped))
			})
		}
	}
	t.Run("a-profile-that-is-not-built-in", func(t *testing.T) {
		check(t, "a-profile-that-is-not-built-in", unknownProfileTranscript(t, engineBinary(t)))
	})
}
