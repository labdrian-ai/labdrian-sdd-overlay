package main

// review subcommand, verb 'stop-hook': the Claude Code Stop hook that blocks the stop of an agent, once for each session,
// commit and need, when a commit of its branch misses anything 'review check' asks of it: a Risk trailer, a sweep record,
// the sweeper's answer to a fired sweep, a polisher record for a declared item, a review (own-rdd T6a, T6b6, S6, L18). When no
// commit misses anything it asks, once for each session and result, for the classification of the stored results of the
// branch whose kept findings docs/wiki/evidence.md does not hold yet (S6, T4w). It is an internal hook command, not one
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
//     stopping. What the hook could not tell is one line on stderr, with CR, LF, VT, FF and the UTF-8 forms of NEL, LS and
//     PS folded to a space, the control bytes written as \xHH (C0 but those four, DEL, a lone byte 0x80-0x9F and the UTF-8
//     form of U+0080-U+009F but NEL) and every other byte kept (oneLine), which Claude Code shows only in verbose mode; a
//     block can come with such a line, the block on stdout and the line on stderr, both with exit 0. Input it cannot use is
//     silent on both streams, and so is every case with nothing to ask.
//
// It asks git and reads the working tree, and writes one marker in the state home when it blocks a stop (review/fsstore's
// Asked). It binds nothing, makes no worktree and stores no result: the agent runs the command the block names.

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/evidencesh"
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
			fmt.Fprintf(p.stderr, "review %s: internal error: %s\n", reviewStopVerb, oneLine(fmt.Sprint(r)))
			p.exit(0)
		}
	}()
	out, err := stopHookAnswer(p, d)
	if err != nil {
		fmt.Fprintf(p.stderr, "review %s: %s\n", reviewStopVerb, oneLine(err.Error()))
	}
	if len(out) > 0 {
		_, _ = p.stdout.Write(out) // nothing to do if the write fails: the stop happens.
	}
	p.exit(0)
}

// oneLine is text with each run of line breaks made one space, and the runs at its start and its end dropped: what the
// hook prints on stderr, which is one line. The line breaks are those of lineBreakAt, found first; then a control, which
// controlAt names, is written as \x and two lowercase hex digits for each of its bytes, so the escape is ASCII; every other
// byte is written as it is, invalid UTF-8 included, and a valid UTF-8 character of more than one byte is written whole,
// so a continuation byte inside it is not mistaken for a lone one. An escape is not a line break: it is kept at the start
// of the text.
func oneLine(text string) string {
	var b strings.Builder
	broken := false
	for i := 0; i < len(text); {
		if n := lineBreakAt(text, i); n > 0 {
			broken = true
			i += n
			continue
		}
		if broken && b.Len() > 0 {
			b.WriteByte(' ')
		}
		broken = false
		if n := controlAt(text, i); n > 0 {
			for _, c := range []byte(text[i : i+n]) {
				fmt.Fprintf(&b, "\\x%02x", c)
			}
			i += n
			continue
		}
		_, n := utf8.DecodeRuneInString(text[i:]) // 1 for ASCII and for a byte that is not UTF-8
		b.WriteString(text[i : i+n])
		i += n
	}
	return b.String()
}

// controlAt is the length in bytes of the control that starts at text[i], and 0 when none does: a C0 byte (0x00-0x1F), DEL
// (0x7F), a byte 0x80-0x9F that starts no valid UTF-8 character, and the UTF-8 form of U+0080-U+009F (C2 80 to C2 9F). The
// ranges are written out, not asked of unicode.IsControl. It does not ask whether the control is a line break (CR, LF, VT,
// FF, NEL): oneLine asks lineBreakAt first, so those never reach it, and a caller that asks it first escapes them.
func controlAt(text string, i int) int {
	b := text[i]
	switch {
	case b <= 0x1F, b == 0x7F, b >= 0x80 && b <= 0x9F:
		return 1
	case b == 0xC2 && i+1 < len(text) && text[i+1] >= 0x80 && text[i+1] <= 0x9F:
		return 2
	}
	return 0
}

// lineBreakAt is the length in bytes of the line break that starts at text[i], and 0 when none does: CR, LF, VT and FF
// (one byte), and the UTF-8 forms of NEL (U+0085, C2 85), LS (U+2028, E2 80 A8) and PS (U+2029, E2 80 A9), the mandatory
// breaks of Unicode UAX #14. A lone byte 0x85 is not NEL: the hook does not know the encoding of a text that is not UTF-8.
func lineBreakAt(text string, i int) int {
	switch text[i] {
	case '\r', '\n', '\v', '\f':
		return 1
	case 0xC2:
		if strings.HasPrefix(text[i:], "\xc2\x85") {
			return 2
		}
	case 0xE2:
		if strings.HasPrefix(text[i:], "\xe2\x80\xa8") || strings.HasPrefix(text[i:], "\xe2\x80\xa9") {
			return 3
		}
	}
	return 0
}

// stopHookAnswer reads the hook input and returns what the hook prints on stdout: the block of the stop, or nothing. Input
// that cannot be read or used is nothing, in silence, and so is the input of another event; what the hook could not tell
// is an error with no block, because a block that cannot be backed by the stores or by git would be asked at every stop.
// A block can come with an error (a result the pass refused, named beside the ruling): both are returned and both are
// printed, because a refusal returned beside a ruling is recorded as named. The classification question runs the evidence
// store deployed under HOME (evidenceScript) through evidencesh.
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
	classified := evidencesh.New(execrunner.New().WithMaxOutput(evidenceScriptMaxOutput), evidencesh.Options{Script: evidenceScript(d), Environ: d.environment()})
	ruling, err := newReviewService(d).WithAsked(asked).WithClassified(classified).Stop(dir, in.SessionID, in.Continuing)
	if !ruling.Block {
		return nil, err
	}
	out, encErr := hookwire.StopReply{Block: true, Reason: stopReason(ruling)}.Encode()
	switch {
	case encErr == nil:
		return out, err
	case err == nil:
		return nil, fmt.Errorf("encode the block: %w", encErr)
	}
	return nil, fmt.Errorf("%w; encode the block: %w", err, encErr)
}

// evidenceScriptMaxOutput bounds what the evidence script may print on a stream: its answer is four bytes for each result
// at most, and an error line is short.
const evidenceScriptMaxOutput = 1 << 20

// evidenceScript is the path of the evidence.sh the Claude Code skills tree holds: <HOME>/.claude/skills/_shared/evidence/
// evidence.sh, where `labdrian apply` deploys the evidence store by the claude-skill route (T20). HOME is read through the
// deps' environment; unset or relative it makes a relative path, which the adapter refuses at the call, so the review
// question never depends on it and the error appears only at a stop that has a result with kept findings.
func evidenceScript(d deps) string {
	return filepath.Join(d.env("HOME"), ".claude", "skills", "_shared", "evidence", "evidence.sh")
}

// stopReason words the question of a block for the agent that reads it (own-rdd T6b6c, S6 e): for the one commit asked,
// one line for each thing it misses, in the order review.Needs lists them (a Risk trailer, a sweep, the sweeper's answer,
// the polisher records, the review), each with the exact command that answers it, and what counts as a record. The
// question of a repository that tells no default branch has its own reason. <feature> is left for the agent to fill, as
// the hook reads the records of every feature of the tree.
func stopReason(r reviewapp.Ruling) string {
	if len(r.Classify) > 0 {
		return classifyReason(r.Classify)
	}
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
	return fmt.Sprintf("Commit %s of branch %s has unfinished review steps. Before you stop, for that commit alone (base %s):\n\n%s\n\n"+
		"<feature> is the feature whose odd/tasks/<feature>.md holds the task of the commit. A record counts once it is stored and binds the commit, "+
		"whatever its outcome; 'labdrian review check' judges the outcome. The stop hook asks for each thing a commit misses once for each session and commit.",
		q.Head, q.Branch, q.Base, strings.Join(lines, "\n"))
}

// classifyReason words the block that asks for the classification of results (S6, L67, L84): a block with no review
// question, so it names no branch or commit to review. Each line names the result, its commit and its kept findings, and
// the command that lists them; <feature> and <stored> are the parts of the identifier before and after its first "/".
func classifyReason(results []reviewapp.KeptResult) string {
	lines := make([]string, len(results))
	for i, r := range results {
		feature, stored, _ := strings.Cut(r.Review, "/")
		lines[i] = fmt.Sprintf("- %s (commit %s, %d kept): labdrian review findings --feature %s --stored %s.json", r.Review, head12(r.Head), r.Kept, feature, stored)
	}
	return "Review results of the branch hold findings that docs/wiki/evidence.md does not hold yet:\n\n" + strings.Join(lines, "\n") + "\n\n" +
		"Run the Classify step of the role-chain skill for each. The stop hook names each result once for each session, and names it again beside a result it has not named yet."
}

// head12 is the first 12 characters of a commit name, the whole name when it is shorter.
func head12(head string) string {
	if len(head) > 12 {
		return head[:12]
	}
	return head
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
