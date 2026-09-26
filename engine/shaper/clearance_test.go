package shaper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// overlapInput returns complete readiness evidence whose Goal non_goals
// byte-match the first handoff stage, so Evaluate raises exactly one flag.
func overlapInput(t *testing.T) ReadinessInput {
	t.Helper()
	return inputWith(t, []byte(validHandoffJSON), []byte(goalV2JSONWithNonGoals(`["Extract jsonstrict."]`)))
}

// freshAssessment evaluates in without clearance and requires a bound
// Subject and a presented view.
func freshAssessment(t *testing.T, in ReadinessInput) Assessment {
	t.Helper()
	a := Evaluate(in, nil)
	if a.Subject == nil {
		t.Fatalf("Subject = nil, blockers %v", blockerReasons(a))
	}
	if len(a.View) == 0 {
		t.Fatalf("View is empty for a bound Subject")
	}
	return a
}

// resolveAll returns one complete resolution per flag.
func resolveAll(flags []Flag) []FlagResolution {
	resolutions := []FlagResolution{}
	for _, f := range flags {
		resolutions = append(resolutions, FlagResolution{
			FlagID:   f.ID,
			Reason:   "Intended: the stage restates a deferred non-goal on purpose.",
			Evidence: "Reviewed the Goal non_goals list in the presented view.",
		})
	}
	return resolutions
}

// recordFor builds an affirmative clearance record bound to a.
func recordFor(t *testing.T, a Assessment) ClearanceRecord {
	t.Helper()
	verified := false
	return ClearanceRecord{
		Version: ClearanceRecordVersion,
		Subject: ClearanceSubject{
			ProjectID:        a.Subject.ProjectID,
			GoalID:           a.Subject.GoalID,
			GoalSHA256:       a.Subject.GoalSHA256,
			HandoffSHA256:    a.Subject.HandoffSHA256,
			ProvenanceSHA256: a.Subject.ProvenanceSHA256(),
			ViewSHA256:       ViewDigest(a.View),
		},
		FlagResolutions: resolveAll(a.Flags),
		Decision:        DecisionAffirm,
		Channel:         ChannelProvenance{Runtime: "pi", Mode: "tui", Verified: &verified},
	}
}

func marshalRecord(t *testing.T, r ClearanceRecord) []byte {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	return data
}

// recordMap round-trips a record through a generic map so a test can add,
// drop, or replace wire fields.
func recordMap(t *testing.T, r ClearanceRecord) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(marshalRecord(t, r), &m); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	return m
}

func marshalAny(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func TestParseRecordAcceptsAffirmAndDecline(t *testing.T) {
	a := freshAssessment(t, overlapInput(t))
	for _, decision := range []ClearanceDecision{DecisionAffirm, DecisionDecline} {
		t.Run(string(decision), func(t *testing.T) {
			r := recordFor(t, a)
			r.Decision = decision
			got, err := ParseRecord(marshalRecord(t, r))
			if err != nil {
				t.Fatalf("ParseRecord: %v", err)
			}
			if got.Decision != decision {
				t.Errorf("Decision = %q, want %q", got.Decision, decision)
			}
			if got.Channel.Verified == nil || *got.Channel.Verified {
				t.Errorf("Channel.Verified = %v, want explicit false", got.Channel.Verified)
			}
		})
	}
}

func TestParseRecordAcceptsInformationalHostTime(t *testing.T) {
	r := recordFor(t, freshAssessment(t, completeInput(t)))
	informational := true
	r.HostTime = &HostTime{Value: "2026-09-25T10:00:00Z", Informational: &informational}
	got, err := ParseRecord(marshalRecord(t, r))
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if got.HostTime == nil || got.HostTime.Value != "2026-09-25T10:00:00Z" {
		t.Errorf("HostTime = %#v, want the recorded value", got.HostTime)
	}
}

func TestParseRecordRejectsInvalidRecords(t *testing.T) {
	a := freshAssessment(t, overlapInput(t))
	base := recordFor(t, a)
	subject := func(m map[string]any) map[string]any { return m["subject"].(map[string]any) }
	channel := func(m map[string]any) map[string]any { return m["channel"].(map[string]any) }
	resolution := func(m map[string]any) map[string]any {
		return m["flag_resolutions"].([]any)[0].(map[string]any)
	}
	// rename moves one wire key to a case variant, which encoding/json alone
	// would still match to the known field.
	rename := func(obj map[string]any, from, to string) {
		obj[to] = obj[from]
		delete(obj, from)
	}
	for _, tc := range []struct {
		name string
		edit func(m map[string]any)
		raw  []byte
	}{
		{name: "unknown top-level field", edit: func(m map[string]any) { m["actor"] = "alice" }},
		{name: "unknown subject field", edit: func(m map[string]any) { subject(m)["head"] = "abc" }},
		{name: "human identity in channel", edit: func(m map[string]any) { channel(m)["user"] = "alice" }},
		{name: "unknown resolution field", edit: func(m map[string]any) { resolution(m)["approved"] = true }},
		{name: "version 2", edit: func(m map[string]any) { m["version"] = 2 }},
		{name: "version missing", edit: func(m map[string]any) { delete(m, "version") }},
		{name: "subject missing", edit: func(m map[string]any) { delete(m, "subject") }},
		{name: "subject null", edit: func(m map[string]any) { m["subject"] = nil }},
		{name: "blank project_id", edit: func(m map[string]any) { subject(m)["project_id"] = " " }},
		{name: "blank goal_id", edit: func(m map[string]any) { subject(m)["goal_id"] = "" }},
		{name: "uppercase goal digest", edit: func(m map[string]any) {
			subject(m)["goal_sha256"] = strings.ToUpper(a.Subject.GoalSHA256)
		}},
		{name: "short handoff digest", edit: func(m map[string]any) { subject(m)["handoff_sha256"] = "abc" }},
		{name: "missing provenance digest", edit: func(m map[string]any) { delete(subject(m), "provenance_sha256") }},
		{name: "non-hex view digest", edit: func(m map[string]any) { subject(m)["view_sha256"] = strings.Repeat("z", 64) }},
		{name: "flag_resolutions null", edit: func(m map[string]any) { m["flag_resolutions"] = nil }},
		{name: "flag_resolutions missing", edit: func(m map[string]any) { delete(m, "flag_resolutions") }},
		{name: "blank flag id", edit: func(m map[string]any) { resolution(m)["flag_id"] = "" }},
		{name: "blank reason", edit: func(m map[string]any) { resolution(m)["reason"] = "\t" }},
		{name: "missing evidence", edit: func(m map[string]any) { delete(resolution(m), "evidence") }},
		{name: "duplicate flag resolution", edit: func(m map[string]any) {
			list := m["flag_resolutions"].([]any)
			m["flag_resolutions"] = append(list, list[0])
		}},
		{name: "decision approve", edit: func(m map[string]any) { m["decision"] = "approve" }},
		{name: "decision uppercase", edit: func(m map[string]any) { m["decision"] = "AFFIRM" }},
		{name: "decision missing", edit: func(m map[string]any) { delete(m, "decision") }},
		{name: "channel missing", edit: func(m map[string]any) { delete(m, "channel") }},
		{name: "channel verified true", edit: func(m map[string]any) { channel(m)["verified"] = true }},
		{name: "channel verified missing", edit: func(m map[string]any) { delete(channel(m), "verified") }},
		{name: "channel runtime claude", edit: func(m map[string]any) { channel(m)["runtime"] = "claude-code" }},
		{name: "channel mode print", edit: func(m map[string]any) { channel(m)["mode"] = "print" }},
		{name: "channel mode rpc", edit: func(m map[string]any) { channel(m)["mode"] = "rpc" }},
		{name: "host_time null", edit: func(m map[string]any) { m["host_time"] = nil }},
		{name: "host_time not informational", edit: func(m map[string]any) {
			m["host_time"] = map[string]any{"value": "2026-09-25T10:00:00Z", "informational": false}
		}},
		{name: "host_time blank value", edit: func(m map[string]any) {
			m["host_time"] = map[string]any{"value": "", "informational": true}
		}},
		{name: "host_time unknown field", edit: func(m map[string]any) {
			m["host_time"] = map[string]any{"value": "x", "informational": true, "trusted": true}
		}},
		{name: "case-variant top-level key", edit: func(m map[string]any) { rename(m, "decision", "Decision") }},
		{name: "case-variant subject key", edit: func(m map[string]any) { rename(subject(m), "goal_id", "GOAL_ID") }},
		{name: "case-variant resolution key", edit: func(m map[string]any) { rename(resolution(m), "flag_id", "Flag_Id") }},
		{name: "case-variant channel key", edit: func(m map[string]any) { rename(channel(m), "mode", "MODE") }},
		{name: "case-variant host_time key", edit: func(m map[string]any) {
			m["host_time"] = map[string]any{"Value": "2026-09-25T10:00:00Z", "informational": true}
		}},
		{name: "conflicting case-variant resolution key", edit: func(m map[string]any) {
			resolution(m)["FLAG_ID"] = resolution(m)["flag_id"]
			resolution(m)["flag_id"] = "bogus-shown-to-others"
		}},
		{name: "conflicting case-variant channel keys", edit: func(m map[string]any) {
			channel(m)["Runtime"] = "pi"
			channel(m)["MODE"] = "rpc"
		}},
		{name: "conflicting case-variant subject key", edit: func(m map[string]any) {
			subject(m)["Handoff_SHA256"] = subject(m)["handoff_sha256"]
			subject(m)["handoff_sha256"] = strings.Repeat("0", 64)
		}},
		{name: "duplicate key", raw: []byte(`{"version":1,"version":1}`)},
		{name: "trailing data", raw: append(marshalRecord(t, base), []byte(` {}`)...)},
		{name: "invalid utf8", raw: []byte("{\"version\":1,\"decision\":\"\xff\"}")},
		{name: "array root", raw: []byte(`[]`)},
		{name: "empty", raw: []byte(``)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.raw
			if tc.edit != nil {
				m := recordMap(t, base)
				tc.edit(m)
				data = marshalAny(t, m)
			}
			if _, err := ParseRecord(data); err == nil {
				t.Fatalf("ParseRecord accepted %s: %s", tc.name, data)
			}
		})
	}
}

func TestRenderViewIsDeterministicAndCoversEveryPresentedPart(t *testing.T) {
	in := overlapInput(t)
	a := freshAssessment(t, in)
	b := freshAssessment(t, overlapInput(t))
	if !bytes.Equal(a.View, b.View) {
		t.Fatalf("View differs between identical evaluations")
	}
	for _, want := range []string{
		string(in.Goal.GoalBytes),
		string(in.Handoff.Bytes),
		"One project.",
		"Extract jsonstrict.",
		"Runtime clearance UI.",
		"goal_non_goal_overlap:stages:1",
	} {
		if !bytes.Contains(a.View, []byte(want)) {
			t.Errorf("View lacks %q", want)
		}
	}
	// The scope and out_of_scope values also occur inside the verbatim goal
	// and plan bytes, so only their framed sections prove they are presented.
	for _, section := range []string{
		viewValueSection("goal_scope", "One project."),
		viewListSection("out_of_scope", "Runtime clearance UI."),
	} {
		if bytes.Contains(in.Goal.GoalBytes, []byte(section)) || bytes.Contains(in.Handoff.Bytes, []byte(section)) {
			t.Fatalf("section %q occurs inside the goal or plan bytes; the assertion would be vacuous", section)
		}
		if !bytes.Contains(a.View, []byte(section)) {
			t.Errorf("View lacks section %q", section)
		}
	}
	if got := ViewDigest(a.View); !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(got) {
		t.Errorf("ViewDigest = %q, want lowercase hex SHA-256", got)
	}
	if ViewDigest(a.View) != sha256Hex(a.View) {
		t.Errorf("ViewDigest is not the SHA-256 of the view bytes")
	}
}

func TestRenderViewIsUnambiguous(t *testing.T) {
	base := PresentedView{
		GoalBytes:    []byte("g"),
		PlanBytes:    []byte("p"),
		GoalScope:    "s",
		GoalNonGoals: []string{"a", "b"},
		OutOfScope:   []string{},
	}
	shifted := base
	shifted.GoalNonGoals = []string{"a\nitem 1\nb"}
	moved := base
	moved.GoalNonGoals = []string{"a"}
	moved.OutOfScope = []string{"b"}
	flagged := base
	flagged.Flags = []Flag{{ID: "x", Kind: FlagGoalNonGoalOverlap, Field: "stages", Index: 1, Item: "a"}}
	reindexed := flagged
	reindexed.Flags = []Flag{{ID: "x", Kind: FlagGoalNonGoalOverlap, Field: "stages", Index: 2, Item: "a"}}
	views := map[string][]byte{}
	for name, v := range map[string]PresentedView{
		"base": base, "shifted": shifted, "moved": moved, "flagged": flagged, "reindexed": reindexed,
	} {
		views[name] = RenderView(v)
	}
	for n1, v1 := range views {
		for n2, v2 := range views {
			if n1 < n2 && bytes.Equal(v1, v2) {
				t.Errorf("views %q and %q render identically", n1, n2)
			}
		}
	}
}

// viewValueSection is the exact framed section RenderView writes for one
// named value.
func viewValueSection(name, value string) string {
	return fmt.Sprintf("\n%s %d\n%s\n", name, len(value), value)
}

// viewListSection is the exact framed section RenderView writes for one
// named list.
func viewListSection(name string, items ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s %d\n", name, len(items))
	for _, item := range items {
		fmt.Fprintf(&b, "item %d\n%s\n", len(item), item)
	}
	return b.String()
}

func TestViewChangesWithEveryPresentedPart(t *testing.T) {
	base := freshAssessment(t, completeInput(t)).View
	otherScopeGoal := strings.Replace(goalV2JSONWithNonGoals(`[]`), `"scope":"One project."`, `"scope":"Two projects."`, 1)
	otherLimitPlan := documentWith(t, map[string]any{"out_of_scope": []string{"Other limit."}})
	for _, tc := range []struct {
		name string
		in   ReadinessInput
		// Section changes are asserted on the framed section, because the
		// same edit also changes the verbatim goal or plan bytes.
		oldSection, newSection string
	}{
		{name: "goal bytes", in: inputWith(t, []byte(validHandoffJSON), []byte(goalV2JSONWithNonGoals(`[]`)+" "))},
		{name: "plan bytes", in: inputWith(t, []byte(validHandoffJSON+" "), []byte(goalV2JSONWithNonGoals(`[]`)))},
		{name: "non_goals", in: inputWith(t, []byte(validHandoffJSON), []byte(goalV2JSONWithNonGoals(`["Other."]`)))},
		{name: "flags", in: overlapInput(t)},
		{
			name:       "goal scope",
			in:         inputWith(t, []byte(validHandoffJSON), []byte(otherScopeGoal)),
			oldSection: viewValueSection("goal_scope", "One project."),
			newSection: viewValueSection("goal_scope", "Two projects."),
		},
		{
			name:       "out_of_scope",
			in:         inputWith(t, otherLimitPlan, []byte(goalV2JSONWithNonGoals(`[]`))),
			oldSection: viewListSection("out_of_scope", "Runtime clearance UI."),
			newSection: viewListSection("out_of_scope", "Other limit."),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := freshAssessment(t, tc.in).View
			if bytes.Equal(view, base) {
				t.Errorf("View unchanged after a %s change", tc.name)
			}
			if tc.newSection == "" {
				return
			}
			if !bytes.Contains(base, []byte(tc.oldSection)) {
				t.Errorf("base View lacks section %q", tc.oldSection)
			}
			if !bytes.Contains(view, []byte(tc.newSection)) {
				t.Errorf("View lacks section %q after a %s change", tc.newSection, tc.name)
			}
			if bytes.Contains(view, []byte(tc.oldSection)) {
				t.Errorf("View still presents section %q after a %s change", tc.oldSection, tc.name)
			}
		})
	}
}

// TestEvaluatePresentedViewCarriesGoalScopeAndOutOfScope proves Evaluate
// renders exactly the expected PresentedView, and that the Goal scope and the
// handoff out_of_scope are load-bearing parts of it.
func TestEvaluatePresentedViewCarriesGoalScopeAndOutOfScope(t *testing.T) {
	in := overlapInput(t)
	a := freshAssessment(t, in)
	want := PresentedView{
		GoalBytes:    in.Goal.GoalBytes,
		PlanBytes:    in.Handoff.Bytes,
		GoalScope:    "One project.",
		GoalNonGoals: []string{"Extract jsonstrict."},
		OutOfScope:   []string{"Runtime clearance UI."},
		Flags:        a.Flags,
	}
	if !bytes.Equal(a.View, RenderView(want)) {
		t.Fatalf("View = %q, want RenderView of %+v", a.View, want)
	}
	withoutScope := want
	withoutScope.GoalScope = ""
	withoutLimits := want
	withoutLimits.OutOfScope = []string{}
	for name, v := range map[string]PresentedView{"goal scope": withoutScope, "out_of_scope": withoutLimits} {
		if bytes.Equal(a.View, RenderView(v)) {
			t.Errorf("View is identical to a view without the %s; it is not presented", name)
		}
	}
}

func TestVerifyAcceptsAffirmBoundToTheFreshSubjectAndView(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ReadinessInput
	}{
		{name: "zero flags", in: completeInput(t)},
		{name: "one flag", in: overlapInput(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := freshAssessment(t, tc.in)
			c, err := Verify(marshalRecord(t, recordFor(t, a)), *a.Subject, a.Flags, a.View)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if c == nil {
				t.Fatalf("Verify returned nil clearance without error")
			}
		})
	}
}

// TestVerifyInvalidationMatrix proves that a record verified against one
// subject is refused once any bound part of the subject, the presented view,
// or the flag set changes.
func TestVerifyInvalidationMatrix(t *testing.T) {
	a := freshAssessment(t, overlapInput(t))
	record := marshalRecord(t, recordFor(t, a))
	for _, tc := range []struct {
		name  string
		edit  func(s *Subject, flags *[]Flag, view *[]byte)
		match string
	}{
		{name: "project_id", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.ProjectID = "other-project" }, match: "project_id"},
		{name: "goal_id", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.GoalID = "goal-beta" }, match: "goal_id"},
		{name: "goal digest", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.GoalSHA256 = sha256Hex([]byte("g")) }, match: "goal_sha256"},
		{name: "plan digest", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.HandoffSHA256 = sha256Hex([]byte("p")) }, match: "handoff_sha256"},
		{name: "handoff source path", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.HandoffSourcePath = "other.json" }, match: "provenance_sha256"},
		{name: "goal source path", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.GoalSourcePath = "goals/goal.json" }, match: "provenance_sha256"},
		{name: "toplevel", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.Worktree.Toplevel = "/work/other" }, match: "provenance_sha256"},
		{name: "git dir", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.Worktree.GitDir = "/work/other/.git" }, match: "provenance_sha256"},
		{name: "common dir", edit: func(s *Subject, _ *[]Flag, _ *[]byte) { s.Worktree.CommonDir = "/work/other/.git" }, match: "provenance_sha256"},
		{name: "view bytes", edit: func(_ *Subject, _ *[]Flag, v *[]byte) { *v = append(append([]byte{}, *v...), ' ') }, match: "view_sha256"},
		{name: "flag added", edit: func(_ *Subject, f *[]Flag, _ *[]byte) {
			*f = append(append([]Flag{}, *f...), Flag{ID: "goal_non_goal_overlap:acceptance:1"})
		}, match: "unresolved"},
		{name: "flag removed", edit: func(_ *Subject, f *[]Flag, _ *[]byte) { *f = nil }, match: "unknown flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := *a.Subject
			flags := append([]Flag{}, a.Flags...)
			view := append([]byte{}, a.View...)
			tc.edit(&s, &flags, &view)
			c, err := Verify(record, s, flags, view)
			if err == nil {
				t.Fatalf("Verify accepted a record after a %s change", tc.name)
			}
			if c != nil {
				t.Errorf("Verify returned a clearance alongside error %v", err)
			}
			if !strings.Contains(err.Error(), tc.match) {
				t.Errorf("error %q does not name %q", err, tc.match)
			}
		})
	}
}

func TestVerifyRefusesDeclineMissingResolutionAndUnknownField(t *testing.T) {
	a := freshAssessment(t, overlapInput(t))
	declined := recordFor(t, a)
	declined.Decision = DecisionDecline
	unresolved := recordFor(t, a)
	unresolved.FlagResolutions = []FlagResolution{}
	withUnknown := recordMap(t, recordFor(t, a))
	withUnknown["cleared_by"] = "alice"
	// An RPC dialog is answered by a driving process, not a human, so an
	// RPC-captured affirm must never verify (decided TUI-only premise).
	overRPC := recordMap(t, recordFor(t, a))
	overRPC["channel"].(map[string]any)["mode"] = "rpc"
	for _, tc := range []struct {
		name  string
		data  []byte
		match string
	}{
		{name: "decline", data: marshalRecord(t, declined), match: "decline"},
		{name: "missing flag resolution", data: marshalRecord(t, unresolved), match: "unresolved"},
		{name: "unknown field", data: marshalAny(t, withUnknown), match: "unknown"},
		{name: "rpc channel", data: marshalAny(t, overRPC), match: "tui"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Verify(tc.data, *a.Subject, a.Flags, a.View)
			if err == nil || c != nil {
				t.Fatalf("Verify = (%v, %v), want a refusal", c, err)
			}
			if !strings.Contains(err.Error(), tc.match) {
				t.Errorf("error %q does not name %q", err, tc.match)
			}
		})
	}
}

func TestEvaluateAcceptsVerifiedClearanceButHandoffV1StaysDraft(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   func(t *testing.T) ReadinessInput
	}{
		{name: "zero flags", in: completeInput},
		{name: "one resolved flag", in: overlapInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := freshAssessment(t, tc.in(t))
			c, err := Verify(marshalRecord(t, recordFor(t, a)), *a.Subject, a.Flags, a.View)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			got := Evaluate(tc.in(t), c)
			if got.State != StateDraft {
				t.Fatalf("State = %q, want %q", got.State, StateDraft)
			}
			want := []Reason{ReasonAcceptanceVerificationUnrepresentable}
			if r := blockerReasons(got); len(r) != 1 || r[0] != want[0] {
				t.Errorf("blockers = %v, want exactly %v", r, want)
			}
		})
	}
}

func TestEvaluateRefusesVerifiedClearanceForAChangedSubject(t *testing.T) {
	a := freshAssessment(t, completeInput(t))
	c, err := Verify(marshalRecord(t, recordFor(t, a)), *a.Subject, a.Flags, a.View)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	for _, tc := range []struct {
		name string
		in   ReadinessInput
	}{
		{name: "goal edit", in: inputWith(t, []byte(validHandoffJSON), []byte(goalV2JSONWithNonGoals(`[]`)+"\n"))},
		{name: "plan edit", in: inputWith(t, []byte(validHandoffJSON+"\n"), []byte(goalV2JSONWithNonGoals(`[]`)))},
		{name: "new flag", in: overlapInput(t)},
		{name: "provenance", in: func() ReadinessInput {
			in := completeInput(t)
			in.Provenance.GitDir = "/work/tree/.git/worktrees/other"
			return in
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in, c)
			if got.State != StateDraft {
				t.Fatalf("State = %q, want %q", got.State, StateDraft)
			}
			if !hasReason(got, ReasonClearanceMismatch) {
				t.Errorf("blockers = %v, want %q", blockerReasons(got), ReasonClearanceMismatch)
			}
		})
	}
}

func TestEvaluateCannotMatchClearanceWithoutCompleteSubject(t *testing.T) {
	a := freshAssessment(t, completeInput(t))
	c, err := Verify(marshalRecord(t, recordFor(t, a)), *a.Subject, a.Flags, a.View)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	in := completeInput(t)
	in.Provenance = nil
	got := Evaluate(in, c)
	if !hasReason(got, ReasonClearanceUnverified) {
		t.Errorf("blockers = %v, want %q", blockerReasons(got), ReasonClearanceUnverified)
	}
	if got.View != nil {
		t.Errorf("View = %q, want nil without a bound Subject", got.View)
	}
}

// TestReadyDocCommentsStateForgeryLimit fails when any doc comment in this
// package's sources that mentions ready omits the same-OS-user forgery limit
// or the statement that clearance is not a signature.
func TestReadyDocCommentsStateForgeryLimit(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	ready := regexp.MustCompile(`(?i)\bready\b`)
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		var docs []*ast.CommentGroup
		docs = append(docs, file.Doc)
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				docs = append(docs, node.Doc)
			case *ast.GenDecl:
				docs = append(docs, node.Doc)
			case *ast.TypeSpec:
				docs = append(docs, node.Doc)
			case *ast.ValueSpec:
				docs = append(docs, node.Doc)
			case *ast.Field:
				docs = append(docs, node.Doc)
			}
			return true
		})
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			text := strings.Join(strings.Fields(doc.Text()), " ")
			if !ready.MatchString(text) {
				continue
			}
			checked++
			if !strings.Contains(text, "same OS user") || !strings.Contains(text, "not a signature") {
				t.Errorf("%s: doc comment mentions ready without the same OS user forgery limit and not a signature:\n%s",
					fset.Position(doc.Pos()), text)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no doc comment mentioning ready was found")
	}
}

// TestRecordFieldListsMatchWireTags keeps the exact-case allowed-key lists in
// step with the struct JSON tags, so a new field cannot be silently refused
// or left unchecked.
func TestRecordFieldListsMatchWireTags(t *testing.T) {
	for _, tc := range []struct {
		name   string
		typ    reflect.Type
		fields []string
	}{
		{"record", reflect.TypeOf(ClearanceRecord{}), clearanceRecordFields},
		{"subject", reflect.TypeOf(ClearanceSubject{}), clearanceSubjectFields},
		{"flag resolution", reflect.TypeOf(FlagResolution{}), flagResolutionFields},
		{"channel", reflect.TypeOf(ChannelProvenance{}), channelFields},
		{"host_time", reflect.TypeOf(HostTime{}), hostTimeFields},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tags []string
			for i := 0; i < tc.typ.NumField(); i++ {
				tags = append(tags, strings.Split(tc.typ.Field(i).Tag.Get("json"), ",")[0])
			}
			if !reflect.DeepEqual(tags, tc.fields) {
				t.Errorf("wire tags %v, allowed fields %v", tags, tc.fields)
			}
		})
	}
}
