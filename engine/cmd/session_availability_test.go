package main

// The session-time probe of C23 M8 (docs/architecture/adr-c23-m8-session-availability.md): it asks
// each source of the plan whether it can be read, at the same time and with a deadline, and the
// SessionStart hook turns an unavailable one into a line of the instructions block.

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// probeOf is a probe over runner whose longterm-mem binary is "longterm-mem" and whose memorycard
// store is the path given ("" when none can be located).
func probeOf(runner *fakeRunner, store string, registered bool, lock projection.ProceduralSkills) sessionAvailability {
	return sessionAvailability{
		runner:          runner,
		timeout:         200 * time.Millisecond,
		memorycardStore: store,
		stat:            os.Stat,
		longtermBinary:  "longterm-mem",
		entryRegistered: func() bool { return registered },
		procedural:      fakeLock{lock},
	}
}

// storeFile makes the empty regular file a memorycard store is to the probe's stat.
func storeFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
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
		"engram": func(context.Context, []string) error {
			t.Error("the engram program was run: the probe of source engram asks memorycard")
			return nil
		},
		"memorycard": func(_ context.Context, args []string) error {
			if want := "list --topic-prefix labdrian/session-probe/ --project labdrian-session-probe"; strings.Join(args, " ") != want {
				t.Errorf("memorycard was run with %v, want %s", args, want)
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
	got := probeOf(runner, storeFile(t), true, projection.ProceduralSkills{Locked: true}).Probe("/repo", allSources)
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
	failing := func(msg string) map[string]func(context.Context, []string) error {
		return map[string]func(context.Context, []string) error{
			"memorycard": func(context.Context, []string) error { return errors.New(msg) },
		}
	}
	dir := func(t *testing.T) string { return t.TempDir() }
	underAFile := func(t *testing.T) string { return filepath.Join(storeFile(t), "memory.db") }
	absent := func(t *testing.T) string { return filepath.Join(t.TempDir(), "memory.db") }
	cases := []struct {
		name       string
		runner     map[string]func(context.Context, []string) error
		store      func(*testing.T) string // nil: no store can be located
		noMemcard  bool                    // the probe must not run anything for the source
		registered bool
		lock       projection.ProceduralSkills
		source     memoryscope.Source
		want       string // <store> stands for the path store gives
	}{
		{"no store can be located", map[string]func(context.Context, []string) error{"memorycard": succeeds}, nil, true, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the memorycard store cannot be located: MEMORYCARD_DB is not set and no home directory is known"},
		{"the store does not exist", map[string]func(context.Context, []string) error{"memorycard": succeeds}, absent, true, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the memorycard store <store> does not exist"},
		{"the store is a directory", map[string]func(context.Context, []string) error{"memorycard": succeeds}, dir, true, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the memorycard store <store> is not a regular file"},
		{"the store is under a regular file", map[string]func(context.Context, []string) error{"memorycard": succeeds}, underAFile, true, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the memorycard store <store> cannot be read: stat <store>: not a directory"},
		{"memorycard is not on PATH", nil, storeFile, false, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the memorycard program was not found"},
		{"memorycard fails", failing("exit status 1"), storeFile, false, true, projection.ProceduralSkills{}, memoryscope.SourceEngram, "the memorycard program failed: exit status 1"},
		{"longterm-mem has no binary", nil, nil, false, true, projection.ProceduralSkills{}, memoryscope.SourceLongtermMem, "the longterm-mem program was not found"},
		{"longterm-mem fails", map[string]func(context.Context, []string) error{
			"longterm-mem": func(context.Context, []string) error { return errors.New("exit status 2") },
		}, nil, false, true, projection.ProceduralSkills{}, memoryscope.SourceLongtermMem, "the longterm-mem program failed: exit status 2"},
		{"longterm-mem answers but no runtime has its entry", map[string]func(context.Context, []string) error{
			"longterm-mem": succeeds,
		}, nil, false, false, projection.ProceduralSkills{}, memoryscope.SourceLongtermMem, "no longterm-mem MCP entry is registered in Claude Code, OpenCode or Codex"},
		{"there is no procedural lock", nil, nil, false, true, projection.ProceduralSkills{}, memoryscope.SourceProceduralSkills, "no procedural skills lock: none installed"},
		{"the procedural lock is unreadable", nil, nil, false, true, projection.ProceduralSkills{Unreadable: "unsupported version 2"}, memoryscope.SourceProceduralSkills, "the procedural skills lock cannot be read: unsupported version 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := ""
			if tc.store != nil {
				store = tc.store(t)
			}
			runner := &fakeRunner{known: tc.runner}
			got := probeOf(runner, store, tc.registered, tc.lock).Probe("/repo", []memoryscope.Source{tc.source})
			want := strings.ReplaceAll(tc.want, "<store>", store)
			if len(got) != 1 || got[0].Source != tc.source || got[0].Available || got[0].Detail != want {
				t.Errorf("Probe = %+v, want %s unavailable with %q", got, tc.source, want)
			}
			if tc.noMemcard && runner.ran.Load() != 0 {
				t.Errorf("the probe ran %d programs, want none: a store that cannot be used is reported without running memorycard", runner.ran.Load())
			}
			if tc.name == "the store does not exist" {
				if _, err := os.Stat(store); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("the probe left %s behind (stat: %v): it must never create the store", store, err)
				}
			}
		})
	}
}

// The store is where the memorycard program keeps it: the path MEMORYCARD_DB names, else the default
// under the home; the adapter reads both through the deps, never the process.
func TestTheMemorycardStoreIsMEMORYCARD_DBElseTheDefault(t *testing.T) {
	home := func(h string) func() (string, error) { return func() (string, error) { return h, nil } }
	noHome := func() (string, error) { return "", errors.New("no home") }
	for _, tc := range []struct {
		name string
		env  map[string]string
		home func() (string, error)
		want string
	}{
		{"MEMORYCARD_DB names it", map[string]string{"MEMORYCARD_DB": "/x/m.db"}, home("/h"), "/x/m.db"},
		{"empty MEMORYCARD_DB gives the default", map[string]string{"MEMORYCARD_DB": ""}, home("/h"), "/h/.memorycard/memory.db"},
		{"no home gives none", nil, noHome, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := memorycardStorePath(environmentOf(tc.env), tc.home); got != tc.want {
				t.Errorf("memorycardStorePath = %q, want %q", got, tc.want)
			}
		})
	}
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"MEMORYCARD_DB": "/from/deps.db"})
	d.userHomeDir = func() (string, error) { return "/deps/home", nil }
	if got := newSessionAvailability(d).memorycardStore; got != "/from/deps.db" {
		t.Errorf("newSessionAvailability holds the store %q, want the one the deps' MEMORYCARD_DB names", got)
	}
	d.getenv = environmentOf(nil)
	if got := newSessionAvailability(d).memorycardStore; got != "/deps/home/.memorycard/memory.db" {
		t.Errorf("newSessionAvailability holds the store %q, want the default under the deps' home", got)
	}
}

// Any runtime's entry makes longterm-mem available (the owner's decision 3); the lock is read for the
// directory of the session.
func TestTheLockIsReadForTheDirectoryOfTheSession(t *testing.T) {
	var asked string
	p := probeOf(&fakeRunner{}, "", true, projection.ProceduralSkills{})
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
	runner := &fakeRunner{known: map[string]func(context.Context, []string) error{"memorycard": hang, "longterm-mem": hang}}
	start := time.Now()
	got := byName(probeOf(runner, storeFile(t), true, projection.ProceduralSkills{Locked: true}).Probe("/repo", allSources))
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
	got := probeOf(&fakeRunner{}, "", true, projection.ProceduralSkills{}).Probe("/repo", []memoryscope.Source{"something-else"})
	if len(got) != 1 || got[0].Available || got[0].Detail == "" {
		t.Errorf("Probe = %+v, want the unknown source unavailable with a reason", got)
	}
}

// A probe that panics costs its source, never the hook.
func TestAProbeThatPanicsMakesItsSourceUnavailable(t *testing.T) {
	runner := &fakeRunner{known: map[string]func(context.Context, []string) error{
		"memorycard": func(context.Context, []string) error { panic("boom") },
	}}
	got := probeOf(runner, storeFile(t), true, projection.ProceduralSkills{Locked: true}).Probe("/repo", []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceProceduralSkills})
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
	if len(r.codes) != 1 || r.codes[0] != 0 || !strings.Contains(r.stdout, want) || strings.Contains(r.stdout, "Do not call the memorycard tools") {
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

// The help of 'projection hook' and the README describe the degradation lines the hook gives: each
// line the SessionStart hook gives with every source unavailable is in both texts, each followed by
// the sources it is for (the source of the generic line stands for longterm-mem and
// procedural-skills), and the lines the hook gives with no reason are those lines without " (<reason>)",
// which both texts say.
func TestUsageAndReadmeGiveTheDegradationLinesTheHookGives(t *testing.T) {
	// degradationLinesWith renders the SessionStart block with every source unavailable for reason.
	degradationLinesWith := func(reason string) []string {
		e := newHookEnv(t)
		e.running(t, "proj-1", "wf-1", "odd")
		d := e.deps
		d.sourceProber = func() app.SourceAvailabilityProber {
			return fixedProber{unavailable: map[memoryscope.Source]string{
				memoryscope.SourceEngram:           reason,
				memoryscope.SourceLongtermMem:      reason,
				memoryscope.SourceProceduralSkills: reason,
			}}
		}
		r := runProjectionArgsWith(d, []string{"hook", "--event", "SessionStart"}, sessionStartInput(t, e.repo, "startup"), e.dir)
		var out struct {
			HookSpecificOutput struct {
				AdditionalContext string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
			t.Fatalf("stdout %q is not the hook's JSON: %v", r.stdout, err)
		}
		var lines []string
		for _, line := range strings.Split(out.HookSpecificOutput.AdditionalContext, "\n") {
			if strings.HasPrefix(line, "- Do not") {
				lines = append(lines, line)
			}
		}
		if len(lines) != 3 {
			t.Fatalf("the hook gave %d lines starting %q, want one for each of the three sources: %q", len(lines), "- Do not", lines)
		}
		return lines
	}
	lines := degradationLinesWith("<reason>")
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	flat := func(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, "`", "")), " ") }
	texts := map[string]string{"usage()": flat(captureUsage(t)), "README": flat(string(readme))}
	generic := strings.NewReplacer("Do not read longterm-mem", "Do not read <source>", "Do not read procedural-skills", "Do not read <source>")
	for _, line := range lines {
		want := generic.Replace(strings.TrimPrefix(line, "- "))
		forSources := `" for longterm-mem and procedural-skills`
		if strings.HasPrefix(line, "- Do not call the memorycard tools") {
			forSources = `" for engram`
		}
		for name, text := range texts {
			if !strings.Contains(text, want+forSources) {
				t.Errorf("%s does not contain the line the hook gives followed by %q: %q", name, forSources, want)
			}
		}
	}
	for i, line := range degradationLinesWith("") {
		if want := strings.Replace(lines[i], " (<reason>)", "", 1); line != want {
			t.Errorf("the line with no reason is %q, want the first run's line without \" (<reason>)\": %q", line, want)
		}
	}
	for name, text := range texts {
		if want := `without " (<reason>)" when the probe gives no reason`; !strings.Contains(text, want) {
			t.Errorf("%s does not say %q", name, want)
		}
	}
}
