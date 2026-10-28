package review

// Link is a range of commits that a review covers: the base a stored result or exemption was made against and the
// head it binds, both full names. It carries no verdict: a review that blocks covers its range as one that is clear
// does, because what the stop hook asks is whether a commit was reviewed, and 'review check' is the judge of how it
// turned out (own-rdd T6a, S6).
type Link struct{ Base, Head string }

// FirstUnreviewed is the next commit of a branch that no review covers (own-rdd T6a, S6, L18). from is the commit the
// branch was made on and commits are the commits it has that the default branch has not, oldest first, each the first
// parent of the next; links are the ranges the stored reviews cover. A commit is reached when the chain of reviews
// leads to it from the commit the branch was made on: a review reaches its head when its base is reached, so a
// review of one commit, of several at once, and the reviews of a branch made one by one, all reach the commit they
// end at, and a review that starts at a commit nothing reached reaches nothing. The order of the links is not
// significant.
//
// It returns the first commit after the last one that is reached, and the commit before it, which is the base of the
// review to ask for: the last reached commit, or from when none is. found is false when the last commit is reached,
// and when there are no commits.
func FirstUnreviewed(from string, commits []string, links []Link) (base, head string, found bool) {
	reached := map[string]bool{from: true}
	for changed := true; changed; {
		changed = false
		for _, l := range links {
			if reached[l.Base] && !reached[l.Head] {
				reached[l.Head], changed = true, true
			}
		}
	}
	last := -1
	for i, commit := range commits {
		if reached[commit] {
			last = i
		}
	}
	if last == len(commits)-1 {
		return "", "", false
	}
	base = from
	if last >= 0 {
		base = commits[last]
	}
	return base, commits[last+1], true
}
