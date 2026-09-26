package shaper

import "fmt"

// HandoffSource is a Shaper handoff read from a caller-designated file inside
// a worktree root: the parsed Handoff, the exact raw bytes that were parsed,
// and their SHA-256. It is evidence of what was read at one point in time; it
// is not readiness and grants no execution authority.
type HandoffSource struct {
	// SourcePath is the cleaned, root-relative path actually read.
	SourcePath string
	// Bytes is the exact byte sequence parsed.
	Bytes []byte
	// SHA256 is the lowercase hex SHA-256 of Bytes. It is for drift
	// detection only (did these bytes change?) and is not a signature,
	// proof of origin, or authorization.
	SHA256  string
	Handoff Handoff
}

// LoadHandoff reads handoffPath strictly inside worktreeRoot under the same
// containment rules as BindGoal (relative, non-traversing, a regular file
// that is not a symlink and whose opened descriptor lies inside the resolved
// root, read from that same descriptor), then strictly parses it with Parse.
// It fails closed: every rejection is a plain error and no partial
// HandoffSource is ever returned.
func LoadHandoff(worktreeRoot, handoffPath string) (HandoffSource, error) {
	cleaned, err := cleanContainedRelPath(worktreeRoot, "handoffPath", handoffPath)
	if err != nil {
		return HandoffSource{}, fmt.Errorf("load handoff: %w", err)
	}
	data, err := readContainedRegularFile(worktreeRoot, cleaned, "handoff source")
	if err != nil {
		return HandoffSource{}, fmt.Errorf("load handoff: %w", err)
	}
	h, err := Parse(data)
	if err != nil {
		return HandoffSource{}, fmt.Errorf("load handoff: %w", err)
	}
	return HandoffSource{
		SourcePath: cleaned,
		Bytes:      data,
		SHA256:     sha256Hex(data),
		Handoff:    h,
	}, nil
}
