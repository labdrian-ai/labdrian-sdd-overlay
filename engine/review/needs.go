package review

import (
	"errors"
	"fmt"
	"strings"
)

// This file is what a commit needs before it is ready for delivery (own-rdd T6b5, S6 d and f, L63): one pure function
// over the facts the use case read for the commit, which the stop hook and `review check` both call. It judges nothing
// the facts do not hold and reads no file.

var (
	// ErrNoSweep is a commit that no stored sweep record binds: its sweep never ran.
	ErrNoSweep = errors.New("review: no sweep record binds the commit")
	// ErrNoSweeper is a commit whose latest sweep fired and that no sweeper record answers.
	ErrNoSweeper = errors.New("review: no sweeper record answers the fired sweep record")
	// ErrNoPolisher is a commit that declares a high-risk item and that no polisher record binds.
	ErrNoPolisher = errors.New("review: no polisher record binds a commit that declares a high-risk item")
)

// passivePrefixes are the directories a passive commit may change: the task documents and the records the review
// engine stores, which nothing executes.
var passivePrefixes = []string{"odd/tasks/", "odd/review-results/", "odd/review-receipts/"}

// IsPassive says whether a commit is passive (S6 d): its trailer is a valid Risk: none, every path it changes is under
// odd/tasks/, odd/review-results/ or odd/review-receipts/, and its diff deletes or renames nothing. risk and riskErr are
// what DeclaredRisk gave; paths are the paths the commit changes and removed the paths it deletes or renames. A passive
// commit needs nothing else and covers its link of the chain without any record.
func IsPassive(risk []int, riskErr error, paths, removed []string) bool {
	if riskErr != nil || len(risk) > 0 || len(removed) > 0 {
		return false
	}
	for _, path := range paths {
		if !underPassive(path) {
			return false
		}
	}
	return true
}

func underPassive(path string) bool {
	for _, prefix := range passivePrefixes {
		if len(path) > len(prefix) && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// Missing is the kind of thing a commit lacks.
type Missing int

// The things a commit may lack, in the order Needs lists them (S6 d).
const (
	// MissingRisk is a trailer that does not declare the risk.
	MissingRisk Missing = iota + 1
	// MissingSweep is a sweep record of the commit.
	MissingSweep
	// MissingSweeper is a sweeper record that answers the fired sweep.
	MissingSweeper
	// MissingPolisher is a polisher record of a commit that declares an item.
	MissingPolisher
	// MissingReview is a stored result or exemption that covers the commit in the chain.
	MissingReview
)

// Need is one thing a commit lacks and the reason `review check` refuses with.
type Need struct {
	Missing Missing
	// Err is the reason. Apart from the trailer's, which is the error of DeclaredRisk, and the review's, which is the
	// error the coverage gave, it wraps exactly one of ErrNoSweep, ErrNoSweeper and ErrNoPolisher and names the commit.
	Err error
}

// Facts are what the use case read for one commit.
type Facts struct {
	// Commit is the full name of the commit.
	Commit string
	// Passive is IsPassive for the commit.
	Passive bool
	// Risk and RiskErr are what DeclaredRisk gave for its trailer: the items, or the error that wraps ErrRiskTrailer.
	Risk    []int
	RiskErr error
	// SweepCommit says the commit is a sweep commit (S6 d): its subject starts "chore(sweep)" and a sweeper record of its
	// first parent names it. It needs no sweep record of its own.
	SweepCommit bool
	// Sweep is the repository-relative path of the latest sweep record that holds for the commit, and "" when there is
	// none; SweepFired says that record fired.
	Sweep      string
	SweepFired bool
	// Answered says a sweeper record that holds for the commit names Sweep.
	Answered bool
	// Polished says a polisher record that holds binds the commit.
	Polished bool
	// AskCoverage says Needs asks for the review; Coverage is why no stored result or exemption covers the commit, and
	// nil when one does. A commit inside a range a review covers does not ask.
	AskCoverage bool
	Coverage    error
}

// Needs lists what the commit lacks, in the order of S6 (d): a valid trailer, a sweep record, the sweeper's answer to a
// fired sweep, a polisher record for a declared item, and the review that covers it. A passive commit lacks nothing. A
// commit whose trailer fails is unknown risk, so only its risk and its sweep are listed: what else it needs depends on
// the risk it declares. A sweep commit lacks no sweep of its own, but one that has a sweep record is held to it.
func Needs(f Facts) []Need {
	if f.Passive {
		return nil
	}
	var needs []Need
	if f.RiskErr != nil {
		needs = append(needs, Need{Missing: MissingRisk, Err: f.RiskErr})
		if f.Sweep == "" && !f.SweepCommit {
			needs = append(needs, f.noSweep())
		}
		return needs
	}
	switch {
	case f.Sweep == "" && !f.SweepCommit:
		needs = append(needs, f.noSweep())
	case f.Sweep != "" && f.SweepFired && !f.Answered:
		needs = append(needs, Need{Missing: MissingSweeper, Err: fmt.Errorf("commit %s: %w: %s", f.Commit, ErrNoSweeper, f.Sweep)})
	}
	if len(f.Risk) > 0 && !f.Polished {
		needs = append(needs, Need{Missing: MissingPolisher, Err: fmt.Errorf("commit %s: %w: Risk: %s", f.Commit, ErrNoPolisher, RiskValue(f.Risk))})
	}
	if f.AskCoverage && f.Coverage != nil {
		needs = append(needs, Need{Missing: MissingReview, Err: f.Coverage})
	}
	return needs
}

func (f Facts) noSweep() Need {
	return Need{Missing: MissingSweep, Err: fmt.Errorf("commit %s: %w", f.Commit, ErrNoSweep)}
}
