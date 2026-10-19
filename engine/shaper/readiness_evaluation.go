package shaper

import (
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
)

// evaluation is what one Evaluate builds up as it looks at the evidence in turn: the handoff it
// parsed, every blocker found so far in the order they were found, whether the evidence for a
// Subject is still whole, and what the later steps need of the earlier ones (the digests, the
// Goal, the flags and the worktree). It is local to one call and keeps nothing between calls.
//
// The steps run in a fixed order, which is the order of the blockers an Assessment lists:
// checkHandoffSource, checkGoal, checkWorktree, subjectAndView, then conclude.
type evaluation struct {
	in         ReadinessInput
	h          Handoff
	blockers   []Blocker
	subjectOK  bool
	handoffSHA string
	goalSHA    string
	g          goal.Goal
	flags      []Flag
	worktree   WorktreeProvenance
}

// block records one blocker.
func (e *evaluation) block(r Reason, format string, args ...any) {
	e.blockers = append(e.blockers, Blocker{Reason: r, Detail: fmt.Sprintf(format, args...)})
}

// refuse records a blocker that leaves the evidence for a Subject incomplete or contradictory,
// so none is given.
func (e *evaluation) refuse(r Reason, format string, args ...any) {
	e.block(r, format, args...)
	e.subjectOK = false
}

// checkHandoffSource holds the recorded digest of the handoff bytes to the bytes, and requires the
// path they were read from to be recorded.
func (e *evaluation) checkHandoffSource() {
	e.handoffSHA = SourceSHA256(e.in.Handoff.Bytes)
	if e.in.Handoff.SHA256 != e.handoffSHA {
		e.refuse(ReasonHandoffDigestMismatch, "recorded handoff sha256 %q, bytes hash to %q", e.in.Handoff.SHA256, e.handoffSHA)
	}
	if e.in.Handoff.SourcePath == "" {
		e.refuse(ReasonSourceUnrecorded, "handoff source path is empty")
	}
}

// checkGoal holds the Goal binding to the same rules as the handoff source, then parses the Goal
// bytes and requires a Goal of goalVersionFor the handoff's version (2, or 3 for a version 4
// handoff) and of the handoff's own project and goal id. A Goal that passes raises the overlap
// with its non_goals: refused as blockers for a version 3 or 4 handoff (OD6), flagged for human
// review for the others.
func (e *evaluation) checkGoal() {
	binding := e.in.Goal
	if binding == nil {
		e.refuse(ReasonGoalUnbound, "no Goal binding supplied")
		return
	}
	e.goalSHA = SourceSHA256(binding.GoalBytes)
	if binding.GoalSHA256 != e.goalSHA {
		e.refuse(ReasonGoalDigestMismatch, "recorded goal sha256 %q, bytes hash to %q", binding.GoalSHA256, e.goalSHA)
	}
	if binding.SourcePath == "" {
		e.refuse(ReasonSourceUnrecorded, "goal source path is empty")
	}
	parsed, err := goal.Parse(binding.GoalBytes)
	e.g = parsed
	switch {
	case err != nil:
		e.refuse(ReasonGoalInvalid, "%v", err)
	case e.g.Version != goalVersionFor(e.h.Version):
		e.refuse(ReasonGoalInvalid, "goal has version %d, want %d", e.g.Version, goalVersionFor(e.h.Version))
	case e.g.ProjectID != e.h.ProjectID || e.g.GoalID != e.h.GoalID:
		e.refuse(ReasonGoalIdentityMismatch, "handoff (%q, %q), goal (%q, %q)", e.h.ProjectID, e.h.GoalID, e.g.ProjectID, e.g.GoalID)
	default:
		if e.h.Version >= 3 {
			e.blockers = append(e.blockers, od6Overlaps(e.h, e.g.NonGoals)...)
		} else {
			e.flags = nonGoalOverlapFlags(e.h, e.g.NonGoals)
		}
	}
}

// checkWorktree requires the observed worktree to be present, complete, and the one the sources
// were read from: an absolute, clean root equal to the observed toplevel.
func (e *evaluation) checkWorktree() {
	in := e.in
	switch p := in.Provenance; {
	case p == nil:
		e.refuse(ReasonWorktreeProvenanceUnobserved, "no worktree observation supplied")
	case p.Toplevel == "" || p.GitDir == "" || p.CommonDir == "":
		e.refuse(ReasonWorktreeProvenanceIncomplete, "observation lacks toplevel, git dir, or common dir")
	case !filepath.IsAbs(in.WorktreeRoot) || filepath.Clean(in.WorktreeRoot) != in.WorktreeRoot || in.WorktreeRoot != p.Toplevel:
		e.refuse(ReasonWorktreeProvenanceMismatch, "worktree root %q, observed toplevel %q", in.WorktreeRoot, p.Toplevel)
	default:
		e.worktree = *p
	}
}

// subjectAndView returns the Subject a clearance would have to bind and the view the host must
// present, or nil for both when the evidence for them is not whole. A view that holds a terminal
// control or invisible formatting rune is refused with ReasonViewUnpresentable and gives neither,
// so it is never displayed and never cleared.
func (e *evaluation) subjectAndView() (*Subject, []byte) {
	if !e.subjectOK {
		return nil, nil
	}
	subject := &Subject{
		ProjectID:         e.h.ProjectID,
		GoalID:            e.h.GoalID,
		HandoffSourcePath: e.in.Handoff.SourcePath,
		HandoffSHA256:     e.handoffSHA,
		GoalSourcePath:    e.in.Goal.SourcePath,
		GoalSHA256:        e.goalSHA,
		Worktree:          e.worktree,
	}
	presented := e.presentedView()
	if err := checkPresentable(presented); err != nil {
		// Refuse, never rewrite: withholding Subject and View means no
		// caller can display this view and no clearance can match it.
		e.block(ReasonViewUnpresentable, "%v", err)
		return nil, nil
	}
	return subject, RenderView(presented)
}

// presentedView is the content the view is rendered from: the Goal and the plan as bytes and in
// their parts, the flags, and for a version 3 or 4 handoff the six sections it adds.
func (e *evaluation) presentedView() PresentedView {
	presented := PresentedView{
		GoalBytes:    e.in.Goal.GoalBytes,
		PlanBytes:    e.in.Handoff.Bytes,
		GoalScope:    e.g.Scope,
		GoalNonGoals: e.g.NonGoals,
		OutOfScope:   e.h.OutOfScope,
		Acceptance:   e.h.AcceptanceItems,
		Flags:        e.flags,
	}
	if e.h.Version >= 3 {
		presented.V3 = &PresentedViewV3{
			Roles:                e.h.Roles,
			Tests:                e.h.Tests,
			Risks:                e.h.Risks,
			Estimates:            e.h.Estimates,
			MemoryScope:          e.h.MemoryScope,
			GoalMemoryScope:      e.g.MemoryScope,
			MemoryDirective:      e.h.MemoryDirective,
			GoalMemoryDirective:  e.g.MemoryDirective,
			DeliveryLimit:        e.h.DeliveryLimit,
			GoalDeliveryBoundary: e.g.DeliveryBoundary,
		}
	}
	return presented
}

// conclude weighs the clearance against the Subject, the view and the flags, adds the blockers that
// depend on it (flags still unresolved, the clearance itself, the version 1 acceptance cap), and
// returns the Assessment: StateReady when no blocker remains, StateDraft otherwise.
func (e *evaluation) conclude(clearance *VerifiedClearance, subject *Subject, view []byte) Assessment {
	clearanceBlocker, cleared := judgeClearance(clearance, subject, view, e.flags)

	if len(e.flags) > 0 && !cleared {
		e.block(ReasonFlagsUnresolved, "%d flag(s) await a host-captured human resolution", len(e.flags))
	}
	if clearanceBlocker != nil {
		e.blockers = append(e.blockers, *clearanceBlocker)
	}
	if e.h.Version == 1 {
		e.block(ReasonAcceptanceVerificationUnrepresentable, "handoff version 1 has no per-criterion verification method or adjudication path")
	}

	a := Assessment{State: StateDraft, Blockers: e.blockers, Flags: e.flags, Subject: subject, View: view}
	if len(e.blockers) == 0 {
		a.State = StateReady
	}
	return a
}

// judgeClearance says whether clearance counts for exactly this subject, view and flags, and when
// it does not, the one blocker that says why: none was supplied, it was not produced by Verify,
// there is no subject to match it to, or it was verified against something else.
func judgeClearance(clearance *VerifiedClearance, subject *Subject, view []byte, flags []Flag) (blocker *Blocker, cleared bool) {
	switch {
	case clearance == nil:
		return &Blocker{Reason: ReasonClearanceMissing, Detail: "no host-owned clearance supplied"}, false
	case !clearance.verified:
		return &Blocker{Reason: ReasonClearanceUnverified, Detail: "clearance was not produced by Verify"}, false
	case subject == nil:
		return &Blocker{Reason: ReasonClearanceUnverified, Detail: "subject evidence is incomplete or refused, so the clearance cannot be matched"}, false
	case !clearance.matches(*subject, ViewDigest(view), flags):
		return &Blocker{Reason: ReasonClearanceMismatch, Detail: "clearance was verified against a different subject, view, or flag set"}, false
	default:
		return nil, true
	}
}
