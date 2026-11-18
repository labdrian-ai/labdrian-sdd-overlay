package review

import (
	"errors"
	"strings"
)

// The CI markers and the refusals of `review ci` (labdrian-orchestrator-opencode T46, S3 L123): the engine, not the foreman,
// establishes that a repository has no CI. The list is closed: a provider outside it is added here, with a test, when it
// appears.
var (
	// ErrHeadNotLanded is a landing commit that does not hold the delivered head.
	ErrHeadNotLanded = errors.New("review: the landing does not hold the head")
	// ErrCINotEstablished is a repository with a remote or a CI marker in the tree of the landing: a CI may exist.
	ErrCINotEstablished = errors.New("review: no CI is established for the repository")
)

// ciMarkerDirs are the directories whose every file, at any depth, is a CI marker, matched as a prefix of a path from the
// root of the tree.
var ciMarkerDirs = []string{".github/workflows/", ".gitea/workflows/", ".forgejo/workflows/", ".buildkite/", ".woodpecker/"}

// ciMarkerFiles are the files that are CI markers, matched by their whole path from the root of the tree.
var ciMarkerFiles = []string{
	".gitlab-ci.yml", ".circleci/config.yml", ".travis.yml", "azure-pipelines.yml", "Jenkinsfile", "bitbucket-pipelines.yml",
	".drone.yml", ".woodpecker.yml", "appveyor.yml", ".appveyor.yml",
}

// CIMarkers returns the paths of a tree listing that are CI markers, in the order of the listing. The paths are from the
// root of the tree, written with '/': `sub/.github/workflows/x.yml` is not a marker. No marker is none.
func CIMarkers(paths []string) []string {
	var found []string
	for _, path := range paths {
		if isCIMarker(path) {
			found = append(found, path)
		}
	}
	return found
}

func isCIMarker(path string) bool {
	for _, dir := range ciMarkerDirs {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	for _, file := range ciMarkerFiles {
		if path == file {
			return true
		}
	}
	return false
}

// ciRefusal is a refusal whose text is the sentence the verb prints and that wraps its sentinel.
type ciRefusal struct {
	text     string
	sentinel error
}

func (e *ciRefusal) Error() string { return e.text }
func (e *ciRefusal) Unwrap() error { return e.sentinel }

const ciDoesNotApply = "the no-CI closure does not apply"

// HeadNotLanded is the refusal of a landing that does not hold the head.
func HeadNotLanded(landing, head string) error {
	return &ciRefusal{text: "the landing " + landing + " does not hold the head " + head + "; " + ciDoesNotApply, sentinel: ErrHeadNotLanded}
}

// CINotEstablished is the refusal of a repository that has the markers or the remotes given, at least one of them: one
// sentence that names what was found and says that the no-CI closure does not apply.
func CINotEstablished(landing string, markers, remotes []string) error {
	var found []string
	if len(markers) > 0 {
		found = append(found, "the tree of the landing "+landing+" holds CI markers: "+strings.Join(markers, ", "))
	}
	if len(remotes) > 0 {
		found = append(found, "the repository has remotes: "+strings.Join(remotes, ", "))
	}
	return &ciRefusal{text: strings.Join(append(found, ciDoesNotApply), "; "), sentinel: ErrCINotEstablished}
}
