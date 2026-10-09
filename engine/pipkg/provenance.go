package pipkg

import "strings"

// sourceTreePaths are the parts of the overlay that a build copies from the working tree: a
// change to any of them makes the recorded commit a claim the package does not keep.
var sourceTreePaths = []string{"skills", "agents", "skills.registry.yaml"}

// versionTagPattern is the pattern of the tags that name a version of the overlay.
const versionTagPattern = "v*"

// resolveBuildRev resolves overlayRoot's checked-out HEAD commit SHA
// (R-005), returning "" when overlayRoot is not a git repo, HEAD cannot be
// resolved, or the source tree is dirty (R3-001: files Build is about to
// copy would not match a recorded HEAD). Local, no-fetch lookup.
func (p Packages) resolveBuildRev(overlayRoot string) string {
	if p.isSourceDirty(overlayRoot) {
		return ""
	}
	rev, err := p.Source.Resolve(overlayRoot, "HEAD")
	if err != nil {
		return ""
	}
	return rev
}

// isSourceDirty reports whether skills/, agents/, or skills.registry.yaml
// under overlayRoot have any uncommitted change, tracked or untracked
// (R3-001). A non-git overlayRoot or any git error is reported as clean.
func (p Packages) isSourceDirty(overlayRoot string) bool {
	dirty, err := p.Source.HasChanges(overlayRoot, sourceTreePaths...)
	return err == nil && dirty
}

// resolvePackageVersion resolves the newest reachable "v*"-tag reachable
// from rev (D5: the same resolved commit labdrian.builtFrom records,
// "single source"), stripped of its leading "v", falling back to
// "0.0.0-dev" when no tag is reachable, rev is empty, or overlayRoot is not
// a git repo (this is a local, no-fetch lookup — never touches the
// network).
func (p Packages) resolvePackageVersion(overlayRoot, rev string) string {
	tag, err := p.Source.LatestTag(overlayRoot, rev, versionTagPattern)
	if err != nil {
		return "0.0.0-dev"
	}
	return strings.TrimPrefix(tag, "v")
}
