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
// In this version readiness is hard-capped at draft. Evaluate cannot return
// StateReady, because VerifiedClearance has no constructor yet and because
// handoff version 1 cannot represent a verification method or adjudication
// path for each acceptance criterion.
type State string

const (
	// StateInvalid means the handoff bytes are not a structurally valid
	// handoff; nothing else about them is assessed.
	StateInvalid State = "invalid"
	// StateDraft means the handoff is structurally valid but at least one
	// blocker prevents readiness.
	StateDraft State = "draft"
	// StateReady means no blocker remains. It is unreachable in this
	// version, and when reachable it will still grant no execution
	// authority.
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
	// but this version has no verifier, so it is never accepted.
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

// FlagGoalNonGoalOverlap marks a handoff stages or acceptance item that is
// byte-identical to a bound Goal non_goals item. It is raised for human
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
	FlagID   string
	Reason   string
	Evidence string
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
	HandoffSourcePath string
	HandoffSHA256     string
	GoalSourcePath    string
	GoalSHA256        string
	Worktree          WorktreeProvenance
}

// VerifiedClearance is a host-owned human semantic clearance that has been
// verified against a freshly observed Subject. It is opaque and sealed: it
// has no exported fields, and this package has no constructor for it yet, so
// the only values a caller can hold are nil and the zero value, and Evaluate
// accepts neither. Shaper-authored JSON can never supply clearance, and a
// clearance, once verifiable, will still grant no execution authority.
type VerifiedClearance struct {
	subject     Subject
	resolutions []FlagResolution
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
// blocker in a fixed order, the flags raised for human review, and the
// Subject a clearance would have to bind. Subject is nil unless every piece
// of subject evidence is present and consistent.
type Assessment struct {
	State    State
	Blockers []Blocker
	Flags    []Flag
	Subject  *Subject
}

// Evaluate assesses readiness of in, given an optional clearance. It is pure:
// it reads no files and runs no processes.
//
// A handoff whose bytes fail strict Parse is StateInvalid with the single
// blocker ReasonHandoffInvalid. Otherwise every blocker is collected, and the
// state is StateDraft while any blocker remains. In this version a blocker
// always remains, so Evaluate never returns StateReady. Readiness grants no
// execution authority in any case, and Shaper-authored JSON can never supply
// clearance or flag resolutions.
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
		g, err := goal.Parse(in.Goal.GoalBytes)
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

	if len(flags) > 0 {
		block(ReasonFlagsUnresolved, "%d flag(s) await a host-captured human resolution", len(flags))
	}
	if clearance == nil {
		block(ReasonClearanceMissing, "no host-owned clearance supplied")
	} else {
		block(ReasonClearanceUnverified, "no clearance verifier exists in this version")
	}
	if h.Version == 1 {
		block(ReasonAcceptanceVerificationUnrepresentable, "handoff version 1 has no per-criterion verification method or adjudication path")
	}

	a := Assessment{State: StateDraft, Blockers: blockers, Flags: flags}
	if subjectOK {
		a.Subject = &Subject{
			HandoffSourcePath: in.Handoff.SourcePath,
			HandoffSHA256:     handoffSHA,
			GoalSourcePath:    in.Goal.SourcePath,
			GoalSHA256:        goalSHA,
			Worktree:          worktree,
		}
	}
	if len(blockers) == 0 {
		a.State = StateReady
	}
	return a
}

// nonGoalOverlapFlags raises a FlagGoalNonGoalOverlap for every stages or
// acceptance item byte-identical to a Goal non_goals item. No trimming, case
// folding, substring, or other normalization is applied.
func nonGoalOverlapFlags(h Handoff, nonGoals []string) []Flag {
	var flags []Flag
	for _, field := range []struct {
		name   string
		values []string
	}{
		{name: "stages", values: h.Stages},
		{name: "acceptance", values: h.Acceptance},
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
