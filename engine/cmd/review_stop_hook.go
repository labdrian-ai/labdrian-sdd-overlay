package main

// review subcommand, verb 'stop-hook': the Claude Code Stop hook that blocks the stop of an agent, once for each session
// and question, when a commit of its branch has no review (own-rdd T6a, S6, L18). It is an internal hook command, not one
// a person runs: Claude Code starts it when the agent is about to stop, hands it the event as JSON on stdin, and reads what
// it prints. `install-hooks` installs it (the settings review stop-hook family); Claude Code loads hook changes only when
// it starts, so an existing install must re-run install-hooks and restart Claude Code. The tests feed it hook JSON
// directly.
//
// The decision is review/app's Stop, a use case over git and the stores; this file reads stdin (bounded by
// hookwire.MaxEnvelopeBytes), has engine/hookwire read it into a stop and write the block, finds the state home the
// questions are remembered in, and words the question for the agent that reads it. Like 'skills guard-hook' it is not in
// the list of the verbs a person runs that review names to a caller who gets one wrong.
//
// The contract with Claude Code is the projection hook's:
//
//   - stdout is empty, or exactly one JSON object: to block the stop, {"decision":"block","reason":"..."}, and to let it
//     happen, nothing.
//   - The exit code is 0 always, except 1 for a command line that is not understood. It is never 2, which blocks the stop
//     and hands stderr to the agent as the reason: whatever goes wrong (input that cannot be read or decoded, a repository
//     or a store that cannot be read, a panic) the stop happens, so a hook that is broken never keeps an agent from
//     stopping. What the hook could not tell is one line on stderr, which Claude Code shows only in verbose mode; input it
//     cannot use is silent on both streams, and so is every case with nothing to ask.
//
// It asks git and reads the working tree, and writes one marker in the state home when it blocks a stop (review/fsstore's
// Asked). It binds nothing, makes no worktree and stores no result: the agent runs the command the block names.

import (
	"fmt"
	"io"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// reviewStopVerb is the 'review' verb of the Stop hook.
const reviewStopVerb = "stop-hook"

// runReviewStopHook implements 'review stop-hook'. From the moment the command line has been understood, nothing it does
// can change the exit code from 0.
func runReviewStopHook(p process, d deps, args []string) {
	if _, err := parseReviewFlags(args); err != nil {
		fmt.Fprintf(p.stderr, "error: review %s: %v\n", reviewStopVerb, err)
		p.exit(1)
		return
	}
	// A Go panic ends the process with status 2, which Claude Code reads as "block this stop". It is caught, reported
	// in full on stderr and turned into the same exit 0 as every other failure.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(p.stderr, "review %s: internal error: %v\n", reviewStopVerb, r)
			p.exit(0)
		}
	}()
	out, err := stopHookAnswer(p, d)
	if err != nil {
		fmt.Fprintf(p.stderr, "review %s: %v\n", reviewStopVerb, err)
	}
	if len(out) > 0 {
		_, _ = p.stdout.Write(out) // nothing to do if the write fails: the stop happens.
	}
	p.exit(0)
}

// stopHookAnswer reads the hook input and returns what the hook prints on stdout: the block of the stop, or nothing. Input
// that cannot be read or used is nothing, in silence, and so is the input of another event; what the hook could not tell
// is an error with no block, because a block that cannot be backed by the stores or by git would be asked at every stop.
// One byte past the bound is enough for the decoder to see the input is over it, and no more of an endless input is
// ever read.
func stopHookAnswer(p process, d deps) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(p.stdin, hookwire.MaxEnvelopeBytes+1))
	if err != nil {
		return nil, nil
	}
	in, err := hookwire.DecodeStop(data)
	if err != nil || in.Event != "" && in.Event != hookwire.EventStop {
		return nil, nil
	}
	dir, err := reviewPath(d, in.Cwd)
	if err != nil {
		return nil, err
	}
	home, err := statestore.HomeFrom(d.env)
	if err != nil {
		return nil, err
	}
	asked, err := fsstore.NewAsked(home)
	if err != nil {
		return nil, err
	}
	ruling, err := newReviewService(d).WithAsked(asked).Stop(dir, in.SessionID, in.Continuing)
	if err != nil || !ruling.Block {
		return nil, err
	}
	return hookwire.StopReply{Block: true, Reason: stopReason(ruling)}.Encode()
}

// stopReason words the question of a block for the agent that reads it: what is missing, the exact command that starts to
// answer it, the way on, and what counts as an answer.
func stopReason(r reviewapp.Ruling) string {
	q := r.Question
	if r.NoDefault {
		return fmt.Sprintf("The repository does not tell the default branch of branch %s (origin/HEAD is not set, and there is no branch main or master), "+
			"so the commits of the branch that need a review cannot be told. Run 'git remote set-head origin --auto' (or make the default branch main) "+
			"and the stop hook will name the next commit to review. The stop hook asks this once for each session.", q.Branch)
	}
	return fmt.Sprintf("Branch %s has a commit that no review covers. Before you stop, review that commit alone, not the whole branch:\n\n"+
		"  labdrian review bind --base %s --head %s\n\n"+
		"then follow the role-chain skill: 'labdrian review record' the reply of the reviewer and 'labdrian review verify' it, or, when the work of the commit "+
		"takes no review, 'labdrian review exempt' it. A commit counts as reviewed once a stored result or exemption binds it, whatever its outcome; "+
		"'labdrian review check' judges the outcome. The stop hook asks this once for each session and commit.", q.Branch, q.Base, q.Head)
}
