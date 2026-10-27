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
// of git or of the store. A completed review-exemption, the record that closes the exemption of a head, is held
// in the same way to the range and to the stored exemption (checkExemptionEvidence).
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
	if h.PayloadKind == review.PayloadExemption {
		return s.checkExemptionEvidence(dir, h)
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
	if err := review.CheckRange(diff, binding, "result"); err != nil {
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

// checkExemptionEvidence is CheckEvidence for a completed review-exemption, with the same order and the same
// guarantees: it reads and stores nothing else, and the shape of the record and of its refs is checked before the
// repository is located.
//
//   - the review-diff and the review-exemption are both needed, each given once;
//   - the ref of the review-exemption is exactly one stored exemption, review.ExemptionRef, and its sha256 is the
//     digest of the bytes stored now;
//   - the stored file must still be an exemption (review.DecodeExemption) that holds to the rules it was made under
//     (review.CheckExemption): a declared risk that names no high-risk item and a report that is clear, so that an
//     exemption that was edited after the record was made, even with the record's digest changed to match, is
//     refused for what it now says;
//   - the review-diff is the range of the binding stored with that exemption, and its sha256 the diff_sha256 of it.
//     The diff is not computed again here: whether the head being delivered is the one exempted is Check's.
//
// An exemption that does not hold is an *review.EvidenceError of the review-exemption that wraps the rule it broke.
func (s Service) checkExemptionEvidence(dir string, h roles.RoleHandoff) error {
	diff, err := review.RequiredEvidence(h, review.EvidenceDiff)
	if err != nil {
		return err
	}
	exemption, err := review.RequiredEvidence(h, review.EvidenceExemption)
	if err != nil {
		return err
	}
	feature, name, err := review.ExemptionRef(exemption)
	if err != nil {
		return err
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	file, err := s.readEvidence(repo, feature, name, exemption)
	if err != nil {
		return err
	}
	if err := review.CheckFile(exemption, file); err != nil {
		return err
	}
	stored, err := review.DecodeExemption(file)
	if err == nil {
		err = review.CheckExemption(stored)
	}
	if err != nil {
		return &review.EvidenceError{Kind: exemption.Kind, Ref: exemption.Ref, Err: err}
	}
	return review.CheckRange(diff, stored.Binding, "exemption")
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
