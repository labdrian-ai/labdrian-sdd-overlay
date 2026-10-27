package review

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// This file is what the delivery of a head is held to (own-rdd T4, S5): that the head was reviewed, by results
// the engine stored that still hold, and that none of them keeps it from delivery, or that it was exempted from
// review by an exemption the engine stored that still holds and no result blocks it. The use case that reads the
// files and asks git is review/app's Check; the rules here work on the names and the values it hands over.

// The reasons a head is refused. A refusal wraps exactly one of them, so a caller tells them apart with
// errors.Is and reads the result and the findings in the message.
var (
	// ErrBindingDiffers is a stored result whose binding is not the candidate git gives now for the commits it
	// names: the diff, and so its digest, is not the one the reviewer read.
	ErrBindingDiffers = errors.New("review: the stored binding differs from the candidate git gives")
	// ErrNoResult is a head that no stored result binds: it was not reviewed.
	ErrNoResult = errors.New("review: no stored result binds the head")
	// ErrBlocked is a head with a stored result that blocks: a severe finding the diff caused was reproduced.
	ErrBlocked = errors.New("review: a stored result blocks the head")
	// ErrNotClear is a head with a stored result that does not block and is not clear either: a finding that
	// could block was never checked, or nothing decided the outcome. A check that did not run never passes.
	ErrNotClear = errors.New("review: a stored result is not clear")
	// ErrChainLoops is a chain of stored ranges that leads back to a commit it already passed, so it never reaches
	// the base of the delivery.
	ErrChainLoops = errors.New("review: the stored ranges lead back to a commit the chain already passed")
)

// Reviewed is a stored result that binds a head, read back together with what it makes of the review.
type Reviewed struct {
	// Candidate is the candidate git gives now for the commits the result names; CheckBinding has held the
	// stored binding to it.
	Candidate Candidate
	// Result is the path of the stored result, relative to the root of the working tree and written with '/'.
	Result string
	// Verdict is the path of the verdict stored beside it, and "" when the verifier did not run.
	Verdict string
	// Decision is what the findings of the result and the verdicts, when there are any, make of the review
	// (reviewverdict.Decide). A finding with no verdict counts as a check that did not run.
	Decision reviewverdict.Decision
}

// Exempted is a stored exemption that binds a head, read back after CheckExemption, CheckBinding and CheckOneCommit
// held it. Only the use case that reads the exemptions of a head back (review/app's exemptedHead) builds one, and only
// after those three checks: Cleared counts the values it is given and does not look inside them, so an Exempted made
// any other way would clear a head it has not earned.
type Exempted struct {
	// Candidate is the candidate git gives now for the commits the exemption names; CheckBinding has held the
	// stored binding to it.
	Candidate Candidate
	// Exemption is the path of the stored exemption, relative to the root of the working tree and written with '/'.
	Exemption string
}

// ResultsOfHead is the names, among names, of the stored results of the head: the ones CheckResultName accepts
// whose <head12> is the first 12 characters of head, by launch number, the first launch first. The verdict
// stored beside a result, the exemption of a head, a file being written and any other name are not results. A
// head too short to name a file has none, because no result name begins with a dash. names is left as it is.
func ResultsOfHead(names []string, head string) []string { return ofHead(names, head, resultSuffix) }

// ExemptionsOfHead is the names, among names, of the stored exemptions of the head: <head12>-<n>.exempt.json
// where <head12> is the first 12 characters of head, by number, the first one first. A result, a verdict, a
// file being written and any other name are not exemptions. A head too short to name a file has none. names is
// left as it is.
func ExemptionsOfHead(names []string, head string) []string {
	return ofHead(names, head, exemptionSuffix)
}

// ofHead is the names that are <head12>-<n> and suffix, written the way ResultName writes them, for the head,
// by number.
func ofHead(names []string, head, suffix string) []string {
	prefix := Candidate{Head: head}.Short() + "-"
	var found []string
	for _, name := range names {
		if _, _, ok := stem(name, suffix); ok && strings.HasPrefix(name, prefix) {
			found = append(found, name)
		}
	}
	slices.SortFunc(found, func(a, b string) int { return launchNumber(a, suffix) - launchNumber(b, suffix) })
	return found
}

// launchNumber is the <n> of a name that stem accepts with suffix.
func launchNumber(name, suffix string) int {
	_, number, _ := stem(name, suffix)
	n, _ := strconv.Atoi(number)
	return n
}

// CheckBinding holds the binding a result was stored with to c, the candidate git gives now for the same
// commits: the base, the head, the digest of the diff and the paths it changes, in the order git prints them,
// must all be equal. The error wraps ErrBindingDiffers and says the first member that differs with both values.
func CheckBinding(stored reviewresult.Binding, c Candidate) error {
	differs := func(member, inResult, inGit string) error {
		return fmt.Errorf("%w: %s is %s in the result and %s in git", ErrBindingDiffers, member, inResult, inGit)
	}
	switch {
	case stored.Base != c.Base:
		return differs("base", stored.Base, c.Base)
	case stored.Head != c.Head:
		return differs("head", stored.Head, c.Head)
	case stored.DiffSHA256 != c.DiffSHA256:
		return differs("diff_sha256", stored.DiffSHA256, c.DiffSHA256)
	case !slices.Equal(stored.Paths, c.Paths):
		return fmt.Errorf("%w: the paths are not the ones git lists for %s (the result lists %d, git lists %d)",
			ErrBindingDiffers, c.Ref(), len(stored.Paths), len(c.Paths))
	}
	return nil
}

// Clearance decides whether the stored results of a head clear it for delivery: reviewed holds every result
// that binds the head, by launch, and the head is cleared when there is at least one and every one is clear.
//
// A result that blocks keeps the head blocked whatever was recorded after it: a verdict that reproduced a
// severe finding is not undone by another launch on the same head, and a blocked head goes back to the builder
// and comes back as another head. A result whose outcome is not clear without blocking (a severe finding no
// verifier checked, or an outcome nobody decided) keeps it from delivery too. The refusal names a result that
// blocks when there is one, else a result that is not clear. The error wraps ErrNoResult, ErrBlocked or
// ErrNotClear.
func Clearance(reviewed []Reviewed) error {
	if len(reviewed) == 0 {
		return ErrNoResult
	}
	var notClear error
	for _, r := range reviewed {
		switch r.Decision.Outcome {
		case reviewverdict.Clear:
		case reviewverdict.Blocks:
			return fmt.Errorf("%w: %s", ErrBlocked, r.describe("blocking findings", r.Decision.Blocking))
		default:
			if notClear == nil {
				notClear = fmt.Errorf("%w: %s", ErrNotClear, r.describe("findings not checked", r.Decision.Unverified))
			}
		}
	}
	return notClear
}

// Cleared decides whether what is stored for a head clears it for delivery: reviewed holds every result that binds
// the head and exempted every exemption that does, each by number. Both are values the caller has already held to the
// files and to git (see Reviewed and Exempted): Cleared decides from how many there are and from the decision of each
// result, and verifies nothing itself, so a value that was not held must never reach it.
//
// A head with no result is cleared by an exemption, one is enough, and with neither it is refused as Clearance
// refuses it (ErrNoResult). A head with a result is decided by its results alone, as Clearance decides, and an
// exemption changes nothing: a result that blocks wins over any exemption, and a result that does not block and is
// not clear either (a finding no verifier checked, or an outcome nobody decided) keeps the head from delivery
// too, because an exemption is for work that takes no review and is not a way around a review that left something
// unchecked. The error is the one Clearance gives, so a head with no exemption is answered exactly as before.
func Cleared(reviewed []Reviewed, exempted []Exempted) error {
	if len(reviewed) == 0 && len(exempted) > 0 {
		return nil
	}
	return Clearance(reviewed)
}

// describe names the result and the verdict beside it, then the findings of the kind the label says.
func (r Reviewed) describe(label string, ids []string) string {
	verdict := "no verdict of the verifier is stored"
	if r.Verdict != "" {
		verdict = "verdict " + r.Verdict
	}
	if len(ids) == 0 {
		return fmt.Sprintf("%s (%s)", r.Result, verdict)
	}
	return fmt.Sprintf("%s (%s): %s %s", r.Result, verdict, label, strings.Join(ids, ", "))
}
