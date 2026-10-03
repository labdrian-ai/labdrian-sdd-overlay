package projection_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// --- fixtures ---------------------------------------------------------------

// ownedBinding is the store's answer for a repository bound to wf-1 of proj-1.
func ownedBinding() projection.Loaded {
	return projection.Loaded{Classification: projection.ClassificationOwned, Binding: validBinding()}
}

// loadedWorkflow is what the workflow store returns for an owned workflow of
// the given profile and status with the given stages recorded: goal-1, a goal
// digest of "c" characters, and two events whose observations differ, so a test
// can tell the last event's observations from an earlier one's.
func loadedWorkflow(profile string, status workflow.Status, stages ...string) *workflow.Loaded {
	return &workflow.Loaded{
		Classification: workflow.ClassificationOwned,
		Events: []workflow.WorkflowEvent{
			{Kind: workflow.KindCreated, At: "2026-09-29T10:00:00Z", Observations: []workflow.Observation{
				{Capability: "memory:early-only", Status: workflow.ObservationUnavailable},
			}},
			{Kind: workflow.KindStarted, At: "2026-09-29T10:05:00Z", Observations: []workflow.Observation{
				{Capability: "memory:engram", Status: workflow.ObservationUnavailable, Detail: "not probed"},
				{Capability: "memory:longterm-mem", Status: workflow.ObservationAvailable},
				{Capability: "gentle-ai-review", Status: workflow.ObservationUnavailable},
			}},
		},
		State: workflow.State{Status: status, Profile: profile, GoalID: "goal-1", GoalDigest: hex64("c"), Stages: stages},
	}
}

func project(binding projection.Loaded, w *workflow.Loaded) projection.ProjectionResult {
	return projection.Project(projection.ProjectionInput{Binding: binding, Workflow: w})
}

func hasLine(text, line string) bool {
	for _, l := range strings.Split(text, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// lineWithPrefix returns the first line of text that starts with prefix.
func lineWithPrefix(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("no line starts with %q in:\n%s", prefix, text)
	return ""
}

func noLineStartsWith(t *testing.T, text, prefix string) {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, prefix) {
			t.Errorf("unexpected line %q in:\n%s", l, text)
		}
	}
}

// claudeLimits is the capability line the context must carry, derived here from
// the declaration itself, the way the engine does: every Claude Code claim that
// is not supported, as name=status, in the declaration's order.
func claudeLimits(t *testing.T) string {
	t.Helper()
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, c := range d.Claims {
		if c.Status != capability.Supported {
			parts = append(parts, string(c.Capability)+"="+string(c.Status))
		}
	}
	if len(parts) == 0 {
		t.Fatal("test bug: every Claude Code capability is supported, so there is no limit line to compare")
	}
	return "capability limits (claude): " + strings.Join(parts, ", ")
}

// --- constants ----------------------------------------------------------------

func TestContextLimit(t *testing.T) {
	if projection.MaxContextBytes != 16384 {
		t.Errorf("MaxContextBytes = %d, want 16384 (16 KiB)", projection.MaxContextBytes)
	}
}

// --- the binding decides whether anything is projected ----------------------

func TestProjectSaysNothingWithoutABinding(t *testing.T) {
	// The workflow argument is irrelevant when nothing is bound, even a workflow
	// that could be projected.
	for _, w := range []*workflow.Loaded{nil, loadedWorkflow("odd", workflow.StatusRunning)} {
		got := project(projection.Loaded{Classification: projection.ClassificationAbsent}, w)
		if got != (projection.ProjectionResult{}) {
			t.Errorf("Project() = %+v, want the empty result", got)
		}
	}
}

func TestProjectWarnsAboutABindingItCannotUse(t *testing.T) {
	for _, class := range []projection.Classification{
		projection.ClassificationForeign,
		projection.ClassificationMalformed,
		projection.ClassificationUnavailable,
		"a-classification-from-the-future",
	} {
		t.Run(string(class), func(t *testing.T) {
			binding := projection.Loaded{Classification: class, Detail: "permission denied\nsecond line"}
			// A workflow is passed to prove that it is not projected either.
			got := project(binding, loadedWorkflow("odd", workflow.StatusRunning))
			if got.Context != "" || got.Unbind {
				t.Fatalf("Project() = %+v, want a warning only: nothing projected and nothing unbound", got)
			}
			for _, want := range []string{string(class), "permission denied second line", "labdrian workflow binding", "labdrian workflow unbind"} {
				if !strings.Contains(got.Warning, want) {
					t.Errorf("warning %q does not mention %q", got.Warning, want)
				}
			}
			if strings.Contains(got.Warning, "\n") {
				t.Errorf("warning %q spans more than one line", got.Warning)
			}
		})
	}
}

func TestProjectWarnsAndProjectsNothingForAWorkflowItCannotFollow(t *testing.T) {
	for _, tt := range []struct {
		name string
		w    *workflow.Loaded
		want string
	}{
		{"absent", &workflow.Loaded{Classification: workflow.ClassificationAbsent}, "does not exist"},
		{"foreign", &workflow.Loaded{Classification: workflow.ClassificationForeign, Detail: "line 1 is not a workflow event we recognize"}, "is foreign"},
		{"malformed", &workflow.Loaded{Classification: workflow.ClassificationMalformed, Detail: "workflow log is empty"}, "is malformed"},
		{"drifted", &workflow.Loaded{Classification: workflow.ClassificationDrifted, Detail: "prev_digest does not match"}, "is drifted"},
		{"unavailable", &workflow.Loaded{Classification: workflow.ClassificationUnavailable, Detail: "permission denied"}, "is unavailable"},
		{"a classification from the future", &workflow.Loaded{Classification: "quantum"}, "unrecognized state (quantum)"},
		{"no workflow was loaded", nil, "is unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := project(ownedBinding(), tt.w)
			if got.Context != "" {
				t.Errorf("Project() projected %q for a workflow that cannot be followed", got.Context)
			}
			if got.Unbind {
				t.Errorf("Project() asked to unbind: an unknown state is not a closed workflow")
			}
			for _, want := range []string{"wf-1", "proj-1", tt.want, "labdrian workflow status --project proj-1 --workflow wf-1", "labdrian workflow unbind"} {
				if !strings.Contains(got.Warning, want) {
					t.Errorf("warning %q does not mention %q", got.Warning, want)
				}
			}
			if tt.w != nil && tt.w.Detail != "" && !strings.Contains(got.Warning, tt.w.Detail) {
				t.Errorf("warning %q does not carry the detail %q", got.Warning, tt.w.Detail)
			}
			if strings.Contains(got.Warning, "\n") {
				t.Errorf("warning %q spans more than one line", got.Warning)
			}
		})
	}
}

// TestProjectBoundsTheDetailInAWarning: a warning is shown to the user on every
// prompt while the problem lasts, so a long error text must not fill the screen.
func TestProjectBoundsTheDetailInAWarning(t *testing.T) {
	long := strings.Repeat("é", 5000)
	got := project(projection.Loaded{Classification: projection.ClassificationMalformed, Detail: long}, nil)
	if len(got.Warning) > 1024 || !utf8.ValidString(got.Warning) {
		t.Fatalf("warning is %d bytes (valid UTF-8: %v), want a short one", len(got.Warning), utf8.ValidString(got.Warning))
	}
	if !strings.Contains(got.Warning, "...") {
		t.Errorf("warning %q does not show that the detail was cut", got.Warning)
	}
}

// --- a closed workflow ------------------------------------------------------

func TestProjectAnnouncesAClosedWorkflowAndAsksForItsBindingToBeRemoved(t *testing.T) {
	for _, outcome := range []workflow.Outcome{workflow.OutcomeCompleted, workflow.OutcomeAbandoned} {
		t.Run(string(outcome), func(t *testing.T) {
			w := loadedWorkflow("odd", workflow.StatusClosed, "authorize")
			w.State.CloseOutcome = outcome
			w.State.CloseReason = "a reason that must not be repeated\nin the context"

			got := project(ownedBinding(), w)
			if !got.Unbind || got.Warning != "" {
				t.Fatalf("Project() = %+v, want Unbind and no warning", got)
			}
			if strings.Contains(got.Context, "\n") {
				t.Errorf("context %q is more than one line", got.Context)
			}
			for _, want := range []string{"wf-1", "proj-1", "closed (" + string(outcome) + ")"} {
				if !strings.Contains(got.Context, want) {
					t.Errorf("context %q does not mention %q", got.Context, want)
				}
			}
			if strings.Contains(got.Context, "reason") {
				t.Errorf("context %q repeats the close reason", got.Context)
			}
			// Project only decides that the binding should go. Whether it went is
			// known only after the caller tried, so Project itself claims nothing.
			if strings.Contains(got.Context, "removed") || strings.Contains(got.Context, "being removed") {
				t.Errorf("context %q claims a removal before anything was removed", got.Context)
			}
		})
	}
}

// TestAfterUnbindStatesWhatTheRemovalDid: the note of a closed workflow says the
// binding was removed only when the store reported it, says the removal failed
// (and how to finish it) when it errored, and says it was left alone when it was
// already gone or replaced.
func TestAfterUnbindStatesWhatTheRemovalDid(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusClosed, "authorize")
	w.State.CloseOutcome = workflow.OutcomeAbandoned
	base := project(ownedBinding(), w)

	for _, tt := range []struct {
		name    string
		removed bool
		err     error
		want    []string
		notWant []string
	}{
		{"removed", true, nil, []string{"closed (abandoned)", "was removed"}, []string{"failed", "workflow unbind"}},
		{"failed", false, errors.New("permission denied\nsecond line"), []string{"closed (abandoned)", "failed", "labdrian workflow unbind", "permission denied"}, []string{"was removed", "being removed"}},
		{"already gone or replaced", false, nil, []string{"closed (abandoned)", "left alone"}, []string{"was removed", "failed", "being removed"}},
		{"an error wins over a removed flag", true, errors.New("x"), []string{"failed"}, []string{"was removed"}},
		{"the binding changed since it was read", false, fmt.Errorf("wrapped: %w", projection.ErrBindingChanged), []string{"left alone"}, []string{"was removed", "failed", "workflow unbind"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := base.AfterUnbind(tt.removed, tt.err)
			if !got.Unbind || got.Warning != "" || strings.Contains(got.Context, "\n") {
				t.Fatalf("AfterUnbind() = %+v, want a one-line context, Unbind kept, and no warning", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.Context, want) {
					t.Errorf("context %q does not contain %q", got.Context, want)
				}
			}
			for _, bad := range tt.notWant {
				if strings.Contains(got.Context, bad) {
					t.Errorf("context %q must not contain %q", got.Context, bad)
				}
			}
		})
	}

	// Anything that is not a closed workflow's result is returned unchanged.
	open := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning))
	if got := open.AfterUnbind(true, nil); got != open {
		t.Errorf("AfterUnbind() changed a result that asked for no unbind: %+v -> %+v", open, got)
	}
}

// TestHookFailureWarningsAreOneShortSanitizedLine: the warnings for a store that
// cannot be read and for a recovered panic carry text from outside (an error, a
// panic value), so they are sanitized like every other warning and bounded, and
// never span lines.
func TestHookFailureWarningsAreOneShortSanitizedLine(t *testing.T) {
	hostile := "line one\nline two \x1b[31mred\x1b[0m \u202eevil " + strings.Repeat("é", 5000)
	for name, got := range map[string]string{
		"store": projection.StoreWarning(errors.New(hostile)),
		"panic": projection.PanicWarning(projection.OccasionPrompt, hostile),
	} {
		t.Run(name, func(t *testing.T) {
			if strings.ContainsAny(got, "\n\r\x1b") || strings.ContainsRune(got, 0x202e) || !utf8.ValidString(got) {
				t.Errorf("warning %q is not one clean line", got)
			}
			if len(got) > 700 {
				t.Errorf("warning is %d bytes, want a short one", len(got))
			}
			if !strings.Contains(got, "line one line two") || !strings.Contains(got, "...") {
				t.Errorf("warning %q lost its detail or does not show that it was cut", got)
			}
			if !strings.HasPrefix(got, "labdrian:") {
				t.Errorf("warning %q does not start with the labdrian prefix", got)
			}
		})
	}
	if got := projection.StoreWarning(errors.New("boom")); !strings.Contains(got, "no workflow is projected") {
		t.Errorf("store warning %q does not say nothing is projected", got)
	}
	if got := projection.PanicWarning(projection.OccasionPrompt, "boom"); !strings.Contains(got, "boom") || !strings.Contains(got, "internal error") {
		t.Errorf("panic warning %q does not name the internal error", got)
	}
	// Panic values are not always strings.
	if got := projection.PanicWarning(projection.OccasionPrompt, errors.New("an error value")); !strings.Contains(got, "an error value") {
		t.Errorf("panic warning %q does not carry an error value", got)
	}
}

// TestPanicTextIsOneShortSanitizedLine: PanicText is the part of a recovered-panic
// warning that comes from outside, shared with hooks that word the rest
// themselves (the skills approve guard). Whatever the value is, it is one clean
// line, cut short, and PanicWarning carries exactly it.
func TestPanicTextIsOneShortSanitizedLine(t *testing.T) {
	hostile := "line one\nline two \x1b[31mred\x1b[0m " + string(rune(0x202e)) + "evil " + strings.Repeat("é", 5000)
	got := projection.PanicText(hostile)
	if strings.ContainsAny(got, "\n\r\x1b") || strings.ContainsRune(got, 0x202e) || !utf8.ValidString(got) {
		t.Errorf("PanicText %q is not one clean line", got)
	}
	if len(got) > 400 {
		t.Errorf("PanicText is %d bytes, want a short one", len(got))
	}
	if !strings.HasPrefix(got, "line one line two") || !strings.HasSuffix(got, "...") {
		t.Errorf("PanicText %q lost its detail or does not show that it was cut", got)
	}
	if got := projection.PanicText(errors.New("an error value")); got != "an error value" {
		t.Errorf("PanicText of an error = %q, want its message", got)
	}
	if warning := projection.PanicWarning(projection.OccasionToolCall, hostile); !strings.Contains(warning, projection.PanicText(hostile)) {
		t.Errorf("PanicWarning %q does not carry PanicText", warning)
	}
}

// TestPanicWarningNamesTheOccasionItHappenedIn: the recovered-panic warning used to say "the
// prompt was not affected" also when the panic happened in the PreToolUse gate, where no prompt
// is involved. Each occasion now says what it actually did to its own subject, and never
// mentions the other's.
func TestPanicWarningNamesTheOccasionItHappenedIn(t *testing.T) {
	prompt := projection.PanicWarning(projection.OccasionPrompt, "boom")
	if !strings.Contains(prompt, "the prompt was not affected") || strings.Contains(prompt, "tool call") {
		t.Errorf("prompt warning %q, want it to speak of the prompt only", prompt)
	}
	tool := projection.PanicWarning(projection.OccasionToolCall, "boom")
	if !strings.Contains(tool, "tool call") || strings.Contains(tool, "prompt") {
		t.Errorf("tool warning %q, want it to speak of the tool call only", tool)
	}
	for _, w := range []string{prompt, tool} {
		if !strings.Contains(w, "boom") || !strings.Contains(w, "internal error") || !strings.HasPrefix(w, "labdrian:") {
			t.Errorf("warning %q lost the labdrian prefix, the error, or its cause", w)
		}
	}
	// An occasion nobody named gets the neutral wording, not either subject.
	other := projection.PanicWarning(projection.OccasionUnknown, "boom")
	if strings.Contains(other, "prompt") || strings.Contains(other, "tool call") || !strings.Contains(other, "boom") {
		t.Errorf("warning for an unknown occasion %q, want neutral wording that keeps the cause", other)
	}
	if projection.OccasionUnknown != 0 {
		t.Error("the zero Occasion must be the unknown one, so a caller that sets none gets the neutral wording")
	}
}

// TestProjectOnlyEverUnbindsAClosedWorkflow pins that Unbind is the verdict of a
// state that was read and understood, never a default.
func TestProjectOnlyEverUnbindsAClosedWorkflow(t *testing.T) {
	for _, tt := range []struct {
		status     workflow.Status
		wantUnbind bool
	}{
		{workflow.StatusCreated, false},
		{workflow.StatusRunning, false},
		{workflow.StatusPaused, false},
		{workflow.StatusClosed, true},
		{workflow.StatusNone, false},
		{"exploded", false},
	} {
		t.Run(string(tt.status), func(t *testing.T) {
			got := project(ownedBinding(), loadedWorkflow("standalone-minimal", tt.status))
			if got.Unbind != tt.wantUnbind {
				t.Fatalf("Project(status %q).Unbind = %v, want %v", tt.status, got.Unbind, tt.wantUnbind)
			}
		})
	}
}

func TestProjectWarnsAboutAnUnrecognizedStatusAndProjectsNothing(t *testing.T) {
	for _, status := range []workflow.Status{workflow.StatusNone, "exploded"} {
		got := project(ownedBinding(), loadedWorkflow("odd", status))
		if got.Context != "" || got.Unbind || !strings.Contains(got.Warning, "unrecognized status") {
			t.Errorf("Project(status %q) = %+v, want a warning about the status only", status, got)
		}
	}
}

// --- the projected context --------------------------------------------------

// TestProjectContextOfARunningWorkflowExactly pins the whole text, so a change
// to the projection is a change to this test and shows up in review.
func TestProjectContextOfARunningWorkflowExactly(t *testing.T) {
	got := project(ownedBinding(), loadedWorkflow("standalone-minimal", workflow.StatusRunning, "authorize", "bound-scope"))
	if got.Warning != "" || got.Unbind {
		t.Fatalf("Project() = %+v, want a context only", got)
	}
	want := strings.Join([]string{
		"labdrian workflow projection: this repository follows the workflow below. Its state is read from the workflow log on disk, not from this session.",
		"workflow: wf-1 (project: proj-1)",
		"profile: standalone-minimal",
		"status: running",
		"goal: goal-1 (digest cccccccccccc)",
		"current stage: bound-scope",
		"next stage: execute-one-bounded-sequential-path",
		"memory plan (read-only: it executes no query and grants no memory write): scope=none sources=none project_id=none goal_id=none write=none",
		"omitted filters: project_id, goal_id",
		claudeLimits(t),
		"progress commands: labdrian workflow stage --project proj-1 --workflow wf-1 --stage <name>; labdrian workflow verify --project proj-1 --workflow wf-1 --goal <goal file>; labdrian workflow close --project proj-1 --workflow wf-1 --outcome completed|abandoned",
		"unavailable dependencies at the last recorded event: memory:engram, gentle-ai-review",
		"stages recorded (2): authorize, bound-scope",
	}, "\n")
	if got.Context != want {
		t.Fatalf("context differs.\ngot:\n%s\n\nwant:\n%s", got.Context, want)
	}
}

func TestProjectContextStatesTheStatusOfEachOpenWorkflow(t *testing.T) {
	created := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusCreated)).Context
	if line := lineWithPrefix(t, created, "status:"); !strings.Contains(line, "created") ||
		!strings.Contains(line, "labdrian workflow start --project proj-1 --workflow wf-1") {
		t.Errorf("created: status line %q does not say the workflow is not started, or how to start it", line)
	}
	noLineStartsWith(t, created, "PAUSED")

	running := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning, "authorize")).Context
	if !hasLine(running, "status: running") {
		t.Errorf("running: no plain status line in:\n%s", running)
	}
	noLineStartsWith(t, running, "PAUSED")

	paused := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusPaused, "authorize")).Context
	if !hasLine(paused, "status: paused") {
		t.Errorf("paused: no status line in:\n%s", paused)
	}
	notice := lineWithPrefix(t, paused, "PAUSED")
	for _, want := range []string{"paused", "do not advance", "labdrian workflow resume --project proj-1 --workflow wf-1"} {
		if !strings.Contains(strings.ToLower(notice), want) {
			t.Errorf("paused notice %q does not say %q", notice, want)
		}
	}
	// The gate that would deny tools belongs to a later step; the context must
	// not claim a control that does not exist.
	for _, claim := range []string{"blocked", "denied", "deny", "forbidden", "prevented"} {
		if strings.Contains(strings.ToLower(paused), claim) {
			t.Errorf("paused context claims a tool control (%q):\n%s", claim, paused)
		}
	}
}

func TestProjectContextNamesTheGoalAndTheStageProgress(t *testing.T) {
	tests := []struct {
		name          string
		profile       string
		stages        []string
		wantCurrent   string
		wantNext      string
		wantRecorded  string
		noNextGuide   bool // the next-stage line must say guidance is not available
		guideContains string
	}{
		{"nothing recorded yet", "odd", nil, "none yet", "authorize", "stages recorded (0): none", false, ""},
		{"part of the way", "odd", []string{"authorize", "explore"}, "explore", "resolve-uncertainty", "stages recorded (2): authorize, explore", false, ""},
		{"the last stage still to record", "standalone-minimal", []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check"}, "check", "report", "", false, ""},
		{"every declared stage recorded", "standalone-minimal", []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report"}, "report", "none: every declared stage is recorded", "", false, ""},
		{"stages that do not follow the profile's order", "odd", []string{"explore"}, "explore", "", "stages recorded (1): explore", true, `do not follow the declared order of profile "odd"`},
		{"more stages than the profile declares", "standalone-minimal", []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report", "extra"}, "extra", "", "", true, "do not follow the declared order"},
		{"a profile that does not resolve", "ghost", []string{"authorize"}, "authorize", "", "stages recorded (1): authorize", true, `profile "ghost" does not resolve`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := project(ownedBinding(), loadedWorkflow(tt.profile, workflow.StatusRunning, tt.stages...)).Context
			if !hasLine(ctx, "goal: goal-1 (digest cccccccccccc)") {
				t.Errorf("no goal line with the goal id and the first 12 digest characters in:\n%s", ctx)
			}
			if !hasLine(ctx, "current stage: "+tt.wantCurrent) {
				t.Errorf("current stage is not %q in:\n%s", tt.wantCurrent, ctx)
			}
			next := lineWithPrefix(t, ctx, "next stage:")
			if tt.noNextGuide {
				if !strings.Contains(next, "not available") || !strings.Contains(next, tt.guideContains) || !strings.Contains(next, "stage guidance is omitted") {
					t.Errorf("next-stage line %q does not say stage guidance is omitted because %q", next, tt.guideContains)
				}
			} else if next != "next stage: "+tt.wantNext {
				t.Errorf("next-stage line = %q, want %q", next, "next stage: "+tt.wantNext)
			}
			if tt.wantRecorded != "" && !hasLine(ctx, tt.wantRecorded) {
				t.Errorf("no line %q in:\n%s", tt.wantRecorded, ctx)
			}
		})
	}
}

// TestProjectContextTakesTheNextStageFromTheProfileNotFromTheLog: the workflow
// log never names the next stage, so the projection reads it from the profile;
// every built-in profile walks its own declared list.
func TestProjectContextTakesTheNextStageFromTheProfileNotFromTheLog(t *testing.T) {
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		profile, err := workflowprofile.Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		var recorded []string
		for i, stage := range profile.Stages {
			ctx := project(ownedBinding(), loadedWorkflow(name, workflow.StatusRunning, recorded...)).Context
			if !hasLine(ctx, "next stage: "+stage.Name) {
				t.Errorf("%s with %d stages recorded: next stage is not %q in:\n%s", name, i, stage.Name, ctx)
			}
			recorded = append(recorded, stage.Name)
		}
		ctx := project(ownedBinding(), loadedWorkflow(name, workflow.StatusRunning, recorded...)).Context
		if !hasLine(ctx, "next stage: none: every declared stage is recorded") {
			t.Errorf("%s with every stage recorded: no 'none' next stage in:\n%s", name, ctx)
		}
	}
}

func TestProjectContextStatesTheMemoryPlanOfTheProfile(t *testing.T) {
	const preface = "memory plan (read-only: it executes no query and grants no memory write): "
	for _, tt := range []struct {
		profile     string
		wantPlan    string
		wantOmitted string // "" when the plan omits nothing
	}{
		{"odd", "scope=project sources=engram,longterm-mem,procedural-skills project_id=proj-1 goal_id=none write=none", "omitted filters: goal_id"},
		{"maintenance", "scope=project sources=engram,longterm-mem,procedural-skills project_id=proj-1 goal_id=none write=none", "omitted filters: goal_id"},
		{"sdd", "scope=project sources=engram project_id=proj-1 goal_id=none write=none", "omitted filters: goal_id"},
		{"incident-recovery", "scope=goal sources=engram,procedural-skills project_id=proj-1 goal_id=goal-1 write=none", ""},
		{"standalone-minimal", "scope=none sources=none project_id=none goal_id=none write=none", "omitted filters: project_id, goal_id"},
	} {
		t.Run(tt.profile, func(t *testing.T) {
			ctx := project(ownedBinding(), loadedWorkflow(tt.profile, workflow.StatusRunning)).Context
			if !hasLine(ctx, preface+tt.wantPlan) {
				t.Errorf("no memory plan line %q in:\n%s", preface+tt.wantPlan, ctx)
			}
			if tt.wantOmitted == "" {
				noLineStartsWith(t, ctx, "omitted filters")
			} else if !hasLine(ctx, tt.wantOmitted) {
				t.Errorf("no line %q in:\n%s", tt.wantOmitted, ctx)
			}
		})
	}
}

func TestProjectContextSaysWhyAMemoryPlanIsNotAvailable(t *testing.T) {
	t.Run("the plan needs an identifier the goal does not have", func(t *testing.T) {
		w := loadedWorkflow("incident-recovery", workflow.StatusRunning)
		w.State.GoalID = "   "
		ctx := project(ownedBinding(), w).Context
		line := lineWithPrefix(t, ctx, "memory plan:")
		if !strings.Contains(line, "not available") || !strings.Contains(line, "goal_id is required") {
			t.Errorf("memory plan line %q does not say the plan is unavailable and why", line)
		}
		if !hasLine(ctx, "status: running") {
			t.Errorf("a missing memory plan must not hide the rest of the context:\n%s", ctx)
		}
	})
	t.Run("the profile does not resolve", func(t *testing.T) {
		ctx := project(ownedBinding(), loadedWorkflow("ghost", workflow.StatusRunning)).Context
		line := lineWithPrefix(t, ctx, "memory plan:")
		if !strings.Contains(line, "not available") || !strings.Contains(line, "ghost") {
			t.Errorf("memory plan line %q does not say the plan is unavailable and why", line)
		}
	})
}

func TestProjectContextListsOnlyTheUnavailableObservationsOfTheLastEvent(t *testing.T) {
	ctx := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning)).Context
	if !hasLine(ctx, "unavailable dependencies at the last recorded event: memory:engram, gentle-ai-review") {
		t.Errorf("wrong unavailable-dependencies line in:\n%s", ctx)
	}
	for _, leaked := range []string{"early-only", "memory:longterm-mem", "not probed"} {
		if strings.Contains(ctx, leaked) {
			t.Errorf("the context leaks %q: only the names of the last event's unavailable observations belong in it:\n%s", leaked, ctx)
		}
	}

	allAvailable := loadedWorkflow("odd", workflow.StatusRunning)
	last := &allAvailable.Events[len(allAvailable.Events)-1]
	for i := range last.Observations {
		last.Observations[i].Status = workflow.ObservationAvailable
	}
	noLineStartsWith(t, project(ownedBinding(), allAvailable).Context, "unavailable dependencies")

	noEvents := loadedWorkflow("odd", workflow.StatusRunning)
	noEvents.Events = nil
	noLineStartsWith(t, project(ownedBinding(), noEvents).Context, "unavailable dependencies")
}

// TestProjectContextCarriesTheCapabilityLimitsOfTheDeclaration compares the line
// with the declaration itself: it lists exactly the Claude Code claims that are
// not supported, so when a claim is upgraded the line changes with it and no
// hard-coded sentence goes stale.
func TestProjectContextCarriesTheCapabilityLimitsOfTheDeclaration(t *testing.T) {
	ctx := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning)).Context
	if line := lineWithPrefix(t, ctx, "capability limits"); line != claudeLimits(t) {
		t.Errorf("capability line = %q, want %q", line, claudeLimits(t))
	}
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	line := lineWithPrefix(t, ctx, "capability limits")
	for _, c := range d.Claims {
		mentioned := strings.Contains(line, string(c.Capability)+"=")
		if mentioned == (c.Status == capability.Supported) {
			t.Errorf("claim %s is %s but the capability line mentions it: %v", c.Capability, c.Status, mentioned)
		}
	}
}

// TestProjectContextListsTheSkillsCapabilityAsPartial pins the claim the README
// makes about a bound Claude Code session: its projected context lists
// skills=partial with the other limits, so a session that asks what it can rely on
// is told that skill projection is not fully proven. The comparison test above
// derives the line from the declaration, which would keep passing if the skills
// claim stopped being a limit; this one names the claim and its status.
func TestProjectContextListsTheSkillsCapabilityAsPartial(t *testing.T) {
	ctx := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning)).Context
	line := lineWithPrefix(t, ctx, "capability limits (claude): ")
	limits := strings.Split(strings.TrimPrefix(line, "capability limits (claude): "), ", ")

	// The literal is the README's wording, on purpose: it is what a session reads.
	const want = "skills=partial"
	for _, limit := range limits {
		if limit == want {
			return
		}
	}
	t.Errorf("limits %q do not include %q", limits, want)
}

func TestProjectContextNamesTheCommandsThatRecordProgress(t *testing.T) {
	ctx := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning)).Context
	line := lineWithPrefix(t, ctx, "progress commands:")
	for _, want := range []string{
		"labdrian workflow stage --project proj-1 --workflow wf-1 --stage <name>",
		"labdrian workflow verify --project proj-1 --workflow wf-1",
		"labdrian workflow close --project proj-1 --workflow wf-1",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("progress line %q does not name %q", line, want)
		}
	}
}

// TestProjectContextIsDeterministic: the same state gives the same bytes, and
// nothing that changes between two prompts (a time, a session, a prompt) is in
// them, so a new session is told exactly what the last one was.
func TestProjectContextIsDeterministic(t *testing.T) {
	first := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning, "authorize", "explore"))
	for i := 0; i < 20; i++ {
		if again := project(ownedBinding(), loadedWorkflow("odd", workflow.StatusRunning, "authorize", "explore")); again != first {
			t.Fatalf("call %d differs:\n%s\nvs\n%s", i, again.Context, first.Context)
		}
	}
	// The times in the state (the binding's, the events') never reach the text.
	later := loadedWorkflow("odd", workflow.StatusRunning, "authorize", "explore")
	for i := range later.Events {
		later.Events[i].At = "2031-01-01T00:00:00Z"
	}
	binding := ownedBinding()
	binding.Binding.BoundAt = "2031-01-01T00:00:00Z"
	if got := project(binding, later); got != first {
		t.Fatalf("a change of times changed the context:\n%s\nvs\n%s", got.Context, first.Context)
	}
	for _, timeText := range []string{"2026", "2031", "T10:", "00:00Z"} {
		if strings.Contains(first.Context, timeText) {
			t.Errorf("the context contains %q, which looks like a timestamp:\n%s", timeText, first.Context)
		}
	}
}

// --- sanitizing and bounding ------------------------------------------------

// TestProjectSanitizesEveryStringItPrints puts control and format characters and
// line breaks into every free-text field the workflow log can carry (they are
// only validated for shape, so a hand-edited log can hold anything), and checks
// that none reaches the output: the text stays the same number of lines, and
// every character is printable.
func TestProjectSanitizesEveryStringItPrints(t *testing.T) {
	zeroWidth := string(rune(0x200B))
	bidi := string(rune(0x202E))
	lineSep := string(rune(0x2028))
	dirty := "x\ny\r\nz\x00\x1b[31m" + zeroWidth + bidi + lineSep + "\x7f\ttail"

	clean := loadedWorkflow("odd", workflow.StatusRunning, "authorize")
	lines := strings.Count(project(ownedBinding(), clean).Context, "\n")

	w := loadedWorkflow("odd", workflow.StatusRunning, "authorize", dirty)
	w.State.GoalID = "goal-" + dirty
	w.Events[len(w.Events)-1].Observations = append(w.Events[len(w.Events)-1].Observations,
		workflow.Observation{Capability: "cap-" + dirty, Status: workflow.ObservationUnavailable})
	got := project(ownedBinding(), w)

	// The stage list is not in the profile's order, so one line changes shape,
	// but the number of lines must not grow with the injected line breaks.
	if n := strings.Count(got.Context, "\n"); n != lines {
		t.Errorf("the context has %d line breaks, want %d: a line break in a field split a line:\n%q", n, lines, got.Context)
	}
	assertPrintable(t, got.Context)
	// "x", a line break, "y", a line break, "z", an escape (removed), "[31m", a
	// zero-width space and a bidirectional override (removed), a line
	// separator, a delete character (removed), a tab, "tail".
	for _, want := range []string{"goal-x y z[31m tail", "cap-x y z[31m tail"} {
		if !strings.Contains(got.Context, want) {
			t.Errorf("the sanitized context does not contain %q (control characters removed, line breaks collapsed to a space):\n%s", want, got.Context)
		}
	}

	warning := project(projection.Loaded{Classification: projection.ClassificationMalformed, Detail: dirty}, nil).Warning
	assertPrintable(t, warning)
	if strings.Contains(warning, "\n") {
		t.Errorf("warning %q spans more than one line", warning)
	}
}

// assertPrintable requires every character of text to be printable, except that
// a line break separates lines.
func assertPrintable(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Fatalf("text is not valid UTF-8: %q", text)
	}
	for _, r := range text {
		if r != '\n' && !unicode.IsPrint(r) {
			t.Fatalf("text contains the non-printable character %U: %q", r, text)
		}
	}
}

// TestProjectContextIsBoundedForTheLargestWorkflowThatCanBeRecorded records the
// most stages a workflow may hold, each as long as a stage name may be, in
// four-byte characters, so the raw text is far over the bound.
func TestProjectContextIsBoundedForTheLargestWorkflowThatCanBeRecorded(t *testing.T) {
	name := strings.Repeat(string(rune(0x1F600)), workflow.MaxStageLength) // 256 characters of 4 bytes each
	stages := make([]string, workflow.MaxStages)
	for i := range stages {
		stages[i] = name
	}
	w := loadedWorkflow("odd", workflow.StatusPaused, stages...)
	w.State.GoalID = strings.Repeat("g", workflow.MaxGoalIDLength)
	last := &w.Events[len(w.Events)-1]
	for i := 0; i < 400; i++ {
		last.Observations = append(last.Observations, workflow.Observation{Capability: strings.Repeat("c", workflow.MaxObservationCapabilityLength), Status: workflow.ObservationUnavailable})
	}

	got := project(ownedBinding(), w).Context
	if len(got) > projection.MaxContextBytes {
		t.Fatalf("the context is %d bytes, want at most %d", len(got), projection.MaxContextBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatal("the truncated context is not valid UTF-8: it was cut inside a character")
	}
	marker := lineWithPrefix(t, got, "[labdrian:")
	if !strings.Contains(marker, "truncated") || !strings.Contains(marker, "16384") || !strings.HasSuffix(got, marker) {
		t.Errorf("the context does not end with an explicit truncation marker; last line %q", marker)
	}
	// What matters most comes before what can be large, so the cut removes the
	// lists, not the status or the memory plan.
	for _, want := range []string{"workflow: wf-1 (project: proj-1)", "status: paused", claudeLimits(t)} {
		if !hasLine(got, want) {
			t.Errorf("the truncated context lost the line %q", want)
		}
	}
	lineWithPrefix(t, got, "PAUSED")
	lineWithPrefix(t, got, "memory plan")
	lineWithPrefix(t, got, "progress commands:")
}

func TestProjectContextOfATypicalWorkflowIsNotTruncated(t *testing.T) {
	got := project(ownedBinding(), loadedWorkflow("incident-recovery", workflow.StatusRunning, "preserve-evidence", "classify")).Context
	if len(got) > projection.MaxContextBytes/4 {
		t.Errorf("a two-stage workflow needs %d bytes, want well under the %d byte bound", len(got), projection.MaxContextBytes)
	}
	noLineStartsWith(t, got, "[labdrian:")
}

// --- realistic states --------------------------------------------------------

// TestProjectOfRealisticStatesSaysSomethingWithinTheBound runs Project over the states the CLI
// produces: each has something to tell the session (the context of an open workflow, the note
// of a closed one) and no warning, within the bound. How that is written for Claude Code is the
// adapter's (engine/hookwire); the golden files of the hooks in engine/cmd pin the bytes.
func TestProjectOfRealisticStatesSaysSomethingWithinTheBound(t *testing.T) {
	for _, status := range []workflow.Status{workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused, workflow.StatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			got := project(ownedBinding(), loadedWorkflow("odd", status, "authorize"))
			if got.Context == "" || got.Warning != "" {
				t.Fatalf("Project() = %+v, want a context and no warning", got)
			}
			if len(got.Context) > projection.MaxContextBytes {
				t.Fatalf("the context is %d bytes, over the bound", len(got.Context))
			}
		})
	}
}
