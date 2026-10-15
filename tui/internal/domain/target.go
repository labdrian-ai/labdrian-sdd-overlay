package domain

// TargetKind says how the backend deploys to a target. It is the backend's
// own classification (the second column of `labdrian-overlay targets`), not a
// guess made here.
type TargetKind string

const (
	// KindCopy targets receive individual files copied into a directory
	// (claude, opencode, codex today). Actions that work file by file, such as
	// capture and restore, only apply to these.
	KindCopy TargetKind = "copy"
	// KindPackage targets are built and installed as a whole package (pi
	// today); the backend refuses the file-by-file actions for them.
	KindPackage TargetKind = "package"
)

// Target is one deployment target, exactly as the backend's catalog lists it.
// The TUI keeps no list of targets of its own: every Target comes from a
// TargetCatalog.
type Target struct {
	Name string
	Kind TargetKind
}
