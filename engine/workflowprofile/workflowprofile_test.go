package workflowprofile

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestBuiltinsSatisfyApprovedContract(t *testing.T) {
	wantNames := []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"}
	for _, name := range wantNames {
		t.Run(name, func(t *testing.T) {
			profile, err := Resolve(name)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", name, err)
			}
			if profile.Name != name || len(profile.Stages) == 0 || len(profile.Roles) == 0 || len(profile.Checks) == 0 || profile.MemoryPolicy == "" || profile.ReviewPolicy == "" || profile.DeliveryPolicy == "" {
				t.Fatalf("profile does not define all seven contract fields: %+v", profile)
			}
			if err := Validate(profile); err != nil {
				t.Fatalf("Validate(%q): %v", name, err)
			}
		})
	}
}

func TestStageDependenciesAreOrderedAndKnown(t *testing.T) {
	profile, err := Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(profile); err != nil {
		t.Fatal(err)
	}

	profile.Stages[0].DependsOn = []string{"implement-task-by-task"}
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("forward dependency must fail closed, got %v", err)
	}
	profile, _ = Resolve("sdd")
	profile.Stages = profile.Stages[:4]
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("missing mandatory archive stage must fail closed, got %v", err)
	}
}

func TestRequiredWorkflowGatesArePresent(t *testing.T) {
	profile, _ := Resolve("sdd")
	if !testStageBefore(profile.Stages, "verify", "archive") {
		t.Fatal("SDD verify must execute before archive")
	}
	if !contains(profile.Checks, "record verify report before archive; findings do not automatically block archive") {
		t.Fatalf("SDD verify gate/report semantics missing: %#v", profile.Checks)
	}
	for name, required := range map[string][]string{
		"odd":                {"TDD only when configured", "applicable functional checks", "coordinator spot-check"},
		"standalone-minimal": {"relevant available checks", "preserve unavailable/unverified; never synthesize PASS"},
		"maintenance":        {"before/after evidence", "focused checks"},
		"incident-recovery":  {"capture initial state", "confirm final state", "stop when evidence is missing"},
	} {
		profile, err := Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, gate := range required {
			if !contains(profile.Checks, gate) {
				t.Errorf("%s missing required gate %q", name, gate)
			}
		}
	}
}

func TestSDDRequiresEveryApprovedCheck(t *testing.T) {
	requiredChecks := []string{
		"native dispatcher/dependencies",
		"configured TDD and apply checks",
		"record verify report before archive; findings do not automatically block archive",
	}
	for _, check := range requiredChecks {
		t.Run(check, func(t *testing.T) {
			profile, err := Resolve("sdd")
			if err != nil {
				t.Fatal(err)
			}
			profile.Checks = without(profile.Checks, check)
			if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
				t.Fatalf("SDD profile without mandatory check %q must be rejected, got %v", check, err)
			}
		})
	}
}

func TestIncidentRecoveryRequiresAuditedRecoveryCheck(t *testing.T) {
	profile, err := Resolve("incident-recovery")
	if err != nil {
		t.Fatal(err)
	}
	profile.Checks = without(profile.Checks, "use only supported audited recovery")
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("incident-recovery profile without audited recovery check must be rejected, got %v", err)
	}
}

func TestUnknownProfileFailsClosed(t *testing.T) {
	if _, err := Resolve("future-profile"); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("unknown profile must fail closed, got %v", err)
	}
	if err := Validate(WorkflowProfile{Name: "future-profile"}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("a profile that is not built in must not validate, got %v", err)
	}
}

// Each built-in profile carries, as typed data, how far its memory read reaches by default and
// whether it relies on Gentle AI's review, so that no consumer decides either by the profile's name.
func TestBuiltinsCarryTheirMemoryDefaultAndReviewDependencyAsTypedData(t *testing.T) {
	all := []MemorySource{MemorySourceEngram, MemorySourceLongtermMem, MemorySourceProceduralSkills}
	for _, tc := range []struct {
		profile     string
		scope       MemoryScope
		sources     []MemorySource
		reliesOnRDD bool
	}{
		{"odd", MemoryScopeProject, all, true},
		{"sdd", MemoryScopeProject, []MemorySource{MemorySourceEngram}, true},
		{"standalone-minimal", MemoryScopeNone, []MemorySource{}, false},
		{"maintenance", MemoryScopeProject, all, true},
		{"incident-recovery", MemoryScopeGoal, []MemorySource{MemorySourceEngram, MemorySourceProceduralSkills}, true},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			profile, err := Resolve(tc.profile)
			if err != nil {
				t.Fatal(err)
			}
			if profile.MemoryDefault.Scope != tc.scope {
				t.Errorf("memory default scope = %q, want %q", profile.MemoryDefault.Scope, tc.scope)
			}
			if !slices.Equal(profile.MemoryDefault.Sources, tc.sources) || profile.MemoryDefault.Sources == nil {
				t.Errorf("memory default sources = %#v, want the non-nil %#v", profile.MemoryDefault.Sources, tc.sources)
			}
			if profile.ReliesOnGentleReview != tc.reliesOnRDD {
				t.Errorf("relies on Gentle review = %v, want %v", profile.ReliesOnGentleReview, tc.reliesOnRDD)
			}
		})
	}
}

// The typed review dependency is declared beside the review policy prose, and the two must not
// drift: a profile whose policy inherits RDD without disclaiming Gentle relies on Gentle review,
// and no other does. When a policy's wording changes this fails and a person decides which of the
// two is wrong.
func TestTheReviewDependencyAgreesWithTheReviewPolicy(t *testing.T) {
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		profile, err := Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		policyRelies := strings.Contains(profile.ReviewPolicy, "RDD") && !strings.Contains(profile.ReviewPolicy, "no Gentle")
		if profile.ReliesOnGentleReview != policyRelies {
			t.Errorf("profile %q: relies on Gentle review = %v, but its review_policy %q relies on it = %v", name, profile.ReliesOnGentleReview, profile.ReviewPolicy, policyRelies)
		}
	}
}

// Every non-Engram source a profile's memory default grants is named by that profile's own
// memory_policy prose, so the typed ceiling is derived from the policy text and not independently
// interpreted.
func TestTheMemoryDefaultSourcesAreNamedByTheMemoryPolicy(t *testing.T) {
	aliases := map[MemorySource]string{
		MemorySourceLongtermMem:      "long-term memory",
		MemorySourceProceduralSkills: "procedural skills",
	}
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		profile, err := Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range profile.MemoryDefault.Sources {
			if alias, ok := aliases[source]; ok && !strings.Contains(profile.MemoryPolicy, alias) {
				t.Errorf("profile %q grants source %q by default, but its memory_policy %q does not name %q", name, source, profile.MemoryPolicy, alias)
			}
		}
	}
}

// A resolved profile is detached from the catalog: changing what Resolve returned, typed data
// included, changes the next answer of Resolve in no way.
func TestAResolvedProfileIsDetachedFromTheCatalog(t *testing.T) {
	first, _ := Resolve("odd")
	first.MemoryDefault.Sources[0] = "tampered"
	first.MemoryDefault.Scope = MemoryScopeNone
	first.ReliesOnGentleReview = false
	first.Stages[0].Name = "tampered"
	first.Checks[0] = "tampered"
	second, _ := Resolve("odd")
	if second.MemoryDefault.Sources[0] != MemorySourceEngram || second.MemoryDefault.Scope != MemoryScopeProject || !second.ReliesOnGentleReview || second.Stages[0].Name != "authorize" || second.Checks[0] != "TDD only when configured" {
		t.Errorf("the catalog changed through a resolved profile: %+v", second)
	}
}

// The checks a profile must declare are the checks the catalog lists for it, but for the ones the
// catalog marks as optional; dropping any other makes the profile invalid.
func TestValidateRequiresEveryCheckExceptTheOptionalOnes(t *testing.T) {
	optional := map[string][]string{"maintenance": {"expand with impact, not ceremony"}}
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		profile, err := Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range profile.Checks {
			t.Run(name+"/"+check, func(t *testing.T) {
				dropped := profile
				dropped.Checks = without(profile.Checks, check)
				err := Validate(dropped)
				if contains(optional[name], check) {
					if err != nil {
						t.Errorf("dropping the optional check %q made the profile invalid: %v", check, err)
					}
					return
				}
				if !errors.Is(err, ErrInvalidProfile) {
					t.Errorf("dropping the mandatory check %q was accepted, got %v", check, err)
				}
			})
		}
	}
}

// A profile's stages are exactly the catalog's, in the catalog's order, each depending on the one
// before; any other sequence is invalid.
func TestValidateRequiresTheCatalogsStageSequence(t *testing.T) {
	profile, _ := Resolve("maintenance")
	swapped := profile
	swapped.Stages = append([]Stage(nil), profile.Stages...)
	swapped.Stages[1], swapped.Stages[2] = Stage{Name: swapped.Stages[2].Name, DependsOn: []string{swapped.Stages[0].Name}}, Stage{Name: swapped.Stages[1].Name, DependsOn: []string{swapped.Stages[2].Name}}
	if err := Validate(swapped); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("swapped stages were accepted, got %v", err)
	}
	unchained := profile
	unchained.Stages = append([]Stage(nil), profile.Stages...)
	unchained.Stages[2] = Stage{Name: profile.Stages[2].Name, DependsOn: []string{profile.Stages[0].Name}}
	if err := Validate(unchained); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("a stage that does not depend on the one before it was accepted, got %v", err)
	}
}

// The typed data is validated like the rest: a profile whose memory default or review dependency
// differs from the catalog's for its name does not validate, so a tampered or empty default cannot
// pass for the profile it is called.
func TestValidateRequiresTheCatalogsTypedData(t *testing.T) {
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		for _, tc := range []struct {
			what string
			edit func(p *WorkflowProfile)
		}{
			{"an empty memory default", func(p *WorkflowProfile) { p.MemoryDefault = MemoryDefault{} }},
			{"a wider scope", func(p *WorkflowProfile) {
				p.MemoryDefault.Scope = MemoryScopeProject
				p.MemoryDefault.Sources = append(p.MemoryDefault.Sources, "extra")
			}},
			{"another scope", func(p *WorkflowProfile) {
				if p.MemoryDefault.Scope == MemoryScopeNone {
					p.MemoryDefault.Scope = MemoryScopeGoal
				} else {
					p.MemoryDefault.Scope = MemoryScopeNone
				}
			}},
			{"a source dropped or added", func(p *WorkflowProfile) {
				if len(p.MemoryDefault.Sources) > 0 {
					p.MemoryDefault.Sources = p.MemoryDefault.Sources[1:]
				} else {
					p.MemoryDefault.Sources = []MemorySource{MemorySourceEngram}
				}
			}},
			{"the review dependency flipped", func(p *WorkflowProfile) { p.ReliesOnGentleReview = !p.ReliesOnGentleReview }},
		} {
			t.Run(name+"/"+tc.what, func(t *testing.T) {
				profile, err := Resolve(name)
				if err != nil {
					t.Fatal(err)
				}
				if err := Validate(profile); err != nil {
					t.Fatalf("the catalog's own profile is invalid: %v", err)
				}
				tc.edit(&profile)
				if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
					t.Errorf("Validate accepted %s, got %v", tc.what, err)
				}
			})
		}
	}
}

// The last stage is named by the catalog too, and no dependency says so: renaming it keeps every
// dependency intact, so only the comparison of names refuses it.
func TestValidateRefusesARenamedLastStage(t *testing.T) {
	profile, _ := Resolve("sdd")
	profile.Stages[len(profile.Stages)-1].Name = "ship"
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("a profile whose last stage is renamed was accepted, got %v", err)
	}
}

// Verify before archive is not a rule of its own: the stage sequence must equal the catalog's, and
// the catalog's puts verify before archive, so a profile that swaps them is invalid.
func TestValidateRejectsAnSDDProfileThatArchivesBeforeItVerifies(t *testing.T) {
	profile, _ := Resolve("sdd")
	n := len(profile.Stages)
	profile.Stages[n-2] = Stage{Name: "archive", DependsOn: []string{"apply"}}
	profile.Stages[n-1] = Stage{Name: "verify", DependsOn: []string{"archive"}}
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("a profile that archives before it verifies was accepted, got %v", err)
	}
}

// The stages a workflow has recorded must be a prefix of its profile's declared order; this is the
// one rule that says so, and the one that names the next stage.
func TestTheRecordedStagesMustBeAPrefixOfTheDeclaredOrder(t *testing.T) {
	profile, _ := Resolve("standalone-minimal")
	for _, tc := range []struct {
		name     string
		recorded []string
		wantErr  string
	}{
		{"none", nil, ""},
		{"one", []string{"authorize"}, ""},
		{"all", []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report"}, ""},
		{"too many", []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report", "extra"}, `recorded 6 stages exceeds profile "standalone-minimal"'s 5 declared stages`},
		{"out of order", []string{"authorize", "check"}, `recorded stage 1 is "check", want "bound-scope" per profile "standalone-minimal"'s declared order`},
		{"unknown", []string{"nope"}, `recorded stage 0 is "nope", want "authorize" per profile "standalone-minimal"'s declared order`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := profile.CheckStagePrefix(tc.recorded)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("CheckStagePrefix = %v, want nil", err)
			case tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr):
				t.Errorf("CheckStagePrefix = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestTheNextStageIsTheOneAfterTheRecordedOnes(t *testing.T) {
	profile, _ := Resolve("standalone-minimal")
	for _, tc := range []struct {
		recorded int
		want     string
		ok       bool
	}{
		{0, "authorize", true},
		{1, "bound-scope", true},
		{4, "report", true},
		{5, "", false},
		{9, "", false},
	} {
		got, ok := profile.NextStage(tc.recorded)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NextStage(%d) = %q, %v; want %q, %v", tc.recorded, got, ok, tc.want, tc.ok)
		}
	}
}

func TestProfileDataDoesNotGrantAuthority(t *testing.T) {
	profile, _ := Resolve("standalone-minimal")
	if profile.MemoryPolicy != "no persistence required; allow only a store explicitly configured by the caller" {
		t.Fatalf("unexpected standalone-minimal memory policy %q", profile.MemoryPolicy)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 7 {
		t.Fatalf("profile must serialize exactly the seven declarative contract fields, got %v", fields)
	}
}

func testStageBefore(stages []Stage, first, second string) bool {
	firstIndex, secondIndex := -1, -1
	for i, stage := range stages {
		if stage.Name == first {
			firstIndex = i
		}
		if stage.Name == second {
			secondIndex = i
		}
	}
	return firstIndex >= 0 && secondIndex > firstIndex
}

func without(values []string, unwanted string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
