package app_test

// CheckNoCI (labdrian-orchestrator-opencode T46, S3 L123): the engine, not the foreman, establishes that a repository has no
// CI. The landing holds the head, the repository has no remote, and the tree of the landing commit holds no CI marker.

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// noCIWorld is a fake git whose landing (HEAD, headSHA) holds the delivered head (HEAD~1, baseSHA).
func noCIWorld() *fakeGit {
	g := newFakeGit()
	g.ancestors = map[string]bool{baseSHA: true}
	g.treeFiles = map[string][]string{headSHA: {"README.md", "src/main.go"}}
	return g
}

func TestCheckNoCIEstablishesNoCIFromTheLandingTheHeadAndTheRemotes(t *testing.T) {
	g := noCIWorld()

	got, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, "HEAD", "HEAD~1")

	if err != nil || got != (app.NoCI{Landing: headSHA, Head: baseSHA}) {
		t.Errorf("CheckNoCI() = %+v, %v; want the landing %s and the head %s, nil", got, err, headSHA, baseSHA)
	}
	want := []string{"Locate " + evidenceDir, "Commit HEAD", "Commit HEAD~1", "IsAncestor " + baseSHA + " " + headSHA, "TreeFiles " + headSHA, "Remotes"}
	if !slices.Equal(g.calls, want) {
		t.Errorf("git calls %q; want %q (the commits, never the working tree)", g.calls, want)
	}
}

func TestCheckNoCIRefusesALandingThatDoesNotHoldTheHeadAndReadsNothingMore(t *testing.T) {
	g := noCIWorld()
	g.ancestors = nil

	_, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, "HEAD", "HEAD~1")

	if !errors.Is(err, review.ErrHeadNotLanded) || err == nil || !strings.Contains(err.Error(), baseSHA) || !strings.Contains(err.Error(), headSHA) {
		t.Fatalf("CheckNoCI() error = %v; want ErrHeadNotLanded naming both commits", err)
	}
	if g.called("TreeFiles") != 0 || g.called("Remotes") != 0 {
		t.Errorf("git calls %q; want no tree and no remotes read for a head the landing does not hold", g.calls)
	}
}

func TestCheckNoCIRefusesARemoteAndNamesIt(t *testing.T) {
	g := noCIWorld()
	g.remotes = []string{"origin", "backup"}

	_, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, "HEAD", "HEAD~1")

	want := "the repository has remotes: origin, backup; the no-CI closure does not apply"
	if !errors.Is(err, review.ErrCINotEstablished) || err == nil || err.Error() != want {
		t.Errorf("CheckNoCI() error = %v; want ErrCINotEstablished with %q", err, want)
	}
}

func TestCheckNoCIRefusesMarkersOfTheLandingTreeInTreeOrderAndNamesThem(t *testing.T) {
	g := noCIWorld()
	g.treeFiles[headSHA] = []string{".github/workflows/ci.yml", "README.md", "Jenkinsfile"}

	_, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, "HEAD", "HEAD~1")

	want := "the tree of the landing " + headSHA + " holds CI markers: .github/workflows/ci.yml, Jenkinsfile; the no-CI closure does not apply"
	if !errors.Is(err, review.ErrCINotEstablished) || err == nil || err.Error() != want {
		t.Errorf("CheckNoCI() error = %v; want ErrCINotEstablished with %q", err, want)
	}
}

func TestCheckNoCINamesTheMarkersAndTheRemotesTogether(t *testing.T) {
	g := noCIWorld()
	g.treeFiles[headSHA] = []string{".travis.yml"}
	g.remotes = []string{"origin"}

	_, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, "HEAD", "HEAD~1")

	want := "the tree of the landing " + headSHA + " holds CI markers: .travis.yml; the repository has remotes: origin; the no-CI closure does not apply"
	if err == nil || err.Error() != want {
		t.Errorf("CheckNoCI() error = %v; want %q", err, want)
	}
}

func TestCheckNoCIIsNeverNoneConfiguredWhenGitCannotAnswer(t *testing.T) {
	for _, call := range []string{"Locate", "Commit HEAD", "Commit HEAD~1", "IsAncestor", "TreeFiles", "Remotes"} {
		t.Run(call, func(t *testing.T) {
			g := noCIWorld()
			g.fail[call] = errors.New("git is broken")

			got, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, "HEAD", "HEAD~1")

			if err == nil || !strings.Contains(err.Error(), "git is broken") || got != (app.NoCI{}) {
				t.Errorf("CheckNoCI() = %+v, %v; want no answer and an error that says why", got, err)
			}
			if errors.Is(err, review.ErrCINotEstablished) || errors.Is(err, review.ErrHeadNotLanded) {
				t.Errorf("CheckNoCI() error %v is a refusal; want a failure of git", err)
			}
		})
	}
}

func TestCheckNoCIRefusesARevisionThatReadsAsAnOption(t *testing.T) {
	g := noCIWorld()

	for _, args := range [][2]string{{"--all", "HEAD~1"}, {"HEAD", "-x"}, {"", "HEAD~1"}} {
		if _, err := app.New(g, newFakeResults()).CheckNoCI(evidenceDir, args[0], args[1]); !errors.Is(err, review.ErrInvalidRev) {
			t.Errorf("CheckNoCI(%q, %q) error = %v; want ErrInvalidRev", args[0], args[1], err)
		}
	}
	if len(g.calls) != 0 {
		t.Errorf("git calls %q; want none before the revisions are checked", g.calls)
	}
}
