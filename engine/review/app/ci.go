package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// NoCI is the answer of CheckNoCI: the landing and the delivered head, as full object names, for which no CI is configured.
type NoCI struct {
	Landing, Head string
}

// CheckNoCI establishes that the repository that holds dir (absolute) has no CI for the landing commit that holds the
// delivered head (labdrian-orchestrator-opencode T46, S3 L123). All three must hold: the landing holds the head; `git remote` lists
// no remote, because a hosted CI runs only on commits pushed to one; and the tree of the landing commit, never the working
// tree, holds no CI marker (review.CIMarkers). landing and head are revisions as a person types them. Any other case is a
// refusal that names what was found (review.ErrHeadNotLanded, review.ErrCINotEstablished), and a revision git cannot
// resolve or a git that cannot answer is an error: never an answer of no CI. Nothing is stored.
func (s Service) CheckNoCI(dir, landing, head string) (NoCI, error) {
	for _, rev := range []string{landing, head} {
		if err := review.CheckRev(rev); err != nil {
			return NoCI{}, err
		}
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return NoCI{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	landingName, err := s.git.Commit(repo, landing)
	if err != nil {
		return NoCI{}, fmt.Errorf("resolve landing %q: %w", landing, err)
	}
	headName, err := s.git.Commit(repo, head)
	if err != nil {
		return NoCI{}, fmt.Errorf("resolve head %q: %w", head, err)
	}
	holds, err := s.git.IsAncestor(repo, headName, landingName)
	if err != nil {
		return NoCI{}, fmt.Errorf("tell whether the landing %s holds the head %s: %w", landingName, headName, err)
	}
	if !holds {
		return NoCI{}, review.HeadNotLanded(landingName, headName)
	}
	files, err := s.git.TreeFiles(repo, landingName)
	if err != nil {
		return NoCI{}, fmt.Errorf("list the tree of the landing %s: %w", landingName, err)
	}
	remotes, err := s.git.Remotes(repo)
	if err != nil {
		return NoCI{}, fmt.Errorf("list the remotes of the repository: %w", err)
	}
	markers := review.CIMarkers(files)
	if len(markers) > 0 || len(remotes) > 0 {
		return NoCI{}, review.CINotEstablished(landingName, markers, remotes)
	}
	return NoCI{Landing: landingName, Head: headName}, nil
}
