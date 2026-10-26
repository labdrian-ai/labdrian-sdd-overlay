package presence_test

// The memory:procedural-skills signal (C23 M8 made it a look at the lock of the project; C10c makes it
// ask the project whether the lock can be used). The prober asks through the port it owns, one
// question, and puts the answer into words; it never opens the lock itself, which the static test pins.
// The answer the adapter gives is a reading of the lock made by the reader the session probe uses, so
// these tests hand the prober readings and look at the observations that come out.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

const (
	proceduralCapability = "memory:procedural-skills"
	unknownProject       = "the project directory is unknown, so the procedural skills lock was not looked for"
	// eAcute is a letter of two bytes, written as bytes so that no tool rewrites it.
	eAcute = "\xc3\xa9"
)

// project is the port answered with one reading, counting how often it is asked.
type project struct {
	reading presence.LockReading
	asked   int
}

var _ presence.ProjectContext = (*project)(nil)

func (p *project) ReadLock() presence.LockReading {
	p.asked++
	return p.reading
}

func proceduralOf(t *testing.T, reading presence.LockReading) workflow.Observation {
	t.Helper()
	return probeOne(t, presence.Prober{Project: &project{reading: reading}}, proceduralCapability)
}

func TestTheProceduralSignalIsAvailableWhenTheLockOfTheProjectIsValid(t *testing.T) {
	got := proceduralOf(t, presence.LockReading{State: presence.LockValid})
	want := "the procedural skills lock is present and valid (the skills it lists were not checked)"
	if got.Status != workflow.ObservationAvailable || got.Detail != want {
		t.Errorf("observation = %+v, want available with %q", got, want)
	}
}

func TestTheProceduralSignalIsUnavailableWhenTheProjectHasNoLock(t *testing.T) {
	got := proceduralOf(t, presence.LockReading{State: presence.LockAbsent})
	want := "the procedural skills lock was not found: no such file"
	if got.Status != workflow.ObservationUnavailable || got.Detail != want {
		t.Errorf("observation = %+v, want unavailable with %q", got, want)
	}
}

// A lock that is there and cannot be used is unavailable, in the words the session probe gives: the
// stat the prober used to make said available for any regular file.
func TestTheProceduralSignalIsUnavailableWhenTheLockIsThereButCannotBeUsed(t *testing.T) {
	got := proceduralOf(t, presence.LockReading{State: presence.LockInvalid, Reason: "parse project lock: unsupported version 2, want 1"})
	want := "the procedural skills lock cannot be read: parse project lock: unsupported version 2, want 1"
	if got.Status != workflow.ObservationUnavailable || got.Detail != want {
		t.Errorf("observation = %+v, want unavailable with %q", got, want)
	}
}

// A project the port cannot name is unknown, whether there is no port, the port says so, or it says
// something the prober has no word for: nothing is looked for and nothing is ever available.
func TestTheProceduralSignalIsUnavailableWhenTheProjectIsUnknown(t *testing.T) {
	for name, p := range map[string]presence.Prober{
		"no port":                 {},
		"the port says unknown":   {Project: &project{reading: presence.LockReading{State: presence.LockProjectUnknown}}},
		"the zero reading":        {Project: &project{}},
		"a state with no meaning": {Project: &project{reading: presence.LockReading{State: presence.LockState(99), Reason: "something"}}},
	} {
		t.Run(name, func(t *testing.T) {
			got := probeOne(t, p, proceduralCapability)
			if got.Status != workflow.ObservationUnavailable || got.Detail != unknownProject {
				t.Errorf("observation = %+v, want unavailable with %q", got, unknownProject)
			}
		})
	}
}

// The reason is the adapter's and can be as long as the lock file is, so the prober makes it fit what a
// workflow event accepts: one line, valid text, cut with a mark where it is cut.
func TestTheReasonOfAnInvalidLockIsMadeToFitAnObservation(t *testing.T) {
	const prefix = "the procedural skills lock cannot be read: "
	for name, tc := range map[string]struct {
		reason string
		want   string // "" when only the bounds are pinned
	}{
		"a very long reason":                        {reason: strings.Repeat("x", 10*workflow.MaxObservationDetailLength)},
		"a long reason of multi-byte runes":         {reason: strings.Repeat(eAcute, 10*workflow.MaxObservationDetailLength)},
		"lines and tabs become one line":            {reason: "first line\nsecond\tline\r\n  third", want: prefix + "first line second line third"},
		"bytes that are not text are dropped":       {reason: "bad \xff\xfe bytes", want: prefix + "bad bytes"},
		"nothing but blanks is no reason at all":    {reason: " \n\t ", want: prefix + "no reason was given"},
		"an empty reason is no reason at all":       {reason: "", want: prefix + "no reason was given"},
		"control characters become blanks":          {reason: "red \x1b[31m text\x00end", want: prefix + "red [31m text end"},
		"a reason that fits is left as it is":       {reason: "parse project lock: EOF", want: prefix + "parse project lock: EOF"},
		"a reason at the limit is not cut":          {reason: strings.Repeat("y", presence.MaxReasonRunes), want: prefix + strings.Repeat("y", presence.MaxReasonRunes)},
		"a reason one over the limit is cut":        {reason: strings.Repeat("y", presence.MaxReasonRunes+1), want: prefix + strings.Repeat("y", presence.MaxReasonRunes-3) + "..."},
		"a reason cut in the middle of a multibyte": {reason: strings.Repeat(eAcute, presence.MaxReasonRunes+1), want: prefix + strings.Repeat(eAcute, presence.MaxReasonRunes-3) + "..."},
	} {
		t.Run(name, func(t *testing.T) {
			got := proceduralOf(t, presence.LockReading{State: presence.LockInvalid, Reason: tc.reason})
			if got.Status != workflow.ObservationUnavailable || !strings.HasPrefix(got.Detail, prefix) {
				t.Fatalf("observation = %+v, want unavailable, beginning %q", got, prefix)
			}
			if n := len([]rune(got.Detail)); n > workflow.MaxObservationDetailLength || !utf8.ValidString(got.Detail) || strings.IndexFunc(got.Detail, unicode.IsControl) >= 0 {
				t.Errorf("detail has %d runes (valid UTF-8: %v) and may hold control characters, want at most %d runes of one line of text", n, utf8.ValidString(got.Detail), workflow.MaxObservationDetailLength)
			}
			if tc.want != "" && got.Detail != tc.want {
				t.Errorf("detail = %q, want %q", got.Detail, tc.want)
			}
			if tc.want == "" && !strings.HasSuffix(got.Detail, "...") {
				t.Errorf("detail %q of a reason that was cut does not say so", got.Detail)
			}
		})
	}
}

// The project is asked only for the procedural skills, once for each time they are asked about, so a
// probe of anything else never reads the project.
func TestTheProjectIsAskedOnlyWhenTheProceduralSkillsAre(t *testing.T) {
	spy := &project{reading: presence.LockReading{State: presence.LockValid}}
	p := presence.Prober{Home: fixtureHome(t), Path: binDir(t), Project: spy}
	if _, err := p.Probe(context.Background(), []string{"memory:engram", "memory:longterm-mem", "gentle-ai-review", "credentials:pi", "unknown"}); err != nil {
		t.Fatal(err)
	}
	if spy.asked != 0 {
		t.Errorf("the project was asked %d times for capabilities that are not the procedural skills, want none", spy.asked)
	}
	got, err := p.Probe(context.Background(), []string{proceduralCapability, "memory:engram", proceduralCapability})
	if err != nil || len(got) != 3 || spy.asked != 2 {
		t.Errorf("Probe = %+v, %v with the project asked %d times, want three observations and two questions", got, err, spy.asked)
	}
}

// A context that is done stops the probe before the project is asked, as it stops it before any stat.
func TestTheProjectIsNotAskedOnceTheContextIsDone(t *testing.T) {
	spy := &project{reading: presence.LockReading{State: presence.LockValid}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := presence.Prober{Project: spy}.Probe(ctx, []string{proceduralCapability})
	if !errors.Is(err, context.Canceled) || got != nil || spy.asked != 0 {
		t.Errorf("Probe() = %v, %v with the project asked %d times, want no observations, the context's error and no question", got, err, spy.asked)
	}
}
