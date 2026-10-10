package shaper

// WorktreeObserver is how the shaper learns which worktree a root is: the top level, the git
// directory and the common directory the readiness binds a clearance to. The domain owns this
// port; the command wires an adapter over git (engine/gitprov). An observer that cannot say
// is an error, and the shaper goes on without a provenance, reporting why.
type WorktreeObserver interface {
	Observe(root string) (WorktreeProvenance, error)
}

// LoadReadinessInput assembles what Evaluate is given: the handoff and the Goal read strictly
// inside root through files (see ContainedSource), and the worktree as observed. root is the
// absolute, symlink-resolved root.
//
// A source that is there but fails its strict parse or binding is not an error: it is handed on
// as the bytes it holds (its cleaned path, the bytes, their digest), so that Evaluate reports it
// as a blocker. A handoff that does not parse stops there, since the Goal binds to the handoff:
// the input then holds the handoff alone and the worktree is not observed. A source that cannot
// be read at all is an error, the one LoadHandoff or BindGoal returned (it begins "load
// handoff: " or "bind goal: "), with no partial input.
//
// A worktree that cannot be observed (or no observer, when worktree is nil) leaves Provenance nil;
// the failure is returned as a note for the caller to show.
func LoadReadinessInput(files ContainedSource, worktree WorktreeObserver, root, handoffPath, goalPath string) (ReadinessInput, []string, error) {
	in := ReadinessInput{WorktreeRoot: root}
	var notes []string

	src, loadErr := LoadHandoff(files, root, handoffPath)
	if loadErr != nil {
		cleaned, data, err := ReadContainedSource(files, root, handoffPath)
		if err != nil {
			return ReadinessInput{}, nil, loadErr
		}
		in.Handoff = HandoffSource{SourcePath: cleaned, Bytes: data, SHA256: SourceSHA256(data)}
		return in, notes, nil
	}
	in.Handoff = src

	binding, bindErr := BindGoal(files, src.Handoff, root, goalPath)
	if bindErr != nil {
		cleaned, data, err := ReadContainedSource(files, root, goalPath)
		if err != nil {
			return ReadinessInput{}, nil, bindErr
		}
		binding = GoalBinding{SourcePath: cleaned, GoalBytes: data, GoalSHA256: SourceSHA256(data)}
	}
	in.Goal = &binding

	if worktree != nil {
		provenance, err := worktree.Observe(root)
		if err != nil {
			notes = append(notes, "worktree observation failed: "+err.Error())
		} else {
			in.Provenance = &provenance
		}
	}
	return in, notes, nil
}
