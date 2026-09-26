package shaper

import (
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
)

// State is the readiness outcome of one evaluation. Readiness is a separate
// assessment from structural validity, and it never grants execution
// authority: even a ready handoff does not authorize, permit, or dispatch any
// work, and it is not RDD review, Goal fulfillment, or Goal closure.
//
// A handoff version 1 is hard-capped at draft: it cannot represent a
// verification method or adjudication path for each acceptance criterion, so
// ReasonAcceptanceVerificationUnrepresentable always remains. A handoff
// version 2 carries that plan per criterion and can reach StateReady once
// every other blocker is gone. The plan is not executed: check and
// adjudication results are downstream fulfillment evidence, not a readiness
// prerequisite.
//
// Ready is claimed on the approved core plus per-criterion acceptance
// verification only. The roadmap's full Phase 3 plan outcome is not yet met:
// roles, tests, risks, estimates, memory_scope, and delivery_limit are
// absent (see ReadyDisclosure).
//
// A ready outcome is not a signature: any process running as the same OS
// user can forge a clearance record, so ready rests only on content
// digests and the runtime deny guards that keep the model from writing
// records.
type State string

const (
	// StateInvalid means the handoff bytes are not a structurally valid
	// handoff; nothing else about them is assessed.
	StateInvalid State = "invalid"
	// StateDraft means the handoff is structurally valid but at least one
	// blocker prevents readiness.
	StateDraft State = "draft"
	// StateReady means no blocker remains. Only a handoff version 2 can
	// reach it, and it grants no execution authority. A ready outcome is
	// not a signature: any process running as the same OS user, including
	// any installed Pi extension, can forge a clearance record. Every
	// output reporting it must print ReadyDisclosure.
	StateReady State = "ready"
)

// ReasonKind splits the closed Reason vocabulary: an unprovable reason means
// the available evidence cannot establish readiness; a refused reason means
// the evidence proves a contradiction or violation. Both block readiness.
type ReasonKind string

const (
	ReasonKindUnprovable ReasonKind = "unprovable"
	ReasonKindRefused    ReasonKind = "refused"
)

// Reason is one member of the closed vocabulary of readiness blockers.
// Reasons returns every member; any other value is not a Reason Evaluate
// emits.
type Reason string

const (
	// ReasonHandoffInvalid (refused): the handoff bytes fail strict Parse.
	ReasonHandoffInvalid Reason = "handoff_invalid"
	// ReasonHandoffDigestMismatch (refused): the recorded handoff SHA-256
	// does not match the handoff bytes.
	ReasonHandoffDigestMismatch Reason = "handoff_digest_mismatch"
	// ReasonGoalInvalid (refused): the Goal bytes are not a valid version 2
	// Goal.
	ReasonGoalInvalid Reason = "goal_invalid"
	// ReasonGoalDigestMismatch (refused): the recorded Goal SHA-256 does not
	// match the Goal bytes.
	ReasonGoalDigestMismatch Reason = "goal_digest_mismatch"
	// ReasonGoalIdentityMismatch (refused): the Goal project_id or goal_id
	// differs from the handoff's.
	ReasonGoalIdentityMismatch Reason = "goal_identity_mismatch"
	// ReasonWorktreeProvenanceMismatch (refused): the worktree root is not
	// absolute and clean, or differs from the observed toplevel.
	ReasonWorktreeProvenanceMismatch Reason = "worktree_provenance_mismatch"
	// ReasonClearanceMismatch (refused): a verified clearance was bound to a
	// different subject, presented view, or flag set than this evaluation
	// derives, so it no longer describes this content.
	ReasonClearanceMismatch Reason = "clearance_mismatch"
	// ReasonViewUnpresentable (refused): the view the host would present
	// holds a C0 control other than LF and TAB, DEL, a C1 control, a Unicode
	// format control (bidirectional or invisible), or invalid UTF-8, any of
	// which can hide or disguise text from the human while the view digest
	// still binds it. The view is refused, never rewritten, so no Subject or
	// View is produced and nothing about this content can be cleared. The
	// detail names the section and rune offset.
	ReasonViewUnpresentable Reason = "view_unpresentable"

	// ReasonSourceUnrecorded (unprovable): a handoff or Goal source path is
	// missing, so the subject's provenance is incomplete.
	ReasonSourceUnrecorded Reason = "source_unrecorded"
	// ReasonGoalUnbound (unprovable): no Goal binding was supplied.
	ReasonGoalUnbound Reason = "goal_unbound"
	// ReasonWorktreeProvenanceUnobserved (unprovable): no worktree
	// observation was supplied.
	ReasonWorktreeProvenanceUnobserved Reason = "worktree_provenance_unobserved"
	// ReasonWorktreeProvenanceIncomplete (unprovable): the observation lacks
	// its toplevel, git dir, or common dir.
	ReasonWorktreeProvenanceIncomplete Reason = "worktree_provenance_incomplete"
	// ReasonFlagsUnresolved (unprovable): at least one flag awaits a human
	// resolution captured by the host.
	ReasonFlagsUnresolved Reason = "flags_unresolved"
	// ReasonClearanceMissing (unprovable): no clearance was supplied.
	ReasonClearanceMissing Reason = "clearance_missing"
	// ReasonClearanceUnverified (unprovable): a clearance value was supplied
	// that Verify did not produce, or the subject evidence is incomplete or
	// refused, so the clearance cannot be matched to it.
	ReasonClearanceUnverified Reason = "clearance_unverified"
	// ReasonAcceptanceVerificationUnrepresentable (unprovable): handoff
	// version 1 carries no verification method or adjudication path per
	// acceptance criterion, so readiness is capped at draft.
	ReasonAcceptanceVerificationUnrepresentable Reason = "acceptance_verification_unrepresentable"
)

var reasonKinds = map[Reason]ReasonKind{
	ReasonHandoffInvalid:                        ReasonKindRefused,
	ReasonHandoffDigestMismatch:                 ReasonKindRefused,
	ReasonGoalInvalid:                           ReasonKindRefused,
	ReasonGoalDigestMismatch:                    ReasonKindRefused,
	ReasonGoalIdentityMismatch:                  ReasonKindRefused,
	ReasonWorktreeProvenanceMismatch:            ReasonKindRefused,
	ReasonClearanceMismatch:                     ReasonKindRefused,
	ReasonViewUnpresentable:                     ReasonKindRefused,
	ReasonSourceUnrecorded:                      ReasonKindUnprovable,
	ReasonGoalUnbound:                           ReasonKindUnprovable,
	ReasonWorktreeProvenanceUnobserved:          ReasonKindUnprovable,
	ReasonWorktreeProvenanceIncomplete:          ReasonKindUnprovable,
	ReasonFlagsUnresolved:                       ReasonKindUnprovable,
	ReasonClearanceMissing:                      ReasonKindUnprovable,
	ReasonClearanceUnverified:                   ReasonKindUnprovable,
	ReasonAcceptanceVerificationUnrepresentable: ReasonKindUnprovable,
}

// Reasons returns every member of the closed Reason vocabulary, refused
// reasons first, in a fixed order.
func Reasons() []Reason {
	return []Reason{
		ReasonHandoffInvalid,
		ReasonHandoffDigestMismatch,
		ReasonGoalInvalid,
		ReasonGoalDigestMismatch,
		ReasonGoalIdentityMismatch,
		ReasonWorktreeProvenanceMismatch,
		ReasonClearanceMismatch,
		ReasonViewUnpresentable,
		ReasonSourceUnrecorded,
		ReasonGoalUnbound,
		ReasonWorktreeProvenanceUnobserved,
		ReasonWorktreeProvenanceIncomplete,
		ReasonFlagsUnresolved,
		ReasonClearanceMissing,
		ReasonClearanceUnverified,
		ReasonAcceptanceVerificationUnrepresentable,
	}
}

// Valid reports whether r belongs to the closed vocabulary.
func (r Reason) Valid() bool {
	_, ok := reasonKinds[r]
	return ok
}

// Kind classifies r as unprovable or refused, or returns "" for a value
// outside the closed vocabulary.
func (r Reason) Kind() ReasonKind {
	return reasonKinds[r]
}

// Blocker is one reason readiness was not reached, with a human-readable
// detail.
type Blocker struct {
	Reason Reason
	Detail string
}

// FlagKind is the category of a flag raised for human review.
type FlagKind string

// FlagGoalNonGoalOverlap marks a handoff stages or acceptance item (for
// version 2, its criterion text) that is byte-identical to a bound Goal
// non_goals item. It is raised for human
// review, not rejected: whether such overlap is a deterministic violation is
// an open product decision (OD6). Like the out_of_scope rule it detects
// literal overlap only and proves nothing about semantic scope.
const FlagGoalNonGoalOverlap FlagKind = "goal_non_goal_overlap"

// Flag is an issue Evaluate raises for human review. A flag never carries its
// own resolution: resolutions are FlagResolution values that only a verified
// host-owned clearance can carry, so an unresolved flag stays visible and
// blocks readiness.
type Flag struct {
	// ID is deterministic for the same kind, field, and 1-based index.
	ID    string
	Kind  FlagKind
	Field string
	Index int
	Item  string
}

// FlagResolution is a human's resolution of one flag, with a reason and
// evidence, captured by the active agent runtime. It is a separate type from
// Flag and appears only inside a host-owned clearance record, never in
// Shaper-authored JSON.
type FlagResolution struct {
	FlagID   string `json:"flag_id"`
	Reason   string `json:"reason"`
	Evidence string `json:"evidence"`
}

// WorktreeProvenance is the bound part of a gitprov.Observation. HEAD is
// deliberately excluded: it is informational only and not bound.
type WorktreeProvenance struct {
	Toplevel  string
	GitDir    string
	CommonDir string
}

// Subject is what a host-owned clearance must bind to: the exact handoff and
// Goal bytes, by digest, plus their provenance as observed at evaluation.
// Digests are lowercase hex SHA-256 over raw bytes, for drift detection only;
// they are not identity, authority, or a signature. The Subject lives only in
// the clearance record, never inside the handoff it binds, so there is no
// circular digest.
type Subject struct {
	ProjectID         string
	GoalID            string
	HandoffSourcePath string
	HandoffSHA256     string
	GoalSourcePath    string
	GoalSHA256        string
	Worktree          WorktreeProvenance
}

// VerifiedClearance is a host-owned human semantic clearance that has been
// verified against a freshly evaluated Subject, flag set, and presented view.
// It is opaque and sealed: it has no exported fields, and Verify is its only
// constructor. Evaluate refuses the zero value and re-checks the binding
// against its own fresh evaluation. Shaper-authored JSON can never supply
// clearance, and a clearance grants no execution authority.
//
// It helps reach ready only as evidence of a claimed host capture. It is not
// a signature: any process running as the same OS user can forge the record
// it was verified from.
type VerifiedClearance struct {
	verified    bool
	subject     Subject
	viewSHA256  string
	resolutions []FlagResolution
}

// matches reports whether c was verified against exactly this subject,
// view digest, and flag set.
func (c *VerifiedClearance) matches(subject Subject, viewSHA256 string, flags []Flag) bool {
	return c.subject == subject && c.viewSHA256 == viewSHA256 && checkResolutionsExact(c.resolutions, flags) == nil
}

// ReadinessInput is the evidence one readiness evaluation considers. Evaluate
// derives everything from the raw bytes: the parsed Handoff and Goal fields
// of Handoff and Goal are ignored and re-parsed, so they cannot be forged
// independently of the bytes they claim to describe.
type ReadinessInput struct {
	// WorktreeRoot is the absolute, symlink-resolved root the sources were
	// read from; it must equal the observed toplevel.
	WorktreeRoot string
	Handoff      HandoffSource
	// Goal is nil when no Goal was bound.
	Goal *GoalBinding
	// Provenance is nil when the worktree was not observed.
	Provenance *gitprov.Observation
}

// Assessment is the result of one readiness evaluation: its state, every
// blocker in a fixed order, the flags raised for human review, the Subject a
// clearance would have to bind, and the rendered view the host must present.
// Subject and View are nil unless every piece of subject evidence is present
// and consistent and the view is presentable (see ReasonViewUnpresentable).
type Assessment struct {
	State    State
	Blockers []Blocker
	Flags    []Flag
	Subject  *Subject
	// View is RenderView of the evaluated content; the host displays these
	// bytes verbatim and a clearance binds their ViewDigest.
	View []byte
}

// Evaluate assesses readiness of in, given an optional clearance. It is pure:
// it reads no files and runs no processes.
//
// A handoff whose bytes fail strict Parse is StateInvalid with the single
// blocker ReasonHandoffInvalid. Otherwise every blocker is collected, and the
// state is StateDraft while any blocker remains. A view holding a terminal
// control or invisible formatting rune is refused with
// ReasonViewUnpresentable and leaves Subject and View nil, so it is never
// displayed and never cleared. A clearance counts only when
// Verify produced it for exactly the Subject, view, and flags derived here;
// it then resolves the flags and satisfies the clearance requirement. Handoff
// version 1 always keeps ReasonAcceptanceVerificationUnrepresentable, so it
// never reaches StateReady; handoff version 2 reaches it when no blocker
// remains. Planned checks are never executed here. Readiness grants no
// execution authority in any case, and Shaper-authored JSON can never supply
// clearance or flag resolutions.
//
// A ready outcome is not a signature: any process running as the same OS
// user, including any installed Pi extension, can forge a clearance record.
func Evaluate(in ReadinessInput, clearance *VerifiedClearance) Assessment {
	h, err := Parse(in.Handoff.Bytes)
	if err != nil {
		return Assessment{
			State:    StateInvalid,
			Blockers: []Blocker{{Reason: ReasonHandoffInvalid, Detail: err.Error()}},
		}
	}

	var blockers []Blocker
	block := func(r Reason, format string, args ...any) {
		blockers = append(blockers, Blocker{Reason: r, Detail: fmt.Sprintf(format, args...)})
	}
	subjectOK := true

	handoffSHA := sha256Hex(in.Handoff.Bytes)
	if in.Handoff.SHA256 != handoffSHA {
		block(ReasonHandoffDigestMismatch, "recorded handoff sha256 %q, bytes hash to %q", in.Handoff.SHA256, handoffSHA)
		subjectOK = false
	}
	if in.Handoff.SourcePath == "" {
		block(ReasonSourceUnrecorded, "handoff source path is empty")
		subjectOK = false
	}

	var flags []Flag
	var goalSHA string
	var g goal.Goal
	if in.Goal == nil {
		block(ReasonGoalUnbound, "no Goal binding supplied")
		subjectOK = false
	} else {
		goalSHA = sha256Hex(in.Goal.GoalBytes)
		if in.Goal.GoalSHA256 != goalSHA {
			block(ReasonGoalDigestMismatch, "recorded goal sha256 %q, bytes hash to %q", in.Goal.GoalSHA256, goalSHA)
			subjectOK = false
		}
		if in.Goal.SourcePath == "" {
			block(ReasonSourceUnrecorded, "goal source path is empty")
			subjectOK = false
		}
		parsed, err := goal.Parse(in.Goal.GoalBytes)
		g = parsed
		switch {
		case err != nil:
			block(ReasonGoalInvalid, "%v", err)
			subjectOK = false
		case g.Version != 2:
			block(ReasonGoalInvalid, "goal has version %d, want 2", g.Version)
			subjectOK = false
		case g.ProjectID != h.ProjectID || g.GoalID != h.GoalID:
			block(ReasonGoalIdentityMismatch, "handoff (%q, %q), goal (%q, %q)", h.ProjectID, h.GoalID, g.ProjectID, g.GoalID)
			subjectOK = false
		default:
			flags = nonGoalOverlapFlags(h, g.NonGoals)
		}
	}

	var worktree WorktreeProvenance
	switch p := in.Provenance; {
	case p == nil:
		block(ReasonWorktreeProvenanceUnobserved, "no worktree observation supplied")
		subjectOK = false
	case p.Toplevel == "" || p.GitDir == "" || p.CommonDir == "":
		block(ReasonWorktreeProvenanceIncomplete, "observation lacks toplevel, git dir, or common dir")
		subjectOK = false
	case !filepath.IsAbs(in.WorktreeRoot) || filepath.Clean(in.WorktreeRoot) != in.WorktreeRoot || in.WorktreeRoot != p.Toplevel:
		block(ReasonWorktreeProvenanceMismatch, "worktree root %q, observed toplevel %q", in.WorktreeRoot, p.Toplevel)
		subjectOK = false
	default:
		worktree = WorktreeProvenance{Toplevel: p.Toplevel, GitDir: p.GitDir, CommonDir: p.CommonDir}
	}

	var subject *Subject
	var view []byte
	if subjectOK {
		subject = &Subject{
			ProjectID:         h.ProjectID,
			GoalID:            h.GoalID,
			HandoffSourcePath: in.Handoff.SourcePath,
			HandoffSHA256:     handoffSHA,
			GoalSourcePath:    in.Goal.SourcePath,
			GoalSHA256:        goalSHA,
			Worktree:          worktree,
		}
		presented := PresentedView{
			GoalBytes:    in.Goal.GoalBytes,
			PlanBytes:    in.Handoff.Bytes,
			GoalScope:    g.Scope,
			GoalNonGoals: g.NonGoals,
			OutOfScope:   h.OutOfScope,
			Acceptance:   h.AcceptanceItems,
			Flags:        flags,
		}
		if err := checkPresentable(presented); err != nil {
			// Refuse, never rewrite: withholding Subject and View means no
			// caller can display this view and no clearance can match it.
			block(ReasonViewUnpresentable, "%v", err)
			subject = nil
		} else {
			view = RenderView(presented)
		}
	}

	var clearanceBlocker *Blocker
	cleared := false
	switch {
	case clearance == nil:
		clearanceBlocker = &Blocker{Reason: ReasonClearanceMissing, Detail: "no host-owned clearance supplied"}
	case !clearance.verified:
		clearanceBlocker = &Blocker{Reason: ReasonClearanceUnverified, Detail: "clearance was not produced by Verify"}
	case subject == nil:
		clearanceBlocker = &Blocker{Reason: ReasonClearanceUnverified, Detail: "subject evidence is incomplete or refused, so the clearance cannot be matched"}
	case !clearance.matches(*subject, ViewDigest(view), flags):
		clearanceBlocker = &Blocker{Reason: ReasonClearanceMismatch, Detail: "clearance was verified against a different subject, view, or flag set"}
	default:
		cleared = true
	}

	if len(flags) > 0 && !cleared {
		block(ReasonFlagsUnresolved, "%d flag(s) await a host-captured human resolution", len(flags))
	}
	if clearanceBlocker != nil {
		blockers = append(blockers, *clearanceBlocker)
	}
	if h.Version == 1 {
		block(ReasonAcceptanceVerificationUnrepresentable, "handoff version 1 has no per-criterion verification method or adjudication path")
	}

	a := Assessment{State: StateDraft, Blockers: blockers, Flags: flags, Subject: subject, View: view}
	if len(blockers) == 0 {
		a.State = StateReady
	}
	return a
}

// nonGoalOverlapFlags raises a FlagGoalNonGoalOverlap for every stages or
// acceptance item byte-identical to a Goal non_goals item; for handoff
// version 2 the acceptance item is its criterion text, never its check or
// adjudication. No trimming, case folding, substring, or other normalization
// is applied.
func nonGoalOverlapFlags(h Handoff, nonGoals []string) []Flag {
	acceptance := h.Acceptance
	if h.AcceptanceItems != nil {
		acceptance = acceptanceCriteria(h.AcceptanceItems)
	}
	var flags []Flag
	for _, field := range []struct {
		name   string
		values []string
	}{
		{name: "stages", values: h.Stages},
		{name: "acceptance", values: acceptance},
	} {
		for i, value := range field.values {
			for _, nonGoal := range nonGoals {
				if value == nonGoal {
					flags = append(flags, Flag{
						ID:    fmt.Sprintf("%s:%s:%d", FlagGoalNonGoalOverlap, field.name, i+1),
						Kind:  FlagGoalNonGoalOverlap,
						Field: field.name,
						Index: i + 1,
						Item:  value,
					})
					break
				}
			}
		}
	}
	return flags
}
