package review

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// Handoff is the skeleton of the RoleHandoff record that closes a review, for `labdrian roles append
// --stdin`: the members the command knows are filled, and the ones only the parent of the chain knows are
// left as an empty value of their type (ParentFields names them). A record with those left empty is
// refused by `roles validate`, which names the first one, so a skeleton is never appended by accident.
//
// What it fills: the version (1), the role that hands off (the reviewer), the kind of the payload
// (PayloadKind) and its digest, which is the digest of the stored file; two pieces of evidence, the range
// that was reviewed with the digest of its diff (EvidenceDiff) and the stored file with its digest
// (EvidenceResult), which are what the delivery recomputes to trust the record only on equality; the status
// (completed: a result that was stored is a review that ran; a review that did not run stores nothing and
// is recorded interrupted by the parent); and the arrays of the context as empty arrays. storedRef is the
// path of the stored file relative to the repository, and storedSHA256 the SHA-256 of its bytes.
func Handoff(c Candidate, storedRef, storedSHA256 string) roles.RoleHandoff {
	return roles.RoleHandoff{
		Version:       roles.HandoffVersion,
		FromRole:      roles.RoleReviewer,
		PayloadKind:   PayloadKind,
		PayloadSHA256: storedSHA256,
		Evidence: []roles.Evidence{
			{Kind: EvidenceDiff, Ref: c.Ref(), SHA256: c.DiffSHA256},
			{Kind: EvidenceResult, Ref: storedRef, SHA256: storedSHA256},
		},
		Context: roles.Context{Decisions: []string{}, OpenQuestions: []string{}},
		Status:  roles.StatusCompleted,
	}
}

// FileRef is a stored file as a record binds it: its path relative to the working tree, written with '/', and
// the SHA-256 of its bytes.
type FileRef struct {
	Ref    string
	SHA256 string
}

// VerifiedHandoff is the skeleton of the record that closes a review the verifier has run on, for the same
// `labdrian roles append --stdin`. It is the one Handoff makes for the range b binds and the stored result,
// with the stored verdict bound as one more piece of evidence (EvidenceVerifier), and with who is next filled
// from the outcome of the verdict: the builder when a finding blocks, the delivery when the review is clear.
// When a check did not run (reviewverdict.Unverified) it says nobody, and the parent fills to_role once the
// check has run: the review is not clear, and not blocked.
func VerifiedHandoff(b reviewresult.Binding, result, verdict FileRef, outcome reviewverdict.Outcome) roles.RoleHandoff {
	h := Handoff(Candidate{Base: b.Base, Head: b.Head, DiffSHA256: b.DiffSHA256}, result.Ref, result.SHA256)
	h.Evidence = append(h.Evidence, roles.Evidence{Kind: EvidenceVerifier, Ref: verdict.Ref, SHA256: verdict.SHA256})
	switch outcome {
	case reviewverdict.Blocks:
		h.ToRole = roles.RoleBuilder
	case reviewverdict.Clear:
		h.ToRole = roles.RoleDelivery
	}
	return h
}

// ParentFields are the members of the skeleton only the parent of the chain knows, by their names in the
// record: the identity of the chain it appends to (project_id, goal_id, chain_id), the place of the record
// in it (seq, prev_sha256), who gets the work next (to_role: builder when a severe finding blocks,
// delivery otherwise, per skills/role-chain/SKILL.md, which VerifiedHandoff fills once the verifier has
// run) and the narrative (context.summary). Every other member the record needs, Handoff has filled.
func ParentFields() []string {
	return []string{"project_id", "goal_id", "chain_id", "seq", "prev_sha256", "to_role", "context.summary"}
}
