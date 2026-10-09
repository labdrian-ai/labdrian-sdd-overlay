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

// developmentVersion is the version of a package built where no version tag is reachable.
const developmentVersion = "0.0.0-dev"

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
		if couldNotAnswer(err) {
			return "", unanswered(err)
		}
		return "", nil // git answered that HEAD names nothing, or there is no repository: no commit to record
	}
	return rev, nil
}

// asked is the error of a yes-or-no question git could not answer, or the adapter refused to ask.
func asked(err error) error { return fmt.Errorf("pipkg: asking git: %w", err) }

// unanswered is the error to return for a value method whose failure couldNotAnswer: git was
// stopped or killed, or could not be started. It is always an error. The callers decide first
// whether a failure is that or the answer "there is none" (not under version control, no such
// ref, no tag), and only in the second case take their fallback value.
func unanswered(err error) error {
	return fmt.Errorf("pipkg: git could not answer: %w", err)
}

// isSourceDirty reports whether skills/, agents/, or skills.registry.yaml
// under overlayRoot have any uncommitted change, tracked or untracked
// (R3-001). A non-git overlayRoot or a git that answers with an error is reported as clean; a git
// that could not answer is an error.
func (p Packages) isSourceDirty(overlayRoot string) (bool, error) {
	dirty, err := p.Source.HasChanges(overlayRoot, sourceTreePaths...)
	if err != nil {
		if couldNotAnswer(err) {
			return false, unanswered(err)
		}
		return false, nil // no repository, or a status git refused: nothing known to be uncommitted
	}
	return dirty, nil
}

// resolvePackageVersion resolves the newest reachable "v*"-tag reachable
// from rev (D5: the same resolved commit labdrian.builtFrom records,
// "single source"), stripped of its leading "v", falling back to
// developmentVersion when no tag is reachable, rev is empty, or overlayRoot is not
// a git repo (this is a local, no-fetch lookup — never touches the
// network). The fallback is the answer to "there is no tag"; a git that could
// not answer returns the empty string and the error.
func (p Packages) resolvePackageVersion(overlayRoot, rev string) (string, error) {
	tag, err := p.Source.LatestTag(overlayRoot, rev, versionTagPattern)
	if err != nil {
		if couldNotAnswer(err) {
			return "", unanswered(err)
		}
		return developmentVersion, nil
	}
	return strings.TrimPrefix(tag, "v"), nil
}
