package main

// The session-time probe of C23 M8 (docs/architecture/adr-c23-m8-session-availability.md): it asks
// each source of the plan whether it can be read, at the same time and with a deadline, and the
// SessionStart hook turns an unavailable one into a line of the instructions block.

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
)

// fakeRunner stands for the programs of the machine: a program it knows runs through run, any other
// is not on PATH.
type fakeRunner struct {
	known map[string]func(ctx context.Context, args []string) error
	ran   atomic.Int32
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if _, ok := f.known[name]; ok {
		return name, nil
	}
	return "", errors.New("not found")
}

func (f *fakeRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	f.ran.Add(1)
	return nil, f.known[bin](ctx, args)
}

type fakeLock struct{ lock projection.ProceduralSkills }

func (f fakeLock) Load(string) projection.ProceduralSkills { return f.lock }

func succeeds(context.Context, []string) error { return nil }

// probeOf is a probe over runner whose longterm-mem binary is "longterm-mem".
func probeOf(runner *fakeRunner, registered bool, lock projection.ProceduralSkills) sessionAvailability {
	return sessionAvailability{
		runner:          runner,
		timeout:         200 * time.Millisecond,
		longtermBinary:  "longterm-mem",
		entryRegistered: func() bool { return registered },
		procedural:      fakeLock{lock},
	}
}

func byName(got []projection.SourceAvailability) map[memoryscope.Source]projection.SourceAvailability {
	m := map[memoryscope.Source]projection.SourceAvailability{}
	for _, a := range got {
		m[a.Source] = a
	}
	return m
}

var allSources = []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceLongtermMem, memoryscope.SourceProceduralSkills}

func TestEverySourceThatAnswersIsAvailable(t *testing.T) {
	runner := &fakeRunner{known: map[string]func(context.Context, []string) error{
		"engram": func(_ context.Context, args []string) error {
			if strings.Join(args, " ") != "stats" {
				t.Errorf("engram was run with %v, want stats", args)
			}
			return nil
		},
		"longterm-mem": func(_ context.Context, args []string) error {
			if strings.Join(args, " ") != "status" {
				t.Errorf("longterm-mem was run with %v, want status", args)
			}
			return nil
		},
	}}
	got := probeOf(runner, true, projection.ProceduralSkills{Locked: true}).Probe("/repo", allSources)
	if len(got) != 3 {
		t.Fatalf("Probe gave %d answers, want one per source: %+v", len(got), got)
	}
	for i, a := range got {
		if a.Source != allSources[i] || !a.Available || a.Detail != "" {
			t.Errorf("answer %d = %+v, want %s available with no detail", i, a, allSources[i])
		}
	}
}

func TestASourceThatFailsIsUnavailableWithTheReason(t *testing.T) {
	cases := []struct {
		name       string
		runner     map[string]func(context.Context, []string) error
		registered bool
		lock       projection.ProceduralSkills
		source     memoryscope.Source
		want       string
	}{
		{"engram is not on PATH", nil, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the engram program was not found"},
		{"engram fails", map[string]func(context.Context, []string) error{
			"engram": func(context.Context, []string) error { return errors.New("exit status 1") },
		}, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the engram program failed: exit status 1"},
		{"longterm-mem has no binary", nil, true, projection.ProceduralSkills{}, memoryscope.SourceLongtermMem, "the longterm-mem program was not found"},
		{"longterm-mem fails", map[string]func(context.Context, []string) error{
			"longterm-mem": func(context.Context, []string) error { return errors.New("exit status 2") },
		}, true, projection.ProceduralSkills{}, memoryscope.SourceLongtermMem, "the longterm-mem program failed: exit status 2"},
		{"longterm-mem answers but no runtime has its entry", map[string]func(context.Context, []string) error{
			"longterm-mem": succeeds,
		}, false, projection.ProceduralSkills{}, memoryscope.SourceLongtermMem, "no longterm-mem MCP entry is registered in Claude Code, OpenCode or Codex"},
		{"there is no procedural lock", nil, true, projection.ProceduralSkills{}, memoryscope.SourceProceduralSkills, "no procedural skills lock: none installed"},
		{"the procedural lock is unreadable", nil, true, projection.ProceduralSkills{Unreadable: "unsupported version 2"}, memoryscope.SourceProceduralSkills, "the procedural skills lock cannot be read: unsupported version 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := probeOf(&fakeRunner{known: tc.runner}, tc.registered, tc.lock).Probe("/repo", []memoryscope.Source{tc.source})
			if len(got) != 1 || got[0].Source != tc.source || got[0].Available || got[0].Detail != tc.want {
				t.Errorf("Probe = %+v, want %s unavailable with %q", got, tc.source, tc.want)
			}
		})
	}
}

// Any runtime's entry makes longterm-mem available (the owner's decision 3); the lock is read for the
// directory of the session.
func TestTheLockIsReadForTheDirectoryOfTheSession(t *testing.T) {
	var asked string
	p := probeOf(&fakeRunner{}, true, projection.ProceduralSkills{})
	p.procedural = lockSpy{asked: &asked}
	p.Probe("/the/repo", []memoryscope.Source{memoryscope.SourceProceduralSkills})
	if asked != "/the/repo" {
		t.Errorf("the lock was read for %q, want the session's directory", asked)
	}
}

type lockSpy struct{ asked *string }

func (s lockSpy) Load(dir string) projection.ProceduralSkills {
	*s.asked = dir
	return projection.ProceduralSkills{Locked: true}
}

// A source that never answers costs the deadline, not more, and the others are not held back: all are
// asked at the same time.
func TestASourceThatNeverAnswersIsUnavailableWithinTheDeadline(t *testing.T) {
	var running, most atomic.Int32
	hang := func(ctx context.Context, _ []string) error {
		now := running.Add(1)
		defer running.Add(-1)
		for {
			if prev := most.Load(); now <= prev || most.CompareAndSwap(prev, now) {
				break
			}
		}
		<-ctx.Done()
		return ctx.Err()
	}
	runner := &fakeRunner{known: map[string]func(context.Context, []string) error{"engram": hang, "longterm-mem": hang}}
	start := time.Now()
	got := byName(probeOf(runner, true, projection.ProceduralSkills{Locked: true}).Probe("/repo", allSources))
	if most.Load() != 2 {
		t.Errorf("at most %d hung sources were being asked at once, want both (2): the sources must be asked at the same time", most.Load())
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Errorf("the probe took %v with two hung sources and a 200ms deadline each", elapsed)
	}
	for _, s := range []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceLongtermMem} {
		if a := got[s]; a.Available || !strings.Contains(a.Detail, "did not answer within 200ms") {
			t.Errorf("%s = %+v, want unavailable because it did not answer within 200ms", s, a)
		}
	}
	if !got[memoryscope.SourceProceduralSkills].Available {
		t.Errorf("procedural-skills = %+v, want it available: a hung program does not hold it back", got[memoryscope.SourceProceduralSkills])
	}
}

// A source the plan could name that the probe has no check for is unavailable, never silently fine.
func TestASourceWithNoCheckIsUnavailable(t *testing.T) {
	got := probeOf(&fakeRunner{}, true, projection.ProceduralSkills{}).Probe("/repo", []memoryscope.Source{"something-else"})
	if len(got) != 1 || got[0].Available || got[0].Detail == "" {
		t.Errorf("Probe = %+v, want the unknown source unavailable with a reason", got)
	}
}

// A probe that panics costs its source, never the hook.
func TestAProbeThatPanicsMakesItsSourceUnavailable(t *testing.T) {
	runner := &fakeRunner{known: map[string]func(context.Context, []string) error{
		"engram": func(context.Context, []string) error { panic("boom") },
	}}
	got := probeOf(runner, true, projection.ProceduralSkills{Locked: true}).Probe("/repo", []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceProceduralSkills})
	if got[0].Available || !strings.Contains(got[0].Detail, "boom") || !got[1].Available {
		t.Errorf("Probe = %+v, want engram unavailable with the panic and procedural-skills answered", got)
	}
}

// --- the hook ---------------------------------------------------------------------------------

type fixedProber struct{ unavailable map[memoryscope.Source]string }

func (f fixedProber) Probe(_ string, sources []memoryscope.Source) []projection.SourceAvailability {
	out := make([]projection.SourceAvailability, len(sources))
	for i, s := range sources {
		detail, down := f.unavailable[s]
		out[i] = projection.SourceAvailability{Source: s, Available: !down, Detail: detail}
	}
	return out
}

// The SessionStart hook says which sources of the plan cannot be read; the gate-task rewrite and
// 'memory plan --instructions' use the block without the probe.
func TestSessionStartSaysWhichSourcesOfThePlanCannotBeRead(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	d := e.deps
	d.sourceProber = func() app.SourceAvailabilityProber {
		return fixedProber{unavailable: map[memoryscope.Source]string{memoryscope.SourceLongtermMem: "down in the test"}}
	}
	r := runProjectionArgsWith(d, []string{"hook", "--event", "SessionStart"}, sessionStartInput(t, e.repo, "startup"), e.dir)
	want := "- Do not read longterm-mem: it is unavailable in this session (down in the test)."
	if len(r.codes) != 1 || r.codes[0] != 0 || !strings.Contains(r.stdout, want) || strings.Contains(r.stdout, "Do not read engram") {
		t.Errorf("exits %v, stdout %s, want exit 0 and exactly the line %q", r.codes, r.stdout, want)
	}

	plain := runProjectionArgsWith(e.deps, []string{"hook", "--event", "SessionStart"}, sessionStartInput(t, e.repo, "startup"), e.dir)
	if strings.Contains(plain.stdout, "Do not read") {
		t.Errorf("a hook with no probe in the deps says not to read a source: %s", plain.stdout)
	}
	if got := boundInstructions(d, e.dir)(e.repo); strings.Contains(got, "Do not read") {
		t.Errorf("the gate-task and 'memory plan' route probes: %q", got)
	}
}
