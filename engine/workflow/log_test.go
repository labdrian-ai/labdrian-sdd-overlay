package workflow

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// logOf renders events as the JSONL a workflow log holds: one canonical line per
// event, each ending in a newline.
func logOf(t *testing.T, events ...WorkflowEvent) []byte {
	t.Helper()
	var log []byte
	for _, e := range events {
		line, err := e.MarshalLine()
		if err != nil {
			t.Fatalf("MarshalLine() = %v, want nil", err)
		}
		log = append(log, line...)
	}
	return log
}

func TestClassifyLogAcceptsAnOwnedLogAndReplaysIt(t *testing.T) {
	created, started := validCreatedEvent(), validStartedEvent()
	got := ClassifyLog("proj-1", "wf-1", logOf(t, created, started))
	if got.Classification != ClassificationOwned {
		t.Fatalf("ClassifyLog() = %q (%s), want %q", got.Classification, got.Detail, ClassificationOwned)
	}
	if len(got.Events) != 2 || got.Events[0].Kind != KindCreated || got.Events[1].Kind != KindStarted {
		t.Errorf("events = %+v, want created then started", got.Events)
	}
	if got.State.Status != StatusRunning {
		t.Errorf("state = %q, want %q", got.State.Status, StatusRunning)
	}
	if got.Detail != "" {
		t.Errorf("detail = %q, want none for an owned log", got.Detail)
	}
}

// Every way a log can fail to be ours is one classification, with the detail a
// person debugging the file by hand needs: line numbers are 1-based, as an
// editor's gutter shows them.
func TestClassifyLogNamesWhyALogIsNotOwned(t *testing.T) {
	created := validCreatedEvent()
	createdLine := string(logOf(t, created))

	foreignWorkflow := validCreatedEvent()
	foreignWorkflow.WorkflowID = "other-workflow"
	foreignProject := validCreatedEvent()
	foreignProject.ProjectID = "other-project"

	brokenChain := validStartedEvent()
	brokenChain.PrevDigest = strings.Repeat("f", 64)

	startedFirst := validCreatedEvent()
	startedFirst.Kind = KindStarted
	startedFirst.GoalID, startedFirst.GoalDigest, startedFirst.Profile = "", "", ""

	tests := []struct {
		name       string
		data       []byte
		want       Classification
		wantDetail string
	}{
		{"empty", nil, ClassificationMalformed, "workflow log is empty"},
		{"not UTF-8", []byte{0xff, 0xfe, 0xfd, '\n'}, ClassificationMalformed, "workflow log is not valid UTF-8"},
		{"no trailing newline", []byte(strings.TrimSuffix(createdLine, "\n")), ClassificationMalformed, "does not end with a trailing newline"},
		{"a blank line", []byte(createdLine + "\n"), ClassificationMalformed, "line 2 is blank"},
		{"not JSON", []byte("not json at all\n"), ClassificationMalformed, "line 1 is not valid JSON"},
		{"not JSON on a later line", []byte(createdLine + "{nope\n"), ClassificationMalformed, "line 2 is not valid JSON"},
		{"JSON that is not an event", []byte(`{"hello":"world"}` + "\n"), ClassificationForeign, "line 1 is not a workflow event we recognize"},
		{"another workflow's event", logOf(t, foreignWorkflow), ClassificationForeign, `line 1 declares project_id="proj-1" workflow_id="other-workflow", want "proj-1"/"wf-1"`},
		{"another project's event", logOf(t, foreignProject), ClassificationForeign, `project_id="other-project"`},
		{"a broken hash chain", logOf(t, created, brokenChain), ClassificationDrifted, "prev_digest"},
		{"an illegal first transition", logOf(t, startedFirst), ClassificationDrifted, "first event must be a created event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyLog("proj-1", "wf-1", tt.data)
			if got.Classification != tt.want {
				t.Fatalf("ClassifyLog() = %q (%s), want %q", got.Classification, got.Detail, tt.want)
			}
			if !strings.Contains(got.Detail, tt.wantDetail) {
				t.Errorf("detail = %q, want it to contain %q", got.Detail, tt.wantDetail)
			}
			if len(got.Events) != 0 {
				t.Errorf("events = %d, want none: only an owned log carries them", len(got.Events))
			}
		})
	}
}

// A log is read whole, so its size is bounded: one byte over is malformed and says
// by how much, one byte under is not judged on size at all.
func TestClassifyLogBoundsTheSizeItWillRead(t *testing.T) {
	over := make([]byte, MaxLogBytes+1)
	got := ClassifyLog("proj-1", "wf-1", over)
	want := fmt.Sprintf("workflow log is %d bytes, exceeding the maximum of %d", MaxLogBytes+1, MaxLogBytes)
	if got.Classification != ClassificationMalformed || got.Detail != want {
		t.Errorf("ClassifyLog(over) = %q, %q, want malformed and %q", got.Classification, got.Detail, want)
	}

	under := ClassifyLog("proj-1", "wf-1", over[:MaxLogBytes])
	if under.Classification != ClassificationMalformed || strings.Contains(under.Detail, "exceeding") {
		t.Errorf("ClassifyLog(at the bound) = %q, %q, want a failure that is not about size", under.Classification, under.Detail)
	}
}

// OversizedLog is the one answer for a log that is too large, whether ClassifyLog
// was handed the bytes or an adapter knows the size without having read them.
func TestOversizedLogNamesTheSizeAndIsTheAnswerClassifyLogGives(t *testing.T) {
	const size = 3 * MaxLogBytes
	got := OversizedLog(size)
	want := fmt.Sprintf("workflow log is %d bytes, exceeding the maximum of %d", int64(size), MaxLogBytes)
	if got.Classification != ClassificationMalformed || got.Detail != want || len(got.Events) != 0 {
		t.Errorf("OversizedLog(%d) = %+v, want malformed, detail %q, no events", int64(size), got, want)
	}

	handed := ClassifyLog("proj-1", "wf-1", make([]byte, MaxLogBytes+1))
	if again := OversizedLog(MaxLogBytes + 1); !reflect.DeepEqual(handed, again) {
		t.Errorf("ClassifyLog(over) = %+v, OversizedLog(over) = %+v, want the same answer", handed, again)
	}
}

func owned(t *testing.T, events ...WorkflowEvent) Loaded {
	t.Helper()
	got := ClassifyLog("proj-1", "wf-1", logOf(t, events...))
	if got.Classification != ClassificationOwned {
		t.Fatalf("fixture log classified %q (%s), want owned", got.Classification, got.Detail)
	}
	return got
}

func TestAdmitAppendAcceptsTheCreatedEventOfAnAbsentWorkflowAndReturnsItsLine(t *testing.T) {
	created := validCreatedEvent()
	line, err := AdmitAppend("proj-1", "wf-1", Loaded{Classification: ClassificationAbsent}, created)
	if err != nil {
		t.Fatalf("AdmitAppend() = %v, want nil", err)
	}
	if want := logOf(t, created); string(line) != string(want) {
		t.Errorf("line = %q, want the canonical line %q", line, want)
	}
}

func TestAdmitAppendAcceptsTheNextEventOfAnOwnedWorkflow(t *testing.T) {
	loaded := owned(t, validCreatedEvent())
	started := validStartedEvent()
	line, err := AdmitAppend("proj-1", "wf-1", loaded, started)
	if err != nil {
		t.Fatalf("AdmitAppend() = %v, want nil", err)
	}
	if want := logOf(t, started); string(line) != string(want) {
		t.Errorf("line = %q, want %q", line, want)
	}
}

// An absent workflow has no prior event, so what may start it is checked on its
// own terms, as a first line of defense beside CheckTransition.
func TestAdmitAppendHoldsTheFirstEventOfAWorkflowToTheCreatedEventAtSeqZero(t *testing.T) {
	bareStarted := validStartedEvent()
	bareStarted.Seq, bareStarted.PrevDigest = 0, ""
	skipped := validCreatedEvent()
	skipped.Seq, skipped.PrevDigest = 1, strings.Repeat("a", 64)
	chained := validCreatedEvent()
	chained.PrevDigest = strings.Repeat("a", 64)
	mismatched := validCreatedEvent()
	mismatched.ProjectID = "other-project"

	tests := []struct {
		name string
		next WorkflowEvent
		want string
	}{
		{"a first event that is not created", bareStarted, `the first event must be a created event at seq 0, got kind "started" at seq 0`},
		{"a first event at seq 1", skipped, "the first event must be a created event at seq 0"},
		{"a first event with a prev_digest", chained, "prev_digest"},
		{"an event of another project", mismatched, `event project_id/workflow_id ("other-project"/"wf-1") does not match the requested workflow ("proj-1"/"wf-1")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, err := AdmitAppend("proj-1", "wf-1", Loaded{Classification: ClassificationAbsent}, tt.next)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("AdmitAppend() = %v, want an error containing %q", err, tt.want)
			}
			if line != nil {
				t.Errorf("line = %q, want none for a refused event", line)
			}
		})
	}
}

func TestAdmitAppendRefusesAnEventThatDoesNotExtendTheOwnedLog(t *testing.T) {
	loaded := owned(t, validCreatedEvent())
	stale := validCreatedEvent() // the same event again: the log already moved past seq 0
	skipped := validStartedEvent()
	skipped.Seq = 5
	wrongPrev := validStartedEvent()
	wrongPrev.PrevDigest = strings.Repeat("f", 64)
	illegal := validStartedEvent()
	illegal.Kind = KindPaused

	t.Run("the seq of an event already stored is stale, not a conflict", func(t *testing.T) {
		err := mustRefuse(t, loaded, stale)
		if !errors.Is(err, ErrStaleSeq) || errors.Is(err, ErrAppendConflict) {
			t.Errorf("err = %v, want ErrStaleSeq and not ErrAppendConflict", err)
		}
		if !strings.Contains(err.Error(), "got 0, want 1") {
			t.Errorf("err = %v, want it to say which seq was expected", err)
		}
	})
	t.Run("a seq that skips ahead is stale too", func(t *testing.T) {
		if err := mustRefuse(t, loaded, skipped); !errors.Is(err, ErrStaleSeq) {
			t.Errorf("err = %v, want ErrStaleSeq", err)
		}
	})
	t.Run("a prev_digest that is not the last event's digest", func(t *testing.T) {
		err := mustRefuse(t, loaded, wrongPrev)
		if err == nil || !strings.Contains(err.Error(), "does not match the last stored event's digest") {
			t.Errorf("err = %v, want it to name the digest mismatch", err)
		}
	})
	t.Run("a transition the lifecycle does not allow", func(t *testing.T) {
		if err := mustRefuse(t, loaded, illegal); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("err = %v, want ErrInvalidTransition", err)
		}
	})
}

func mustRefuse(t *testing.T, loaded Loaded, next WorkflowEvent) error {
	t.Helper()
	line, err := AdmitAppend("proj-1", "wf-1", loaded, next)
	if err == nil {
		t.Fatalf("AdmitAppend() = nil with line %q, want a refusal", line)
	}
	if line != nil {
		t.Errorf("line = %q, want none for a refused event", line)
	}
	return err
}

// A log that is not ours is never written to: each classification has its own
// named refusal, and the detail comes along.
func TestAdmitAppendRefusesALogThatIsNotOurs(t *testing.T) {
	tests := []struct {
		name  string
		class Classification
		want  error
	}{
		{"foreign", ClassificationForeign, ErrRefuseForeignState},
		{"malformed", ClassificationMalformed, ErrRefuseMalformedState},
		{"drifted", ClassificationDrifted, ErrRefuseDriftedState},
		{"unavailable", ClassificationUnavailable, ErrStateUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mustRefuse(t, Loaded{Classification: tt.class, Detail: "because of the detail"}, validCreatedEvent())
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), "because of the detail") {
				t.Errorf("err = %v, want %v carrying the detail", err, tt.want)
			}
		})
	}

	t.Run("a classification from the future", func(t *testing.T) {
		err := mustRefuse(t, Loaded{Classification: "quantum"}, validCreatedEvent())
		if !strings.Contains(err.Error(), `unknown classification "quantum"`) {
			t.Errorf("err = %v, want the unknown classification named", err)
		}
	})
}

// What an event is, on its own, is judged before the state of the log is: an
// invalid event is refused with the reason it is invalid, whatever the log holds.
func TestAdmitAppendJudgesTheEventBeforeTheLog(t *testing.T) {
	invalid := validCreatedEvent()
	invalid.GoalDigest = "not-a-digest"
	err := mustRefuse(t, Loaded{Classification: ClassificationForeign, Detail: "d"}, invalid)
	if errors.Is(err, ErrRefuseForeignState) || !strings.Contains(err.Error(), "goal_digest") {
		t.Errorf("err = %v, want the event's own invalidity, not the state of the log", err)
	}
}
