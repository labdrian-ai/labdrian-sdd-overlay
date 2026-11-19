package review_test

// The CI markers of the no-CI closure (labdrian-orchestrator-opencode T46, S3 L123): the closed list, root paths only.

import (
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// ciMarkerPaths is one path for each entry of the list of S3 L123: a file under each of the five directories and each
// of the ten files.
var ciMarkerPaths = []string{
	".github/workflows/ci.yml", ".gitea/workflows/ci.yml", ".forgejo/workflows/ci.yml", ".buildkite/pipeline.yml",
	".woodpecker/build.yml", ".gitlab-ci.yml", ".circleci/config.yml", ".travis.yml", "azure-pipelines.yml",
	"Jenkinsfile", "bitbucket-pipelines.yml", ".drone.yml", ".woodpecker.yml", "appveyor.yml", ".appveyor.yml",
}

func TestCIMarkersIsEachEntryOfTheClosedListAlone(t *testing.T) {
	for _, path := range ciMarkerPaths {
		t.Run(path, func(t *testing.T) {
			got := review.CIMarkers([]string{"README.md", path, "src/main.go"})

			if want := []string{path}; !reflect.DeepEqual(got, want) {
				t.Errorf("CIMarkers() = %q; want %q", got, want)
			}
		})
	}
}

func TestCIMarkersAreTheRootPathsOnlyInTreeOrder(t *testing.T) {
	paths := []string{
		"Jenkinsfile", "sub/.github/workflows/x.yml", "sub/Jenkinsfile", "sub/.gitlab-ci.yml", ".github/workflows", ".github/workflowsx/y.yml",
		".github/CODEOWNERS", ".buildkite", "Jenkinsfile.bak", ".github/workflows/deep/er/z.yml", "docs/.travis.yml", ".circleci/other.yml",
		".gitlab-ci.yml",
	}

	got := review.CIMarkers(paths)

	if want := []string{"Jenkinsfile", ".github/workflows/deep/er/z.yml", ".gitlab-ci.yml"}; !reflect.DeepEqual(got, want) {
		t.Errorf("CIMarkers() = %q; want %q", got, want)
	}
}

func TestCIMarkersOfNoMarkerIsNone(t *testing.T) {
	if got := review.CIMarkers([]string{"a.txt", "dir/b.txt"}); len(got) != 0 {
		t.Errorf("CIMarkers() = %q; want none", got)
	}
	if got := review.CIMarkers(nil); len(got) != 0 {
		t.Errorf("CIMarkers(nil) = %q; want none", got)
	}
}
