package shaper

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
)

// GoalBinding is evaluation-time evidence only: it records that a specific
// caller-designated Goal file (of the version PairedGoalVersion names), resolved inside a specific worktree root at
// one point in time, matched a Shaper handoff's declared identity. It is not
// readiness, does not prove the bound Goal is authoritative or current among
// duplicate files, and grants no execution authority.
type GoalBinding struct {
	// SourcePath is the cleaned, root-relative path actually read.
	SourcePath string
	// GoalBytes is the exact byte sequence evaluated.
	GoalBytes []byte
	// GoalSHA256 is the lowercase hex SHA-256 of GoalBytes. It is for drift
	// detection only (did the evaluated Goal bytes change?) and is not a
	// signature, proof of origin, or authorization.
	GoalSHA256 string
	Goal       goal.Goal
}

// PairedGoalVersion is the Goal version a handoff of handoffVersion pairs with,
// the one BindGoal and Evaluate require: goal.VersionDirective for a version 4
// handoff, whose memory_directive sits beside the Goal v3 memory_directive, and
// goal.VersionGoalID for versions 1 to 3, whose memory_scope sits beside the
// Goal v2 memory_scope. Pairing them keeps each side of the view showing the
// same kind of memory field. A handoff of any other version pairs with no Goal:
// that is an error, never a fallback. It is exported for a caller that holds an
// already-parsed handoff and Goal (the workflow lifecycle) and must refuse the
// pairing BindGoal refuses.
func PairedGoalVersion(handoffVersion int) (int, error) {
	switch handoffVersion {
	case HandoffVersionCriteria, HandoffVersionVerified, HandoffVersionPlan:
		return goal.VersionGoalID, nil
	case HandoffVersionDirective:
		return goal.VersionDirective, nil
	}
	return 0, fmt.Errorf("unsupported handoff version %d, want 1, 2, 3, or 4", handoffVersion)
}

// BindGoal resolves goalPath strictly inside worktreeRoot (a relative path
// that does not leave it), has src read it (a plain regular file, not
// missing, not a symlink, not a directory, not a FIFO, whose opened file lies
// inside the resolved root and is the one the bytes come from; see
// ContainedSource), strictly parses it as a Goal of PairedGoalVersion(h.Version),
// and requires both project_id and goal_id to match h. It fails closed: every
// rejection is a plain error and no partial GoalBinding is ever returned; a
// handoff of a version PairedGoalVersion pairs with no Goal is refused before
// anything is read.
func BindGoal(src ContainedSource, h Handoff, worktreeRoot, goalPath string) (GoalBinding, error) {
	want, err := PairedGoalVersion(h.Version)
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: %w", err)
	}
	cleaned, data, err := readSource(src, worktreeRoot, "goalPath", goalPath, "goal source")
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: %w", err)
	}

	g, err := goal.Parse(data)
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: %w", err)
	}
	if g.Version != want {
		return GoalBinding{}, fmt.Errorf("bind goal: goal source %q has version %d, want %d", cleaned, g.Version, want)
	}
	if g.ProjectID != h.ProjectID {
		return GoalBinding{}, fmt.Errorf("bind goal: project_id mismatch: handoff %q, goal %q", h.ProjectID, g.ProjectID)
	}
	if g.GoalID != h.GoalID {
		return GoalBinding{}, fmt.Errorf("bind goal: goal_id mismatch: handoff %q, goal %q", h.GoalID, g.GoalID)
	}

	return GoalBinding{
		SourcePath: cleaned,
		GoalBytes:  data,
		GoalSHA256: SourceSHA256(data),
		Goal:       g,
	}, nil
}
