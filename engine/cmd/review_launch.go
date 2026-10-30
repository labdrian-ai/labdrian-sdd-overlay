package main

// review launch (own-rdd T8c, S9): the one review verb that starts a reviewer. It reads the brief of the parent on stdin,
// binds the candidate as 'review bind' does, runs the Codex reviewer read-only in the worktree of the binding (the adapter
// review/codexexec, behind the Reviewer port of review/app) and records its last message as 'review record' records a
// reply. Every refusal that comes before codex starts is exit 1; a reply that 'review record' would refuse is exit 2, and
// what it prints when it is stored is what 'review record' prints.

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/codexexec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// launchRuntime is the only runtime 'review launch' starts: on Claude Code and Pi the parent launches the reviewer
// agent natively.
const launchRuntime = "codex"

// errLaunchRuntime is what a --runtime other than codex is refused with.
var errLaunchRuntime = errors.New("--runtime must be codex: on Claude Code and Pi the parent launches the reviewer agent natively")

// runReviewLaunch implements 'review launch --runtime codex --feature --base --head --rows --model [--cwd]'.
func runReviewLaunch(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "launch", err) }
	f, err := parseReviewFlags(args, "--runtime", "--feature", "--base", "--head", "--rows", "--model", "--cwd")
	if err == nil && f.has("--runtime") && f.runtime != launchRuntime {
		err = errLaunchRuntime
	}
	if err == nil {
		err = f.require("--runtime", "--feature", "--base", "--head", "--rows", "--model")
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
	brief, err := readReviewReply(p, d, "")
	if err != nil {
		fail(err)
		return
	}
	home, err := d.homeDir()
	if err != nil {
		fail(err)
		return
	}
	codexRoot := core.Config{Home: home, CodexHome: d.env("CODEX_HOME")}.DefaultCodexRoot()
	reviewer := codexexec.New(d.launchRunner(), codexexec.Options{Environ: d.environment(), CodexRoot: codexRoot})
	recorded, err := newReviewService(d).WithReviewer(reviewer).Launch(dir, f.feature, f.base, f.head, f.rows, f.model, brief)
	if err != nil {
		fail(err)
		return
	}
	reportRecorded(p, recorded)
}
