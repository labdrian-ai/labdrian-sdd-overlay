package pipkg

import (
	"fmt"
	"strings"
)

// sourceTreePaths are the parts of the overlay that a build copies from the working tree: a
// change to any of them makes the recorded commit a claim the package does not keep.
var sourceTreePaths = []string{"skills", "agents", "skills.registry.yaml"}

// versionTagPattern is the pattern of the tags that name a version of the overlay.
const versionTagPattern = "v*"

// resolveBuildRev resolves overlayRoot's checked-out HEAD commit SHA
// (R-005), returning "" when overlayRoot is not a git repo, HEAD cannot be
// resolved, or the source tree is dirty (R3-001: files Build is about to
// copy would not match a recorded HEAD). Local, no-fetch lookup.
func (p Packages) resolveBuildRev(overlayRoot string) (string, error) {
	dirty, err := p.isSourceDirty(overlayRoot)
	if err != nil || dirty {
		return "", err
	}
	rev, err := p.Source.Resolve(overlayRoot, "HEAD")
	if err != nil {
		return "", surfaced(err)
	}
	return rev, nil
}

// asked is the error of a yes-or-no question git could not answer, or the adapter refused to ask.
func asked(err error) error { return fmt.Errorf("pipkg: asking git: %w", err) }

// surfaced is err when git could not answer (a deadline, a signal, a ref it refused to read), and
// nil when the failure is an answer the builder reads as "there is none": the tree is not under
// version control, the ref is not there.
func surfaced(err error) error {
	if couldNotAnswer(err) {
		return fmt.Errorf("pipkg: git could not answer: %w", err)
	}
	return nil
}

// isSourceDirty reports whether skills/, agents/, or skills.registry.yaml
// under overlayRoot have any uncommitted change, tracked or untracked
// (R3-001). A non-git overlayRoot or a git that answers with an error is reported as clean; a git
// that could not answer is an error.
func (p Packages) isSourceDirty(overlayRoot string) (bool, error) {
	dirty, err := p.Source.HasChanges(overlayRoot, sourceTreePaths...)
	if err != nil {
		return false, surfaced(err)
	}
	return dirty, nil
}

// resolvePackageVersion resolves the newest reachable "v*"-tag reachable
// from rev (D5: the same resolved commit labdrian.builtFrom records,
// "single source"), stripped of its leading "v", falling back to
// "0.0.0-dev" when no tag is reachable, rev is empty, or overlayRoot is not
// a git repo (this is a local, no-fetch lookup — never touches the
// network).
func (p Packages) resolvePackageVersion(overlayRoot, rev string) (string, error) {
	tag, err := p.Source.LatestTag(overlayRoot, rev, versionTagPattern)
	if err != nil {
		return "0.0.0-dev", surfaced(err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}
