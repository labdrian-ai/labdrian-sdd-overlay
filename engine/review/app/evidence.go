package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// CheckEvidence holds the evidence of h, a record of the role chain that is about to be appended, to the files
// the engine stored, in the repository that holds dir (absolute). A record that closes a review that ran (a
// completed review-report, review.NeedsEvidence) carries the range that was reviewed with the digest of its
// diff, the stored result with its digest and, when the verifier ran, the stored verdict with its digest. The
// use case reads those files and checks the three; any other record is accepted as it is and nothing is asked
// of git or of the store.
//
//   - the review-diff and the review-result are both needed, each given once;
//   - the ref of the review-result is exactly one stored result, review.ResultRef, and its sha256 is the
//     digest of the bytes stored now. The stored file must still be a result: one that is not, or whose reply no
//     longer holds to its binding, is refused as Verify refuses it;
//   - the review-diff is the range of the binding stored with that result, and its sha256 the diff_sha256 of it.
//     The diff is not computed again here: the check that the head being delivered is the one reviewed is
//     Check's;
//   - the verifier-report, when there is one, is the verdict stored beside that result, review.VerdictRef, and
//     its sha256 is the digest of the bytes stored now. Its content is not read again: Check decides from it.
//
// The shape of the record and of every ref is checked before the repository is located, so that a record that
// is wrong by its shape reads nothing. The files are read through the store, which refuses a path that leads
// outside the results directory, a link and a file that is not a regular one. A piece of evidence that does not
// hold is an *review.EvidenceError, which names its kind and its ref and wraps the rule it broke; the failure
// of git, or of the store before it can tell, is an error of the use case and not a piece of evidence that is
// wrong. Nothing is stored.
func (s Service) CheckEvidence(dir string, h roles.RoleHandoff) error {
	if !review.NeedsEvidence(h) {
		return nil
	}
	diff, err := review.RequiredEvidence(h, review.EvidenceDiff)
	if err != nil {
		return err
	}
	result, err := review.RequiredEvidence(h, review.EvidenceResult)
	if err != nil {
		return err
	}
	verdict, hasVerdict, err := review.OptionalEvidence(h, review.EvidenceVerifier)
	if err != nil {
		return err
	}
	feature, resultName, err := review.ResultRef(result)
	if err != nil {
		return err
	}
	var verdictName string
	if hasVerdict {
		if verdictName, err = review.VerdictRef(verdict, feature, resultName); err != nil {
			return err
		}
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	resultFile, err := s.readEvidence(repo, feature, resultName, result)
	if err != nil {
		return err
	}
	if err := review.CheckFile(result, resultFile); err != nil {
		return err
	}
	binding, _, err := judged(resultFile)
	if err != nil {
		return &review.EvidenceError{Kind: result.Kind, Ref: result.Ref, Err: err}
	}
	if err := review.CheckRange(diff, binding); err != nil {
		return err
	}
	if !hasVerdict {
		return nil
	}
	verdictFile, err := s.readEvidence(repo, feature, verdictName, verdict)
	if err != nil {
		return err
	}
	return review.CheckFile(verdict, verdictFile)
}

// readEvidence reads the stored file that e names, and makes nothing. A file that cannot be read is the
// evidence's: the error names its kind and its ref, and wraps what the store said.
func (s Service) readEvidence(repo Repository, feature, name string, e roles.Evidence) ([]byte, error) {
	data, err := s.results.Read(repo.Root, feature, name)
	if err != nil {
		return nil, &review.EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: fmt.Errorf("read the stored file: %w", err)}
	}
	return data, nil
}
