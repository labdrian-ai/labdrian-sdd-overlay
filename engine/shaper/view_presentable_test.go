package shaper

import (
	"fmt"
	"strings"
	"testing"
)

// unpresentableRunes are terminal control and invisible formatting runes the
// presented view must refuse: each can hide or disguise text from the human
// while the view digest still binds the hidden bytes.
var unpresentableRunes = []struct {
	name string
	s    string
}{
	{"ESC CSI conceal", "\x1b[8m"},
	{"CR", "\r"},
	{"NUL", "\x00"},
	{"BEL", "\x07"},
	{"DEL", "\x7f"},
	{"C1 CSI", "\u009b"},
	{"C1 NEL", "\u0085"},
	{"RLO U+202E", "\u202e"},
	{"LRE U+202A", "\u202a"},
	{"RLI U+2067", "\u2067"},
	{"PDI U+2069", "\u2069"},
	{"LRM U+200E", "\u200e"},
	{"RLM U+200F", "\u200f"},
	{"ALM U+061C", "\u061c"},
	{"ZWSP U+200B", "\u200b"},
	{"ZWNJ U+200C", "\u200c"},
	{"ZWJ U+200D", "\u200d"},
	{"BOM U+FEFF", "\ufeff"},
}

func cleanPresentedView() PresentedView {
	return PresentedView{
		GoalBytes:    []byte("{\n\t\"goal\": \"clean\"\n}"),
		PlanBytes:    []byte("{\"plan\":\"clean\"}\n"),
		GoalScope:    "One project.",
		GoalNonGoals: []string{"First.", "Second."},
		OutOfScope:   []string{"Runtime UI.", "Delivery."},
		Flags: []Flag{
			{ID: "goal_non_goal_overlap:stages:1", Kind: FlagGoalNonGoalOverlap, Field: "stages", Index: 1, Item: "First."},
			{ID: "goal_non_goal_overlap:stages:2", Kind: FlagGoalNonGoalOverlap, Field: "stages", Index: 2, Item: "Second."},
		},
	}
}

// viewSections injects bad at rune offset 3 of one section each and names the
// section the refusal must report.
func viewSections(bad string) []struct {
	section string
	view    PresentedView
} {
	inject := func(s string) string { return s[:3] + bad + s[3:] }
	var out []struct {
		section string
		view    PresentedView
	}
	add := func(section string, mutate func(v *PresentedView)) {
		v := cleanPresentedView()
		v.GoalNonGoals = append([]string(nil), v.GoalNonGoals...)
		v.OutOfScope = append([]string(nil), v.OutOfScope...)
		v.Flags = append([]Flag(nil), v.Flags...)
		mutate(&v)
		out = append(out, struct {
			section string
			view    PresentedView
		}{section, v})
	}
	add("goal", func(v *PresentedView) { v.GoalBytes = []byte(inject(string(v.GoalBytes))) })
	add("plan", func(v *PresentedView) { v.PlanBytes = []byte(inject(string(v.PlanBytes))) })
	add("goal_scope", func(v *PresentedView) { v.GoalScope = inject(v.GoalScope) })
	add("goal_non_goals[1]", func(v *PresentedView) { v.GoalNonGoals[1] = inject(v.GoalNonGoals[1]) })
	add("out_of_scope[1]", func(v *PresentedView) { v.OutOfScope[1] = inject(v.OutOfScope[1]) })
	add("flags[1].item", func(v *PresentedView) { v.Flags[1].Item = inject(v.Flags[1].Item) })
	return out
}

func TestCheckPresentableAcceptsCleanViewWithLFAndTab(t *testing.T) {
	v := cleanPresentedView()
	if err := checkPresentable(v); err != nil {
		t.Fatalf("checkPresentable(clean) = %v, want nil", err)
	}
	if err := checkViewBytesPresentable(RenderView(v)); err != nil {
		t.Fatalf("checkViewBytesPresentable(clean) = %v, want nil", err)
	}
	v.GoalScope = "Tabs\tand\nnewlines, plus non-ASCII: café, 日本."
	if err := checkPresentable(v); err != nil {
		t.Fatalf("checkPresentable(LF, TAB, printable non-ASCII) = %v, want nil", err)
	}
}

func TestCheckPresentableRefusesControlRunesInEverySection(t *testing.T) {
	for _, bad := range unpresentableRunes {
		for _, tc := range viewSections(bad.s) {
			t.Run(bad.name+"/"+tc.section, func(t *testing.T) {
				err := checkPresentable(tc.view)
				if err == nil {
					t.Fatalf("checkPresentable accepted %q in %s", bad.s, tc.section)
				}
				want := fmt.Sprintf("section %s at rune offset 3", tc.section)
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
				if err := checkViewBytesPresentable(RenderView(tc.view)); err == nil {
					t.Errorf("checkViewBytesPresentable accepted the rendered view with %q in %s", bad.s, tc.section)
				}
			})
		}
	}
}

func TestCheckViewBytesPresentableRefusesInvalidUTF8(t *testing.T) {
	view := RenderView(cleanPresentedView())
	view = append(view, 0x9b)
	if err := checkViewBytesPresentable(view); err == nil {
		t.Fatalf("checkViewBytesPresentable accepted a raw 0x9b byte (8-bit CSI)")
	}
}

func TestReasonViewUnpresentableIsRefused(t *testing.T) {
	if !ReasonViewUnpresentable.Valid() {
		t.Fatalf("ReasonViewUnpresentable is not in the closed vocabulary")
	}
	if k := ReasonViewUnpresentable.Kind(); k != ReasonKindRefused {
		t.Errorf("Kind = %q, want refused", k)
	}
}

// TestEvaluateRefusesUnpresentableView injects control runes through every
// path a model-authored document can carry them: a JSON escape that decodes
// to a control (raw bytes stay ASCII), and raw bytes JSON permits.
func TestEvaluateRefusesUnpresentableView(t *testing.T) {
	goalWith := func(scope, nonGoals string) string {
		return `{"version":2,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha",` +
			`"objective":"Bind the handoff to real intent.","scope":` + scope + `,` +
			`"constraints":[],"non_goals":` + nonGoals + `,"acceptance_criteria":["Binding succeeds."],` +
			`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
	}
	cleanGoal := goalWith(`"One project."`, `[]`)
	for _, tc := range []struct {
		name    string
		handoff string
		goal    string
		section string
	}{
		{"escaped ESC conceal in goal scope", validHandoffJSON, goalWith(`"One\u001b[8m hidden"`, `[]`), "goal_scope"},
		{"escaped CR in goal scope", validHandoffJSON, goalWith(`"Safe\rEvil"`, `[]`), "goal_scope"},
		{"escaped DEL in goal non_goals", validHandoffJSON, goalWith(`"One project."`, `["Ok.","No\u007f."]`), "goal_non_goals[1]"},
		{"escaped C1 CSI in out_of_scope", strings.Replace(validHandoffJSON, `"Runtime clearance UI."`, `"Runtime\u009b8m UI."`, 1), cleanGoal, "out_of_scope[0]"},
		{"escaped RLO in goal scope", validHandoffJSON, goalWith(`"One \u202eproject."`, `[]`), "goal_scope"},
		{"escaped ZWSP in out_of_scope", strings.Replace(validHandoffJSON, `"Runtime clearance UI."`, `"Runtime\u200b UI."`, 1), cleanGoal, "out_of_scope[0]"},
		{"raw CR whitespace in plan bytes", strings.Replace(validHandoffJSON, `{"version"`, "{\r\"version\"", 1), cleanGoal, "plan"},
		{"raw CR whitespace in goal bytes", validHandoffJSON, strings.Replace(cleanGoal, `{"version"`, "{\r\"version\"", 1), "goal"},
		{"raw RLO in goal bytes", validHandoffJSON, goalWith("\"One \u202eproject.\"", `[]`), "goal"},
		{"raw DEL in plan bytes", strings.Replace(validHandoffJSON, `"Layered CLI`, "\"Layered\x7f CLI", 1), cleanGoal, "plan"},
		{"raw C1 in plan bytes", strings.Replace(validHandoffJSON, `"Layered CLI`, "\"Layered\u0085 CLI", 1), cleanGoal, "plan"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(inputWith(t, []byte(tc.handoff), []byte(tc.goal)), nil)
			var detail string
			for _, b := range got.Blockers {
				if b.Reason == ReasonViewUnpresentable {
					detail = b.Detail
				}
			}
			if detail == "" {
				t.Fatalf("blockers %v lack %q", got.Blockers, ReasonViewUnpresentable)
			}
			if !strings.Contains(detail, "section "+tc.section+" ") {
				t.Errorf("detail %q does not name section %q", detail, tc.section)
			}
			if got.State != StateDraft {
				t.Errorf("State = %q, want draft", got.State)
			}
			if got.Subject != nil || got.View != nil {
				t.Errorf("Subject/View present for an unpresentable view: subject %v, view %q", got.Subject, got.View)
			}
		})
	}
}

// TestVerifyAndCheckRecordBindingRefuseUnpresentableView proves the record
// path refuses a view carrying a control rune even when every digest binds.
func TestVerifyAndCheckRecordBindingRefuseUnpresentableView(t *testing.T) {
	a := freshAssessment(t, overlapInput(t))
	for _, bad := range unpresentableRunes {
		t.Run(bad.name, func(t *testing.T) {
			view := append(append([]byte(nil), a.View...), []byte("tail"+bad.s)...)
			r := recordFor(t, a)
			r.Subject.ViewSHA256 = ViewDigest(view)
			for _, decision := range []ClearanceDecision{DecisionAffirm, DecisionDecline} {
				r.Decision = decision
				data := marshalRecord(t, r)
				if _, err := CheckRecordBinding(data, *a.Subject, a.Flags, view); err == nil || !strings.Contains(err.Error(), "unpresentable") {
					t.Errorf("CheckRecordBinding(%s) error = %v, want an unpresentable refusal", decision, err)
				}
				if decision != DecisionAffirm {
					continue
				}
				vc, err := Verify(data, *a.Subject, a.Flags, view)
				if vc != nil || err == nil || !strings.Contains(err.Error(), "unpresentable") {
					t.Errorf("Verify = (%v, %v), want nil and an unpresentable refusal", vc, err)
				}
			}
		})
	}
}
