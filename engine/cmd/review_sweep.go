package main

// 'review sweep --feature <name> --base <rev> --head <rev> [--cwd <repo>]' runs the checks T1 to T4 of the sweeper on
// the one commit from base to head and stores their findings as <head12>-<n>.sweep.json, and 'review record --role
// sweeper|polisher --feature <name> --base <rev> --head <rev> [--result <file>|-] [--cwd <repo>]' stores the report of
// the sweeper or the findings of one polisher judge (own-rdd T6b4, S6 b and c). The rules are Service.Sweep,
// Service.RecordSweeper and Service.RecordPolisher of review/app; this file parses the command line, wires the programs
// of the sweep (review/analyzers) and the place of its copies and its lock (review/fsstore) at the composition root, and
// says what they did. The default role of 'review record' is the reviewer, which review.go keeps as it always was.

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/analyzers"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// The roles 'review record' stores the reply of. The reviewer is the default.
const (
	roleReviewer = "reviewer"
	roleSweeper  = "sweeper"
	rolePolisher = "polisher"
)

// newSweepService is the use cases of the review verbs with the programs of the sweep, which run on the PATH of the
// process under the environment of the deps, and the copies of the sweep and its lock, which live under the state home
// outside every working tree.
func newSweepService(d deps) (reviewapp.Service, error) {
	home, err := statestore.HomeFrom(d.env)
	if err != nil {
		return reviewapp.Service{}, err
	}
	copies, err := fsstore.NewCopies(home)
	if err != nil {
		return reviewapp.Service{}, err
	}
	tools := analyzers.New(analyzers.ExecRunner{}, analyzers.Options{Environ: d.environment()})
	return newReviewService(d).WithSweep(tools, copies), nil
}

// sweptJSON is what 'review sweep' prints: the outcome, where the record is stored and the digest of the stored file,
// and, when the sweep fired, the candidates the sweeper answers for, in the order of review.StoredSweep.Candidates.
type sweptJSON struct {
	Outcome    string   `json:"outcome"`
	Path       string   `json:"path"`
	SHA256     string   `json:"sha256"`
	Candidates []string `json:"candidates"`
}

// runReviewSweep implements 'review sweep --feature --base --head [--cwd]'. A sweep that is stored, clean or fired, is
// exit 0. A command line it cannot read is exit 1. Anything after that which keeps it from storing the sweep is exit 2:
// a binding that is refused, another sweep of the repository running, and a check that did not run, because a sweep
// that could not run its checks never passes.
func runReviewSweep(p process, d deps, args []string) {
	usage := func(err error) {
		fmt.Fprintf(p.stderr, "error: review sweep: %v\n", err)
		p.exit(1)
	}
	refused := func(err error) {
		fmt.Fprintf(p.stderr, "error: review sweep: %v\n", err)
		p.exit(2)
	}
	f, err := parseReviewFlags(args, "--feature", "--base", "--head", "--cwd")
	if err == nil {
		err = f.require("--feature", "--base", "--head")
	}
	if err != nil {
		usage(err)
		return
	}
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		refused(err)
		return
	}
	service, err := newSweepService(d)
	if err != nil {
		refused(err)
		return
	}
	swept, err := service.Sweep(dir, f.feature, f.base, f.head)
	if err != nil {
		refused(err)
		return
	}
	fmt.Fprintf(p.stderr, "review sweep: stored %s (sha256 %s)\n", swept.Path, swept.SHA256)
	if swept.NotFlushed != nil {
		fmt.Fprintf(p.stderr, "review sweep: warning: its directory could not be flushed, so the record may not survive a crash of the machine; it is stored, do not sweep again: %v\n", swept.NotFlushed)
	}
	fmt.Fprintf(p.stderr, "review sweep: the sweep is %s\n", swept.Outcome)
	out := sweptJSON{Outcome: swept.Outcome, Path: swept.Path, SHA256: swept.SHA256, Candidates: swept.Candidates}
	if out.Candidates == nil {
		out.Candidates = []string{}
	}
	if err := writeReviewJSON(p, out); err != nil {
		refused(fmt.Errorf("the sweep is stored at %s, but the answer could not be written: %w", swept.Path, err))
		return
	}
	p.exit(0)
}

// checkRecordRole holds the role and the flags of 'review record' to each other: the role is one of the three, and the
// rows belong to the reviewer alone.
func checkRecordRole(f reviewFlags) error {
	switch f.role {
	case roleReviewer, roleSweeper, rolePolisher:
	default:
		if f.has("--role") {
			return fmt.Errorf("--role must be %s, %s or %s, not %q", roleReviewer, roleSweeper, rolePolisher, f.role)
		}
	}
	if !recordsAs(f, roleReviewer) && f.has("--rows") {
		return errors.New("--rows is only for --role reviewer")
	}
	return nil
}

// recordsAs says whether 'review record' stores the reply of the role: a command line without --role is the reviewer's.
func recordsAs(f reviewFlags, role string) bool {
	if !f.has("--role") {
		return role == roleReviewer
	}
	return f.role == role
}

// recordedRoleJSON is what 'review record --role sweeper|polisher' prints: where the record is stored and the digest of
// the stored file. There is no skeleton for the role chain, which has no payload kind for these records.
type recordedRoleJSON struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// runReviewRecordRole implements 'review record' for the sweeper and the polisher, whose flags checkRecordRole and
// runReviewRecord have already held. A report that is refused is exit 2 and nothing is written; any other failure is
// exit 1.
func runReviewRecordRole(p process, d deps, f reviewFlags) {
	fail := func(err error) { reviewFailed(p, "record", err) }
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		fail(err)
		return
	}
	reply, err := readReviewReply(p, d, f.result)
	if err != nil {
		fail(err)
		return
	}
	service := newReviewService(d)
	var recorded reviewapp.RoleRecorded
	if recordsAs(f, roleSweeper) {
		recorded, err = service.RecordSweeper(dir, f.feature, f.base, f.head, reply)
	} else {
		recorded, err = service.RecordPolisher(dir, f.feature, f.base, f.head, reply)
	}
	if err != nil {
		fail(err)
		return
	}
	fmt.Fprintf(p.stderr, "review record: stored %s (sha256 %s)\n", recorded.Path, recorded.SHA256)
	if recorded.NotFlushed != nil {
		fmt.Fprintf(p.stderr, "review record: warning: its directory could not be flushed, so the record may not survive a crash of the machine; it is stored, do not record it again: %v\n", recorded.NotFlushed)
	}
	if err := writeReviewJSON(p, recordedRoleJSON{Path: recorded.Path, SHA256: recorded.SHA256}); err != nil {
		fail(fmt.Errorf("the record is stored at %s, but the answer could not be written: %w", recorded.Path, err))
		return
	}
	p.exit(0)
}
