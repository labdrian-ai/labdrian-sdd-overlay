package main

// The evidence check of 'roles append' (own-rdd T4, S5). A record that closes a review carries the range that
// was reviewed with the digest of its diff, the stored result with its digest and, when the verifier ran, the
// stored verdict with its digest; 'roles append' stores it only when that evidence is the evidence of the files
// the two review verbs stored in the repository it runs in. The record that closes the exemption of a head
// carries the range and the stored exemption with its digest, and is held in the same way to the file 'review
// exempt' stored. The rules are review/app's CheckEvidence over the git of the machine and the results
// directory; this file wires them to the verb and says what is wrong in the words of the command.

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// evidenceCheck holds the evidence of a record to the files it names before 'roles append' stores the record.
// An error refuses the record, and its words are the ones the command prints.
type evidenceCheck func(h roles.RoleHandoff) error

// reviewEvidenceCheck is the check of the program over d: the repository is the one that holds the working
// directory of d, and the files are read from the results directory of its working tree. A record that is held
// to no evidence (any record but a completed review-report or review-exemption) is accepted before the working
// directory is asked for, so that such a record is stored wherever, and whatever the deps can say of it, as it
// always was.
//
// A piece of evidence that does not hold is an error that names its kind and its ref (review.EvidenceError). A
// check that could not run (no working directory, no repository, git or the store failing) is an error that
// says so and wraps the cause: a record whose evidence cannot be checked is not stored.
func reviewEvidenceCheck(d deps) evidenceCheck {
	return func(h roles.RoleHandoff) error {
		if !review.NeedsEvidence(h) {
			return nil
		}
		dir, err := reviewPath(d, "")
		if err == nil {
			err = newReviewService(d).CheckEvidence(dir, h)
		}
		var evidence *review.EvidenceError
		if err != nil && !errors.As(err, &evidence) {
			// h is a completed review-report or review-exemption: review.NeedsEvidence admits no other kind, so the
			// kind this names is never empty or unexpected.
			return fmt.Errorf("the evidence of the %s could not be checked: %w", h.PayloadKind, err)
		}
		return err
	}
}
