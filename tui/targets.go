package main

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

// sameTargetNames reports whether a and b name exactly the same targets, in
// any order. It is the test for "this selection is the whole catalog": the
// only case in which `--target all` is allowed to stand for the selection. A
// name repeated on either side makes the two unequal, since neither a catalog
// nor a selection of catalog entries ever repeats one.
func sameTargetNames(a, b []Target) bool {
	if len(a) != len(b) {
		return false
	}
	pending := make(map[string]bool, len(a))
	for _, t := range a {
		pending[t.Name] = true
	}
	for _, t := range b {
		if !pending[t.Name] {
			return false
		}
		delete(pending, t.Name)
	}
	return len(pending) == 0
}
