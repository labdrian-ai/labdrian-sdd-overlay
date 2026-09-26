package shaper

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// validHandoffV2JSON is a structurally valid handoff version 2 with one
// criterion verified by a deterministic check and one by a human
// adjudication path.
const validHandoffV2JSON = `{"version":2,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha",` +
	`"architecture":"Layered CLI with a shared jsonstrict wire gate.",` +
	`"stages":["Extract jsonstrict.","Refactor goal.Parse onto jsonstrict."],` +
	`"acceptance":[` +
	`{"criterion":"go test ./... passes.","verification":{"check":"cd engine && go test -count=1 ./..."}},` +
	`{"criterion":"The CLI help reads clearly.","verification":{"adjudication":"A maintainer judges whether the help text names every verb."}}` +
	`],"out_of_scope":["Runtime clearance UI."]}`

// v2DocumentWith returns validHandoffV2JSON with top-level fields replaced.
func v2DocumentWith(t *testing.T, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validHandoffV2JSON), &fields); err != nil {
		t.Fatalf("unmarshal v2 fixture: %v", err)
	}
	for k, v := range changes {
		fields[k] = v
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal v2 fixture: %v", err)
	}
	return data
}

// v2WithAcceptance returns validHandoffV2JSON with the raw acceptance array
// replaced verbatim, so tests control key case, nulls, and duplicates.
func v2WithAcceptance(rawAcceptance string) []byte {
	start := strings.Index(validHandoffV2JSON, `"acceptance":[`)
	end := strings.Index(validHandoffV2JSON, `],"out_of_scope"`)
	return []byte(validHandoffV2JSON[:start] + `"acceptance":` + rawAcceptance + validHandoffV2JSON[end+1:])
}

func TestParseV2AcceptsCriterionWithExactlyOneVerification(t *testing.T) {
	got, err := Parse([]byte(validHandoffV2JSON))
	if err != nil {
		t.Fatalf("Parse(valid v2): %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2", got.Version)
	}
	if got.Acceptance != nil {
		t.Errorf("Acceptance = %#v, want nil for version 2", got.Acceptance)
	}
	want := []AcceptanceItem{
		{Criterion: "go test ./... passes.", Verification: AcceptanceVerification{Check: "cd engine && go test -count=1 ./..."}},
		{Criterion: "The CLI help reads clearly.", Verification: AcceptanceVerification{Adjudication: "A maintainer judges whether the help text names every verb."}},
	}
	if !reflect.DeepEqual(got.AcceptanceItems, want) {
		t.Errorf("AcceptanceItems = %#v\nwant %#v", got.AcceptanceItems, want)
	}
}

func TestParseV2PreservesAuthoredCriterionText(t *testing.T) {
	data := v2WithAcceptance(`[{"criterion":"  keep  ","verification":{"check":"  go test  "}}]`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.AcceptanceItems[0].Criterion != "  keep  " || got.AcceptanceItems[0].Verification.Check != "  go test  " {
		t.Errorf("Parse rewrote authored v2 strings: %#v", got.AcceptanceItems[0])
	}
}

func TestParseV1LeavesAcceptanceItemsNil(t *testing.T) {
	got, err := Parse([]byte(validHandoffJSON))
	if err != nil {
		t.Fatalf("Parse(v1): %v", err)
	}
	if got.AcceptanceItems != nil {
		t.Errorf("AcceptanceItems = %#v, want nil for version 1", got.AcceptanceItems)
	}
}

func TestParseV2RejectsInvalidAcceptanceWithoutPartialHandoff(t *testing.T) {
	check := `"verification":{"check":"go test"}`
	cases := []struct {
		name  string
		data  []byte
		match string
	}{
		{name: "v1 string items", data: v2WithAcceptance(`["go test ./... passes."]`)},
		{name: "empty acceptance", data: v2WithAcceptance(`[]`), match: "acceptance"},
		{name: "null acceptance", data: v2WithAcceptance(`null`), match: "acceptance"},
		{name: "null item", data: v2WithAcceptance(`[null]`), match: "acceptance"},
		{name: "item not object", data: v2WithAcceptance(`[17]`)},
		{name: "missing criterion", data: v2WithAcceptance(`[{` + check + `}]`), match: "criterion"},
		{name: "blank criterion", data: v2WithAcceptance(`[{"criterion":" \t ",` + check + `}]`), match: "criterion"},
		{name: "null criterion", data: v2WithAcceptance(`[{"criterion":null,` + check + `}]`), match: "criterion"},
		{name: "non-string criterion", data: v2WithAcceptance(`[{"criterion":3,` + check + `}]`)},
		{name: "missing verification", data: v2WithAcceptance(`[{"criterion":"x"}]`), match: "verification"},
		{name: "null verification", data: v2WithAcceptance(`[{"criterion":"x","verification":null}]`), match: "verification"},
		{name: "verification not object", data: v2WithAcceptance(`[{"criterion":"x","verification":"go test"}]`)},
		{name: "empty verification", data: v2WithAcceptance(`[{"criterion":"x","verification":{}}]`), match: "exactly one"},
		{name: "check and adjudication", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":"go test","adjudication":"judge"}}]`), match: "exactly one"},
		{name: "blank check", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":"  "}}]`), match: "check"},
		{name: "empty check", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":""}}]`), match: "check"},
		{name: "blank adjudication", data: v2WithAcceptance(`[{"criterion":"x","verification":{"adjudication":"\n"}}]`), match: "adjudication"},
		{name: "null check", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":null}}]`), match: "check"},
		{name: "null check beside adjudication", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":null,"adjudication":"judge"}}]`), match: "exactly one"},
		{name: "non-string check", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":["go","test"]}}]`)},
		{name: "unknown item key", data: v2WithAcceptance(`[{"criterion":"x",` + check + `,"result":"pass"}]`), match: "unknown"},
		{name: "unknown verification key", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":"go test","passed":true}}]`), match: "unknown"},
		{name: "contradiction flags in item", data: v2WithAcceptance(`[{"criterion":"x",` + check + `,"flags":[]}]`), match: "unknown"},
		{name: "case-variant criterion", data: v2WithAcceptance(`[{"CRITERION":"x",` + check + `}]`), match: "unknown"},
		{name: "case-variant criterion beside criterion", data: v2WithAcceptance(`[{"criterion":"x","Criterion":"y",` + check + `}]`), match: "unknown"},
		{name: "case-variant verification", data: v2WithAcceptance(`[{"criterion":"x","Verification":{"check":"go test"}}]`), match: "unknown"},
		{name: "case-variant check", data: v2WithAcceptance(`[{"criterion":"x","verification":{"CHECK":"go test"}}]`), match: "unknown"},
		{name: "case-variant check beside adjudication", data: v2WithAcceptance(`[{"criterion":"x","verification":{"adjudication":"judge","Check":"go test"}}]`), match: "unknown"},
		{name: "duplicate verification key", data: v2WithAcceptance(`[{"criterion":"x","verification":{"check":"a","check":"b"}}]`), match: "duplicate"},
		{name: "top-level flags field", data: v2DocumentWith(t, map[string]any{"flags": []any{}}), match: "unknown"},
		{name: "blank stage keeps v1 rule", data: v2DocumentWith(t, map[string]any{"stages": []string{" "}}), match: "stages"},
		{name: "null out_of_scope keeps v1 rule", data: v2DocumentWith(t, map[string]any{"out_of_scope": nil}), match: "out_of_scope"},
		{name: "blank project_id keeps v1 rule", data: v2DocumentWith(t, map[string]any{"project_id": " "}), match: "project_id"},
		{name: "blank goal_id keeps v1 rule", data: v2DocumentWith(t, map[string]any{"goal_id": " "}), match: "goal_id"},
		{name: "blank architecture keeps v1 rule", data: v2DocumentWith(t, map[string]any{"architecture": " "}), match: "architecture"},
		{name: "criterion overlaps out_of_scope", data: v2DocumentWith(t, map[string]any{"out_of_scope": []string{"go test ./... passes."}}), match: "byte-identical"},
		{name: "stage overlaps out_of_scope", data: v2DocumentWith(t, map[string]any{"out_of_scope": []string{"Extract jsonstrict."}}), match: "byte-identical"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("Parse accepted invalid v2 document %s", tc.data)
			}
			if !reflect.DeepEqual(got, Handoff{}) {
				t.Errorf("Parse returned partial Handoff on error: %#v", got)
			}
			if tc.match != "" && !strings.Contains(err.Error(), tc.match) {
				t.Errorf("error %q does not name %q", err, tc.match)
			}
		})
	}
}

func TestParseV2OverlapRuleIsByteIdenticalOnly(t *testing.T) {
	// A check or adjudication text equal to an out_of_scope item, or a
	// criterion differing only in case or spacing, is not literal criterion
	// overlap.
	data := v2DocumentWith(t, map[string]any{"out_of_scope": []string{
		"cd engine && go test -count=1 ./...",
		"GO TEST ./... PASSES.",
		"go test ./... passes. ",
	}})
	if _, err := Parse(data); err != nil {
		t.Errorf("Parse rejected non-identical overlap: %v", err)
	}
}

func sampleHandoffV2() Handoff {
	return Handoff{
		Version:      2,
		ProjectID:    "p",
		GoalID:       "g",
		Architecture: "a",
		Stages:       []string{"s"},
		AcceptanceItems: []AcceptanceItem{
			{Criterion: "c", Verification: AcceptanceVerification{Check: "go test"}},
		},
		OutOfScope: []string{},
	}
}

func TestValidateV2(t *testing.T) {
	if err := sampleHandoffV2().Validate(); err != nil {
		t.Fatalf("Validate(valid v2): %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Handoff)
	}{
		{name: "nil items", mutate: func(h *Handoff) { h.AcceptanceItems = nil }},
		{name: "empty items", mutate: func(h *Handoff) { h.AcceptanceItems = []AcceptanceItem{} }},
		{name: "v1 string acceptance present", mutate: func(h *Handoff) { h.Acceptance = []string{"c"} }},
		{name: "blank criterion", mutate: func(h *Handoff) { h.AcceptanceItems[0].Criterion = " " }},
		{name: "no verification", mutate: func(h *Handoff) { h.AcceptanceItems[0].Verification = AcceptanceVerification{} }},
		{name: "both", mutate: func(h *Handoff) { h.AcceptanceItems[0].Verification.Adjudication = "judge" }},
		{name: "blank check", mutate: func(h *Handoff) { h.AcceptanceItems[0].Verification.Check = "\t" }},
		{name: "criterion overlap", mutate: func(h *Handoff) { h.OutOfScope = []string{"c"} }},
		{name: "unsupported version 3", mutate: func(h *Handoff) { h.Version = 3 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := sampleHandoffV2()
			h.AcceptanceItems = append([]AcceptanceItem(nil), h.AcceptanceItems...)
			tc.mutate(&h)
			if err := h.Validate(); err == nil {
				t.Errorf("Validate accepted %s", tc.name)
			}
		})
	}
}

func TestValidateV1RejectsAcceptanceItems(t *testing.T) {
	h := sampleHandoff()
	h.AcceptanceItems = []AcceptanceItem{{Criterion: "c", Verification: AcceptanceVerification{Check: "go test"}}}
	if err := h.Validate(); err == nil {
		t.Errorf("Validate accepted a version 1 handoff carrying v2 acceptance items")
	}
}

// --- readiness ---

func completeV2Input(t *testing.T) ReadinessInput {
	t.Helper()
	return inputWith(t, []byte(validHandoffV2JSON), []byte(goalV2JSONWithNonGoals(`[]`)))
}

// verifiedFor evaluates in without clearance and returns a clearance Verify
// built from an affirmative record resolving every raised flag.
func verifiedFor(t *testing.T, in ReadinessInput) *VerifiedClearance {
	t.Helper()
	a := freshAssessment(t, in)
	c, err := Verify(marshalRecord(t, recordFor(t, a)), *a.Subject, a.Flags, a.View)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return c
}

func TestEvaluateV2DropsOnlyTheAcceptanceVerificationCap(t *testing.T) {
	got := Evaluate(completeV2Input(t), nil)
	if got.State != StateDraft {
		t.Fatalf("State = %q, want draft", got.State)
	}
	if want := []Reason{ReasonClearanceMissing}; !reflect.DeepEqual(blockerReasons(got), want) {
		t.Errorf("blockers = %v, want %v", blockerReasons(got), want)
	}
}

func TestEvaluateV2WithVerifiedClearanceIsReady(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        func(t *testing.T) ReadinessInput
		wantFlags int
	}{
		{name: "zero flags", in: completeV2Input, wantFlags: 0},
		{name: "one resolved criterion flag", in: func(t *testing.T) ReadinessInput {
			return inputWith(t, []byte(validHandoffV2JSON), []byte(goalV2JSONWithNonGoals(`["go test ./... passes."]`)))
		}, wantFlags: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := verifiedFor(t, tc.in(t))
			got := Evaluate(tc.in(t), c)
			if got.State != StateReady {
				t.Fatalf("State = %q, want ready (blockers %v)", got.State, blockerReasons(got))
			}
			if len(got.Blockers) != 0 {
				t.Errorf("blockers = %v, want none", blockerReasons(got))
			}
			if len(got.Flags) != tc.wantFlags {
				t.Errorf("flags = %v, want %d", got.Flags, tc.wantFlags)
			}
			if got.Subject == nil || len(got.View) == 0 {
				t.Errorf("ready assessment lacks its subject or view")
			}
		})
	}
}

func TestEvaluateV2EveryMissingPrerequisiteKeepsDraft(t *testing.T) {
	cleared := verifiedFor(t, completeV2Input(t))
	declined := func(t *testing.T) *VerifiedClearance {
		a := freshAssessment(t, completeV2Input(t))
		r := recordFor(t, a)
		r.Decision = DecisionDecline
		c, err := Verify(marshalRecord(t, r), *a.Subject, a.Flags, a.View)
		if err == nil || c != nil {
			t.Fatalf("Verify cleared a decline: (%v, %v)", c, err)
		}
		return c
	}
	escCriterion := []byte(strings.Replace(validHandoffV2JSON, `"criterion":"go test ./... passes."`, `"criterion":"go test\u001b[8m hidden"`, 1))
	overlapGoal := []byte(goalV2JSONWithNonGoals(`["go test ./... passes."]`))
	for _, tc := range []struct {
		name      string
		in        func(t *testing.T) ReadinessInput
		clearance func(t *testing.T) *VerifiedClearance
		want      Reason
	}{
		{name: "no clearance", in: completeV2Input, clearance: func(*testing.T) *VerifiedClearance { return nil }, want: ReasonClearanceMissing},
		{name: "unverified clearance", in: completeV2Input, clearance: func(*testing.T) *VerifiedClearance { return &VerifiedClearance{} }, want: ReasonClearanceUnverified},
		{name: "declined clearance", in: completeV2Input, clearance: declined, want: ReasonClearanceMissing},
		{name: "goal unbound", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Goal = nil
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonGoalUnbound},
		{name: "goal version 1", in: func(t *testing.T) ReadinessInput {
			g := strings.Replace(strings.Replace(goalV2JSONWithNonGoals(`[]`), `"goal_id":"goal-alpha",`, "", 1), `"version":2`, `"version":1`, 1)
			return inputWith(t, []byte(validHandoffV2JSON), []byte(g))
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonGoalInvalid},
		{name: "goal digest mismatch", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Goal.GoalSHA256 = strings.Repeat("0", 64)
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonGoalDigestMismatch},
		{name: "handoff digest mismatch", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Handoff.SHA256 = strings.Repeat("0", 64)
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonHandoffDigestMismatch},
		{name: "handoff source unrecorded", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Handoff.SourcePath = ""
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonSourceUnrecorded},
		{name: "goal source unrecorded", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Goal.SourcePath = ""
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonSourceUnrecorded},
		{name: "goal project identity mismatch", in: func(t *testing.T) ReadinessInput {
			g := strings.Replace(goalV2JSONWithNonGoals(`[]`), `"project_id":"standalone-shaper-handoff"`, `"project_id":"other-project"`, 1)
			return inputWith(t, []byte(validHandoffV2JSON), []byte(g))
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonGoalIdentityMismatch},
		{name: "goal id identity mismatch", in: func(t *testing.T) ReadinessInput {
			g := strings.Replace(goalV2JSONWithNonGoals(`[]`), `"goal_id":"goal-alpha"`, `"goal_id":"goal-beta"`, 1)
			return inputWith(t, []byte(validHandoffV2JSON), []byte(g))
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonGoalIdentityMismatch},
		{name: "provenance unobserved", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Provenance = nil
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonWorktreeProvenanceUnobserved},
		{name: "provenance incomplete", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.Provenance.CommonDir = ""
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonWorktreeProvenanceIncomplete},
		{name: "provenance mismatch", in: func(t *testing.T) ReadinessInput {
			in := completeV2Input(t)
			in.WorktreeRoot = "/work/other"
			return in
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonWorktreeProvenanceMismatch},
		{name: "view unpresentable", in: func(t *testing.T) ReadinessInput {
			return inputWith(t, escCriterion, []byte(goalV2JSONWithNonGoals(`[]`)))
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonViewUnpresentable},
		{name: "unresolved flag", in: func(t *testing.T) ReadinessInput {
			return inputWith(t, []byte(validHandoffV2JSON), overlapGoal)
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonFlagsUnresolved},
		{name: "plan edited after clearance", in: func(t *testing.T) ReadinessInput {
			return inputWith(t, []byte(validHandoffV2JSON+"\n"), []byte(goalV2JSONWithNonGoals(`[]`)))
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonClearanceMismatch},
		{name: "verification changed after clearance", in: func(t *testing.T) ReadinessInput {
			edited := strings.Replace(validHandoffV2JSON, `{"check":"cd engine && go test -count=1 ./..."}`, `{"adjudication":"cd engine && go test -count=1 ./..."}`, 1)
			return inputWith(t, []byte(edited), []byte(goalV2JSONWithNonGoals(`[]`)))
		}, clearance: func(*testing.T) *VerifiedClearance { return cleared }, want: ReasonClearanceMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in(t), tc.clearance(t))
			if got.State != StateDraft {
				t.Fatalf("State = %q, want draft (blockers %v)", got.State, blockerReasons(got))
			}
			if !hasReason(got, tc.want) {
				t.Errorf("blockers = %v, want to include %q", blockerReasons(got), tc.want)
			}
			if hasReason(got, ReasonAcceptanceVerificationUnrepresentable) {
				t.Errorf("v2 handoff kept the v1 acceptance cap: %v", blockerReasons(got))
			}
		})
	}
}

func TestEvaluateV2CriterionOverlappingOutOfScopeIsInvalid(t *testing.T) {
	data := v2DocumentWith(t, map[string]any{"out_of_scope": []string{"The CLI help reads clearly."}})
	in := completeV2Input(t)
	in.Handoff.Bytes = data
	in.Handoff.SHA256 = sha256Hex(data)
	got := Evaluate(in, nil)
	if got.State != StateInvalid {
		t.Fatalf("State = %q, want invalid", got.State)
	}
}

func TestEvaluateV2FlagsCriterionOverlappingGoalNonGoal(t *testing.T) {
	in := inputWith(t, []byte(validHandoffV2JSON), []byte(goalV2JSONWithNonGoals(`["The CLI help reads clearly.","cd engine && go test -count=1 ./..."]`)))
	got := Evaluate(in, nil)
	want := []Flag{{
		ID:    "goal_non_goal_overlap:acceptance:2",
		Kind:  FlagGoalNonGoalOverlap,
		Field: "acceptance",
		Index: 2,
		Item:  "The CLI help reads clearly.",
	}}
	if !reflect.DeepEqual(got.Flags, want) {
		t.Errorf("Flags = %#v\nwant %#v (check text must not raise a flag)", got.Flags, want)
	}
	if !hasReason(got, ReasonFlagsUnresolved) {
		t.Errorf("blockers = %v, want %q", blockerReasons(got), ReasonFlagsUnresolved)
	}
}

// --- presented view ---

func TestRenderViewV2ShowsEachCriterionWithItsVerification(t *testing.T) {
	a := freshAssessment(t, completeV2Input(t))
	view := string(a.View)
	for _, want := range []string{
		"labdrian shaper clearance view 2\n",
		"acceptance 2\n",
		"criterion 21\ngo test ./... passes.\n",
		"verification_check 35\ncd engine && go test -count=1 ./...\n",
		"criterion 27\nThe CLI help reads clearly.\n",
		"verification_adjudication 59\nA maintainer judges whether the help text names every verb.\n",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("v2 view lacks %q:\n%s", want, view)
		}
	}
	if i, j := strings.Index(view, "acceptance 2\n"), strings.Index(view, "\nflags "); i < 0 || j < i {
		t.Errorf("acceptance section must precede flags:\n%s", view)
	}
}

func TestRenderViewV1IsUnchangedByV2(t *testing.T) {
	a := freshAssessment(t, completeInput(t))
	if !bytes.HasPrefix(a.View, []byte("labdrian shaper clearance view 1\n")) {
		t.Errorf("v1 view header changed:\n%s", a.View)
	}
	if bytes.Contains(a.View, []byte("\nacceptance ")) || bytes.Contains(a.View, []byte("verification_")) {
		t.Errorf("v1 view gained a v2 acceptance section:\n%s", a.View)
	}
}

func TestRenderViewV2DistinguishesCheckFromAdjudication(t *testing.T) {
	item := AcceptanceItem{Criterion: "c", Verification: AcceptanceVerification{Check: "same text"}}
	other := AcceptanceItem{Criterion: "c", Verification: AcceptanceVerification{Adjudication: "same text"}}
	a := RenderView(PresentedView{Acceptance: []AcceptanceItem{item}})
	b := RenderView(PresentedView{Acceptance: []AcceptanceItem{other}})
	if bytes.Equal(a, b) {
		t.Errorf("a check and an adjudication with the same text render identically")
	}
}

func TestEvaluateV2RefusesUnpresentableAcceptanceText(t *testing.T) {
	// JSON escapes keep the plan section printable ASCII, so only the
	// rendered acceptance sections expose the decoded control rune.
	for _, tc := range []struct {
		name    string
		old     string
		new     string
		section string
	}{
		{name: "criterion", old: `"criterion":"go test ./... passes."`, new: `"criterion":"go test\u001b[8m passes."`, section: "acceptance[0].criterion"},
		{name: "check", old: `"check":"cd engine && go test -count=1 ./..."`, new: `"check":"go test\r rm -rf"`, section: "acceptance[0].check"},
		{name: "adjudication", old: `"adjudication":"A maintainer judges whether the help text names every verb."`, new: `"adjudication":"judge\u202eadmin"`, section: "acceptance[1].adjudication"},
		{name: "adjudication c1", old: `"adjudication":"A maintainer judges whether the help text names every verb."`, new: `"adjudication":"judge\u009b8m"`, section: "acceptance[1].adjudication"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(strings.Replace(validHandoffV2JSON, tc.old, tc.new, 1))
			if bytes.Equal(data, []byte(validHandoffV2JSON)) {
				t.Fatalf("fixture edit did not apply")
			}
			if err := checkPresentableText("plan", data); err != nil {
				t.Fatalf("fixture must keep the plan bytes presentable: %v", err)
			}
			got := Evaluate(inputWith(t, data, []byte(goalV2JSONWithNonGoals(`[]`))), nil)
			if got.Subject != nil || got.View != nil {
				t.Errorf("unpresentable v2 view produced a subject or view")
			}
			var detail string
			for _, b := range got.Blockers {
				if b.Reason == ReasonViewUnpresentable {
					detail = b.Detail
				}
			}
			if !strings.Contains(detail, tc.section) {
				t.Errorf("view_unpresentable detail %q does not name %q (blockers %v)", detail, tc.section, blockerReasons(got))
			}
		})
	}
}

func TestPlannedVerificationIsNeverExecuted(t *testing.T) {
	// A check is recorded as planned verification only: evaluating a handoff
	// whose check would fail if run still reaches ready.
	data := []byte(strings.Replace(validHandoffV2JSON, `"check":"cd engine && go test -count=1 ./..."`, `"check":"false"`, 1))
	in := inputWith(t, data, []byte(goalV2JSONWithNonGoals(`[]`)))
	if got := Evaluate(in, verifiedFor(t, in)); got.State != StateReady {
		t.Errorf("State = %q, want ready: checks are not readiness prerequisites (blockers %v)", got.State, blockerReasons(got))
	}
}

func TestPlanIncompleteDisclosureNamesAbsentPhase3Fields(t *testing.T) {
	for _, want := range []string{"Phase 3", "not yet met", "roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit"} {
		if !strings.Contains(ReadyDisclosure, want) {
			t.Errorf("ReadyDisclosure does not state %q:\n%s", want, ReadyDisclosure)
		}
	}
	if !strings.HasPrefix(ReadyDisclosure, ForgeryDisclosure) {
		t.Errorf("ReadyDisclosure must keep the forgery-limit disclosure first")
	}
}
