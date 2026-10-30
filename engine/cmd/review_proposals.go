package main

import "fmt"

// runReviewProposals implements 'review proposals [--cwd <repo>]' (own-rdd T12e, S15 g): it prints, as one JSON object,
// the worktrees and the local branches the owner may remove, each with the git command that removes it, and runs none of
// them. It reads the repository and stores nothing. A listing is exit 0; a command line it cannot read, a repository with
// no default branch and a git that fails are exit 1.
func runReviewProposals(p process, d deps, args []string) {
	refused := func(err error) {
		fmt.Fprintf(p.stderr, "error: review proposals: %v\n", err)
		p.exit(1)
	}
	f, err := parseReviewFlags(args, "--cwd")
	if err != nil {
		refused(err)
		return
	}
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		refused(err)
		return
	}
	proposals, err := newReviewService(d).Proposals(dir)
	if err != nil {
		refused(err)
		return
	}
	if err := writeReviewJSON(p, proposals); err != nil {
		refused(err)
		return
	}
	p.exit(0)
}
