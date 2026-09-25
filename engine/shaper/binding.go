package shaper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
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
	Goal      goal.Goal
}

// BindGoal resolves goalPath strictly inside worktreeRoot, requires the
// target to be a plain regular file (not missing, not a symlink, not a
// directory) reached with no ancestor symlink escaping the root, strictly
// parses it as a version 2 Goal, and requires both project_id and goal_id to
// match h. It fails closed: every rejection is a plain error and no partial
// GoalBinding is ever returned.
func BindGoal(h Handoff, worktreeRoot, goalPath string) (GoalBinding, error) {
	if worktreeRoot == "" {
		return GoalBinding{}, fmt.Errorf("bind goal: worktreeRoot must not be empty")
	}
	if !filepath.IsAbs(worktreeRoot) {
		return GoalBinding{}, fmt.Errorf("bind goal: worktreeRoot must be absolute, got %q", worktreeRoot)
	}
	if goalPath == "" {
		return GoalBinding{}, fmt.Errorf("bind goal: goalPath must not be empty")
	}
	if filepath.IsAbs(goalPath) {
		return GoalBinding{}, fmt.Errorf("bind goal: goalPath must be relative, got %q", goalPath)
	}

	cleaned := filepath.Clean(goalPath)
	if cleaned == "." {
		return GoalBinding{}, fmt.Errorf("bind goal: goalPath must not resolve to the worktree root itself, got %q", goalPath)
	}
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return GoalBinding{}, fmt.Errorf("bind goal: goalPath must not traverse outside the worktree root, got %q", goalPath)
		}
	}

	joined := filepath.Join(worktreeRoot, cleaned)

	info, err := os.Lstat(joined)
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: goal source %q is not accessible: %w", cleaned, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return GoalBinding{}, fmt.Errorf("bind goal: goal source %q must not be a symlink", cleaned)
	}
	if !info.Mode().IsRegular() {
		return GoalBinding{}, fmt.Errorf("bind goal: goal source %q must be a regular file", cleaned)
	}

	within, err := pathguard.ResolvedWithinRoot(worktreeRoot, joined)
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: could not prove containment of %q: %w", cleaned, err)
	}
	if !within {
		return GoalBinding{}, fmt.Errorf("bind goal: goal source %q resolves outside the worktree root", cleaned)
	}

	data, err := os.ReadFile(joined)
	if err != nil {
		return GoalBinding{}, fmt.Errorf("bind goal: read goal source %q: %w", cleaned, err)
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
		Goal:       g,
	}, nil
}
