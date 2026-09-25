package shaper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
)

const readinessRoot = "/work/tree"

func goalV2JSONWithNonGoals(nonGoals string) string {
	return `{"version":2,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":` + nonGoals + `,"acceptance_criteria":["Binding succeeds."],` +
		`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

// completeInput returns readiness evidence in which every piece a caller can
// supply is present and consistent. Only clearance and acceptance
// verification are left for Evaluate to find missing.
func completeInput(t *testing.T) ReadinessInput {
	t.Helper()
	return inputWith(t, []byte(validHandoffJSON), []byte(goalV2JSONWithNonGoals(`[]`)))
}

func inputWith(t *testing.T, handoffBytes, goalBytes []byte) ReadinessInput {
	t.Helper()
	return ReadinessInput{
		WorktreeRoot: readinessRoot,
		Handoff: HandoffSource{
			SourcePath: "handoff.json",
			Bytes:      handoffBytes,
			SHA256:     sha256Hex(handoffBytes),
		},
		Goal: &GoalBinding{
			SourcePath: "goal.json",
			GoalBytes:  goalBytes,
			GoalSHA256: sha256Hex(goalBytes),
		},
		Provenance: &gitprov.Observation{
			Toplevel:  readinessRoot,
			GitDir:    readinessRoot + "/.git",
			CommonDir: readinessRoot + "/.git",
			Head:      "0123456789abcdef0123456789abcdef01234567",
		},
	}
}

func blockerReasons(a Assessment) []Reason {
	reasons := make([]Reason, 0, len(a.Blockers))
	for _, b := range a.Blockers {
		reasons = append(reasons, b.Reason)
	}
	return reasons
}

func hasReason(a Assessment, r Reason) bool {
	for _, b := range a.Blockers {
		if b.Reason == r {
			return true
		}
	}
	return false
}

func TestEvaluateCompleteEvidenceWithNilClearanceStaysDraft(t *testing.T) {
	in := completeInput(t)

	got := Evaluate(in, nil)

	if got.State != StateDraft {
		t.Fatalf("State = %q, want %q (blockers %v)", got.State, StateDraft, blockerReasons(got))
	}
	want := []Reason{ReasonClearanceMissing, ReasonAcceptanceVerificationUnrepresentable}
	if !reflect.DeepEqual(blockerReasons(got), want) {
		t.Errorf("blockers = %v, want %v", blockerReasons(got), want)
	}
	if len(got.Flags) != 0 {
		t.Errorf("Flags = %v, want none", got.Flags)
	}
	if got.Subject == nil {
		t.Fatalf("Subject = nil, want the bound subject")
	}
	wantSubject := Subject{
		ProjectID:         "standalone-shaper-handoff",
		GoalID:            "goal-alpha",
		HandoffSourcePath: "handoff.json",
		HandoffSHA256:     sha256Hex([]byte(validHandoffJSON)),
		GoalSourcePath:    "goal.json",
		GoalSHA256:        sha256Hex([]byte(goalV2JSONWithNonGoals(`[]`))),
		Worktree: WorktreeProvenance{
			Toplevel:  readinessRoot,
			GitDir:    readinessRoot + "/.git",
			CommonDir: readinessRoot + "/.git",
		},
	}
	if *got.Subject != wantSubject {
		t.Errorf("Subject = %#v, want %#v", *got.Subject, wantSubject)
	}
}

func TestEvaluateNeverReturnsReadyForAnyObtainableClearance(t *testing.T) {
	for _, tc := range []struct {
		name      string
		clearance *VerifiedClearance
		want      Reason
	}{
		{name: "nil clearance", clearance: nil, want: ReasonClearanceMissing},
		{name: "zero value clearance", clearance: &VerifiedClearance{}, want: ReasonClearanceUnverified},
		{name: "new clearance", clearance: new(VerifiedClearance), want: ReasonClearanceUnverified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(completeInput(t), tc.clearance)
			if got.State == StateReady {
				t.Fatalf("Evaluate returned ready for %s", tc.name)
			}
			if got.State != StateDraft {
				t.Errorf("State = %q, want %q", got.State, StateDraft)
			}
			if !hasReason(got, tc.want) {
				t.Errorf("blockers = %v, want to include %q", blockerReasons(got), tc.want)
			}
			if !hasReason(got, ReasonAcceptanceVerificationUnrepresentable) {
				t.Errorf("blockers = %v, want the v1 acceptance cap", blockerReasons(got))
			}
		})
	}
}

func TestEvaluateStructurallyInvalidHandoffIsInvalidWithoutSubject(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "malformed json", data: []byte(`{"version":1`)},
		{name: "unsupported version", data: documentWith(t, map[string]any{"version": 2})},
		{name: "unknown field", data: documentWith(t, map[string]any{"clearance": true})},
		{name: "out_of_scope overlap", data: documentWith(t, map[string]any{"out_of_scope": []string{"Extract jsonstrict."}})},
		{name: "empty bytes", data: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := completeInput(t)
			in.Handoff.Bytes = tc.data
			in.Handoff.SHA256 = sha256Hex(tc.data)

			got := Evaluate(in, nil)

			if got.State != StateInvalid {
				t.Fatalf("State = %q, want %q", got.State, StateInvalid)
			}
			if want := []Reason{ReasonHandoffInvalid}; !reflect.DeepEqual(blockerReasons(got), want) {
				t.Errorf("blockers = %v, want %v", blockerReasons(got), want)
			}
			if got.Subject != nil {
				t.Errorf("Subject = %#v, want nil for an invalid handoff", got.Subject)
			}
		})
	}
}

func TestEvaluateMissingEvidenceIsUnprovable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(in *ReadinessInput)
		want   Reason
	}{
		{name: "goal unbound", mutate: func(in *ReadinessInput) { in.Goal = nil }, want: ReasonGoalUnbound},
		{name: "provenance unobserved", mutate: func(in *ReadinessInput) { in.Provenance = nil }, want: ReasonWorktreeProvenanceUnobserved},
		{name: "provenance missing git dir", mutate: func(in *ReadinessInput) { in.Provenance.GitDir = "" }, want: ReasonWorktreeProvenanceIncomplete},
		{name: "provenance missing common dir", mutate: func(in *ReadinessInput) { in.Provenance.CommonDir = "" }, want: ReasonWorktreeProvenanceIncomplete},
		{name: "handoff source path unrecorded", mutate: func(in *ReadinessInput) { in.Handoff.SourcePath = "" }, want: ReasonSourceUnrecorded},
		{name: "goal source path unrecorded", mutate: func(in *ReadinessInput) { in.Goal.SourcePath = "" }, want: ReasonSourceUnrecorded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := completeInput(t)
			tc.mutate(&in)

			got := Evaluate(in, nil)

			if got.State != StateDraft {
				t.Fatalf("State = %q, want %q", got.State, StateDraft)
			}
			if !hasReason(got, tc.want) {
				t.Fatalf("blockers = %v, want to include %q", blockerReasons(got), tc.want)
			}
			if tc.want.Kind() != ReasonKindUnprovable {
				t.Errorf("%q.Kind() = %q, want %q", tc.want, tc.want.Kind(), ReasonKindUnprovable)
			}
			if got.Subject != nil {
				t.Errorf("Subject = %#v, want nil when evidence is missing", got.Subject)
			}
		})
	}
}

func TestEvaluateContradictoryEvidenceIsRefused(t *testing.T) {
	otherProject := strings.Replace(goalV2JSONWithNonGoals(`[]`), "standalone-shaper-handoff", "other-project", 1)
	otherGoal := strings.Replace(goalV2JSONWithNonGoals(`[]`), `"goal-alpha"`, `"goal-beta"`, 1)
	for _, tc := range []struct {
		name   string
		mutate func(in *ReadinessInput)
		want   Reason
	}{
		{name: "handoff digest mismatch", mutate: func(in *ReadinessInput) { in.Handoff.SHA256 = sha256Hex([]byte("x")) }, want: ReasonHandoffDigestMismatch},
		{name: "goal digest mismatch", mutate: func(in *ReadinessInput) { in.Goal.GoalSHA256 = sha256Hex([]byte("x")) }, want: ReasonGoalDigestMismatch},
		{name: "goal bytes invalid", mutate: func(in *ReadinessInput) {
			in.Goal.GoalBytes = []byte(`{}`)
			in.Goal.GoalSHA256 = sha256Hex(in.Goal.GoalBytes)
		}, want: ReasonGoalInvalid},
		{name: "goal version one", mutate: func(in *ReadinessInput) {
			in.Goal.GoalBytes = []byte(strings.Replace(strings.Replace(goalV2JSONWithNonGoals(`[]`), `"goal_id":"goal-alpha",`, "", 1), `"version":2`, `"version":1`, 1))
			in.Goal.GoalSHA256 = sha256Hex(in.Goal.GoalBytes)
		}, want: ReasonGoalInvalid},
		{name: "project_id mismatch", mutate: func(in *ReadinessInput) {
			in.Goal.GoalBytes = []byte(otherProject)
			in.Goal.GoalSHA256 = sha256Hex(in.Goal.GoalBytes)
		}, want: ReasonGoalIdentityMismatch},
		{name: "goal_id mismatch", mutate: func(in *ReadinessInput) {
			in.Goal.GoalBytes = []byte(otherGoal)
			in.Goal.GoalSHA256 = sha256Hex(in.Goal.GoalBytes)
		}, want: ReasonGoalIdentityMismatch},
		{name: "toplevel differs from worktree root", mutate: func(in *ReadinessInput) { in.Provenance.Toplevel = "/elsewhere" }, want: ReasonWorktreeProvenanceMismatch},
		{name: "relative worktree root", mutate: func(in *ReadinessInput) { in.WorktreeRoot = "work/tree" }, want: ReasonWorktreeProvenanceMismatch},
		{name: "empty worktree root", mutate: func(in *ReadinessInput) { in.WorktreeRoot = "" }, want: ReasonWorktreeProvenanceMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := completeInput(t)
			tc.mutate(&in)

			got := Evaluate(in, nil)

			if got.State != StateDraft {
				t.Fatalf("State = %q, want %q", got.State, StateDraft)
			}
			if !hasReason(got, tc.want) {
				t.Fatalf("blockers = %v, want to include %q", blockerReasons(got), tc.want)
			}
			if tc.want.Kind() != ReasonKindRefused {
				t.Errorf("%q.Kind() = %q, want %q", tc.want, tc.want.Kind(), ReasonKindRefused)
			}
			if got.Subject != nil {
				t.Errorf("Subject = %#v, want nil when evidence is refused", got.Subject)
			}
		})
	}
}

func TestEvaluateFlagsGoalNonGoalOverlapForHumanReviewInsteadOfRejecting(t *testing.T) {
	goalBytes := []byte(goalV2JSONWithNonGoals(`["Extract jsonstrict.","go test ./... passes."]`))
	in := inputWith(t, []byte(validHandoffJSON), goalBytes)

	got := Evaluate(in, nil)

	if got.State != StateDraft {
		t.Fatalf("State = %q, want %q: non_goals overlap is a human-review flag, not a rejection", got.State, StateDraft)
	}
	want := []Flag{
		{ID: "goal_non_goal_overlap:stages:1", Kind: FlagGoalNonGoalOverlap, Field: "stages", Index: 1, Item: "Extract jsonstrict."},
		{ID: "goal_non_goal_overlap:acceptance:1", Kind: FlagGoalNonGoalOverlap, Field: "acceptance", Index: 1, Item: "go test ./... passes."},
	}
	if !reflect.DeepEqual(got.Flags, want) {
		t.Errorf("Flags = %#v, want %#v", got.Flags, want)
	}
	if !hasReason(got, ReasonFlagsUnresolved) {
		t.Errorf("blockers = %v, want to include %q", blockerReasons(got), ReasonFlagsUnresolved)
	}
	if got.Subject == nil {
		t.Errorf("Subject = nil, want the bound subject: a flag does not invalidate the evidence")
	}
}

func TestEvaluateNonGoalOverlapAppliesNoNormalization(t *testing.T) {
	for _, nonGoal := range []string{"Extract jsonstrict. ", "extract jsonstrict.", "Extract jsonstrict"} {
		t.Run(nonGoal, func(t *testing.T) {
			goalBytes := []byte(goalV2JSONWithNonGoals(`["` + nonGoal + `"]`))
			got := Evaluate(inputWith(t, []byte(validHandoffJSON), goalBytes), nil)
			if len(got.Flags) != 0 {
				t.Errorf("Flags = %v, want none for non-identical non_goal %q", got.Flags, nonGoal)
			}
			if hasReason(got, ReasonFlagsUnresolved) {
				t.Errorf("blockers = %v, want no %q", blockerReasons(got), ReasonFlagsUnresolved)
			}
		})
	}
}

func TestEvaluateIgnoresHandoffStructAndDerivesFromBytes(t *testing.T) {
	in := completeInput(t)
	in.Handoff.Handoff = Handoff{Version: 99, Stages: []string{"forged"}}

	got := Evaluate(in, nil)

	if got.State != StateDraft {
		t.Fatalf("State = %q, want %q: Evaluate must re-parse the exact bytes", got.State, StateDraft)
	}
}

func TestEvaluateSubjectDoesNotBindHead(t *testing.T) {
	a := completeInput(t)
	b := completeInput(t)
	b.Provenance.Head = "fedcba9876543210fedcba9876543210fedcba98"

	sa, sb := Evaluate(a, nil).Subject, Evaluate(b, nil).Subject
	if sa == nil || sb == nil {
		t.Fatalf("Subject missing: %v, %v", sa, sb)
	}
	if *sa != *sb {
		t.Errorf("Subject changed with HEAD alone: %#v vs %#v", *sa, *sb)
	}
}

func TestEvaluateSubjectChangesWithEitherDigest(t *testing.T) {
	base := Evaluate(completeInput(t), nil).Subject
	editedGoal := []byte(goalV2JSONWithNonGoals(`[]`) + "\n")
	editedHandoff := []byte(validHandoffJSON + "\n")
	for _, tc := range []struct {
		name string
		in   ReadinessInput
	}{
		{name: "goal bytes", in: inputWith(t, []byte(validHandoffJSON), editedGoal)},
		{name: "handoff bytes", in: inputWith(t, editedHandoff, []byte(goalV2JSONWithNonGoals(`[]`)))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in, nil).Subject
			if got == nil || base == nil {
				t.Fatalf("Subject missing: %v, %v", got, base)
			}
			if *got == *base {
				t.Errorf("Subject unchanged after a %s edit", tc.name)
			}
		})
	}
}

func TestReasonVocabularyIsClosedAndClassified(t *testing.T) {
	seen := map[Reason]bool{}
	for _, r := range Reasons() {
		if seen[r] {
			t.Errorf("reason %q listed twice", r)
		}
		seen[r] = true
		if !r.Valid() {
			t.Errorf("%q.Valid() = false for a listed reason", r)
		}
		if k := r.Kind(); k != ReasonKindUnprovable && k != ReasonKindRefused {
			t.Errorf("%q.Kind() = %q, want unprovable or refused", r, k)
		}
	}
	for _, r := range []Reason{"", "ready", "clearance_granted", "HANDOFF_INVALID"} {
		if r.Valid() {
			t.Errorf("%q.Valid() = true for an unlisted reason", r)
		}
		if k := r.Kind(); k != "" {
			t.Errorf("%q.Kind() = %q, want empty for an unlisted reason", r, k)
		}
	}
}

func TestEvaluateEmitsOnlyListedReasons(t *testing.T) {
	in := completeInput(t)
	in.Goal = nil
	in.Provenance = nil
	in.Handoff.SHA256 = ""
	for _, a := range []Assessment{Evaluate(in, nil), Evaluate(completeInput(t), &VerifiedClearance{})} {
		for _, b := range a.Blockers {
			if !b.Reason.Valid() {
				t.Errorf("Evaluate emitted unlisted reason %q", b.Reason)
			}
		}
	}
}

func TestVerifiedClearanceHasNoExportedFields(t *testing.T) {
	typ := reflect.TypeOf(VerifiedClearance{})
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			t.Errorf("VerifiedClearance exports field %s; clearance must not be buildable outside its verifier", f.Name)
		}
	}
}

// TestPackageExposesNoVerifiedClearanceConstructor parses every non-test
// source file of this package and fails if any function other than Verify
// returns a VerifiedClearance, or if any code outside Verify builds one with
// a non-empty composite literal. Verify is the only constructor, so a
// clearance can be obtained only by verifying a record against a freshly
// evaluated Subject and view.
func TestPackageExposesNoVerifiedClearanceConstructor(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	fset := token.NewFileSet()
	checked := 0
	constructors := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		checked++
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			inVerify := isFunc && fn.Recv == nil && fn.Name.Name == "Verify"
			ast.Inspect(decl, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.FuncDecl:
					if node.Type.Results == nil {
						return true
					}
					for _, field := range node.Type.Results.List {
						if !mentionsVerifiedClearance(field.Type) {
							continue
						}
						if inVerify {
							constructors++
							continue
						}
						t.Errorf("%s: func %s returns a VerifiedClearance", fset.Position(node.Pos()), node.Name.Name)
					}
				case *ast.FuncLit:
					if node.Type.Results == nil {
						return true
					}
					for _, field := range node.Type.Results.List {
						if mentionsVerifiedClearance(field.Type) {
							t.Errorf("%s: function literal returns a VerifiedClearance", fset.Position(node.Pos()))
						}
					}
				case *ast.CompositeLit:
					// An elided element type (inside a slice or map literal) has
					// a nil Type.
					if !inVerify && node.Type != nil && mentionsVerifiedClearance(node.Type) && len(node.Elts) > 0 {
						t.Errorf("%s: non-empty VerifiedClearance composite literal outside Verify", fset.Position(node.Pos()))
					}
				}
				return true
			})
		}
	}
	if checked == 0 {
		t.Fatalf("no package sources checked")
	}
	if constructors != 1 {
		t.Errorf("found %d Verify results of type VerifiedClearance, want exactly 1", constructors)
	}
}

func mentionsVerifiedClearance(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "VerifiedClearance" {
			found = true
		}
		return !found
	})
	return found
}
