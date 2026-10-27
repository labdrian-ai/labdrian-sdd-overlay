package main

// review subcommand: 'review bind --base <rev> --head <rev> [--cwd <repo>]' computes the candidate of a
// review from git and makes the detached worktree a reviewer reads, 'review record --feature <name>
// --base <rev> --head <rev> [--result <file>|-] [--cwd <repo>]' holds the reply of a reviewer to the candidate
// it recomputes and stores it under odd/review-results/<feature>/, and 'review verify --feature <name>
// --stored <result> [--verdicts <file>|-] [--cwd <repo>]' holds the reply of the verifier to a stored result,
// decides whether the review blocks and stores the reply beside the result. 'review check --feature <name>
// [--head <rev>] [--cwd <repo>]' is the check of the delivery: it recomputes the candidate of the head from git
// and refuses unless the results stored for it still hold and clear it. The rules are the use cases of
// review/app over the adapters review/gitexec (git) and review/fsstore (the results directory); this file
// parses the command line, wires them and says what they did. No verb launches an agent or dispatches
// work: a reply is a file somebody else produced.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/gitexec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// reviewGitTimeout stops one git call of the review verbs that runs longer. Every call is local and takes
// milliseconds, the checkout of a worktree of a large repository seconds; a git past five minutes is stuck,
// and the verb reports it as a git that could not answer instead of hanging.
const reviewGitTimeout = 5 * time.Minute

// reviewGitMaxOutput bounds what the program holds of one stream of one git call. The only large answer is
// the diff of a candidate, and a candidate whose diff does not fit is not one a reviewer reads; 256 MiB stops a
// runaway before it fills memory.
const reviewGitMaxOutput = 256 << 20

// runReview implements the 'review <verb>' subcommand. Every exit is followed by a return, because tests hand
// in an exit that ends nothing.
func runReview(p process, d deps, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(p.stderr, "error: review requires a verb: bind, record, verify, check")
		p.exit(1)
		return
	}
	switch args[0] {
	case "bind":
		runReviewBind(p, d, args[1:])
	case "record":
		runReviewRecord(p, d, args[1:])
	case "verify":
		runReviewVerify(p, d, args[1:])
	case "check":
		runReviewCheck(p, d, args[1:])
	default:
		fmt.Fprintf(p.stderr, "error: review: unknown verb %q (expected bind, record, verify or check)\n", args[0])
		p.exit(1)
	}
}

// reviewGitOptions is what the git adapter of the review verbs is told of the process: its environment, of
// which the adapter removes the variables of git, and the longest one start of git may run.
func reviewGitOptions(d deps) gitexec.Options {
	return gitexec.Options{Environ: d.environment(), Timeout: reviewGitTimeout}
}

// newReviewGit is the git of the review verbs: the git of the machine, started by the process adapter under
// reviewGitOptions.
func newReviewGit(d deps) gitexec.Git {
	return gitexec.New(execrunner.New().WithMaxOutput(reviewGitMaxOutput), reviewGitOptions(d))
}

// newReviewService is the use cases of the review verbs over the git of the machine and the results directory
// of the working tree.
func newReviewService(d deps) reviewapp.Service {
	return reviewServiceOver(newReviewGit(d), fsstore.New())
}

// reviewServiceOver is the use cases of the review verbs over the given git and results directory. They wait,
// between the tries to take a free number for a result, on the clock of the machine: review/app is in the
// application ring, where nothing reads the clock, so the program that composes it hands the clock over here.
func reviewServiceOver(git reviewapp.Git, results reviewapp.Results) reviewapp.Service {
	return reviewapp.New(git, results).WithSleep(time.Sleep)
}

// reviewFlags are the parsed flags of the review verbs. A verb takes some of them.
type reviewFlags struct {
	base, head, feature, result, stored, verdicts, cwd string
	// given says which flags the command line gave, with whatever value, an empty one included. A flag that is
	// optional and has a default tells an empty value that was given from one that was not.
	given map[string]bool
}

// parseReviewFlags reads args, the flags the verb takes (each with a value, once) and nothing else. It fails
// loud on an unknown flag, a flag without its value, a flag given twice, or a positional argument, and does
// not guess: a command line it cannot read is not run.
func parseReviewFlags(args []string, takes ...string) (reviewFlags, error) {
	var f reviewFlags
	given := map[string]bool{}
	f.given = given
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !slices.Contains(takes, a) {
			if strings.HasPrefix(a, "-") {
				return f, fmt.Errorf("unknown flag %q", a)
			}
			return f, fmt.Errorf("unexpected argument %q", a)
		}
		if i+1 >= len(args) {
			return f, fmt.Errorf("%s requires a value", a)
		}
		i++
		if given[a] {
			return f, fmt.Errorf("%s given twice", a)
		}
		given[a] = true
		switch a {
		case "--base":
			f.base = args[i]
		case "--head":
			f.head = args[i]
		case "--feature":
			f.feature = args[i]
		case "--result":
			f.result = args[i]
		case "--stored":
			f.stored = args[i]
		case "--verdicts":
			f.verdicts = args[i]
		case "--cwd":
			f.cwd = args[i]
		}
	}
	return f, nil
}

// has reports whether the command line gave the flag.
func (f reviewFlags) has(name string) bool { return f.given[name] }

// require fails with the first of the named flags that has no value. A name it does not know is a mistake of
// the caller, and says so rather than reading as a flag that was left out.
func (f reviewFlags) require(names ...string) error {
	values := map[string]string{"--base": f.base, "--head": f.head, "--feature": f.feature, "--stored": f.stored}
	for _, name := range names {
		value, known := values[name]
		if !known {
			return fmt.Errorf("review: require: %s is not a flag that can be required", name)
		}
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

// reviewPath is path as an absolute one: a relative path is relative to the working directory of the process,
// which the deps say, and --cwd, when it is given, is the repository the verb runs in.
func reviewPath(d deps, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	wd, err := d.workingDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine the working directory: %w", err)
	}
	if path == "" {
		return wd, nil
	}
	return filepath.Join(wd, path), nil
}

// reviewBindingJSON is what 'review bind' prints. The first five members are the binding the role chain
// holds; the rest says how big the diff is and whether the worktree was made now.
type reviewBindingJSON struct {
	Base            string   `json:"base"`
	Head            string   `json:"head"`
	DiffSHA256      string   `json:"diff_sha256"`
	Paths           []string `json:"paths"`
	Worktree        string   `json:"worktree"`
	WorktreeCreated bool     `json:"worktree_created"`
	DiffBytes       int      `json:"diff_bytes"`
	DiffLimitBytes  int      `json:"diff_limit_bytes"`
	OverLimit       bool     `json:"over_limit"`
	Note            string   `json:"note,omitempty"`
}

// runReviewBind implements 'review bind --base --head [--cwd]'.
func runReviewBind(p process, d deps, args []string) {
	fail := func(code int, err error) {
		fmt.Fprintf(p.stderr, "error: review bind: %v\n", err)
		p.exit(code)
	}
	f, err := parseReviewFlags(args, "--base", "--head", "--cwd")
	if err == nil {
		err = f.require("--base", "--head")
	}
	if err != nil {
		fail(1, err)
		return
	}
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		fail(1, err)
		return
	}
	bound, err := newReviewService(d).Bind(dir, f.base, f.head)
	if err != nil {
		fail(1, err)
		return
	}
	c := bound.Candidate
	out := reviewBindingJSON{
		Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Paths: c.Paths,
		Worktree: bound.Worktree, WorktreeCreated: bound.Created,
		DiffBytes: c.DiffBytes, DiffLimitBytes: review.DiffLimitBytes, OverLimit: c.OverLimit(),
	}
	if c.OverLimit() {
		out.Note = fmt.Sprintf("the diff is %d bytes, over the %d-byte limit of one launch: the candidate is bound and the worktree is made, "+
			"and the parent splits the candidate into one launch per commit (skills/role-chain/SKILL.md)", c.DiffBytes, review.DiffLimitBytes)
	}
	if err := writeReviewJSON(p, out); err != nil {
		fail(1, fmt.Errorf("the worktree is at %s, but the binding could not be written: %w", bound.Worktree, err))
		return
	}
	p.exit(0)
}

// runReviewRecord implements 'review record --feature --base --head [--result] [--cwd]'. A reply that is
// refused is exit 2 and nothing is written; any other failure is exit 1.
func runReviewRecord(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "record", err) }
	f, err := parseReviewFlags(args, "--feature", "--base", "--head", "--result", "--cwd")
	if err == nil {
		err = f.require("--feature", "--base", "--head")
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
	reply, err := readReviewReply(p, d, f.result)
	if err != nil {
		fail(err)
		return
	}
	recorded, err := newReviewService(d).Record(dir, f.feature, f.base, f.head, reply)
	if err != nil {
		fail(err)
		return
	}
	reportRecorded(p, recorded)
}

// reportRecorded tells what record stored and hands the skeleton of the record on stdout. A result whose
// directory could not be flushed is stored all the same, and the person is told not to record it again.
func reportRecorded(p process, recorded reviewapp.Recorded) {
	fmt.Fprintf(p.stderr, "review record: stored %s (sha256 %s)\n", recorded.Path, recorded.SHA256)
	if recorded.NotFlushed != nil {
		fmt.Fprintf(p.stderr, "review record: warning: its directory could not be flushed, so the result may not survive a crash of the machine; it is stored, do not record it again: %v\n", recorded.NotFlushed)
	}
	fmt.Fprintf(p.stderr, "review record: stdout is the skeleton of the record for 'roles append --stdin'; the parent of the chain fills %s\n",
		strings.Join(review.ParentFields(), ", "))
	if err := writeReviewJSON(p, recorded.Handoff); err != nil {
		reviewFailed(p, "record", fmt.Errorf("the result is stored at %s, but the handoff could not be written: %w", recorded.Path, err))
		return
	}
	p.exit(0)
}

// runReviewVerify implements 'review verify --feature --stored [--verdicts] [--cwd]'. A reply that is refused
// is exit 2 and nothing is written; any other failure, a spent correction round included, is exit 1.
func runReviewVerify(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "verify", err) }
	f, err := parseReviewFlags(args, "--feature", "--stored", "--verdicts", "--cwd")
	if err == nil {
		err = f.require("--feature", "--stored")
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
	reply, err := readReviewReply(p, d, f.verdicts)
	if err != nil {
		fail(err)
		return
	}
	verified, err := newReviewService(d).Verify(dir, f.feature, f.stored, reply)
	if err != nil {
		fail(err)
		return
	}
	reportVerified(p, verified)
}

// decisionSays is what verify says of each outcome of the verdicts.
var decisionSays = map[reviewverdict.Outcome]string{
	reviewverdict.Blocks:     "the review blocks: the record goes to the builder, and the fix is reviewed against the same base (one correction round)",
	reviewverdict.Unverified: "the review is not clear: a check did not run, so nobody is next yet",
	reviewverdict.Clear:      "the review is clear: the record goes to the delivery",
}

// reportVerified tells what verify stored and decided, and hands the skeleton of the record on stdout. A
// verdict whose directory could not be flushed is stored all the same, and the person is told not to verify it
// again.
func reportVerified(p process, v reviewapp.Verified) {
	fmt.Fprintf(p.stderr, "review verify: stored %s (sha256 %s)\n", v.Path, v.SHA256)
	if v.NotFlushed != nil {
		fmt.Fprintf(p.stderr, "review verify: warning: its directory could not be flushed, so the verdict may not survive a crash of the machine; it is stored, do not verify it again: %v\n", v.NotFlushed)
	}
	fmt.Fprintf(p.stderr, "review verify: %s\n", decisionSays[v.Decision.Outcome])
	for _, list := range []struct {
		label string
		ids   []string
	}{{"blocking", v.Decision.Blocking}, {"unverified", v.Decision.Unverified}, {"fix before delivery", v.Decision.Fix}, {"carry", v.Decision.Carry}} {
		if len(list.ids) > 0 {
			fmt.Fprintf(p.stderr, "review verify: %s: %s\n", list.label, strings.Join(list.ids, ", "))
		}
	}
	fills := review.ParentFields() // a new slice: deleting from it changes no other caller's
	if v.Handoff.ToRole != "" {
		fills = slices.DeleteFunc(fills, func(field string) bool { return field == "to_role" })
	}
	fmt.Fprintf(p.stderr, "review verify: stdout is the skeleton of the record for 'roles append --stdin'; the parent of the chain fills %s\n", strings.Join(fills, ", "))
	if err := writeReviewJSON(p, v.Handoff); err != nil {
		reviewFailed(p, "verify", fmt.Errorf("the verdict is stored at %s, but the handoff could not be written: %w", v.Path, err))
		return
	}
	p.exit(0)
}

// reviewCheckJSON is what 'review check' prints for a head that is cleared for delivery: the head as a full
// object name, the outcome, and the stored results that bind it, by launch.
type reviewCheckJSON struct {
	Head    string              `json:"head"`
	Outcome string              `json:"outcome"`
	Results []reviewCheckedJSON `json:"results"`
}

// reviewCheckedJSON is one stored result that binds the head, with the files used and the base and the digest of
// the diff that git gave when the check recomputed them, which the stored binding equals. fix and carry are the
// findings to be fixed before delivery and to be carried as follow-ups; they are left out when there are none.
type reviewCheckedJSON struct {
	Result     string   `json:"result"`
	Verdict    string   `json:"verdict,omitempty"`
	Base       string   `json:"base"`
	DiffSHA256 string   `json:"diff_sha256"`
	Fix        []string `json:"fix,omitempty"`
	Carry      []string `json:"carry,omitempty"`
}

// runReviewCheck implements 'review check --feature [--head] [--cwd]', the check of the delivery. A head that the
// stored results clear is exit 0, and stdout names the files used. Anything that keeps a head from delivery, and
// any failure to tell, is exit 1 with the reason on stderr and nothing on stdout. The head is HEAD when --head is
// not given; an empty one that was given is not the default.
func runReviewCheck(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "check", err) }
	f, err := parseReviewFlags(args, "--feature", "--head", "--cwd")
	if err == nil {
		err = f.require("--feature")
	}
	head := "HEAD"
	if err == nil && f.has("--head") {
		head, err = f.head, f.require("--head")
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
	checked, err := newReviewService(d).Check(dir, f.feature, head)
	if err != nil {
		fail(err)
		return
	}
	reportChecked(p, checked)
}

// reportChecked hands the files used on stdout. When they cannot be written the parent never received that the
// head is cleared, and the verb says so with exit 1.
func reportChecked(p process, checked reviewapp.Checked) {
	out := reviewCheckJSON{Head: checked.Head, Outcome: string(reviewverdict.Clear)}
	for _, r := range checked.Results {
		out.Results = append(out.Results, reviewCheckedJSON{
			Result: r.Result, Verdict: r.Verdict, Base: r.Candidate.Base, DiffSHA256: r.Candidate.DiffSHA256,
			Fix: r.Decision.Fix, Carry: r.Decision.Carry,
		})
	}
	if err := writeReviewJSON(p, out); err != nil {
		reviewFailed(p, "check", fmt.Errorf("the head is cleared, but the answer could not be written: %w", err))
		return
	}
	p.exit(0)
}

// reviewFailed is how record, verify and check end on an error: the error on stderr, and exit 2 for a reply that
// was refused, 1 for anything else.
func reviewFailed(p process, verb string, err error) {
	code := 1
	var refused *reviewapp.ReplyRefusedError
	if errors.As(err, &refused) {
		code = 2
	}
	fmt.Fprintf(p.stderr, "error: review %s: %v\n", verb, err)
	p.exit(code)
}

// readReviewReply reads the reply of the reviewer or of the verifier from the file at path, or from stdin when
// path is empty or "-". It reads at most one byte past review.MaxReplyBytes, which is enough for the use case
// to refuse a reply that is too large, and never the rest.
func readReviewReply(p process, d deps, path string) ([]byte, error) {
	var r io.Reader = p.stdin
	if path != "" && path != "-" {
		abs, err := reviewPath(d, path)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		r = file
	}
	return io.ReadAll(io.LimitReader(r, review.MaxReplyBytes+1))
}

// writeReviewJSON prints v as indented JSON followed by a newline on standard output. It returns what stopped
// it: the value could not be encoded, or standard output refused the bytes (a pipe whose reader has gone away,
// a full disk). Ending the verb, and saying what it had done by then, is the caller's.
func writeReviewJSON(p process, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the JSON: %w", err)
	}
	if _, err = p.stdout.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write to stdout: %w", err)
	}
	return nil
}
