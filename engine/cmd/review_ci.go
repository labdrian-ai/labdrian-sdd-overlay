package main

// review ci: 'review ci --landing <rev> --head <rev> [--cwd <repo>]' says whether the repository has no CI for the landing
// commit that holds the delivered head (labdrian-orchestrator-opencode T46, S3 L123). The decision is the use case
// reviewapp.Service.CheckNoCI; this file parses the command line, composes the service as the other review verbs do and says
// what it found.

import "fmt"

// runReviewCI implements 'review ci'. No CI established is exit 0 with one line on stdout that starts `CI: none configured`
// and names the full landing and head. Every refusal is exit 1 with one line on stderr that names what was found and says
// that the no-CI closure does not apply, and a missing flag, a revision git cannot resolve or a git that fails is non-zero
// too, with nothing on stdout. Nothing is written.
func runReviewCI(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "ci", err) }
	f, err := parseReviewFlags(args, "--landing", "--head", "--cwd")
	if err == nil {
		err = f.require("--landing", "--head")
	}
	if err != nil {
		fail(err)
		return
	}
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		fail(err)
		return
	}
	none, err := newReviewService(d).CheckNoCI(dir, f.landing, f.head)
	if err != nil {
		fail(err)
		return
	}
	if _, err := fmt.Fprintf(p.stdout, "CI: none configured: landing %s holds head %s, no remote, no CI marker in its tree\n", none.Landing, none.Head); err != nil {
		fail(fmt.Errorf("no CI is configured, but the answer could not be written: %w", err))
		return
	}
	p.exit(0)
}
