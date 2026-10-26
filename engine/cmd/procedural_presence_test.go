package main

// What the create-time record of memory:procedural-skills says of the lock of the project (C10c,
// docs/architecture/adr-c23-m8-session-availability.md decision 4): the record is made over the reader
// the SessionStart probe reads the lock with, so a lock that is there and cannot be used is unavailable
// at creation too, in the words the session probe gives, while a valid lock and a missing one say what
// they said before.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

const proceduralCapability = "memory:procedural-skills"

// lockCannotBeRead is how both probes begin the reason of a lock that is there and cannot be used.
const lockCannotBeRead = "the procedural skills lock cannot be read: "

var (
	lockEntryDigest = strings.Repeat("a", 64)

	validLockWithNoSkill = `{"version":1,"skills":[]}`
	validLockWithASkill  = `{"version":1,"skills":[{"id":"deploy-checks","provenance":"candidate","candidate":"cand-1","sha256":"` + lockEntryDigest + `","revision":1,"targets":[".claude/skills/deploy-checks/SKILL.md"]}]}`
)

// skillEntry is one entry of a lock, with the id the caller gives it.
func skillEntry(id string) string {
	return `{"id":"` + id + `","provenance":"candidate","candidate":"cand-1","sha256":"` + lockEntryDigest + `","revision":1,"targets":[]}`
}

// invalidLocks are locks that are there and that the reader of the program refuses.
var invalidLocks = []struct{ name, content string }{
	{"a version the program does not read", `{"version":2,"skills":[]}`},
	{"no version", `{"skills":[]}`},
	{"a skill listed twice", `{"version":1,"skills":[` + skillEntry("deploy-checks") + `,` + skillEntry("deploy-checks") + `]}`},
	{"an unknown field", `{"version":1,"skills":[],"owner":"someone"}`},
	{"text that is not JSON", `this is not a lock`},
	{"an empty file", ``},
}

// reasonOf is the reason the parser of the lock gives for refusing content: the words both probes
// repeat.
func reasonOf(t *testing.T, content string) string {
	t.Helper()
	_, err := skills.ParseProjectLock([]byte(content))
	if err == nil {
		t.Fatalf("the fixture %q is a valid lock", content)
	}
	return err.Error()
}

func lockPathIn(root string) string {
	return filepath.Join(root, filepath.FromSlash(skills.ProjectLockRelPath))
}

// projectWithLock is a repository whose procedural skills lock holds content, and the deps of a process
// that runs in it.
func projectWithLock(t *testing.T, content string) (repo string, d deps) {
	t.Helper()
	repo = fixtureRepo(t, "repo")
	writeFixtureFile(t, lockPathIn(repo), content)
	return repo, depsRunningIn(repo)
}

// projectWithoutLock is a repository with no lock, and the deps of a process that runs in it.
func projectWithoutLock(t *testing.T) (repo string, d deps) {
	t.Helper()
	repo = fixtureRepo(t, "repo")
	return repo, depsRunningIn(repo)
}

func depsRunningIn(dir string) deps {
	d := testDeps()
	d.getwd = func() (string, error) { return dir, nil }
	return d
}

// proceduralObservation is what the prober of the deps records for the procedural skills of the project.
func proceduralObservation(t *testing.T, d deps) workflow.Observation {
	t.Helper()
	got, err := d.dependencyProber().Probe(context.Background(), []string{proceduralCapability})
	if err != nil || len(got) != 1 || got[0].Capability != proceduralCapability {
		t.Fatalf("Probe = %+v, %v, want the one observation asked for", got, err)
	}
	return got[0]
}

// sessionAnswer is what the SessionStart probe says of the procedural skills of the project that holds
// dir, over the reader of the program.
func sessionAnswer(dir string) projection.SourceAvailability {
	probe := sessionAvailability{
		runner:          &fakeRunner{},
		timeout:         200 * time.Millisecond,
		longtermBinary:  "longterm-mem",
		entryRegistered: func() bool { return true },
		procedural:      newProceduralLockReader(),
	}
	return probe.Probe(dir, []memoryscope.Source{memoryscope.SourceProceduralSkills})[0]
}

// A lock that is there and that the reader refuses is not available, whatever the file holds, and the
// reason is the one the session probe gives. Before C10c the record only stat the file and said
// available.
func TestACreateTimeProbeSaysAPresentButInvalidProceduralLockIsUnavailable(t *testing.T) {
	for _, tc := range invalidLocks {
		t.Run(tc.name, func(t *testing.T) {
			_, d := projectWithLock(t, tc.content)
			got := proceduralObservation(t, d)
			want := lockCannotBeRead + reasonOf(t, tc.content)
			if got.Status != workflow.ObservationUnavailable || got.Detail != want {
				t.Errorf("observation = %+v, want unavailable with %q", got, want)
			}
		})
	}
}

func TestTheCreateTimeProbeUsesTheWordsOfTheSessionProbeForAnInvalidLock(t *testing.T) {
	for _, tc := range invalidLocks {
		t.Run(tc.name, func(t *testing.T) {
			repo, d := projectWithLock(t, tc.content)
			session := sessionAnswer(repo)
			created := proceduralObservation(t, d)
			if session.Available || !strings.HasPrefix(session.Detail, lockCannotBeRead) {
				t.Fatalf("the session probe says %+v, want it unavailable because the lock cannot be read", session)
			}
			if created.Status != workflow.ObservationUnavailable || created.Detail != session.Detail {
				t.Errorf("create time says %+v, want unavailable with the words of the session probe %q", created, session.Detail)
			}
		})
	}
}

// The two probes ask one reader, so they cannot disagree on whether the lock of a project can be used.
func TestTheTwoProbesAgreeOnWhetherTheProceduralLockCanBeUsed(t *testing.T) {
	type world struct {
		name string
		repo string
		d    deps
	}
	var worlds []world
	for _, tc := range append([]struct{ name, content string }{
		{"a valid lock with no skill", validLockWithNoSkill},
		{"a valid lock with a skill", validLockWithASkill},
	}, invalidLocks...) {
		repo, d := projectWithLock(t, tc.content)
		worlds = append(worlds, world{tc.name, repo, d})
	}
	repo, d := projectWithoutLock(t)
	worlds = append(worlds, world{"no lock", repo, d})

	for _, w := range worlds {
		t.Run(w.name, func(t *testing.T) {
			session := sessionAnswer(w.repo)
			created := proceduralObservation(t, w.d)
			if session.Available != (created.Status == workflow.ObservationAvailable) {
				t.Errorf("the session probe says %+v and create time says %+v", session, created)
			}
		})
	}
}

// A valid lock is as available as it was: with a skill or with none, and without naming a path.
func TestAValidProceduralLockIsStillAvailableAtCreateTime(t *testing.T) {
	for name, content := range map[string]string{"with no skill": validLockWithNoSkill, "with a skill": validLockWithASkill} {
		t.Run(name, func(t *testing.T) {
			repo, d := projectWithLock(t, content)
			got := proceduralObservation(t, d)
			if got.Status != workflow.ObservationAvailable || !strings.HasPrefix(got.Detail, "the procedural skills lock is present") {
				t.Errorf("observation = %+v, want available, saying the lock is present", got)
			}
			if strings.Contains(got.Detail, repo) {
				t.Errorf("detail %q names the path of the project", got.Detail)
			}
		})
	}
}

// A project with no lock is as unavailable as it was, with the same words.
func TestAMissingProceduralLockIsStillUnavailableAtCreateTime(t *testing.T) {
	_, d := projectWithoutLock(t)
	got := proceduralObservation(t, d)
	if got.Status != workflow.ObservationUnavailable || got.Detail != "the procedural skills lock was not found: no such file" {
		t.Errorf("observation = %+v, want unavailable, saying the lock was not found", got)
	}
}

// What a lock that cannot be opened says never names the project's path: the record is kept in the log
// of the workflow, and a path is the one thing the prober has never written there.
func TestAProceduralLockThatCannotBeOpenedIsUnavailableWithoutNamingAPath(t *testing.T) {
	repo := fixtureRepo(t, "repo")
	if err := os.MkdirAll(lockPathIn(repo), 0o755); err != nil {
		t.Fatal(err)
	}
	got := proceduralObservation(t, depsRunningIn(repo))
	if got.Status != workflow.ObservationUnavailable || !strings.HasPrefix(got.Detail, lockCannotBeRead) {
		t.Errorf("observation = %+v, want unavailable because the lock cannot be read", got)
	}
	if strings.Contains(got.Detail, repo) {
		t.Errorf("detail %q names the path of the project", got.Detail)
	}
}

// The reason of a lock is what its file makes it, so it can be as long as the file is: the observation
// stays within what a workflow event accepts, and the workflow is still created.
func TestAnInvalidProceduralLockNeverMakesTheObservationTooLongToRecord(t *testing.T) {
	twice := skillEntry(strings.Repeat("x", 3*workflow.MaxObservationDetailLength))
	content := `{"version":1,"skills":[` + twice + `,` + twice + `]}`
	if reason := reasonOf(t, content); len([]rune(reason)) <= workflow.MaxObservationDetailLength {
		t.Fatalf("the fixture gives a reason of %d runes, want one over the bound of %d", len([]rune(reason)), workflow.MaxObservationDetailLength)
	}

	_, d := projectWithLock(t, content)
	got := proceduralObservation(t, d)
	if got.Status != workflow.ObservationUnavailable || !strings.HasPrefix(got.Detail, lockCannotBeRead) {
		t.Errorf("observation = %+v, want unavailable because the lock cannot be read", got)
	}
	if n := len([]rune(got.Detail)); n > workflow.MaxObservationDetailLength || !utf8.ValidString(got.Detail) {
		t.Errorf("detail has %d runes (valid UTF-8: %v), want at most %d", n, utf8.ValidString(got.Detail), workflow.MaxObservationDetailLength)
	}

	stateHome := presenceWorld(t)
	createOddWorkflowWith(t, d)
	if o := recordedProceduralObservation(t, stateHome); o.Status != workflow.ObservationUnavailable {
		t.Errorf("the workflow recorded %+v, want unavailable", o)
	}
}

// recordedProceduralObservation is what the created event of workflow wf-1 of proj-1 holds for the
// procedural skills.
func recordedProceduralObservation(t *testing.T, stateHome string) workflow.Observation {
	t.Helper()
	return recordedObservations(t, stateHome)[proceduralCapability]
}

func recordedObservations(t *testing.T, stateHome string) map[string]workflow.Observation {
	t.Helper()
	events := phase6LoadOwned(t, stateHome, "proj-1", "wf-1").Events
	if len(events) == 0 {
		t.Fatal("no events recorded")
	}
	byName := map[string]workflow.Observation{}
	for _, o := range events[0].Observations {
		byName[o.Capability] = o
	}
	if _, ok := byName[proceduralCapability]; !ok {
		t.Fatalf("the created event records %+v, want an observation of %s", events[0].Observations, proceduralCapability)
	}
	return byName
}

// besides is the observations but the one named.
func besides(observations map[string]workflow.Observation, name string) map[string]workflow.Observation {
	rest := map[string]workflow.Observation{}
	for n, o := range observations {
		if n != name {
			rest[n] = o
		}
	}
	return rest
}

// 'workflow create' records what the lock of the project is, and nothing else it records changes with
// the lock: the other dependencies are the same whatever the lock holds.
func TestWorkflowCreateRecordsWhetherTheProceduralLockOfTheProjectCanBeUsed(t *testing.T) {
	type state struct {
		name       string
		lock       *string
		wantStatus string
		wantDetail func(detail string) bool
	}
	valid, invalid := validLockWithASkill, `{"version":2,"skills":[]}`
	states := []state{
		{"a valid lock", &valid, workflow.ObservationAvailable, func(d string) bool { return strings.HasPrefix(d, "the procedural skills lock is present") }},
		{"no lock", nil, workflow.ObservationUnavailable, func(d string) bool { return d == "the procedural skills lock was not found: no such file" }},
		{"an invalid lock", &invalid, workflow.ObservationUnavailable, func(d string) bool {
			return d == lockCannotBeRead+"parse project lock: unsupported version 2, want 1"
		}},
	}
	others := map[string]map[string]workflow.Observation{}
	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			stateHome := presenceWorld(t)
			repo := fixtureRepo(t, "repo")
			if s.lock != nil {
				writeFixtureFile(t, lockPathIn(repo), *s.lock)
			}
			createOddWorkflowWith(t, depsRunningIn(repo))
			recorded := recordedObservations(t, stateHome)
			if o := recorded[proceduralCapability]; o.Status != s.wantStatus || !s.wantDetail(o.Detail) {
				t.Errorf("the workflow recorded %+v, want %s", o, s.wantStatus)
			}
			others[s.name] = besides(recorded, proceduralCapability)
		})
	}
	baseline, ran := others[states[0].name]
	for _, s := range states[1:] {
		// A state that did not record (a subtest that was filtered out or failed) has nothing to compare.
		if got, ok := others[s.name]; ok && ran && !reflect.DeepEqual(got, baseline) {
			t.Errorf("with %s the workflow recorded %+v for the other dependencies, with %s %+v: only the procedural skills may differ", s.name, got, states[0].name, baseline)
		}
	}
}

// 'runtime probe' prints the same of the lock, with exit 0 and nothing on stderr, and the rest of its
// report is the one it printed before.
func TestRuntimeProbeSaysWhetherTheProceduralLockOfTheProjectCanBeUsed(t *testing.T) {
	home, path := probeFixture(t)
	reportIn := func(d deps) probeReportJSON { return decodeProbe(t, runProbeTestWith(d)) }
	procedural := func(report probeReportJSON) workflow.Observation {
		for _, o := range report.Observations {
			if o.Capability == proceduralCapability {
				return o
			}
		}
		t.Fatalf("the report %+v has no observation of %s", report, proceduralCapability)
		return workflow.Observation{}
	}
	rest := func(report probeReportJSON) []workflow.Observation {
		var out []workflow.Observation
		for _, o := range report.Observations {
			if o.Capability != proceduralCapability {
				out = append(out, o)
			}
		}
		return out
	}
	// The home and PATH of the probe are the fixture's; the project is the one the process runs in.
	runningIn := func(project deps) deps {
		probe := probeDeps(home, path)
		probe.getwd = project.getwd
		return probe
	}

	_, withValid := projectWithLock(t, validLockWithASkill)
	_, withInvalid := projectWithLock(t, `{"version":2,"skills":[]}`)
	_, withNone := projectWithoutLock(t)
	valid, invalid, none := reportIn(runningIn(withValid)), reportIn(runningIn(withInvalid)), reportIn(runningIn(withNone))

	if o := procedural(valid); o.Status != workflow.ObservationAvailable || !strings.HasPrefix(o.Detail, "the procedural skills lock is present") {
		t.Errorf("a valid lock: %+v, want available, saying the lock is present", o)
	}
	if o := procedural(none); o.Status != workflow.ObservationUnavailable || o.Detail != "the procedural skills lock was not found: no such file" {
		t.Errorf("no lock: %+v, want unavailable, saying the lock was not found", o)
	}
	if o := procedural(invalid); o.Status != workflow.ObservationUnavailable || o.Detail != lockCannotBeRead+"parse project lock: unsupported version 2, want 1" {
		t.Errorf("an invalid lock: %+v, want unavailable, with the words of the session probe", o)
	}
	for name, report := range map[string]probeReportJSON{"an invalid lock": invalid, "no lock": none} {
		if !reflect.DeepEqual(rest(report), rest(valid)) {
			t.Errorf("with %s the report holds %+v besides the procedural skills, with a valid lock %+v: only that observation may differ", name, rest(report), rest(valid))
		}
	}
}

// --- the adapter -------------------------------------------------------------------------------

// askedLock is the reader of the lock answering with one lock and remembering which directory it was
// asked about.
type askedLock struct {
	lock  projection.ProceduralSkills
	asked []string
}

func (a *askedLock) Load(dir string) projection.ProceduralSkills {
	a.asked = append(a.asked, dir)
	return a.lock
}

// processProjectOver is the adapter over a working directory, a worktree root and a reader of its own.
func processProjectOver(dir string, dirErr error, root string, locks *askedLock) processProject {
	return processProject{
		workingDir:   func() (string, error) { return dir, dirErr },
		worktreeRoot: func(string) string { return root },
		locks:        locks,
	}
}

// The adapter turns what the reader says into the reading the prober owns: a lock the reader found is
// valid, none is absent, and one it could not use is invalid with the reader's reason.
func TestTheProcessProjectReadsTheLockWithTheReaderOfTheSessionProbe(t *testing.T) {
	for _, tc := range []struct {
		name string
		lock projection.ProceduralSkills
		want presence.LockReading
	}{
		{"a lock with skills", projection.ProceduralSkills{Locked: true, Skills: []projection.ProceduralSkill{{ID: "deploy-checks"}}}, presence.LockReading{State: presence.LockValid}},
		{"a lock with none", projection.ProceduralSkills{Locked: true}, presence.LockReading{State: presence.LockValid}},
		{"no lock", projection.ProceduralSkills{}, presence.LockReading{State: presence.LockAbsent}},
		{"a lock the reader cannot use", projection.ProceduralSkills{Unreadable: "unsupported version 2"}, presence.LockReading{State: presence.LockInvalid, Reason: "unsupported version 2"}},
		// The reader says why and nothing more; if it ever said both, the lock cannot be used.
		{"a reason and a lock", projection.ProceduralSkills{Locked: true, Unreadable: "both"}, presence.LockReading{State: presence.LockInvalid, Reason: "both"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locks := &askedLock{lock: tc.lock}
			got := processProjectOver("/work/repo/sub", nil, "/work/repo", locks).ReadLock()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ReadLock = %+v, want %+v", got, tc.want)
			}
			if want := []string{"/work/repo/sub"}; !reflect.DeepEqual(locks.asked, want) {
				t.Errorf("the reader was asked about %v, want the working directory %v, as the session probe asks it", locks.asked, want)
			}
		})
	}
}

// With no working directory or no worktree around it the project is not known, and the reader is not
// asked: there is no lock to look for.
func TestTheProcessProjectIsUnknownWithoutAWorkingDirectoryOrAWorktree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dir    string
		dirErr error
		root   string
	}{
		{"the working directory cannot be determined", "", errNoEnvironment, "/work/repo"},
		{"the working directory is in no worktree", "/tmp/elsewhere", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locks := &askedLock{lock: projection.ProceduralSkills{Locked: true}}
			got := processProjectOver(tc.dir, tc.dirErr, tc.root, locks).ReadLock()
			if got != (presence.LockReading{}) || len(locks.asked) != 0 {
				t.Errorf("ReadLock = %+v with the reader asked %v, want the unknown project and no question", got, locks.asked)
			}
		})
	}
}

// The file system words its errors with the path it was given, which is the lock's place on this
// machine; the reading says where the lock is in the project instead, and leaves any other reason as
// the reader gave it.
func TestTheProcessProjectKeepsThePathOfTheLockOutOfItsReason(t *testing.T) {
	const root = "/work/my repo"
	for _, tc := range []struct {
		name   string
		reason string
		want   string
	}{
		{"an error of the file system", "read " + root + "/.labdrian/procedural-skills.lock.json: is a directory", "read .labdrian/procedural-skills.lock.json: is a directory"},
		{"the path named twice", "open " + root + "/.labdrian/procedural-skills.lock.json: " + root + "/.labdrian/procedural-skills.lock.json: too many levels of symbolic links", "open .labdrian/procedural-skills.lock.json: .labdrian/procedural-skills.lock.json: too many levels of symbolic links"},
		{"a reason with no path", "parse project lock: unsupported version 2, want 1", "parse project lock: unsupported version 2, want 1"},
		{"another project's path is not this one's", "read /work/other/.labdrian/procedural-skills.lock.json: is a directory", "read /work/other/.labdrian/procedural-skills.lock.json: is a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locks := &askedLock{lock: projection.ProceduralSkills{Unreadable: tc.reason}}
			got := processProjectOver(root+"/docs", nil, root, locks).ReadLock()
			if got.State != presence.LockInvalid || got.Reason != tc.want {
				t.Errorf("ReadLock = %+v, want invalid with the reason %q", got, tc.want)
			}
		})
	}
}

// The deps hand the prober the adapter over their own working directory and the reader of the program,
// asked when the prober asks: a deps whose working directory changes between building the prober and
// probing is read where it is then.
func TestTheDepsAskForTheWorkingDirectoryWhenTheProberAsks(t *testing.T) {
	first, second := fixtureRepo(t, "first"), fixtureRepo(t, "second")
	writeFixtureFile(t, lockPathIn(second), validLockWithNoSkill)
	dir := first
	d := testDeps()
	d.getwd = func() (string, error) { return dir, nil }
	prober := d.dependencyProber()
	dir = second
	got, err := prober.Probe(context.Background(), []string{proceduralCapability})
	if err != nil || len(got) != 1 || got[0].Status != workflow.ObservationAvailable {
		t.Errorf("Probe = %+v, %v, want the lock of the directory the process is in when it probes (available)", got, err)
	}
}

// The reader of the program is the one both probes use, built in one place: a deps with no environment
// has no project, and its prober says so.
func TestADepsWithNoWorkingDirectoryProbesAnUnknownProject(t *testing.T) {
	got, err := deps{}.dependencyProber().Probe(context.Background(), []string{proceduralCapability})
	want := "the project directory is unknown, so the procedural skills lock was not looked for"
	if err != nil || len(got) != 1 || got[0].Status != workflow.ObservationUnavailable || got[0].Detail != want {
		t.Errorf("Probe = %+v, %v, want unavailable with %q", got, err, want)
	}
}
