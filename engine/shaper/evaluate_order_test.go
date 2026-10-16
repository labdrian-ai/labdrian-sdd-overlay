package shaper

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// TestEvaluateListsEveryBlockerInTheOrderOfTheEvidence pins what Evaluate says when several things
// are wrong at once: every blocker, with its words, in the order the evidence is looked at (the
// handoff, the Goal, the worktree, the view, the flags, the clearance, the acceptance cap). Evaluate
// is a sequence of steps over one evaluation, and this is what says the steps still run in that
// order and say what they always said.
func TestEvaluateListsEveryBlockerInTheOrderOfTheEvidence(t *testing.T) {
	goalBytes := []byte(goalV2JSONWithNonGoals(`[]`))
	handoffSum := sha256hex.Sum([]byte(validHandoffJSON))
	goalSum := sha256hex.Sum(goalBytes)
	_, goalParseErr := goal.Parse([]byte(`{}`))
	if goalParseErr == nil {
		t.Fatal("setup: goal.Parse of an empty object = nil, want an error")
	}
	otherProject := []byte(strings.Replace(string(goalBytes), "standalone-shaper-handoff", "other-project", 1))
	const (
		noClearance   = "no host-owned clearance supplied"
		v1Cap         = "handoff version 1 has no per-criterion verification method or adjudication path"
		unobserved    = "no worktree observation supplied"
		incomplete    = "observation lacks toplevel, git dir, or common dir"
		unverified    = "clearance was not produced by Verify"
		cannotMatch   = "subject evidence is incomplete or refused, so the clearance cannot be matched"
		differentFrom = "clearance was verified against a different subject, view, or flag set"
	)

	for _, tc := range []struct {
		name      string
		input     func(t *testing.T) ReadinessInput
		clearance func(t *testing.T, in ReadinessInput) *VerifiedClearance
		want      []Blocker
		noSubject bool
	}{
		{
			name: "everything a caller can get wrong at once, on a version 1 handoff",
			input: func(t *testing.T) ReadinessInput {
				in := completeInput(t)
				in.Handoff.SHA256, in.Handoff.SourcePath = "stale-handoff", ""
				in.Goal.GoalSHA256, in.Goal.SourcePath = "stale-goal", ""
				in.Provenance = nil
				return in
			},
			want: []Blocker{
				{Reason: ReasonHandoffDigestMismatch, Detail: `recorded handoff sha256 "stale-handoff", bytes hash to "` + handoffSum + `"`},
				{Reason: ReasonSourceUnrecorded, Detail: "handoff source path is empty"},
				{Reason: ReasonGoalDigestMismatch, Detail: `recorded goal sha256 "stale-goal", bytes hash to "` + goalSum + `"`},
				{Reason: ReasonSourceUnrecorded, Detail: "goal source path is empty"},
				{Reason: ReasonWorktreeProvenanceUnobserved, Detail: unobserved},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
				{Reason: ReasonAcceptanceVerificationUnrepresentable, Detail: v1Cap},
			},
			noSubject: true,
		},
		{
			name: "no Goal and an incomplete observation, on a version 2 handoff",
			input: func(t *testing.T) ReadinessInput {
				in := inputWith(t, []byte(validHandoffV2JSON), goalBytes)
				in.Goal = nil
				in.Provenance.GitDir = ""
				return in
			},
			want: []Blocker{
				{Reason: ReasonGoalUnbound, Detail: "no Goal binding supplied"},
				{Reason: ReasonWorktreeProvenanceIncomplete, Detail: incomplete},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
			},
			noSubject: true,
		},
		{
			name: "an invalid Goal, then a worktree root that is not absolute",
			input: func(t *testing.T) ReadinessInput {
				in := completeInput(t)
				in.Goal.GoalBytes = []byte(`{}`)
				in.Goal.GoalSHA256 = sha256hex.Sum(in.Goal.GoalBytes)
				in.WorktreeRoot = "work/tree"
				return in
			},
			want: []Blocker{
				{Reason: ReasonGoalInvalid, Detail: goalParseErr.Error()},
				{Reason: ReasonWorktreeProvenanceMismatch, Detail: `worktree root "work/tree", observed toplevel "/work/tree"`},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
				{Reason: ReasonAcceptanceVerificationUnrepresentable, Detail: v1Cap},
			},
			noSubject: true,
		},
		{
			name: "a Goal of another project, then another toplevel",
			input: func(t *testing.T) ReadinessInput {
				in := completeInput(t)
				in.Goal.GoalBytes = otherProject
				in.Goal.GoalSHA256 = sha256hex.Sum(otherProject)
				in.Provenance.Toplevel = "/elsewhere"
				return in
			},
			want: []Blocker{
				{Reason: ReasonGoalIdentityMismatch, Detail: `handoff ("standalone-shaper-handoff", "goal-alpha"), goal ("other-project", "goal-alpha")`},
				{Reason: ReasonWorktreeProvenanceMismatch, Detail: `worktree root "/work/tree", observed toplevel "/elsewhere"`},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
				{Reason: ReasonAcceptanceVerificationUnrepresentable, Detail: v1Cap},
			},
			noSubject: true,
		},
		{
			name:  "flags unresolved are said before the clearance",
			input: overlapInput,
			want: []Blocker{
				{Reason: ReasonFlagsUnresolved, Detail: "1 flag(s) await a host-captured human resolution"},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
				{Reason: ReasonAcceptanceVerificationUnrepresentable, Detail: v1Cap},
			},
		},
		{
			name: "a version 3 stage that is a Goal non-goal is refused where the Goal is looked at",
			input: func(t *testing.T) ReadinessInput {
				return inputWith(t, []byte(validHandoffV3JSON), []byte(goalV2JSONWithNonGoals(`["Extract jsonstrict."]`)))
			},
			want: []Blocker{
				{Reason: ReasonGoalNonGoalOverlap, Detail: `stages item 1 "Extract jsonstrict." is byte-identical to a Goal non_goals item (OD6)`},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
			},
		},
		{
			name: "a view that cannot be presented comes before the clearance, and leaves no subject",
			input: func(t *testing.T) ReadinessInput {
				data := strings.Replace(validHandoffV3JSON, `"Project-scoped."`, `"Project`+"\\"+`u001b[8m-scoped."`, 1)
				return inputWith(t, []byte(data), goalBytes)
			},
			want: []Blocker{
				{Reason: ReasonViewUnpresentable, Detail: "view section memory_scope at rune offset 7 holds U+001B, a terminal control or invisible formatting character; the view is unpresentable and is refused, not rewritten"},
				{Reason: ReasonClearanceMissing, Detail: noClearance},
			},
			noSubject: true,
		},
		{
			name:      "a clearance nobody verified",
			input:     completeV3Input,
			clearance: func(*testing.T, ReadinessInput) *VerifiedClearance { return &VerifiedClearance{} },
			want:      []Blocker{{Reason: ReasonClearanceUnverified, Detail: unverified}},
		},
		{
			name: "a verified clearance when the subject evidence is not complete",
			input: func(t *testing.T) ReadinessInput {
				in := completeV3Input(t)
				in.Provenance = nil
				return in
			},
			clearance: func(t *testing.T, _ ReadinessInput) *VerifiedClearance { return verifiedFor(t, completeV3Input(t)) },
			want: []Blocker{
				{Reason: ReasonWorktreeProvenanceUnobserved, Detail: unobserved},
				{Reason: ReasonClearanceUnverified, Detail: cannotMatch},
			},
			noSubject: true,
		},
		{
			name: "a clearance verified for another subject",
			input: func(t *testing.T) ReadinessInput {
				in := completeV3Input(t)
				in.Handoff.SourcePath = "elsewhere.json"
				return in
			},
			clearance: func(t *testing.T, _ ReadinessInput) *VerifiedClearance { return verifiedFor(t, completeV3Input(t)) },
			want:      []Blocker{{Reason: ReasonClearanceMismatch, Detail: differentFrom}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.input(t)
			var c *VerifiedClearance
			if tc.clearance != nil {
				c = tc.clearance(t, in)
			}

			got := Evaluate(in, c)

			if got.State != StateDraft {
				t.Fatalf("State = %q, want %q (blockers %v)", got.State, StateDraft, blockerReasons(got))
			}
			// A Blocker is its Reason and its Detail and nothing else, so DeepEqual compares every
			// field of every blocker, and the keyed literals above say which is which.
			if !reflect.DeepEqual(got.Blockers, tc.want) {
				t.Errorf("blockers =\n%v\nwant\n%v", got.Blockers, tc.want)
			}
			if tc.noSubject && (got.Subject != nil || got.View != nil) {
				t.Errorf("Subject = %v, View = %q, want neither", got.Subject, got.View)
			}
			if !tc.noSubject && (got.Subject == nil || len(got.View) == 0) {
				t.Errorf("Subject = %v, View = %q, want both", got.Subject, got.View)
			}
		})
	}
}
