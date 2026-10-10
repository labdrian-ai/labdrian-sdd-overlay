package workflowprofile

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fixtureProfile is a profile that is in no catalog, small enough to write its encoding out by hand.
func fixtureProfile() WorkflowProfile {
	return WorkflowProfile{
		Name:                 "fixture",
		Stages:               []Stage{{Name: "a"}, {Name: "b", DependsOn: []string{"a"}}},
		Roles:                []string{"r"},
		Checks:               []string{"c"},
		MemoryPolicy:         "m",
		ReviewPolicy:         "rv",
		DeliveryPolicy:       "d",
		MemoryDefault:        MemoryDefault{Scope: MemoryScopeGoal, Sources: []MemorySource{MemorySourceEngram}},
		ReliesOnGentleReview: true,
	}
}

func builtInProfiles(t *testing.T) []WorkflowProfile {
	t.Helper()
	var profiles []WorkflowProfile
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		p, err := Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, p)
	}
	return profiles
}

// The encoding and the digest of a snapshot are a contract with every log that holds one: they are
// written here by hand and their SHA-256 was computed outside the program (printf | sha256sum), so a
// change to the field order, the names, the escaping or the way an empty list is written fails.
func TestASnapshotIsEncodedAndDigestedAsRecordedInTheLog(t *testing.T) {
	const encoded = `{"name":"fixture","stages":[{"name":"a","depends_on":[]},{"name":"b","depends_on":["a"]}],"roles":["r"],"checks":["c"],"memory_policy":"m","review_policy":"rv","delivery_policy":"d","memory_default":{"scope":"goal","sources":["engram"]},"relies_on_gentle_review":true}`
	const digest = "773466a9e0f9d3bd8345669581f724c2ff8c0435f40ef976edaa43fb31b0715d"
	snapshot := fixtureProfile().Snapshot()
	got, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != encoded {
		t.Fatalf("encoding = %s, want %s", got, encoded)
	}
	gotDigest, err := snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if gotDigest != digest {
		t.Errorf("digest = %s, want %s", gotDigest, digest)
	}
}

func TestASnapshotGivesTheProfileBackWithAllItsTypedData(t *testing.T) {
	for _, p := range builtInProfiles(t) {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Snapshot().Profile(); !reflect.DeepEqual(got, p) {
				t.Errorf("Snapshot().Profile() = %+v, want %+v", got, p)
			}
		})
	}
}

func TestASnapshotSurvivesItsOwnEncoding(t *testing.T) {
	for _, p := range builtInProfiles(t) {
		t.Run(p.Name, func(t *testing.T) {
			snapshot := p.Snapshot()
			data, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			var back Snapshot
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, snapshot) {
				t.Errorf("decoded = %+v, want %+v", back, snapshot)
			}
			want, _ := snapshot.Digest()
			if got, _ := back.Digest(); got != want {
				t.Errorf("digest after decoding = %s, want %s", got, want)
			}
			if err := back.Validate(); err != nil {
				t.Errorf("Validate() = %v, want a catalog profile's snapshot to be valid", err)
			}
		})
	}
}

// A snapshot is a copy: nothing done to the profile it was taken of, or to the profile it gives
// back, reaches it.
func TestASnapshotIsDetachedFromTheProfileItWasTakenOfAndTheOneItGivesBack(t *testing.T) {
	p := fixtureProfile()
	snapshot := p.Snapshot()
	want, _ := snapshot.Digest()

	p.Stages[1].DependsOn[0] = "x"
	p.Stages[0].Name = "x"
	p.Roles[0] = "x"
	p.Checks[0] = "x"
	p.MemoryDefault.Sources[0] = MemorySourceLongtermMem
	if got, _ := snapshot.Digest(); got != want {
		t.Errorf("a change to the profile reached the snapshot: digest %s, want %s", got, want)
	}

	back := snapshot.Profile()
	back.Stages[1].DependsOn[0] = "x"
	back.Stages[0].Name = "x"
	back.Roles[0] = "x"
	back.Checks[0] = "x"
	back.MemoryDefault.Sources[0] = MemorySourceLongtermMem
	if got, _ := snapshot.Digest(); got != want {
		t.Errorf("a change to the profile it gave back reached the snapshot: digest %s, want %s", got, want)
	}
}

func TestASnapshotWritesAnEmptyListAsAnEmptyListAndNeverAsNull(t *testing.T) {
	p := fixtureProfile()
	p.Stages[0].DependsOn = nil
	p.MemoryDefault = MemoryDefault{Scope: MemoryScopeNone}
	data, err := json.Marshal(p.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, null := range []string{`"depends_on":null`, `"sources":null`} {
		if strings.Contains(string(data), null) {
			t.Errorf("encoding %s holds %s, want an empty list", data, null)
		}
	}
}

// The digest says whether a profile is the one recorded, so it changes with every field of the
// snapshot, each edit below changing exactly one.
func TestTheDigestOfASnapshotChangesWithEveryField(t *testing.T) {
	base, _ := fixtureProfile().Snapshot().Digest()
	edits := map[string]func(*WorkflowProfile){
		"name":                func(p *WorkflowProfile) { p.Name = "other" },
		"a stage name":        func(p *WorkflowProfile) { p.Stages[1].Name = "c" },
		"a stage dependency":  func(p *WorkflowProfile) { p.Stages[1].DependsOn = []string{} },
		"a stage added":       func(p *WorkflowProfile) { p.Stages = append(p.Stages, Stage{Name: "c", DependsOn: []string{"b"}}) },
		"a role":              func(p *WorkflowProfile) { p.Roles = []string{"s"} },
		"a check":             func(p *WorkflowProfile) { p.Checks = []string{"d"} },
		"the memory policy":   func(p *WorkflowProfile) { p.MemoryPolicy = "n" },
		"the review policy":   func(p *WorkflowProfile) { p.ReviewPolicy = "n" },
		"the delivery policy": func(p *WorkflowProfile) { p.DeliveryPolicy = "n" },
		"the memory scope":    func(p *WorkflowProfile) { p.MemoryDefault.Scope = MemoryScopeProject },
		"a memory source":     func(p *WorkflowProfile) { p.MemoryDefault.Sources = []MemorySource{MemorySourceLongtermMem} },
		"a memory source added": func(p *WorkflowProfile) {
			p.MemoryDefault.Sources = append(p.MemoryDefault.Sources, MemorySourceLongtermMem)
		},
		"the review dependency": func(p *WorkflowProfile) { p.ReliesOnGentleReview = false },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			p := fixtureProfile()
			edit(&p)
			got, err := p.Snapshot().Digest()
			if err != nil {
				t.Fatal(err)
			}
			if got == base {
				t.Errorf("the digest did not change when %s did", name)
			}
		})
	}
}

func TestEverySnapshotOfTheCatalogIsValidAndTheFiveDiffer(t *testing.T) {
	seen := map[string]string{}
	for _, p := range builtInProfiles(t) {
		snapshot := p.Snapshot()
		if err := snapshot.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v", p.Name, err)
		}
		digest, _ := snapshot.Digest()
		if other, dup := seen[digest]; dup {
			t.Errorf("%s and %s have the same digest %s", p.Name, other, digest)
		}
		seen[digest] = p.Name
	}
}

// Validate of a snapshot judges the snapshot alone: it asks nothing of the catalog, so a profile that
// is in no catalog is valid when it is well formed, and a well formed one is not refused for being
// unlike a built-in of the same name.
func TestASnapshotIsValidWhenItIsWellFormedWhateverTheCatalogSays(t *testing.T) {
	if err := fixtureProfile().Snapshot().Validate(); err != nil {
		t.Errorf("a fixture profile that is in no catalog: Validate() = %v, want nil", err)
	}
	p, _ := Resolve("sdd")
	p.Checks = []string{"only one"}
	p.MemoryDefault.Scope = MemoryScopeGoal
	if err := p.Snapshot().Validate(); err != nil {
		t.Errorf("an sdd that differs from the catalog's: Validate() = %v, want nil", err)
	}
}

func TestASnapshotThatIsNotWellFormedIsInvalid(t *testing.T) {
	cases := map[string]func(*Snapshot){
		"a blank name":                   func(s *Snapshot) { s.Name = " " },
		"no stages":                      func(s *Snapshot) { s.Stages = []Stage{} },
		"no roles":                       func(s *Snapshot) { s.Roles = []string{} },
		"no checks":                      func(s *Snapshot) { s.Checks = []string{} },
		"a blank memory policy":          func(s *Snapshot) { s.MemoryPolicy = "" },
		"a blank review policy":          func(s *Snapshot) { s.ReviewPolicy = " " },
		"a blank delivery policy":        func(s *Snapshot) { s.DeliveryPolicy = "" },
		"a blank stage name":             func(s *Snapshot) { s.Stages[0].Name = "" },
		"a duplicate stage":              func(s *Snapshot) { s.Stages[1].Name = "a" },
		"a dependency on a later stage":  func(s *Snapshot) { s.Stages[0].DependsOn = []string{"b"} },
		"a dependency on a missing one":  func(s *Snapshot) { s.Stages[1].DependsOn = []string{"z"} },
		"a null dependency list":         func(s *Snapshot) { s.Stages[0].DependsOn = nil },
		"a null source list":             func(s *Snapshot) { s.MemoryDefault.Sources = nil },
		"a memory scope that is unknown": func(s *Snapshot) { s.MemoryDefault.Scope = "everything" },
		"a blank memory scope":           func(s *Snapshot) { s.MemoryDefault.Scope = "" },
		"a memory source that is unknown": func(s *Snapshot) {
			s.MemoryDefault.Sources = []MemorySource{"the-internet"}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			s := fixtureProfile().Snapshot()
			edit(&s)
			if err := s.Validate(); !errors.Is(err, ErrInvalidProfile) {
				t.Errorf("Validate() = %v, want ErrInvalidProfile", err)
			}
		})
	}
}

// DifferingFields names, by their names in the encoding and in the order of it, the fields in which
// two snapshots of a profile differ: what a drift report says changed.
func TestDifferingFieldsNamesWhatChangedInTheOrderOfTheEncoding(t *testing.T) {
	base := fixtureProfile().Snapshot()
	if got := base.DifferingFields(fixtureProfile().Snapshot()); len(got) != 0 {
		t.Errorf("equal snapshots differ in %v", got)
	}
	cases := []struct {
		name string
		edit func(*WorkflowProfile)
		want []string
	}{
		{"a stage", func(p *WorkflowProfile) { p.Stages[1].Name = "c" }, []string{"stages"}},
		{"a stage dependency", func(p *WorkflowProfile) { p.Stages[1].DependsOn = []string{} }, []string{"stages"}},
		{"the roles", func(p *WorkflowProfile) { p.Roles = []string{"s"} }, []string{"roles"}},
		{"the checks", func(p *WorkflowProfile) { p.Checks = []string{"d"} }, []string{"checks"}},
		{"each policy", func(p *WorkflowProfile) { p.MemoryPolicy, p.ReviewPolicy, p.DeliveryPolicy = "x", "y", "z" }, []string{"memory_policy", "review_policy", "delivery_policy"}},
		{"the memory scope", func(p *WorkflowProfile) { p.MemoryDefault.Scope = MemoryScopeProject }, []string{"memory_default"}},
		{"a memory source", func(p *WorkflowProfile) { p.MemoryDefault.Sources = nil }, []string{"memory_default"}},
		{"the review dependency", func(p *WorkflowProfile) { p.ReliesOnGentleReview = false }, []string{"relies_on_gentle_review"}},
		{"several, in the order of the encoding", func(p *WorkflowProfile) {
			p.ReliesOnGentleReview = false
			p.Checks = []string{"d"}
			p.Stages[0].Name = "q"
		}, []string{"stages", "checks", "relies_on_gentle_review"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fixtureProfile()
			c.edit(&p)
			if got := base.DifferingFields(p.Snapshot()); !reflect.DeepEqual(got, c.want) {
				t.Errorf("DifferingFields = %v, want %v", got, c.want)
			}
		})
	}
}
