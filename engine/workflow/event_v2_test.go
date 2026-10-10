package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// snapshotOfBuiltIn is the snapshot of the built-in profile name.
func snapshotOfBuiltIn(t *testing.T, name string) workflowprofile.Snapshot {
	t.Helper()
	p, err := workflowprofile.Resolve(name)
	if err != nil {
		t.Fatal(err)
	}
	return p.Snapshot()
}

// digestOf is the digest of the snapshot.
func digestOf(t *testing.T, s workflowprofile.Snapshot) string {
	t.Helper()
	digest, err := s.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// withSnapshot makes e a version 2 created event of the profile the snapshot records.
func withSnapshot(t *testing.T, e WorkflowEvent, s workflowprofile.Snapshot) WorkflowEvent {
	t.Helper()
	e.Version = EventVersionSnapshot
	e.Profile = s.Name
	e.ProfileSnapshot = &s
	e.ProfileDigest = digestOf(t, s)
	return e
}

// validCreatedEventV2 is validCreatedEvent as a version 2 event: the odd profile, and a snapshot of it.
func validCreatedEventV2(t *testing.T) WorkflowEvent {
	t.Helper()
	return withSnapshot(t, validCreatedEvent(), snapshotOfBuiltIn(t, "odd"))
}

// asV2 is e as an event of a version 2 log: its version, and nothing else.
func asV2(e WorkflowEvent) WorkflowEvent {
	e.Version = EventVersionSnapshot
	return e
}

// fixtureSnapshot is a snapshot of a profile that is in no catalog.
func fixtureSnapshot() workflowprofile.Snapshot {
	return workflowprofile.WorkflowProfile{
		Name:                 "fixture",
		Stages:               []workflowprofile.Stage{{Name: "a"}, {Name: "b", DependsOn: []string{"a"}}},
		Roles:                []string{"r"},
		Checks:               []string{"c"},
		MemoryPolicy:         "m",
		ReviewPolicy:         "rv",
		DeliveryPolicy:       "d",
		MemoryDefault:        workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeGoal, Sources: []workflowprofile.MemorySource{workflowprofile.MemorySourceEngram}},
		ReliesOnGentleReview: true,
	}.Snapshot()
}

// validPayloadEvent is a valid event of kind, with the payload of its kind.
func validPayloadEvent(kind Kind) WorkflowEvent {
	e := newEventOfKind(kind)
	switch kind {
	case KindStageRecorded:
		e.Stage = "explore"
	case KindVerified:
		e.Checked = &Checked{ChainDigest: strings.Repeat("c", 64), GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
	case KindClosed:
		e.Outcome = string(OutcomeCompleted)
	}
	return e
}

// The line of a version 2 created event is part of the log's format: its fields are in this order,
// the snapshot after the profile's name and its digest after the snapshot. The line is written out by
// hand, and the digest of the event must be the SHA-256 of exactly those bytes.
func TestAVersion2CreatedEventIsEncodedAsRecorded(t *testing.T) {
	const line = `{"version":2,"workflow_id":"wf-1","project_id":"proj-1","seq":0,"prev_digest":"","kind":"created","at":"2026-09-28T10:00:00Z","provenance":{"worktree_root":"/home/labdrian/labdrian-sdd-overlay","git_head":"e1218c2f00000000000000000000000000000000"},"observations":[],"goal_id":"goal-1","goal_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","profile":"fixture","profile_snapshot":{"name":"fixture","stages":[{"name":"a","depends_on":[]},{"name":"b","depends_on":["a"]}],"roles":["r"],"checks":["c"],"memory_policy":"m","review_policy":"rv","delivery_policy":"d","memory_default":{"scope":"goal","sources":["engram"]},"relies_on_gentle_review":true},"profile_digest":"773466a9e0f9d3bd8345669581f724c2ff8c0435f40ef976edaa43fb31b0715d"}`
	e := withSnapshot(t, validCreatedEvent(), fixtureSnapshot())
	got, err := e.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	if string(got) != line+"\n" {
		t.Fatalf("MarshalLine() = %s, want %s", got, line)
	}
	gotDigest, err := EventDigest(e)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(line))
	if want := hex.EncodeToString(sum[:]); gotDigest != want {
		t.Errorf("EventDigest = %s, want the SHA-256 of the line, %s", gotDigest, want)
	}
	parsed, err := ParseWorkflowEvent([]byte(line))
	if err != nil {
		t.Fatalf("ParseWorkflowEvent() = %v, want nil: a profile that is in no catalog is a valid snapshot", err)
	}
	if !reflect.DeepEqual(parsed, e) {
		t.Errorf("ParseWorkflowEvent() = %+v, want %+v", parsed, e)
	}
}

// A version 1 event is written as it always was, with neither of the snapshot's fields.
func TestAVersion1EventIsEncodedWithoutTheSnapshotFields(t *testing.T) {
	line, err := validCreatedEvent().MarshalLine()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"profile_snapshot", "profile_digest"} {
		if strings.Contains(string(line), field) {
			t.Errorf("a version 1 created event holds %q: %s", field, line)
		}
	}
	if !strings.HasPrefix(string(line), `{"version":1,`) {
		t.Errorf("a version 1 created event begins %.20s, want version 1", line)
	}
}

func TestEveryBuiltInProfileMakesAValidVersion2CreatedEvent(t *testing.T) {
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		t.Run(name, func(t *testing.T) {
			e := withSnapshot(t, validCreatedEvent(), snapshotOfBuiltIn(t, name))
			if err := e.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			data, err := e.MarshalLine()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseWorkflowEvent(data[:len(data)-1])
			if err != nil {
				t.Fatalf("ParseWorkflowEvent() = %v, want nil", err)
			}
			if !reflect.DeepEqual(parsed, e) {
				t.Errorf("parsed = %+v, want %+v", parsed, e)
			}
		})
	}
}

func TestOnlyVersionsOneAndTwoAreAccepted(t *testing.T) {
	for _, version := range []int{-1, 0, 3, 99} {
		e := validCreatedEvent()
		e.Version = version
		if err := e.Validate(); err == nil {
			t.Errorf("version %d: Validate() = nil, want an error", version)
		}
	}
	if EventVersion != EventVersionSnapshot {
		t.Errorf("EventVersion = %d, want %d: the program writes the newest version", EventVersion, EventVersionSnapshot)
	}
}

func TestAVersion2CreatedEventNeedsAWellFormedSnapshotOfItsProfile(t *testing.T) {
	// replace puts the snapshot s in the event, with its own digest and, when named, its name.
	replace := func(e *WorkflowEvent, s workflowprofile.Snapshot) {
		e.ProfileSnapshot = &s
		e.ProfileDigest = digestOf(t, s)
	}
	cases := map[string]func(*WorkflowEvent){
		"no snapshot":                    func(e *WorkflowEvent) { e.ProfileSnapshot = nil },
		"no digest":                      func(e *WorkflowEvent) { e.ProfileDigest = "" },
		"a digest that is not hex":       func(e *WorkflowEvent) { e.ProfileDigest = strings.Repeat("z", 64) },
		"a digest in upper case":         func(e *WorkflowEvent) { e.ProfileDigest = strings.ToUpper(e.ProfileDigest) },
		"a short digest":                 func(e *WorkflowEvent) { e.ProfileDigest = e.ProfileDigest[:63] },
		"the digest of another snapshot": func(e *WorkflowEvent) { e.ProfileDigest = strings.Repeat("a", 64) },
		"a snapshot edited after the digest": func(e *WorkflowEvent) {
			s := *e.ProfileSnapshot
			s.Checks = append([]string{"smuggled"}, s.Checks...)
			e.ProfileSnapshot = &s
		},
		"a snapshot of another profile than the event names": func(e *WorkflowEvent) {
			replace(e, snapshotOfBuiltIn(t, "sdd"))
		},
		"a profile name that is not a safe name": func(e *WorkflowEvent) {
			s := *e.ProfileSnapshot
			s.Name = "../odd"
			e.Profile = s.Name
			replace(e, s)
		},
		"a snapshot that is malformed": func(e *WorkflowEvent) {
			s := *e.ProfileSnapshot
			s.Roles = []string{}
			replace(e, s)
		},
		"more stages than a workflow may record": func(e *WorkflowEvent) {
			s := fixtureSnapshot()
			s.Stages = nil
			for i := 0; i <= MaxStages; i++ {
				s.Stages = append(s.Stages, workflowprofile.Stage{Name: fmt.Sprintf("s%d", i), DependsOn: []string{}})
			}
			e.Profile = s.Name
			replace(e, s)
		},
		"a stage name past the bound": func(e *WorkflowEvent) {
			s := fixtureSnapshot()
			s.Stages[1] = workflowprofile.Stage{Name: strings.Repeat("s", MaxStageLength+1), DependsOn: []string{"a"}}
			e.Profile = s.Name
			replace(e, s)
		},
		"a policy past the bound": func(e *WorkflowEvent) {
			s := fixtureSnapshot()
			s.ReviewPolicy = strings.Repeat("p", MaxProfileTextLength+1)
			e.Profile = s.Name
			replace(e, s)
		},
		"a role past the bound": func(e *WorkflowEvent) {
			s := fixtureSnapshot()
			s.Roles = []string{strings.Repeat("r", MaxProfileTextLength+1)}
			e.Profile = s.Name
			replace(e, s)
		},
		"more checks than the bound": func(e *WorkflowEvent) {
			s := fixtureSnapshot()
			s.Checks = make([]string, MaxProfileListLength+1)
			for i := range s.Checks {
				s.Checks[i] = "c"
			}
			e.Profile = s.Name
			replace(e, s)
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			e := validCreatedEventV2(t)
			if err := e.Validate(); err != nil {
				t.Fatalf("before the edit, Validate() = %v, want nil", err)
			}
			edit(&e)
			if err := e.Validate(); err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
		})
	}
}

// A version 2 event is self-contained: it asks nothing of the catalog, so the profile it names may be
// one the catalog has never had or no longer has.
func TestAVersion2CreatedEventOfAProfileThatIsInNoCatalogIsValid(t *testing.T) {
	if _, err := workflowprofile.Resolve("fixture"); err == nil {
		t.Fatal("the fixture profile is in the catalog, so this proves nothing")
	}
	e := withSnapshot(t, validCreatedEvent(), fixtureSnapshot())
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestAVersion1CreatedEventHoldsNoSnapshotAndStillNamesACatalogProfile(t *testing.T) {
	s := snapshotOfBuiltIn(t, "odd")
	cases := map[string]func(*WorkflowEvent){
		"a snapshot":                     func(e *WorkflowEvent) { e.ProfileSnapshot = &s },
		"a digest":                       func(e *WorkflowEvent) { e.ProfileDigest = digestOf(t, s) },
		"a profile that is not built in": func(e *WorkflowEvent) { e.Profile = "fixture" },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			e := validCreatedEvent()
			edit(&e)
			if err := e.Validate(); err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
		})
	}
	if err := validCreatedEvent().Validate(); err != nil {
		t.Errorf("a version 1 created event of a catalog profile: Validate() = %v, want nil", err)
	}
}

// The snapshot belongs to the created event alone.
func TestNoEventButTheCreatedOneMayCarryASnapshotOrItsDigest(t *testing.T) {
	s := snapshotOfBuiltIn(t, "odd")
	edits := map[string]func(*WorkflowEvent){
		"a snapshot": func(e *WorkflowEvent) { e.ProfileSnapshot = &s },
		"a digest":   func(e *WorkflowEvent) { e.ProfileDigest = digestOf(t, s) },
	}
	for _, kind := range []Kind{KindStarted, KindPaused, KindResumed, KindStageRecorded, KindVerified, KindClosed} {
		for field, edit := range edits {
			t.Run(string(kind)+" with "+field, func(t *testing.T) {
				e := asV2(validPayloadEvent(kind))
				if err := e.Validate(); err != nil {
					t.Fatalf("without it, Validate() = %v, want nil", err)
				}
				edit(&e)
				if err := e.Validate(); err == nil {
					t.Fatal("Validate() = nil, want an error")
				}
			})
		}
	}
}

// The profile a verified event names is checked against the catalog in a version 1 log, as it always
// was, and in a version 2 log it is a safe name and nothing more, since the snapshot is the profile.
func TestTheProfileOfAVerifiedEventIsJudgedByTheVersionOfTheLog(t *testing.T) {
	for _, c := range []struct {
		version int
		profile string
		valid   bool
	}{
		{1, "odd", true}, {1, "fixture", false},
		{2, "odd", true}, {2, "fixture", true}, {2, "../odd", false}, {2, "", false},
	} {
		e := validPayloadEvent(KindVerified)
		e.Version = c.version
		e.Checked.Profile = c.profile
		err := e.Validate()
		if c.valid && err != nil {
			t.Errorf("version %d, profile %q: Validate() = %v, want nil", c.version, c.profile, err)
		}
		if !c.valid && err == nil {
			t.Errorf("version %d, profile %q: Validate() = nil, want an error", c.version, c.profile)
		}
	}
}

// mustObject decodes data as a JSON object.
func mustObject(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

// mustEncode encodes the value as JSON.
func mustEncode(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// copyObject is a shallow copy of the object with the edit applied.
func copyObject(object map[string]json.RawMessage, edit func(map[string]json.RawMessage)) map[string]json.RawMessage {
	copied := make(map[string]json.RawMessage, len(object)+1)
	for k, v := range object {
		copied[k] = v
	}
	edit(copied)
	return copied
}

func TestParseWorkflowEventRejectsAnUnknownFieldInsideTheSnapshot(t *testing.T) {
	data, err := validCreatedEventV2(t).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	document := mustObject(t, data)
	snapshot := mustObject(t, document["profile_snapshot"])
	var stages []json.RawMessage
	if err := json.Unmarshal(snapshot["stages"], &stages); err != nil {
		t.Fatal(err)
	}
	memory := mustObject(t, snapshot["memory_default"])
	extra := json.RawMessage(`"x"`)

	// build is the document with the snapshot edited.
	build := func(edit func(map[string]json.RawMessage)) []byte {
		return mustEncode(t, copyObject(document, func(d map[string]json.RawMessage) {
			d["profile_snapshot"] = mustEncode(t, copyObject(snapshot, edit))
		}))
	}
	if _, err := ParseWorkflowEvent(build(func(map[string]json.RawMessage) {})); err != nil {
		t.Fatalf("the document without an extra field: %v", err)
	}
	cases := map[string][]byte{
		"in the snapshot": build(func(s map[string]json.RawMessage) { s["bogus"] = extra }),
		"in a stage": build(func(s map[string]json.RawMessage) {
			first := copyObject(mustObject(t, stages[0]), func(o map[string]json.RawMessage) { o["bogus"] = extra })
			s["stages"] = mustEncode(t, append([]json.RawMessage{mustEncode(t, first)}, stages[1:]...))
		}),
		"in the memory default": build(func(s map[string]json.RawMessage) {
			s["memory_default"] = mustEncode(t, copyObject(memory, func(o map[string]json.RawMessage) { o["bogus"] = extra }))
		}),
		"as a name in another case": build(func(s map[string]json.RawMessage) { s["Name"] = s["name"] }),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseWorkflowEvent(doc); err == nil {
				t.Fatal("ParseWorkflowEvent() = nil, want an error")
			}
		})
	}
}

// A log has the version of its created event from end to end.
func TestALogHasOneVersionFromItsCreatedEventToItsLast(t *testing.T) {
	v2Created := validCreatedEventV2(t)
	t.Run("a version 2 log", func(t *testing.T) {
		state, err := Replay(withDigestChain([]WorkflowEvent{v2Created, asV2(seq(KindStarted))}))
		if err != nil {
			t.Fatalf("Replay() = %v, want nil", err)
		}
		if state.Version != EventVersionSnapshot {
			t.Errorf("state.Version = %d, want %d", state.Version, EventVersionSnapshot)
		}
		if state.ProfileSnapshot == nil || !reflect.DeepEqual(*state.ProfileSnapshot, *v2Created.ProfileSnapshot) {
			t.Errorf("state.ProfileSnapshot = %+v, want the snapshot of the created event", state.ProfileSnapshot)
		}
		if state.Profile != "odd" {
			t.Errorf("state.Profile = %q, want odd", state.Profile)
		}
	})
	t.Run("a version 1 log has no snapshot", func(t *testing.T) {
		state, err := Replay(withDigestChain([]WorkflowEvent{validCreatedEvent(), seq(KindStarted)}))
		if err != nil {
			t.Fatalf("Replay() = %v, want nil", err)
		}
		if state.Version != EventVersionNameOnly || state.ProfileSnapshot != nil {
			t.Errorf("state = version %d, snapshot %+v, want version 1 and none", state.Version, state.ProfileSnapshot)
		}
	})
	t.Run("an event of the other version is refused", func(t *testing.T) {
		for name, events := range map[string][]WorkflowEvent{
			"version 1 after a version 2 created event":  {v2Created, seq(KindStarted)},
			"version 2 after a version 1 created event":  {validCreatedEvent(), asV2(seq(KindStarted))},
			"version 2 in the middle of a version 1 log": {validCreatedEvent(), seq(KindStarted), asV2(validPayloadEvent(KindStageRecorded))},
		} {
			t.Run(name, func(t *testing.T) {
				if _, err := Replay(withDigestChain(events)); !errors.Is(err, ErrMixedVersions) {
					t.Fatalf("Replay() = %v, want ErrMixedVersions", err)
				}
			})
		}
	})
	t.Run("a log that mixes them is drifted", func(t *testing.T) {
		got := ClassifyLog("proj-1", "wf-1", logOf(t, withDigestChain([]WorkflowEvent{v2Created, seq(KindStarted)})...))
		if got.Classification != ClassificationDrifted {
			t.Fatalf("ClassifyLog() = %q (%s), want drifted", got.Classification, got.Detail)
		}
	})
}

func TestAVersion2LogIsOwnedAndExtendedLikeAVersion1Log(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{validCreatedEventV2(t), asV2(seq(KindStarted))})
	got := ClassifyLog("proj-1", "wf-1", logOf(t, events...))
	if got.Classification != ClassificationOwned {
		t.Fatalf("ClassifyLog() = %q (%s), want owned", got.Classification, got.Detail)
	}
	next := asV2(validPayloadEvent(KindStageRecorded))
	next.Seq = 2
	next.PrevDigest, _ = EventDigest(events[1])
	if _, err := AdmitAppend("proj-1", "wf-1", got, next); err != nil {
		t.Fatalf("AdmitAppend() = %v, want nil", err)
	}
	next.Version = EventVersionNameOnly
	if _, err := AdmitAppend("proj-1", "wf-1", got, next); !errors.Is(err, ErrMixedVersions) {
		t.Fatalf("AdmitAppend() of a version 1 event to a version 2 log = %v, want ErrMixedVersions", err)
	}
}

// In a version 2 log the profile a verified event names is the workflow's own; in a version 1 log
// nothing ever compared them, and nothing does now.
func TestAVerifiedEventOfAVersion2LogNamesTheProfileOfTheWorkflow(t *testing.T) {
	created := validCreatedEventV2(t)
	verified := func(profile string) WorkflowEvent {
		e := asV2(validPayloadEvent(KindVerified))
		e.Checked.Profile = profile
		return e
	}
	if _, err := Replay(withDigestChain([]WorkflowEvent{created, verified("odd")})); err != nil {
		t.Fatalf("Replay() with the workflow's profile = %v, want nil", err)
	}
	if _, err := Replay(withDigestChain([]WorkflowEvent{created, verified("sdd")})); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() with another profile = %v, want ErrInvalidTransition", err)
	}
	v1 := validPayloadEvent(KindVerified)
	v1.Checked.Profile = "sdd"
	if _, err := Replay(withDigestChain([]WorkflowEvent{validCreatedEvent(), v1})); err != nil {
		t.Fatalf("Replay() of a version 1 log whose verified event names another profile = %v, want nil: that is how it has always been", err)
	}
}
