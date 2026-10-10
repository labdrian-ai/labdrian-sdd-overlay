package main

// shaper subcommand: 'shaper assess', 'shaper clearance record --stdin', and
// 'shaper guard-hook'. assess is read-only; record re-derives everything it
// binds from disk and accepts the decision only on stdin; guard-hook is the
// Claude Code PreToolUse deny guard that keeps the model from recording a
// clearance. None of it is a signature: any process running as the same OS
// user can forge a clearance record.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// shaperAuthorityNote is printed on every assessment.
const shaperAuthorityNote = "Readiness is evidence only and grants no execution authority; it never permits or dispatches work."

// runShaper implements the 'shaper <verb>' subcommand.
func runShaper(p process, d deps, args []string) {
	runShaperCore(d, args, p.stdin, p.stdout, p.stderr, p.exit)
}

// runShaperCore is the testable core of the shaper subcommand. Every exit(n)
// is followed by a return, because tests inject a non-terminating exit.
func runShaperCore(d deps, args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: shaper requires a verb: assess, clearance, guard-hook")
		exit(1)
		return
	}
	switch args[0] {
	case "assess":
		runShaperAssess(args[1:], stdout, stderr, exit)
	case "clearance":
		if len(args) < 2 || args[1] != "record" {
			fmt.Fprintln(stderr, "error: shaper clearance requires the verb: record")
			exit(1)
			return
		}
		runShaperClearanceRecord(d, args[2:], stdin, stdout, stderr, exit)
	case "guard-hook":
		runShaperGuardHook(stdin, stderr, exit)
	default:
		fmt.Fprintf(stderr, "error: shaper: unknown verb %q (expected assess, clearance, or guard-hook)\n", args[0])
		exit(1)
	}
}

// shaperOpts are the parsed shaper flags.
type shaperOpts struct {
	root, handoff, goal string
	view, stdin         bool
}

// parseShaperArgs parses the source flags plus, when allowed, --view or
// --stdin. It fails loud on an unknown flag, a flag without its value, a
// positional argument, or a missing required flag, so record content can
// never arrive through argv.
func parseShaperArgs(args []string, allowView, allowStdin bool) (shaperOpts, error) {
	var o shaperOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root", "--handoff", "--goal":
			if i+1 >= len(args) {
				return o, fmt.Errorf("%s requires a value", a)
			}
			i++
			switch a {
			case "--root":
				o.root = args[i]
			case "--handoff":
				o.handoff = args[i]
			case "--goal":
				o.goal = args[i]
			}
		case "--view":
			if !allowView {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			o.view = true
		case "--stdin":
			if !allowStdin {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			o.stdin = true
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			return o, fmt.Errorf("unexpected argument (record content is accepted only on stdin)")
		}
	}
	for _, f := range []struct{ name, value string }{{"--root", o.root}, {"--handoff", o.handoff}, {"--goal", o.goal}} {
		if f.value == "" {
			return o, fmt.Errorf("%s is required", f.name)
		}
	}
	if !filepath.IsAbs(o.root) {
		return o, fmt.Errorf("--root must be an absolute path, got %q", o.root)
	}
	return o, nil
}

// loadShaperInput resolves the root the command was given and has the shaper
// read the handoff and Goal strictly inside it and observe the worktree
// (shaper.LoadReadinessInput decides what a source that cannot be parsed, or
// cannot be read, becomes).
func loadShaperInput(o shaperOpts) (shaper.ReadinessInput, []string, error) {
	resolved, err := filepath.EvalSymlinks(o.root)
	if err != nil {
		return shaper.ReadinessInput{}, nil, fmt.Errorf("resolve --root: %w", err)
	}
	return shaper.LoadReadinessInput(newContainedSource(), gitWorktree{}, filepath.Clean(resolved), o.handoff, o.goal)
}

// gitWorktree is the shaper's WorktreeObserver over git, through the gitprov adapter.
type gitWorktree struct{}

func (gitWorktree) Observe(root string) (shaper.WorktreeProvenance, error) {
	obs, err := gitprov.Observe(root)
	if err != nil {
		return shaper.WorktreeProvenance{}, err
	}
	return worktreeProvenanceFrom(obs), nil
}

// worktreeProvenanceFrom maps what the git adapter observed into the value the
// shaper binds. HEAD is informational and Linked follows from the paths, so
// neither is carried across: the shaper never sees them, and so cannot bind them.
func worktreeProvenanceFrom(obs gitprov.Observation) shaper.WorktreeProvenance {
	return shaper.WorktreeProvenance{Toplevel: obs.Toplevel, GitDir: obs.GitDir, CommonDir: obs.CommonDir}
}

// clearanceReport describes the stored clearance lookup of one assessment.
type clearanceReport struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Path   string `json:"path,omitempty"`
}

// lookUpClearance finds the stored record for a's subject and verifies it
// against a's fresh subject, flags, and view.
func lookUpClearance(a shaper.Assessment) (*shaper.VerifiedClearance, clearanceReport) {
	if a.Subject == nil {
		return nil, clearanceReport{Status: "absent", Detail: "subject evidence is incomplete or refused, so no clearance can be looked up"}
	}
	store, err := newClearanceStore()
	if err != nil {
		return nil, clearanceReport{Status: "unavailable", Detail: err.Error()}
	}
	s := a.Subject
	path, err := store.Path(s.ProjectID, s.GoalID, s.HandoffSHA256)
	if err != nil {
		return nil, clearanceReport{Status: "unavailable", Detail: err.Error()}
	}
	data, err := store.Get(s.ProjectID, s.GoalID, s.HandoffSHA256)
	if errors.Is(err, shaper.ErrClearanceNotFound) {
		return nil, clearanceReport{Status: "absent", Path: path}
	}
	if err != nil {
		return nil, clearanceReport{Status: "unavailable", Detail: err.Error(), Path: path}
	}
	vc, err := shaper.Verify(data, *s, a.Flags, a.View)
	if err != nil {
		return nil, clearanceReport{Status: "refused", Detail: err.Error(), Path: path}
	}
	return vc, clearanceReport{Status: "verified", Path: path}
}

// runShaperAssess implements 'shaper assess'. It is read-only.
func runShaperAssess(args []string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseShaperArgs(args, true, false)
	if err != nil {
		fmt.Fprintf(stderr, "error: shaper assess: %v\n", err)
		exit(1)
		return
	}
	in, notes, err := loadShaperInput(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: shaper assess: %v\n", err)
		exit(1)
		return
	}
	a := shaper.Evaluate(in, nil)
	vc, report := lookUpClearance(a)
	if vc != nil {
		a = shaper.Evaluate(in, vc)
	}
	if !o.view {
		for _, n := range notes {
			report.Detail = strings.TrimPrefix(report.Detail+"; "+n, "; ")
		}
	}
	exit(writeShaperAssessment(stdout, stderr, a, report, o.view, in.Handoff.Handoff.Version))
}

type assessBlockerJSON struct {
	Reason string `json:"reason"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type assessFlagJSON struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Field string `json:"field"`
	Index int    `json:"index"`
	Item  string `json:"item"`
}

type assessWorktreeJSON struct {
	Toplevel  string `json:"toplevel"`
	GitDir    string `json:"git_dir"`
	CommonDir string `json:"common_dir"`
}

type assessSubjectJSON struct {
	ProjectID         string             `json:"project_id"`
	GoalID            string             `json:"goal_id"`
	HandoffSourcePath string             `json:"handoff_source_path"`
	HandoffSHA256     string             `json:"handoff_sha256"`
	GoalSourcePath    string             `json:"goal_source_path"`
	GoalSHA256        string             `json:"goal_sha256"`
	ProvenanceSHA256  string             `json:"provenance_sha256"`
	ViewSHA256        string             `json:"view_sha256"`
	Worktree          assessWorktreeJSON `json:"worktree"`
}

type assessJSON struct {
	State      string              `json:"state"`
	Blockers   []assessBlockerJSON `json:"blockers"`
	Flags      []assessFlagJSON    `json:"flags"`
	Subject    *assessSubjectJSON  `json:"subject"`
	Clearance  clearanceReport     `json:"clearance"`
	Authority  string              `json:"authority"`
	Disclosure string              `json:"disclosure,omitempty"`
}

// assessExitCode maps a state to the assess exit code: 0 ready, 3 draft,
// 2 invalid, and 1 for anything else.
func assessExitCode(s shaper.State) int {
	switch s {
	case shaper.StateReady:
		return 0
	case shaper.StateDraft:
		return 3
	case shaper.StateInvalid:
		return 2
	default:
		return 1
	}
}

// writeShaperAssessment prints a as JSON, or with view exactly its rendered
// view bytes, and returns the exit code. A view refused as unpresentable
// (it holds a terminal control or invisible formatting rune) is never
// printed: stdout stays empty, stderr names the blocker, and the exit code is
// the non-zero draft code. Whenever the state is ready it also prints
// shaper.ReadyDisclosureFor(handoffVersion) (the forgery limit, then the
// version-specific plan completeness statement): in the JSON, or on stderr
// with view so the view bytes stay exact.
func writeShaperAssessment(stdout, stderr io.Writer, a shaper.Assessment, report clearanceReport, view bool, handoffVersion int) int {
	code := assessExitCode(a.State)
	disclosure := shaper.ReadyDisclosureFor(handoffVersion)
	if view {
		if a.View == nil {
			fmt.Fprintf(stderr, "shaper assess: no presented view: subject evidence is incomplete or refused (state %s)\n", a.State)
			for _, b := range a.Blockers {
				if b.Reason == shaper.ReasonViewUnpresentable {
					fmt.Fprintf(stderr, "shaper assess: refusing to print the view: %s: %s\n", b.Reason, b.Detail)
				}
			}
		} else if _, err := stdout.Write(a.View); err != nil {
			fmt.Fprintf(stderr, "error: shaper assess: write view: %v\n", err)
			return 1
		}
		if a.State == shaper.StateReady {
			fmt.Fprintln(stderr, disclosure)
		}
		return code
	}

	out := assessJSON{
		State:     string(a.State),
		Blockers:  []assessBlockerJSON{},
		Flags:     []assessFlagJSON{},
		Clearance: report,
		Authority: shaperAuthorityNote,
	}
	for _, b := range a.Blockers {
		out.Blockers = append(out.Blockers, assessBlockerJSON{Reason: string(b.Reason), Kind: string(b.Reason.Kind()), Detail: b.Detail})
	}
	for _, f := range a.Flags {
		out.Flags = append(out.Flags, assessFlagJSON{ID: f.ID, Kind: string(f.Kind), Field: f.Field, Index: f.Index, Item: f.Item})
	}
	if s := a.Subject; s != nil {
		out.Subject = &assessSubjectJSON{
			ProjectID:         s.ProjectID,
			GoalID:            s.GoalID,
			HandoffSourcePath: s.HandoffSourcePath,
			HandoffSHA256:     s.HandoffSHA256,
			GoalSourcePath:    s.GoalSourcePath,
			GoalSHA256:        s.GoalSHA256,
			ProvenanceSHA256:  s.ProvenanceSHA256(),
			ViewSHA256:        shaper.ViewDigest(a.View),
			Worktree:          assessWorktreeJSON{Toplevel: s.Worktree.Toplevel, GitDir: s.Worktree.GitDir, CommonDir: s.Worktree.CommonDir},
		}
	}
	if a.State == shaper.StateReady {
		out.Disclosure = disclosure
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: shaper assess: %v\n", err)
		return 1
	}
	stdout.Write(append(data, '\n'))
	return code
}

// runShaperClearanceRecord implements 'shaper clearance record --stdin'. The
// decision is read only from stdin; the Subject, flags, and presented view
// are re-derived from disk, and any mismatch is refused under Verify's
// binding rules before the immutable store is written.
func runShaperClearanceRecord(d deps, args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	fail := func(format string, a ...any) {
		fmt.Fprintf(stderr, "error: shaper clearance record: "+format+"\n", a...)
		exit(1)
	}
	o, err := parseShaperArgs(args, false, true)
	if err != nil {
		fail("%v", err)
		return
	}
	if !o.stdin {
		fail("--stdin is required: the clearance decision is accepted only on stdin, never from argv")
		return
	}
	if d.agentChild {
		fail("refusing inside a gentle-pi agent child (%s=%s): no human answers its dialogs", agentChildVariable, agentChildValue)
		return
	}
	// The record bound is the domain's (shaper.MaxRecordBytes), not the CLI-wide stdin cap,
	// so every record this verb accepts is one the clearance store can load back. That the
	// two constants hold the same value today (4 MiB) is a coincidence of history, not the
	// link between them: this verb follows the record bound on purpose, and a change to the
	// CLI-wide cap alone must not move it. The tests of this verb pin it at the bound.
	data, err := io.ReadAll(io.LimitReader(stdin, shaper.MaxRecordBytes+1))
	if err != nil {
		fail("read stdin: %v", err)
		return
	}
	if len(data) > shaper.MaxRecordBytes {
		fail("stdin exceeds %d bytes", shaper.MaxRecordBytes)
		return
	}
	if strings.TrimSpace(string(data)) == "" {
		fail("stdin is empty")
		return
	}

	in, notes, err := loadShaperInput(o)
	if err != nil {
		fail("%v", err)
		return
	}
	a := shaper.Evaluate(in, nil)
	if a.Subject == nil {
		var reasons []string
		for _, b := range a.Blockers {
			reasons = append(reasons, string(b.Reason)+": "+b.Detail)
		}
		reasons = append(reasons, notes...)
		fail("no clearance subject can be derived from disk: %s", strings.Join(reasons, "; "))
		return
	}
	rec, err := shaper.CheckRecordBinding(data, *a.Subject, a.Flags, a.View)
	if err != nil {
		fail("%v", err)
		return
	}
	// A non-TUI channel is already refused by ParseRecord (inside
	// CheckRecordBinding), so an RPC-captured record never reaches the store.
	store, err := newClearanceStore()
	if err != nil {
		fail("%v", err)
		return
	}
	path, err := store.Put(data)
	if err != nil {
		fail("%v", err)
		return
	}
	fmt.Fprintf(stdout, "shaper clearance record: stored %s record at %s\n%s\n", rec.Decision, path, shaper.ForgeryDisclosure)
}

// runShaperGuardHook implements 'shaper guard-hook', the Claude Code
// PreToolUse deny guard. It fails closed: unreadable input is denied.
//
// It reads at most hookwire.MaxToolCallBytes, the bound of every guard, and one
// byte more, which is enough to see the input is over it: an input that never
// ends is not read to the end. The bound is generous, because the guard runs on
// every Bash and Write/Edit call and a capped read must not truncate a large
// unrelated payload into undecodable JSON and deny it without judging it. An
// input over the bound is denied like any input the guard cannot read, and the
// denial says the input was too large and names the bound: the guard cannot vouch for a call it was
// not given to judge, and a call that big is not a call it can afford to read.
func runShaperGuardHook(stdin io.Reader, stderr io.Writer, exit func(int)) {
	raw, err := io.ReadAll(io.LimitReader(stdin, hookwire.MaxToolCallBytes+1))
	if err != nil {
		fmt.Fprintf(stderr, "shaper guard-hook: read stdin: %v\n", err)
		exit(hookwire.ExitBlock)
		return
	}
	var verdict shaper.GuardVerdict
	call, err := hookwire.DecodeToolCall(raw)
	switch {
	case errors.Is(err, hookwire.ErrTooLarge):
		verdict = shaper.GuardTooLarge(hookwire.MaxToolCallBytes)
	case err != nil:
		verdict = shaper.GuardUnreadable()
	default:
		verdict = shaper.DecideGuard(shaper.GuardCall{Command: call.Command, FilePath: call.FilePath, NotebookPath: call.NotebookPath})
	}
	reply := hookwire.ExitReply{Block: verdict.Deny, Message: verdict.Reason}
	_, _ = stderr.Write(reply.MessageLine())
	exit(reply.Code())
}
