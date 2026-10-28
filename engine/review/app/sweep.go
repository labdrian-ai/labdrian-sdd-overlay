package app

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// Swept is what Sweep stored.
type Swept struct {
	Candidate review.Candidate
	// Path is where the sweep record is stored, relative to the root of the working tree and written with '/'.
	Path string
	// Number is the number it takes for the head: the <n> of the file name, which the results, the verdicts, the
	// exemptions and the other role records of the head share.
	Number int
	// SHA256 is the digest of the bytes of the stored file.
	SHA256 string
	// Outcome is review.OutcomeClean or review.OutcomeFired.
	Outcome string
	// Findings are what the three checks found.
	Findings review.SweepFindings
	// Candidates are the ids a sweeper answers for when the sweep fired, in the order of review.StoredSweep.Candidates;
	// none when it did not.
	Candidates []string
	// NotFlushed, when it is not nil, wraps ErrNotFlushed: the record is stored, but the directory could not be flushed
	// after it. The record is not stored again.
	NotFlushed error
}

// Sweep runs the checks T1 to T3 of the sweeper (skills/role-chain/SKILL.md) on the one commit from base to head, in the
// repository that holds dir (absolute), and stores their findings as the sweep record of the head in the results
// directory of the feature (own-rdd T6b3, S6 b and c). The candidate is bound from git as Bind and Exempt bind it, and the
// base is the first parent of the head (review.ErrNotOneCommit).
//
// The rules are applied in this order and the first one broken is the refusal, with nothing stored:
//
//   - the feature is a single safe path segment (review.ErrInvalidFeature) and the Service has both ports of WithSweep;
//   - the candidate binds and is one commit;
//   - the lock of the sweep of the repository is free (ErrSweepBusy): a second sweep does not wait. From here to the end
//     the lock is held, and it is given back whatever happens;
//   - T1: the paths the commit removed, and the names its removed lines declared that its head still mentions in a path the
//     diff does not touch;
//   - T2: for each Go module that holds a changed .go, go.mod or go.sum (the innermost module of each path), deadcode over
//     the whole module and staticcheck over the packages of the changed .go files, and shellcheck over the changed .sh
//     files, each on a copy of the base and on a copy of the head; a finding is new when its normalized line occurs more
//     times at the head (review.NewFindings). A check that cannot run is an error wrapping ErrToolDidNotRun, and nothing
//     is stored (S1);
//   - T3: the lines of git status of the working tree as it is, but for the records waiting under odd/review-results/.
//
// A commit that changes no Go module and no shell file makes no copy and runs no tool.
func (s Service) Sweep(dir, feature, base, head string) (Swept, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Swept{}, err
	}
	if s.analyzers == nil || s.copies == nil {
		return Swept{}, errors.New("review: the sweep has no analyzers or no place for its copies")
	}
	repo, candidate, err := s.candidate(dir, base, head)
	if err != nil {
		return Swept{}, err
	}
	if err := s.oneCommit(repo, candidate.Base, candidate.Head); err != nil {
		return Swept{}, err
	}
	unlock, err := s.copies.Lock(repo)
	switch {
	case errors.Is(err, ErrSweepBusy):
		// the message is the one the command prints, word for word (S6 b)
		return Swept{}, ErrSweepBusy
	case err != nil:
		return Swept{}, fmt.Errorf("lock the sweep of %s: %w", repo.Main, err)
	}
	defer unlock()

	t1, err := s.sweepT1(repo, candidate)
	if err != nil {
		return Swept{}, err
	}
	t2, err := s.sweepT2(repo, candidate)
	if err != nil {
		return Swept{}, err
	}
	t3, err := s.sweepT3(repo)
	if err != nil {
		return Swept{}, err
	}
	findings := review.SweepFindings{T1: t1, T2: t2, T3: t3}
	file, err := review.EncodeSweep(candidate, findings)
	if err != nil {
		return Swept{}, err
	}
	number, name, err := s.store(repo.Root, feature, candidate, review.SweepName, file)
	var notFlushed error
	switch {
	case errors.Is(err, ErrNotFlushed):
		notFlushed = err
	case err != nil:
		return Swept{}, err
	}
	outcome, ids := review.OutcomeClean, []string(nil)
	if findings.Fired() {
		outcome, ids = review.OutcomeFired, review.StoredSweep{Outcome: review.OutcomeFired, Findings: findings}.Candidates()
	}
	return Swept{Candidate: candidate, Path: review.ResultPath(feature, name), Number: number, SHA256: sha256hex.Sum(file),
		Outcome: outcome, Findings: findings, Candidates: ids, NotFlushed: notFlushed}, nil
}

// sweepT1 is the first check: the removed paths, and the removed names the tree of the head still mentions outside the
// paths of the diff.
func (s Service) sweepT1(repo Repository, c review.Candidate) (review.SweepT1, error) {
	deleted, err := s.git.DeletedOrRenamed(repo, c.Base, c.Head)
	if err != nil {
		return review.SweepT1{}, fmt.Errorf("list the paths removed by %s..%s: %w", c.Base, c.Head, err)
	}
	removed, added, err := s.git.ChangedLines(repo, c.Base, c.Head)
	if err != nil {
		return review.SweepT1{}, fmt.Errorf("read the changed lines of %s..%s: %w", c.Base, c.Head, err)
	}
	var names []review.RemovedName
	asked := review.RemovedNames(removed, added)
	var mentioned map[string][]string
	if len(asked) > 0 {
		if mentioned, err = s.git.Mentions(repo, c.Head, asked); err != nil {
			return review.SweepT1{}, fmt.Errorf("search %s for the removed names: %w", c.Head, err)
		}
	}
	for _, name := range asked {
		var hits []string
		for _, p := range mentioned[name] {
			if !slices.Contains(c.Paths, p) && !slices.Contains(hits, p) {
				hits = append(hits, p)
			}
		}
		if len(hits) > 0 {
			slices.Sort(hits)
			names = append(names, review.RemovedName{Name: name, Hits: hits})
		}
	}
	return review.SweepT1{DeletedOrRenamed: deleted, RemovedNames: names}, nil
}

// goChange is what a commit changed in one Go module: the module, relative to the root, and the packages of the changed
// .go files, as patterns of the module.
type goChange struct {
	module   string
	packages []string
}

// goChanges groups the changed paths by the innermost module that holds them, sorted by module. Only a .go, go.mod or
// go.sum file counts; a package is a pattern ("." or "./dir") of a changed .go file.
func goChanges(paths, modules []string) []goChange {
	found := map[string]*goChange{}
	for _, p := range paths {
		base := path.Base(p)
		if !strings.HasSuffix(p, ".go") && base != "go.mod" && base != "go.sum" {
			continue
		}
		module, ok := innermostModule(p, modules)
		if !ok {
			continue
		}
		change := found[module]
		if change == nil {
			change = &goChange{module: module}
			found[module] = change
		}
		if strings.HasSuffix(p, ".go") {
			pkg := path.Dir(strings.TrimPrefix(strings.TrimPrefix(p, module), "/"))
			if pkg != "." {
				pkg = "./" + pkg
			}
			if !slices.Contains(change.packages, pkg) {
				change.packages = append(change.packages, pkg)
			}
		}
	}
	var all []goChange
	for _, change := range found {
		slices.Sort(change.packages)
		all = append(all, *change)
	}
	slices.SortFunc(all, func(a, b goChange) int { return strings.Compare(a.module, b.module) })
	return all
}

// innermostModule is the module of modules that holds p and has the longest name.
func innermostModule(p string, modules []string) (string, bool) {
	best, ok := "", false
	for _, m := range modules {
		holds := m == "." || strings.HasPrefix(p, m+"/")
		if holds && (!ok || len(m) > len(best)) {
			best, ok = m, true
		}
	}
	return best, ok
}

// sweepT2 is the second check: the analyzers over the changed Go modules and shell files. The copies are made once, and
// only when there is something to run.
func (s Service) sweepT2(repo Repository, c review.Candidate) ([]review.SweepT2, error) {
	modules, err := s.git.Modules(repo, c.Head)
	if err != nil {
		return nil, fmt.Errorf("list the Go modules of %s: %w", c.Head, err)
	}
	changes := goChanges(c.Paths, modules)
	var shell []string
	for _, p := range c.Paths {
		if strings.HasSuffix(p, ".sh") {
			shell = append(shell, p)
		}
	}
	if len(changes) == 0 && len(shell) == 0 {
		return nil, nil
	}
	baseRoot, err := s.fillCopy(repo, CopyBase, c.Base)
	if err != nil {
		return nil, err
	}
	headRoot, err := s.fillCopy(repo, CopyHead, c.Head)
	if err != nil {
		return nil, err
	}
	a := s.analyzers
	var all []review.SweepT2
	add := func(tool, module string, base, head []string) {
		if lines := review.NewFindings(baseRoot, base, headRoot, head); len(lines) > 0 {
			all = append(all, review.SweepT2{Tool: tool, Module: module, New: lines})
		}
	}
	for _, change := range changes {
		base, err := a.Deadcode(baseRoot, change.module)
		if err != nil {
			return nil, err
		}
		head, err := a.Deadcode(headRoot, change.module)
		if err != nil {
			return nil, err
		}
		add("deadcode", change.module, base, head)
		if len(change.packages) == 0 {
			continue
		}
		base, err = a.Staticcheck(baseRoot, change.module, change.packages)
		if err != nil {
			return nil, err
		}
		head, err = a.Staticcheck(headRoot, change.module, change.packages)
		if err != nil {
			return nil, err
		}
		add("staticcheck", change.module, base, head)
	}
	if len(shell) > 0 {
		base, err := a.Shellcheck(baseRoot, shell)
		if err != nil {
			return nil, err
		}
		head, err := a.Shellcheck(headRoot, shell)
		if err != nil {
			return nil, err
		}
		add("shellcheck", ".", base, head)
	}
	return all, nil
}

// fillCopy empties the copy called side and fills it with the tree of commit.
func (s Service) fillCopy(repo Repository, side, commit string) (string, error) {
	dir, err := s.copies.Reset(repo, side)
	if err != nil {
		return "", fmt.Errorf("make the %s copy of the sweep: %w", side, err)
	}
	if err := s.git.Archive(repo, commit, dir); err != nil {
		return "", fmt.Errorf("fill the %s copy of the sweep with %s: %w", side, commit, err)
	}
	return dir, nil
}

// sweepT3 is the third check: what git says of the working tree when the sweep runs, but for the untracked or changed
// records under odd/review-results/, which wait for the commit that holds them (L41, D6). It is a property of the tree at
// that moment and is stored as it was seen.
func (s Service) sweepT3(repo Repository) ([]string, error) {
	lines, err := s.git.Status(repo)
	if err != nil {
		return nil, fmt.Errorf("read the status of %s: %w", repo.Root, err)
	}
	var kept []string
	for _, line := range lines {
		// `XY path`: git quotes a path with unusual characters, and the quote is not part of the directory.
		if len(line) > 3 && strings.HasPrefix(strings.TrimPrefix(line[3:], `"`), review.ResultsDir+"/") {
			continue
		}
		kept = append(kept, line)
	}
	return kept, nil
}
