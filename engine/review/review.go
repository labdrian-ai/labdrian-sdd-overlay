// Package review is the vocabulary of `labdrian review bind` and `labdrian review record`: what a review
// binds, where its result is stored, and what the record of the role chain that closes it holds. The
// reviewer role (roles/reviewer/body.md) returns one JSON reply about one candidate; the rules a reply is
// held to are reviewresult's, and this package is the candidate they are held to, with the names of the
// files and of the worktree that belong to it.
//
// A Candidate is two commits and the diff between them. It names the commits by their full object names,
// the diff by the SHA-256 and the size of its bytes, and the changed paths one by one. Which bytes those
// are is the contract of the Git port of review/app, whose adapter is review/gitexec; the README of
// odd/review-results states it in full, so that a later command, or any other program, recomputes the
// same digest for the same pair of commits. The candidate is computed from git by the command and never
// taken from a reply, so a reply that echoes anything else is refused (reviewresult.ErrCandidateMismatch).
//
// Names, all of them pure functions of a Candidate:
//
//   - the stored result of a launch is odd/review-results/<feature>/<head12>-<n>.json, where <head12>
//     is the first 12 characters of the head and <n> counts the launches stored for that head, from 1;
//   - the detached worktree a reviewer reads is <repo-parent>/<repo-name>-worktrees/review-<head12>, beside
//     the main checkout of the repository and never inside it;
//   - the feature is a single safe path segment, and nothing a command line gives reaches a path through
//     anything else: a revision that git could read as an option is refused (CheckRev), and a feature that is
//     more than one plain segment is refused (CheckFeature).
//
// The stored file is the reply with the binding it was validated against (EncodeResult, read back by
// DecodeResult), and the record of the role chain that closes the review is a RoleHandoff skeleton (Handoff)
// that holds every member the command knows and leaves empty the ones only the parent of the chain knows
// (ParentFields).
//
// The lens rows the reviewer is sent (own-rdd T5, S8) are written once, as sentences, in roles/reviewer/rows.json,
// and the parent of the chain copies the ones a launch needs into the prompt of the reviewer. This package holds the
// ids of those rows (RowIDs) and the grammar in which a launch declares the ones it was sent (ParseRows). The stored
// result records them (EncodeResult, DecodeResult), so the number of lenses a review ran is counted from the stored
// results of the change and not from the receipt of another tool.
//
// When the verifier ran (own-rdd T3), its verdict on the findings of a stored result is stored beside the
// result with the same <head12>-<n>, as <head12>-<n>.verifier.json (VerdictName, ResultOfVerdict), and the
// skeleton binds it as one more piece of evidence and says who is next (VerifiedHandoff). A review is
// corrected at most once: IsCorrection says which candidate is the correction of which.
//
// The stored files are what the rest of the chain is held to (own-rdd T4): the record that closes a review
// carries evidence that must be the evidence of the files stored (NeedsEvidence, ResultRef, VerdictRef,
// CheckFile, CheckRange), and a head is delivered only when the results stored for it still bind it and clear
// it (ResultsOfHead finds them, CheckBinding holds each stored binding to the candidate git gives now, and
// Clearance decides).
//
// A head whose work takes no review may be exempted from it instead (own-rdd T4, S5, L15-L16). The exemption is
// stored beside the results as <head12>-<n>.exempt.json (ExemptionName, ExemptionsOfHead), numbered with them,
// and holds the binding, the risk the agent declared and the report of the verifier (EncodeExemption, read back
// by DecodeExemption). CheckRisk refuses a declared risk that names an item of the high-risk list, CheckReport a
// report that is not clear, CheckExemption holds a stored exemption to both again, and Cleared lets a head
// through on an exemption only when no stored result binds it, so a result that blocks always wins. The record
// that closes it is ExemptionHandoff, and the ref of its evidence names a stored exemption as the ref of a
// review-report's names a stored result (ExemptionRef, CheckExemptionName).
//
// It is a domain package: pure standard library and the domain packages that say what a git object name,
// a SHA-256 digest, a reviewer reply, a verifier verdict and a role handoff are. It reads no file and runs
// no git.
package review

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitobject"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

const (
	// ResultsDir is where the results are stored, relative to the root of the working tree of the
	// repository, in a directory per feature.
	ResultsDir = "odd/review-results"

	// DiffLimitBytes is the size above which a diff is not sent to one reviewer whole: the planning rule
	// of skills/role-chain/SKILL.md is "up to 150 KB, else one launch per commit". It is 150,000 bytes,
	// so that a diff within it is within 150 KiB as well. A diff above it is still bound: the binding says
	// it is over the limit (Candidate.OverLimit) and the parent splits the candidate.
	DiffLimitBytes = 150000

	// MaxReplyBytes bounds the reply of a reviewer that `review record` reads, and the one of a verifier that
	// `review verify` reads: a reply is one JSON object of findings or verdicts, a few kilobytes, and 1 MiB
	// is far above any real one. The command reads one byte past it to know a reply is too large and never
	// the rest.
	MaxReplyBytes = 1 << 20

	// MaxStoredBytes bounds a stored file that a command reads back, a result or a verdict: a reply of at
	// most MaxReplyBytes with the binding of a candidate whose paths are listed one by one. The store reads
	// one byte past it, so a file that is not one of ours is never read whole.
	MaxStoredBytes = 8 << 20

	// StoredVersion is the version of the stored file (EncodeResult).
	StoredVersion = 1

	// PayloadKind is the payload_kind of the record of the role chain that closes a review.
	PayloadKind = "review-report"
	// EvidenceDiff, EvidenceResult and EvidenceVerifier are the kinds of the evidence that binds the record
	// to the range it reviewed, to the stored bytes of the result and, when the verifier ran, to the stored
	// bytes of its verdict.
	EvidenceDiff     = "review-diff"
	EvidenceResult   = "review-result"
	EvidenceVerifier = "verifier-report"

	// PayloadExemption is the payload_kind of the record that closes the exemption of a head from review (the
	// one spelling is roles.PayloadExemption, which the record's validation reads), and EvidenceExemption the
	// kind of the evidence that binds that record to the stored bytes of the exemption.
	PayloadExemption  = roles.PayloadExemption
	EvidenceExemption = "review-exemption"
)

// The reasons the functions of this package refuse. An error they return wraps exactly one of them, so a
// caller tells them apart with errors.Is and reads the detail in the message.
var (
	// ErrInvalidFeature is a feature that is not a single safe path segment.
	ErrInvalidFeature = errors.New("review: invalid feature name")
	// ErrInvalidRev is a revision that is empty, that git would read as an option, or that holds a control
	// character.
	ErrInvalidRev = errors.New("review: invalid revision")
	// ErrInvalidCandidate is a candidate that names nothing: commits that are not full object names of one
	// length, the same commit as base and as head, or paths that cannot be bound one by one.
	ErrInvalidCandidate = errors.New("review: invalid candidate")
	// ErrReplyTooLarge is a reply of the reviewer above MaxReplyBytes.
	ErrReplyTooLarge = errors.New("review: reply too large")
	// ErrReplyNotJSON is a reply that is not one JSON value, which a JSON file cannot hold.
	ErrReplyNotJSON = errors.New("review: reply is not JSON")
	// ErrInvalidResultName is a name that is not the name of a stored result, <head12>-<n>.json.
	ErrInvalidResultName = errors.New("review: invalid result name")
	// ErrInvalidExemptionName is a name that is not the name of a stored exemption, <head12>-<n>.exempt.json.
	ErrInvalidExemptionName = errors.New("review: invalid exemption name")
	// ErrInvalidStored is a stored file that is not one EncodeResult writes.
	ErrInvalidStored = errors.New("review: invalid stored file")
	// ErrInvalidRows is a declaration of the lens rows a launch was sent that is not a list of the rows of
	// roles/reviewer/rows.json (ParseRows): an empty one, an id nobody wrote, a row written twice.
	ErrInvalidRows = errors.New("review: invalid rows")
)

// Candidate is what a review is about, as computed from git: the zero Candidate names nothing.
type Candidate struct {
	// Base and Head are the full object names of the two commits of the diff.
	Base string
	Head string
	// DiffSHA256 is the SHA-256 of the bytes of the diff, as 64 lowercase hexadecimal characters, and
	// DiffBytes is how many bytes those are.
	DiffSHA256 string
	DiffBytes  int
	// Paths are the paths the diff changes, once each, in the order git prints them.
	Paths []string
}

// NewCandidate binds the diff of two commits. base and head are full object names of one length (40
// characters in a SHA-1 repository, 64 in a SHA-256 one) and are two commits: a range whose base is its head
// covers no commit, so nothing reviewed or exempted over it clears anything. diff is the bytes of the diff and
// paths the paths it changes. Every path is a non-empty string of valid UTF-8, because the binding is JSON and a
// path that JSON cannot carry byte for byte could never be matched by a reviewer; no path is listed twice.
// It returns the zero Candidate and an error that wraps ErrInvalidCandidate when it cannot.
func NewCandidate(base, head string, diff []byte, paths []string) (Candidate, error) {
	if !gitobject.IsName(base) {
		return Candidate{}, fmt.Errorf("%w: base %q is not a full object name (40 or 64 lowercase hexadecimal characters)", ErrInvalidCandidate, base)
	}
	if !gitobject.IsName(head) {
		return Candidate{}, fmt.Errorf("%w: head %q is not a full object name (40 or 64 lowercase hexadecimal characters)", ErrInvalidCandidate, head)
	}
	if len(base) != len(head) {
		return Candidate{}, fmt.Errorf("%w: base has %d characters and head %d, want the same length", ErrInvalidCandidate, len(base), len(head))
	}
	if base == head {
		return Candidate{}, fmt.Errorf("%w: base and head are the same commit %s, so the range covers no commit and clears nothing", ErrInvalidCandidate, head)
	}
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		switch {
		case p == "":
			return Candidate{}, fmt.Errorf("%w: the paths hold an empty path", ErrInvalidCandidate)
		case !utf8.ValidString(p):
			return Candidate{}, fmt.Errorf("%w: path %q is not valid UTF-8, so a JSON binding cannot name it", ErrInvalidCandidate, p)
		case seen[p]:
			return Candidate{}, fmt.Errorf("%w: path %q is listed more than once", ErrInvalidCandidate, p)
		}
		seen[p] = true
	}
	return Candidate{
		Base:       base,
		Head:       head,
		DiffSHA256: sha256hex.Sum(diff),
		DiffBytes:  len(diff),
		Paths:      append([]string{}, paths...),
	}, nil
}

// Binding is the candidate as reviewresult holds a reply to it: the four values the parent computes and the
// reviewer copies or lists.
func (c Candidate) Binding() reviewresult.Binding {
	return reviewresult.Binding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Paths: c.Paths}
}

// Ref is the range as the record of the role chain names it, <base>..<head>.
func (c Candidate) Ref() string { return c.Base + ".." + c.Head }

// shortLength is how many characters of the head name its files: 12 hexadecimal characters, which git
// itself still prints unambiguously in large repositories.
const shortLength = 12

// Short is the first 12 characters of the head: the name of the stored results and of the worktree of
// the candidate. The zero Candidate names nothing, so its short name is empty.
func (c Candidate) Short() string {
	if len(c.Head) < shortLength {
		return ""
	}
	return c.Head[:shortLength]
}

// OverLimit reports whether the diff is larger than DiffLimitBytes.
func (c Candidate) OverLimit() bool { return c.DiffBytes > DiffLimitBytes }
