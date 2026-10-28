package main

// review subcommand, verb 'stop-hook': the Claude Code Stop hook that blocks the stop of an agent, once for each session,
// commit and need, when a commit of its branch misses anything 'review check' asks of it: a Risk trailer, a sweep record,
// the sweeper's answer to a fired sweep, a polisher record for a declared item, a review (own-rdd T6a, T6b6, S6, L18). It is an internal hook command, not one
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
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
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

// stopReason words the question of a block for the agent that reads it (own-rdd T6b6c, S6 e): for the one commit asked,
// one line for each thing it misses, in the order review.Needs lists them (a Risk trailer, a sweep, the sweeper's answer,
// the polisher records, the review), each with the exact command that answers it, and what counts as a record. The
// question of a repository that tells no default branch has its own reason. <feature> is left for the agent to fill, as
// the hook reads the records of every feature of the tree.
func stopReason(r reviewapp.Ruling) string {
	q := r.Question
	if r.NoDefault {
		return fmt.Sprintf("The repository does not tell the default branch of branch %s (origin/HEAD is not set, and there is no branch main or master), "+
			"so the commits of the branch that need a review cannot be told. Run 'git remote set-head origin --auto' (or make the default branch main) "+
			"and the stop hook will name the next commit to review. The stop hook asks this once for each session.", q.Branch)
	}
	lines := make([]string, 0, len(q.Needs))
	for _, need := range q.Needs {
		lines = append(lines, needLine(q, need))
	}
	return fmt.Sprintf("Commit %s of branch %s is not ready for delivery. Before you stop, for that commit alone (base %s):\n\n%s\n\n"+
		"<feature> is the feature whose odd/tasks/<feature>.md holds the task of the commit. A record counts once it is stored and binds the commit, "+
		"whatever its outcome; 'labdrian review check' judges the outcome. The stop hook asks for each thing a commit misses once for each session and commit.",
		q.Head, q.Branch, q.Base, strings.Join(lines, "\n"))
}

// needLine is the line of the block for one thing the commit of q misses.
func needLine(q reviewapp.Question, need review.Need) string {
	switch need.Missing {
	case review.MissingRisk:
		return fmt.Sprintf("- Risk: %s. End the commit message with a final trailer block that holds the one line \"Risk: none\" or \"Risk: <items>\" (for example \"Risk: 3,5\"); "+
			"when the commit is HEAD: git commit --amend --trailer \"Risk: <none|items>\". The rest of what the commit needs is asked once it declares its risk.", riskCause(q.Head, need.Err))
	case review.MissingSweep:
		return fmt.Sprintf("- Sweep: labdrian review sweep --feature <feature> --base %s --head %s", q.Base, q.Head)
	case review.MissingSweeper:
		return fmt.Sprintf("- Sweeper: the sweep record %s fired; launch the sweeper with its candidates, then: labdrian review record --role sweeper --feature <feature> --base %s --head %s",
			q.Sweep, q.Base, q.Head)
	case review.MissingPolisher:
		return fmt.Sprintf("- Polisher: the commit declares Risk: %s; launch the polisher judges on the commit and record each: labdrian review record --role polisher --feature <feature> --base %s --head %s",
			q.Risk, q.Base, q.Head)
	case review.MissingReview:
		line := fmt.Sprintf("- Review: labdrian review bind --base %s --head %s, then 'labdrian review record' the reply of the reviewer and 'labdrian review verify' it", q.Base, q.Head)
		if q.Risk == review.NoRisk {
			line += "; or, since the commit declares Risk: none, 'labdrian review exempt --risk none' it"
		}
		return line
	}
	return fmt.Sprintf("- %v", need.Err)
}

// riskCause is the text of the error of a failing trailer after its first "commit <head>: ", and the whole text when it
// holds none: the cause as S6 (a) words it.
func riskCause(head string, err error) string {
	text := err.Error()
	if _, after, found := strings.Cut(text, "commit "+head+": "); found {
		return after
	}
	return text
}
