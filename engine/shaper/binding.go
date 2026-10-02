package shaper

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
)

// GoalBinding is evaluation-time evidence only: it records that a specific
// caller-designated Goal v2 file, resolved inside a specific worktree root at
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

// BindGoal resolves goalPath strictly inside worktreeRoot (a relative path
// that does not leave it), has src read it (a plain regular file, not
// missing, not a symlink, not a directory, not a FIFO, whose opened file lies
// inside the resolved root and is the one the bytes come from; see
// ContainedSource), strictly parses it as a version 2 Goal, and requires both
// project_id and goal_id to match h. It fails closed: every rejection is a
// plain error and no partial GoalBinding is ever returned.
func BindGoal(src ContainedSource, h Handoff, worktreeRoot, goalPath string) (GoalBinding, error) {
	cleaned, data, err := readSource(src, worktreeRoot, "goalPath", goalPath, "goal source")
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: %w", err)
	}

	g, err := goal.Parse(data)
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: %w", err)
	}
	if g.Version != 2 {
		return GoalBinding{}, fmt.Errorf("bind goal: goal source %q has version %d, want 2", cleaned, g.Version)
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
		GoalSHA256: sha256Hex(data),
		Goal:       g,
	}, nil
}
