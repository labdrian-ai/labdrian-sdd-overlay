package main

// review frozen-context (labdrian-orchestrator-opencode T40, S8): the verb a runtime whose reviewer cannot run git
// uses to give it its diff. It reads the brief of the parent on stdin and prints the brief followed by the frozen
// context of its candidate, the bytes 'review launch --runtime claude' hands the reviewer after its preamble. It
// starts no reviewer and stores no result; the one thing it makes, as 'review launch' does, is the detached worktree
// of the binding. A refusal is exit 2 and one line on stderr, nothing on stdout.

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// runReviewFrozenContext implements 'review frozen-context [--cwd <repo>]'.
func runReviewFrozenContext(p process, d deps, args []string) {
	fail := func(code int, err error) {
		fmt.Fprintf(p.stderr, "error: review frozen-context: %v\n", err)
		p.exit(code)
	}
	f, err := parseReviewFlags(args, "--cwd")
	if err != nil {
		fail(1, err)
		return
	}
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		fail(1, err)
		return
	}
	brief, err := readReviewReply(p, d, "")
	if err != nil {
		fail(1, err)
		return
	}
	out, err := newReviewService(d).FrozenContext(dir, brief)
	if err != nil {
		code := 1
		if errors.Is(err, review.ErrInvalidBrief) || errors.Is(err, review.ErrOverLimit) {
			code = 2
		}
		fail(code, err)
		return
	}
	if _, err := p.stdout.Write(out); err != nil {
		fail(1, fmt.Errorf("write to stdout: %w", err))
		return
	}
	p.exit(0)
}
