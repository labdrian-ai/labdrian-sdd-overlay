package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// usageText names the only flag this guard accepts. --base-ref mirrors the
// flag `gentle-ai review status` itself takes: it switches the projection away
// from the default workspace ("whatever is uncommitted right now") to a
// base-to-HEAD comparison, which is the only projection that still covers work
// already committed.
const usageText = "usage: review-preflight [--base-ref <remote>/<branch>]\n"

// gentleAIBinary is the command this guard probes. It is never invoked through
// a shell: the argv is built as a slice, so a base ref containing shell
// metacharacters is passed as one opaque argument rather than interpreted.
const gentleAIBinary = "gentle-ai"

// statusContract pins the contract token every probe passes. The projection
// field names this guard reads (base_tree, current_candidate_tree, paths,
// intended_untracked) are only meaningful under that contract, so asking for
// the payload without pinning it would make the field reads unfalsifiable.
const statusContract = "gentle-ai.review-integration/v2"

// Process exit codes. The split mirrors the sibling deterministic-check-runner
// (tools/deterministic-check-runner/main.go): 0 clean, 1 the condition this
// tool exists to catch, 2 a bad invocation, 3 no verdict could be reached.
//
// Only outcomeCovered asserts that starting a review here is safe. Every path
// that cannot prove that returns 1 or 3 — never 0 — because the failure this
// guard exists to prevent is precisely a review that reports success while
// having inspected nothing. A guard that guessed "probably fine" when it could
// not read the projection would reproduce that failure one layer up.
const (
	outcomeCovered        = 0
	outcomeEmptyCandidate = 1
	outcomeUsage          = 2
	outcomeUndetermined   = 3
)

// run answers one question: would a review started in this working directory
// actually cover anything? It probes `gentle-ai review status`, reads the
// projection that a review start would freeze, and reports whether that
// candidate is empty.
//
// The order is deliberate. The invocation is validated before any subprocess
// runs, so a typo in a flag can never be reported as a projection verdict; and
// the working directory is resolved before the probe, so the directory named
// in the diagnostics is the directory that was actually inspected.
func run(args []string, stdout, stderr io.Writer) int {
	baseRef, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "review-preflight: %v\n", err)
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "review-preflight: could not resolve the working directory: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	payload, err := runReviewStatus(dir, baseRef)
	if err != nil {
		fmt.Fprintf(stderr, "review-preflight: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	p, err := parseProjection(payload)
	if err != nil {
		fmt.Fprintf(stderr, "review-preflight: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	if reasons := emptyCandidateReasons(p); len(reasons) > 0 {
		reportEmptyCandidate(stderr, p, reasons, baseRef)
		return outcomeEmptyCandidate
	}

	fmt.Fprintf(stdout, "review-preflight: the candidate covers %d changed path(s) and %d intended-untracked path(s); base_tree=%s current_candidate_tree=%s\n",
		len(p.Paths), len(p.IntendedUntracked), p.BaseTree, p.CurrentCandidateTree)
	return outcomeCovered
}

// parseArgs parses the guard's only flag, mirroring the sibling runner's
// parseCheckArgs shape (ContinueOnError plus a discarded FlagSet output, so run
// owns every byte written to stderr). Leftover positional arguments are an
// error rather than being ignored: this tool takes no subcommand, and silently
// accepting one would let `review-preflight check` — the sibling runner's
// shape, which an operator will reasonably try — run as a bare invocation and
// report a verdict about something the operator did not ask for.
func parseArgs(args []string) (string, error) {
	fs := flag.NewFlagSet("review-preflight", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baseRef := fs.String("base-ref", "", "project base-to-HEAD against this ref instead of the workspace")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected argument %q; this guard takes no subcommand", fs.Arg(0))
	}
	return *baseRef, nil
}

// statusProbe is the seam between the guard's decision logic and the gentle-ai
// subprocess it reads from.
type statusProbe func(dir, baseRef string) ([]byte, error)

// runReviewStatus is a var rather than a direct call for the same reason
// toolExecTimeout is a var in the sibling runner: tests must be able to
// substitute it. CI has no gentle-ai installed, so a guard reachable only
// through a real gentle-ai would be untested in exactly the environment that
// runs it, and the emptiness rule below is the part that must never regress.
var runReviewStatus statusProbe = execReviewStatus

// statusExecTimeout bounds the probe subprocess. `gentle-ai review status` is a
// local repository read with no module downloads to wait on, so it does not
// need the sibling runner's 5-minute cold-toolchain headroom; 60 seconds is
// generous for a large repository while still bounding a genuine hang. This
// guard is meant to be cheap enough to run before every review, and a guard
// that can hang indefinitely is one operators learn to skip. A var so tests can
// shrink it and prove enforcement without waiting out the real deadline.
var statusExecTimeout = 60 * time.Second

// statusWaitDelay bounds Wait() itself once statusExecTimeout has fired
// (exec.Cmd.WaitDelay): killing the probe does not kill a grandchild still
// holding the stdout pipe open, and without this the "killed" probe can leave
// Run() blocked on that pipe — silently reintroducing the hang the deadline
// exists to bound. Short and separate: this is cleanup time for an already
// cancelled run, never more budget for legitimate work.
var statusWaitDelay = 5 * time.Second

// execReviewStatus runs the real `gentle-ai review status` probe and returns
// its stdout. Every failure mode is returned as an error rather than an empty
// payload: an empty payload would decode into a zero-valued projection, which
// the emptiness rule would then report as a confidently empty candidate that
// was in fact never observed.
//
// The absent-binary case is detected up front with LookPath rather than left to
// exec's own error, because "gentle-ai is not installed" is the single most
// likely reason this guard cannot answer and it deserves to be said plainly.
// gentle-ai's own stderr is carried into the failure text for the same reason:
// when the probe runs but refuses, its message is the only thing that explains
// why.
func execReviewStatus(dir, baseRef string) ([]byte, error) {
	toolPath, lookErr := exec.LookPath(gentleAIBinary)
	if lookErr != nil {
		return nil, fmt.Errorf("%s is not on PATH, so whether a review started here would cover anything cannot be determined: %w", gentleAIBinary, lookErr)
	}

	argv := []string{"review", "status", "--cwd", dir, "--contract", statusContract}
	if baseRef != "" {
		argv = append(argv, "--base-ref", baseRef)
	}

	ctx, cancel := context.WithTimeout(context.Background(), statusExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, toolPath, argv...)
	cmd.WaitDelay = statusWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%s review status exceeded its %s deadline and was killed, so no projection was observed", gentleAIBinary, statusExecTimeout)
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s review status failed: %v: %s", gentleAIBinary, runErr, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// statusPayload is the subset of the gentle-ai.review-integration.status/v3
// document this guard reads. Every other field (receipt, repair, candidates,
// applicability, …) is deliberately ignored so the guard does not couple itself
// to schema churn in fields it does not depend on.
//
// Projection is a pointer so that an absent "projection" key stays
// distinguishable from one that merely decoded to zero values. Collapsing the
// two would report a payload this guard could not read as a confidently empty
// candidate, sending the operator to the --base-ref workaround for a problem
// that workaround does not fix.
type statusPayload struct {
	Projection *projection `json:"projection"`
}

// projection is the candidate that a review started right now would freeze:
// the two trees it spans and the paths it would inspect.
type projection struct {
	BaseTree             string   `json:"base_tree"`
	CurrentCandidateTree string   `json:"current_candidate_tree"`
	Paths                []string `json:"paths"`
	IntendedUntracked    []string `json:"intended_untracked"`
}

// parseProjection decodes the projection a review start would freeze, or
// explains why the payload cannot answer the question.
//
// A projection missing either tree is rejected rather than compared. The
// emptiness rule below turns on base_tree == current_candidate_tree, and two
// absent fields compare equal to each other, so a payload that simply never
// carried the trees would satisfy that test and be reported as a definitively
// empty candidate. That is a verdict the guard never observed; it fails closed
// as undetermined instead, which is a different fact and a different fix.
func parseProjection(payload []byte) (projection, error) {
	var doc statusPayload
	if err := json.Unmarshal(payload, &doc); err != nil {
		return projection{}, fmt.Errorf("the review status output is not valid JSON, so the projection cannot be inspected: %w", err)
	}
	if doc.Projection == nil {
		return projection{}, errors.New(`the review status payload carries no "projection" object, so what a review would cover cannot be determined`)
	}
	p := *doc.Projection
	if p.BaseTree == "" || p.CurrentCandidateTree == "" {
		return projection{}, fmt.Errorf("the review status projection does not carry both trees (base_tree=%q, current_candidate_tree=%q), so the candidate cannot be compared", p.BaseTree, p.CurrentCandidateTree)
	}
	return p, nil
}

// emptyCandidateReasons returns one line per condition that makes this
// projection an empty candidate; an empty result means the candidate covers
// something and a review started here would have real bytes to inspect.
//
// Both conditions are evaluated independently and either one alone blocks —
// fail closed. They are not redundant: a workspace projection over a clean tree
// trips both at once, but a projection can also name paths while resolving both
// trees to the same object (no net change), or span two different trees while
// naming no path at all. Requiring both to agree before blocking would let each
// of those through, and the whole point of this guard is that a review over
// nothing does not fail — it approves.
//
// Both are reported when both hold, rather than short-circuiting on the first,
// because they answer different operator questions ("what would be inspected?"
// and "is there any change here at all?") and seeing both together is what
// makes the clean-tree case immediately recognizable.
func emptyCandidateReasons(p projection) []string {
	var reasons []string
	if len(p.Paths) == 0 && len(p.IntendedUntracked) == 0 {
		reasons = append(reasons, "the candidate covers 0 changed paths and 0 intended-untracked paths")
	}
	if p.BaseTree == p.CurrentCandidateTree {
		reasons = append(reasons, fmt.Sprintf("base_tree and current_candidate_tree are the same tree (%s), so the candidate contains no change at all", p.BaseTree))
	}
	return reasons
}

// reportEmptyCandidate writes the blocking diagnostic. It is long on purpose.
// Nothing in this repository documents --base-ref today, so an operator hitting
// this for the first time has no other source to consult: the message has to
// state what was observed, what would happen if it were ignored, and what to do
// instead, or the guard just becomes an unexplained non-zero exit that gets
// worked around by not running it.
func reportEmptyCandidate(stderr io.Writer, p projection, reasons []string, baseRef string) {
	fmt.Fprint(stderr, "review-preflight: EMPTY review candidate — a review started here would cover nothing.\n")
	for _, reason := range reasons {
		fmt.Fprintf(stderr, "  - %s\n", reason)
	}
	fmt.Fprintf(stderr, "  observed: base_tree=%s current_candidate_tree=%s paths=%d intended_untracked=%d\n",
		p.BaseTree, p.CurrentCandidateTree, len(p.Paths), len(p.IntendedUntracked))
	fmt.Fprint(stderr, emptyCandidateConsequence)
	if baseRef != "" {
		fmt.Fprintf(stderr, emptyBaseRefRemedy, baseRef)
		return
	}
	fmt.Fprint(stderr, emptyWorkspaceRemedy)
}

// emptyCandidateConsequence states the part that makes this worth blocking on:
// the failure mode is not an error, it is a success over nothing. An operator
// who reads only "empty candidate" may reasonably shrug and start the review
// anyway, which is exactly how six CRITICAL defects shipped from this
// repository under an approved receipt.
const emptyCandidateConsequence = `
A review started against this candidate does not fail. It selects 0 lenses,
freezes a correction budget of 0, inspects 0 files, and still produces an
APPROVED RECEIPT over nothing — every defect in the work you meant to review
ships unreviewed and formally approved.
`

// emptyWorkspaceRemedy is the default-projection case. `gentle-ai review`
// projects the WORKSPACE unless told otherwise, and sdd-apply commits each work
// unit as it goes, so by the time a review starts the tree is clean and the
// workspace projection has nothing left to see.
const emptyWorkspaceRemedy = `
Why: gentle-ai review projects the WORKSPACE by default, and sdd-apply commits
each work unit as it goes. Once the working tree is clean there is nothing left
in the workspace for the review to project.

Fix it either way:
  1. Project base-to-HEAD instead of the workspace. Pass
     --base-ref <remote>/<branch> (for example --base-ref origin/main) to this
     guard AND to the review you start. Use this when the work is already
     committed.
  2. Or leave the work uncommitted until the review has started, so the default
     workspace projection still covers it.
`

// emptyBaseRefRemedy is the case where the operator already took the documented
// workaround and the base-to-HEAD projection is STILL empty. Repeating "pass
// --base-ref" verbatim there would be advice they have already followed; the
// only thing left to question is the ref itself, so the message names it.
const emptyBaseRefRemedy = `
Why: the base-to-HEAD projection against %[1]s is itself empty — HEAD carries no
change relative to that ref, so there is nothing between them to review.

Fix it either way:
  1. Repoint --base-ref <remote>/<branch> at the ref this branch actually
     diverged from (the merge base your work sits on top of), not one that
     already contains it.
  2. Or confirm the work is committed on the branch you are currently standing
     on, and that %[1]s is not already up to date with it.
`

// undeterminedNote closes every exit-3 path. Stating the fail-closed stance out
// loud matters because the alternative reading — "the guard errored, so just go
// ahead" — reintroduces the exact hazard: an unverified review start.
const undeterminedNote = `This guard fails closed: it could not prove that a review started here would
cover anything, so it does not report that starting one is safe. Resolve the
error above and re-run, or inspect the projection by hand with:
  gentle-ai review status --cwd . --contract gentle-ai.review-integration/v2
`
