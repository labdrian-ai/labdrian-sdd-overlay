package app

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// commitFacts reads what review.Needs asks about a commit, a full object name (own-rdd T6b5, S6 d): the Risk trailer of
// its message, whether it is passive, whether it is a sweep commit, and the latest sweep record, the sweeper's answer and
// the polisher record of the feature that hold for it. names is the listing of the directory of the feature. It also
// returns the commit's first parent, "" for a commit that has none.
//
// A role record is held to git again as a reader holds a result (heldRole), and one that cannot be read, is not that
// kind of record, binds another diff or is not for one commit is an error naming its path, never a record passed over.
// A passive commit is read no further than its paths: it needs no record. While the trailer of a commit fails, a role
// record that does not read back or hold counts as absent and its error is dropped (lenient): review.Needs lists the
// failing trailer first and stops, so no pass follows, and the record's own refusal is met once the trailer holds.
// Nothing is stored.
func (s Service) commitFacts(repo Repository, feature string, names []string, commit string) (review.Facts, string, error) {
	parent, err := s.git.Parent(repo, commit)
	if err != nil {
		return review.Facts{}, "", fmt.Errorf("find the first parent of %s: %w", commit, err)
	}
	message, err := s.git.Message(repo, commit)
	if err != nil {
		return review.Facts{}, "", fmt.Errorf("read the message of %s: %w", commit, err)
	}
	trailers, err := s.git.Trailers(repo, message)
	if err != nil {
		return review.Facts{}, "", fmt.Errorf("read the trailers of %s: %w", commit, err)
	}
	risk, riskErr := review.DeclaredRisk(commit, message, trailers)
	facts := review.Facts{Commit: commit, Risk: risk, RiskErr: riskErr}
	if riskErr == nil && len(risk) == 0 {
		if parent == "" {
			return review.Facts{}, "", fmt.Errorf("the commit %s has no parent, so what it changes cannot be told", commit)
		}
		paths, err := s.git.Paths(repo, parent, commit)
		if err != nil {
			return review.Facts{}, "", fmt.Errorf("list the paths of %s..%s: %w", parent, commit, err)
		}
		removed, err := s.git.DeletedOrRenamed(repo, parent, commit)
		if err != nil {
			return review.Facts{}, "", fmt.Errorf("list the paths %s deletes or renames: %w", commit, err)
		}
		if facts.Passive = review.IsPassive(risk, riskErr, paths, removed); facts.Passive {
			return facts, parent, nil
		}
	}
	subject, _, _ := strings.Cut(message, "\n")
	if parent != "" && review.CheckSweepCommit(parent, commit, parent, subject) == nil {
		if facts.SweepCommit, err = s.namedBySweeper(repo, feature, names, parent, commit, riskErr != nil); err != nil {
			return review.Facts{}, "", err
		}
	}
	if err := s.readRoles(repo, feature, names, commit, &facts, riskErr != nil); err != nil {
		return review.Facts{}, "", err
	}
	return facts, parent, nil
}

// readRoles fills in what the records of the roles say about commit: the latest sweep that holds and whether it fired,
// whether a sweeper record that holds answers it, and whether a polisher record that holds binds the commit when its
// trailer names an item. A record that does not read back or hold counts as absent when lenient is true.
func (s Service) readRoles(repo Repository, feature string, names []string, commit string, facts *review.Facts, lenient bool) error {
	var latest string
	for _, name := range review.SweepsOfHead(names, commit) {
		var stored review.StoredSweep
		binds, err := s.heldRole(repo, feature, name, commit, func(file []byte) (review.RecordBinding, error) {
			var err error
			stored, err = review.DecodeSweep(file)
			return stored.Binding, err
		})
		if err != nil {
			if lenient {
				continue
			}
			return err
		}
		if binds {
			latest, facts.SweepFired = name, stored.Fired()
		}
	}
	if len(facts.Risk) > 0 {
		polished, err := s.polisherRecorded(repo, feature, names, commit, lenient)
		if err != nil {
			return err
		}
		facts.Polished = polished
	}
	if latest == "" {
		return nil
	}
	facts.Sweep = review.ResultPath(feature, latest)
	if facts.SweepFired {
		for _, name := range review.SweepersOfHead(names, commit) {
			var stored review.StoredSweeper
			binds, err := s.heldRole(repo, feature, name, commit, func(file []byte) (review.RecordBinding, error) {
				var err error
				stored, err = review.DecodeSweeper(file)
				return stored.Binding, err
			})
			if err != nil {
				if lenient {
					continue
				}
				return err
			}
			if binds && stored.Sweep == latest {
				facts.Answered = true
			}
		}
	}
	return nil
}

// polisherRecorded says whether a polisher record that holds binds commit. It is asked only of a commit whose trailer
// names an item. A record that does not read back or hold counts as absent when lenient is true.
func (s Service) polisherRecorded(repo Repository, feature string, names []string, commit string, lenient bool) (bool, error) {
	found := false
	for _, name := range review.PolishersOfHead(names, commit) {
		binds, err := s.heldRole(repo, feature, name, commit, func(file []byte) (review.RecordBinding, error) {
			stored, err := review.DecodePolisher(file)
			return stored.Binding, err
		})
		if err != nil {
			if lenient {
				continue
			}
			return false, err
		}
		found = found || binds
	}
	return found, nil
}

// namedBySweeper says whether a sweeper record of parent, the first parent of commit, names commit as the commit of its
// change: the record that makes commit a sweep commit. A record that does not read back or hold counts as absent when
// lenient is true.
func (s Service) namedBySweeper(repo Repository, feature string, names []string, parent, commit string, lenient bool) (bool, error) {
	named := false
	for _, name := range review.SweepersOfHead(names, parent) {
		var stored review.StoredSweeper
		binds, err := s.heldRole(repo, feature, name, parent, func(file []byte) (review.RecordBinding, error) {
			var err error
			stored, err = review.DecodeSweeper(file)
			return stored.Binding, err
		})
		if err != nil {
			if lenient {
				continue
			}
			return false, err
		}
		if binds && stored.Report.Commit != nil && *stored.Report.Commit == commit {
			named = true
		}
	}
	return named, nil
}

// heldRole reads the record of a role called name and holds it to git again, as reviewedResult holds a result: decode
// reads the file and gives its binding, which must name head, a full object name, to count, and then must equal the
// candidate git gives now for the commits it names (review.CheckRecordBinding) and be for one commit, the base being the
// first parent of head (review.CheckOneCommit). It reports false for a record that is about another commit than head,
// which only the first 12 characters of the name say it is not.
func (s Service) heldRole(repo Repository, feature, name, head string, decode func([]byte) (review.RecordBinding, error)) (bool, error) {
	path := review.ResultPath(feature, name)
	file, err := s.results.Read(repo.Root, feature, name)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	binding, err := decode(file)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if binding.Head != head {
		return false, nil
	}
	candidate, err := s.bind(repo, binding.Base, head)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if err := review.CheckRecordBinding(binding, candidate); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if err := s.oneCommit(repo, candidate.Base, candidate.Head); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return true, nil
}
