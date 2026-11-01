package main

// review launch (own-rdd T8c, S9; phase9-c25-review-hunt T4, S3 b): the one review verb that starts a reviewer. It reads
// the brief of the parent on stdin, binds the candidate as 'review bind' does, runs the reviewer of the runtime in the
// worktree of the binding (behind the Reviewer port of review/app) and records its reply as 'review record' records one.
// On codex the reviewer is `codex exec --sandbox read-only` (review/codexexec) over the brief; on claude it is one
// tool-free `claude --print` (review/claudeprint) over the brief and the frozen context of the candidate. Every refusal
// that comes before the reviewer starts is exit 1; a reply that 'review record' would refuse is exit 2, and what it prints
// when it is stored is what 'review record' prints.

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/claudeprint"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/codexexec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The runtimes 'review launch' starts a reviewer on: on Pi the parent launches the reviewer agent natively.
const (
	launchRuntimeClaude = "claude"
	launchRuntimeCodex  = "codex"
)

// errLaunchRuntime is what a --runtime other than claude or codex is refused with.
var errLaunchRuntime = errors.New("--runtime must be claude or codex: on Pi the parent launches the reviewer agent natively")

// runReviewLaunch implements 'review launch --runtime claude|codex --feature --base --head --rows --model [--cwd]'.
func runReviewLaunch(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "launch", err) }
	f, err := parseReviewFlags(args, "--runtime", "--feature", "--base", "--head", "--rows", "--model", "--cwd")
	if err == nil && f.has("--runtime") && f.runtime != launchRuntimeClaude && f.runtime != launchRuntimeCodex {
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
	service := newReviewService(d)
	var reviewer app.Reviewer
	if f.runtime == launchRuntimeClaude {
		claudeRoot := core.Config{Home: home}.DefaultClaudeRoot()
		reviewer = claudeprint.New(d.launchClaudeRunner(), claudeprint.Options{Environ: d.environment(), ClaudeRoot: claudeRoot})
		service = service.WithFrozenContext()
	} else {
		codexRoot := core.Config{Home: home, CodexHome: d.env("CODEX_HOME")}.DefaultCodexRoot()
		reviewer = codexexec.New(d.launchRunner(), codexexec.Options{Environ: d.environment(), CodexRoot: codexRoot})
	}
	recorded, err := service.WithReviewer(reviewer).Launch(dir, f.feature, f.base, f.head, f.rows, f.model, brief)
	if err != nil {
		fail(err)
		return
	}
	reportRecorded(p, recorded)
}
