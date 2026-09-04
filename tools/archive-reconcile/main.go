package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// usageText names the flags this guard accepts. --repo lets it scan a
// repository other than the caller's own working directory, which is what
// makes it testable against a temp fixture directory instead of only the live
// repo it ships in. --known-gaps is documented the way the sibling
// tools/archive-anchor-gate documents its own ledger: not just that the flag
// exists, but the two properties that make the ledger a record of a gap rather
// than a permission to keep it.
const usageText = `usage: archive-reconcile [--repo <path>] [--known-gaps <path>]

  --repo PATH        repository root to scan (default: the caller's working
                     directory)
  --known-gaps PATH  ledger of changes already known to be stranded or
                     undetermined, one change directory name per line; '#'
                     starts a comment. A listed change that is stranded or
                     undetermined is reported as a known gap and does not fail
                     the run. A listed change that is neither fails the run as
                     a stale waiver, so the ledger cannot quietly outlive the
                     gap it describes. Absent or empty means no waivers.

exit codes:
  0  every active change was classified and none is stranded (known gaps aside)
  1  at least one unwaived stranded change
  2  invalid usage
  3  no complete verdict could be reached, or a waiver is stale
`

// changesRelDir and specsRelDir are the two fixed openspec locations this
// guard reads. They are relative, not absolute, because every remediation
// line this guard prints (git mv, spec promotion) is itself relative to the
// repository root — printing a temp-fixture absolute path there in the common
// case would be actively misleading to the operator running this against the
// real repo.
const changesRelDir = "openspec/changes"
const specsRelDir = "openspec/specs"
const archiveDirName = "archive"
const tasksFileName = "tasks.md"
const specFileName = "spec.md"

// Process exit codes. The split mirrors the sibling review-preflight guard
// (tools/review-preflight/main.go): 0 clean, 1 the condition this tool exists
// to catch, 2 a bad invocation, 3 no verdict could be reached.
//
// Only outcomeClean asserts that every active openspec/changes/ entry is
// either still in progress or already archived. Every path that cannot prove
// that returns 1 or 3 — never 0 — because the failure this guard exists to
// prevent is precisely a completed change that nobody notices sitting active
// forever. A guard that guessed "probably fine" when it could not read a
// tasks.md would reproduce, one layer up, the exact blind spot that let
// deterministic-verification-evidence sit active for five days and let
// anti-generic-design-runtime-wiring and gadu-portable-operator still be
// active today.
const (
	outcomeClean        = 0
	outcomeStranded     = 1
	outcomeUsage        = 2
	outcomeUndetermined = 3
)

// run answers one question: does any active entry under openspec/changes/
// (excluding archive/) have a tasks.md whose tasks are all checked? If so it
// is a stranded change — the work is done but the folder was never archived
// and its delta specs may never have been promoted. Nothing else notices this
// automatically today; it has been found three times by manual audit.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}

	// The ledger is read before the scan and a read failure is fatal, for the
	// same reason countTasks refuses to treat an unreadable tasks.md as zero
	// tasks: an unreadable ledger is a fact this guard never observed, and
	// continuing with zero waivers would turn a typo in the CI path into a
	// silent behaviour change — either a flood of "new" findings or, worse, a
	// green run whose waivers were never actually applied.
	waived, err := readKnownGaps(opts.KnownGaps)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	root, err := resolveRepoRoot(opts.Repo)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	findings, undetermined, scanned, err := scanChanges(root)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	// Waiving is keyed by change directory name — the identifier both reports
	// already print — and applies to both finding kinds, because a stranded
	// change and an undetermined one are equally real gaps and equally
	// declarable. A waived finding is still printed, on stdout as a known gap,
	// so a waiver is never invisible.
	openStranded, knownStranded := partitionStranded(findings, waived)
	openUndetermined, knownUndetermined := partitionUndetermined(undetermined, waived)
	stale := staleWaivers(waived, findings, undetermined)
	waivedCount := len(knownStranded) + len(knownUndetermined)

	for _, f := range knownStranded {
		fmt.Fprintf(stdout, "archive-reconcile: known gap %s — stranded (%d/%d tasks checked), waived by the ledger.\n", f.Name, f.TotalTasks, f.TotalTasks)
	}
	for _, u := range knownUndetermined {
		fmt.Fprintf(stdout, "archive-reconcile: known gap %s — undetermined (%s), waived by the ledger.\n", u.Name, u.Observed)
	}

	if len(stale) > 0 {
		reportStaleWaivers(stderr, opts.KnownGaps, stale)
	}

	// Both lists are printed whenever either is non-empty. Suppressing the
	// stranded list because something was undetermined would hide work the
	// scan did establish, and suppressing the undetermined list would hide
	// the fact that the stranded list is partial.
	if len(openStranded) > 0 {
		reportStranded(stderr, openStranded)
	}
	if len(openUndetermined) > 0 {
		reportUndetermined(stderr, openUndetermined, len(openStranded))
	}

	// EXIT PRECEDENCE, with waivers.
	//
	// A stale waiver takes exit 3, ahead of everything else. It is a defect in
	// the LEDGER rather than in the repository under scan, and exit 1 is
	// reserved for a claim about the repository ("here are the stranded
	// changes"), so reporting a ledger defect as 1 would put a sentence in the
	// guard's mouth that is not true of the scanned tree. What exit 3 already
	// means is "this run's answer is not one you can rely on as complete",
	// which is exactly the state a stale waiver creates: the waiver set no
	// longer describes this repository, so every suppression it performed on
	// this run is suspect. Ranking it first also guarantees the property the
	// ledger depends on — a stale waiver can never be swallowed by an exit
	// code that means something else, because no other outcome outranks it.
	//
	// Among UNWAIVED findings the rule this guard already established still
	// holds: undetermined outranks stranded. Exit 1 asserts that the scan
	// classified every active change and these are the complete-but-unarchived
	// ones; when even one change could not be classified, that stranded list is
	// knowingly incomplete, and returning 1 would assert a completeness this
	// guard does not have — the same overclaim its countTasks and ambiguousLine
	// comments already refuse at the file and line levels.
	if len(stale) > 0 {
		return outcomeUndetermined
	}
	if len(openUndetermined) > 0 {
		return outcomeUndetermined
	}
	if len(openStranded) > 0 {
		return outcomeStranded
	}

	// Two green summaries, because they assert different things. With no
	// waivers applied, every active change really was classified and none was
	// stranded — today's exact sentence, unchanged. With waivers applied, that
	// sentence would be false: the waived changes are gaps, and the
	// undetermined ones among them were never classified at all. So the
	// summary states the count instead of claiming more than it checked, and a
	// green run can never hide how many gaps it is carrying.
	if waivedCount == 0 {
		fmt.Fprintf(stdout, "archive-reconcile: clean — %d active change(s) checked under %s, all classified, none stranded.\n", scanned, changesRelDir)
		return outcomeClean
	}
	fmt.Fprintf(stdout, "archive-reconcile: ok — %d active change(s) checked under %s, %d known gap(s) waived by %s, no unwaived finding.\n", scanned, changesRelDir, waivedCount, opts.KnownGaps)
	return outcomeClean
}

// options is the parsed command line.
type options struct {
	Repo      string
	KnownGaps string
}

// partitionStranded splits the stranded findings into the ones that must fail
// the run and the ones the ledger already declares.
func partitionStranded(findings []strandedChange, waived map[string]bool) (open, known []strandedChange) {
	for _, f := range findings {
		if waived[f.Name] {
			known = append(known, f)
			continue
		}
		open = append(open, f)
	}
	return open, known
}

// partitionUndetermined does the same for the changes that could not be
// classified.
func partitionUndetermined(undetermined []undeterminedChange, waived map[string]bool) (open, known []undeterminedChange) {
	for _, u := range undetermined {
		if waived[u.Name] {
			known = append(known, u)
			continue
		}
		open = append(open, u)
	}
	return open, known
}

// staleWaivers returns the ledger entries that are neither stranded nor
// undetermined on this run, sorted. This is the property that makes the ledger
// a record rather than a suppression: a gap that closes forces its line to be
// deleted, so the list cannot rot into a permanent exemption by being
// forgotten.
func staleWaivers(waived map[string]bool, findings []strandedChange, undetermined []undeterminedChange) []string {
	failing := make(map[string]bool, len(findings)+len(undetermined))
	for _, f := range findings {
		failing[f.Name] = true
	}
	for _, u := range undetermined {
		failing[u.Name] = true
	}

	var stale []string
	for name := range waived {
		if !failing[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	return stale
}

// reportStaleWaivers names every ledger entry whose gap has closed and says
// plainly what to do about it: delete the line. Naming the ledger path matters
// because the defect is in that file, not in the change it names.
func reportStaleWaivers(stderr io.Writer, ledgerPath string, stale []string) {
	fmt.Fprintf(stderr, "archive-reconcile: STALE waiver(s) — %d entry(ies) in %s are neither stranded nor undetermined on this run.\n", len(stale), ledgerPath)
	for _, name := range stale {
		fmt.Fprintf(stderr, "\n  stale waiver: %s is listed as a known gap, but this run found no gap for it.\n", name)
		fmt.Fprintf(stderr, "    fix: delete the %q line from %s. The gap it recorded has closed.\n", name, ledgerPath)
	}
	fmt.Fprint(stderr, staleWaiverNote)
}

// staleWaiverNote states why a stale waiver blocks at all: the ledger exists
// only as a record of gaps that are real right now, and an entry that outlives
// its gap is the first step back to a permanent, unexamined exemption.
const staleWaiverNote = `
A known-gaps ledger is a record of a gap, never a permission to keep it. An
entry whose gap has closed is a waiver with nothing to waive: left in place it
would silently suppress a future, unrelated regression on the same change.
`

// readKnownGaps reads the ledger of already-declared gaps: one change
// directory name per line, '#' starting a comment, blank lines ignored so the
// file can explain itself. An empty path means no waivers, which is exactly
// this guard's behaviour before the flag existed. A path that cannot be read
// is an error rather than an empty waiver set — see run for why.
func readKnownGaps(path string) (map[string]bool, error) {
	waived := map[string]bool{}
	if path == "" {
		return waived, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("known-gaps ledger %s could not be read: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		waived[line] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("known-gaps ledger %s could not be parsed: %w", path, err)
	}
	return waived, nil
}

// parseArgs parses the guard's only flag, mirroring the sibling
// review-preflight's parseArgs shape (ContinueOnError plus a discarded
// FlagSet output, so run owns every byte written to stderr). Leftover
// positional arguments are an error rather than being ignored: this tool
// takes no subcommand, and silently accepting one would let an operator's
// typo run as a bare invocation and report a verdict about something they did
// not ask for.
func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("archive-reconcile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repo := fs.String("repo", "", "repository root to scan (default: the caller's working directory)")
	knownGaps := fs.String("known-gaps", "", "ledger of changes already known to be stranded or undetermined")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q; this guard takes no subcommand", fs.Arg(0))
	}
	return options{Repo: *repo, KnownGaps: *knownGaps}, nil
}

// resolveRepoRoot resolves the repository root to scan: the --repo flag when
// given, otherwise the caller's working directory. A --repo that does not
// resolve to a real directory is reported as undetermined (3), not as a usage
// error (2): the invocation itself parsed fine, and only the target it names
// could not be reached, which is the same fact review-preflight reports as 3
// when it cannot resolve the working directory it was asked to inspect.
func resolveRepoRoot(repoFlag string) (string, error) {
	root := repoFlag
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not resolve the working directory: %w", err)
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("could not resolve repository root %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("repository root %q could not be resolved: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository root %q is not a directory", abs)
	}
	return abs, nil
}

// strandedChange is one openspec/changes/ entry whose tasks.md has at least
// one checkbox and zero unchecked ones: complete, but still active.
type strandedChange struct {
	Name         string
	TotalTasks   int
	Capabilities []capabilityStatus
}

// capabilityStatus is one delta capability (a subdirectory of
// <change>/specs/) and whether its canonical openspec/specs/<name>/spec.md
// has been promoted.
type capabilityStatus struct {
	Name     string
	Promoted bool
}

// undeterminedChange is one openspec/changes/ entry whose tasks.md contained
// no bullet checkbox at all, so this guard has no completion signal to read
// and cannot say whether the change is stranded or still in progress.
type undeterminedChange struct {
	Name     string
	Observed string
}

// scanChanges walks every non-archive entry under openspec/changes/ and
// returns the stranded ones, the ones it could not classify, and the total
// number of active entries scanned (used only for the clean-case stdout
// summary).
//
// Every error here is returned rather than skipped or defaulted, per the same
// reasoning as review-preflight's parseProjection: a tasks.md this guard
// cannot read or parse is a fact it never observed, and reporting the change
// it belongs to as "not stranded" would silently reproduce the exact blind
// spot this guard exists to close.
//
// Unclassifiable changes are collected rather than returned as the first
// error: four live changes in this repository are in that state at once, and
// a guard that names all of them is worth more to an operator than one that
// dies on the first and hides the rest.
func scanChanges(root string) ([]strandedChange, []undeterminedChange, int, error) {
	changesDir := filepath.Join(root, filepath.FromSlash(changesRelDir))
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("could not read %s: %w", changesDir, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == archiveDirName {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var findings []strandedChange
	var undetermined []undeterminedChange
	for _, name := range names {
		changeDir := filepath.Join(changesDir, name)
		tasksPath := filepath.Join(changeDir, tasksFileName)

		counts, err := countTasks(tasksPath)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("change %q: %w", name, err)
		}
		if counts.Total == 0 {
			// total == 0 does NOT mean "planning-only". It means no bullet
			// checkbox was found, which is equally consistent with a complete
			// task breakdown written in a shape this parser does not read —
			// four live changes in this repository are exactly that, with
			// "**Status**: [x] done" markers or bare "### T-01" headings. This
			// guard cannot tell those apart, and picking either reading is the
			// same guess countTasks and ambiguousLine already refuse to make
			// one and two levels down. Record it as undetermined instead, with
			// the shape actually observed so the operator knows which kind of
			// unreadable it is.
			undetermined = append(undetermined, undeterminedChange{Name: name, Observed: counts.observation()})
			continue
		}
		if counts.Unchecked > 0 {
			// Still in progress.
			continue
		}

		caps, err := capabilityStatuses(root, changeDir)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("change %q: %w", name, err)
		}
		findings = append(findings, strandedChange{Name: name, TotalTasks: counts.Total, Capabilities: caps})
	}
	return findings, undetermined, len(names), nil
}

// checkboxPattern matches a well-formed task checkbox line: a "-", "*", or
// "+" bullet marker (the three CommonMark allows) followed by "[ ]", "[x]",
// or "[X]". checkboxLikePattern matches anything that looks like an attempt
// at one, so a stray "- [oops]" is caught as malformed rather than silently
// ignored or miscounted.
var checkboxPattern = regexp.MustCompile(`^\s*[-*+]\s*\[([ xX])\]`)
var checkboxLikePattern = regexp.MustCompile(`^\s*[-*+]\s*\[`)

// markdownLinkBulletPattern matches an ordinary bullet whose first token is a
// markdown link, e.g. "- [see the design](design.md)". Such a line trips
// checkboxLikePattern without being an attempted checkbox, and failing the
// whole scan on legitimate prose would make this guard unusable on real
// task files.
var markdownLinkBulletPattern = regexp.MustCompile(`^\s*[-*+]\s*\[[^\]]*\]\(`)

// indentWidth counts leading spaces, expanding tabs to the four-column stop
// CommonMark uses, so the indented-code-block boundary is measured the way
// markdown measures it rather than by raw character count.
func indentWidth(line string) int {
	w := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			w++
		case '\t':
			w += 4 - (w % 4)
		default:
			return w
		}
	}
	return w
}

// ambiguousLine is the one refusal this guard makes when a line could be a
// real task or an example and it cannot prove which. Guessing either way has
// already produced four separate silent misses in this tool's history: a
// wrong "clean" hides a stranded change, a wrong "stranded" erodes trust in
// the guard. Exit 3 says "I could not tell", which is the only honest answer
// a line-oriented reader can give about markdown block context.
func ambiguousLine(path string, lineNo int, line, why string) error {
	return fmt.Errorf("%s:%d: cannot classify %q — %s; this guard refuses to guess, so resolve the ambiguity in the file (move the example into a fenced block, or put the comment on its own line) and re-run",
		path, lineNo, strings.TrimSpace(line), why)
}

// taskCounts is what one tasks.md yielded: how many bullet task checkboxes it
// carries, how many of those are unchecked, and how many checkbox-SHAPED
// markers appeared outside a bullet ("**Status**: [x] done", a table cell).
// OtherMarkers is never a completion signal — this guard deliberately does not
// try to interpret those shapes — it exists only so a zero-total report can
// tell the operator which kind of unreadable file they are looking at.
type taskCounts struct {
	Total        int
	Unchecked    int
	OtherMarkers int
}

// observation describes, for a file with zero bullet checkboxes, what was
// actually seen. The two shapes call for different operator responses: other
// markers present means a completion convention this parser does not read,
// while nothing at all means no parseable completion marker exists.
func (c taskCounts) observation() string {
	if c.OtherMarkers > 0 {
		return fmt.Sprintf("0 bullet checkboxes; %d other [x]-shaped marker(s) present — the completion signal is in a shape this guard does not read", c.OtherMarkers)
	}
	return "0 checkboxes of any shape — this file carries no completion marker this guard can read"
}

// checkboxShapedPattern matches a bare "[x]", "[X]", or "[ ]" anywhere in a
// line. It is used only to characterise a file that produced zero bullet
// checkboxes; it never feeds Total or Unchecked.
var checkboxShapedPattern = regexp.MustCompile(`\[[ xX]\]`)

// countTasks reads one tasks.md and returns the total number of task
// checkboxes, how many are unchecked, and how many checkbox-shaped markers
// appeared outside a bullet checkbox.
//
// A missing or unreadable tasks.md is an error, not zero tasks: this guard
// has no way to distinguish "genuinely no tasks" from "the file could not be
// read" without opening it, and treating the latter as the former is exactly
// the guessing this guard exists to refuse. A malformed checkbox line is
// likewise an error rather than a skipped line, for the same reason:
// silently skipping it could turn a stranded change's last unchecked-looking
// line into an invisible false "fully checked."
//
// Lines inside a fenced code block (``` or ~~~, with any info string) or an
// HTML comment (<!-- ... -->, possibly spanning lines) are ignored entirely:
// a checkbox-shaped example in either context must never contribute to
// total or unchecked, nor trigger the malformed-checkbox error below.
func countTasks(path string) (counts taskCounts, err error) {
	f, err := os.Open(path)
	if err != nil {
		return taskCounts{}, fmt.Errorf("%s could not be read: %w", path, err)
	}
	defer f.Close()

	lineNo := 0
	inFence := false
	inComment := false
	var fenceChar byte
	var fenceLen int
	fenceOpenLine := 0
	commentOpenLine := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		if inFence {
			if isFenceClose(line, fenceChar, fenceLen) {
				inFence = false
			}
			continue
		}
		if inComment {
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}
		if ch, n, ok := fenceOpen(line); ok {
			inFence, fenceChar, fenceLen = true, ch, n
			fenceOpenLine = lineNo
			continue
		}
		// A comment delimiter sharing a line with anything checkbox-shaped is
		// ambiguous to a line-oriented reader: "- [x] done <!-- note -->" has a
		// real task, while "<!-- - [x] example -->" has none, and this scanner
		// cannot tell which half of the line the delimiter governs. Skipping the
		// whole line silently drops a real task; counting it silently invents
		// one. Refuse instead.
		if openIdx := strings.Index(line, "<!--"); openIdx >= 0 {
			if checkboxLikePattern.MatchString(line) {
				return taskCounts{}, ambiguousLine(path, lineNo, line,
					"a task checkbox and an HTML comment delimiter share this line")
			}
			if !strings.Contains(line[openIdx+len("<!--"):], "-->") {
				inComment = true
				commentOpenLine = lineNo
			}
			continue
		}

		// CommonMark treats four or more leading spaces as an indented code
		// block, so a checkbox that deep is an example, not a task — but this
		// scanner models fences and comments, not indented blocks, and cannot
		// prove which it is. Refuse rather than pick.
		if indentWidth(line) >= 4 && checkboxLikePattern.MatchString(line) {
			return taskCounts{}, ambiguousLine(path, lineNo, line,
				"a checkbox indented four or more spaces is an indented code block in CommonMark, not a task")
		}

		if m := checkboxPattern.FindStringSubmatch(line); m != nil {
			counts.Total++
			if m[1] == " " {
				counts.Unchecked++
			}
			continue
		}
		// Not a bullet checkbox. Record any checkbox-shaped marker it does
		// carry, purely so a zero-total file can be described accurately later.
		counts.OtherMarkers += len(checkboxShapedPattern.FindAllString(line, -1))
		// A bullet whose first token is a bracket is only a malformed checkbox
		// when the bracket plausibly attempts one. A markdown link such as
		// "- [see the design](design.md)" is ordinary prose and must not fail
		// the scan; anything else bracketed is genuinely undecidable here.
		if checkboxLikePattern.MatchString(line) {
			if markdownLinkBulletPattern.MatchString(line) {
				continue
			}
			return taskCounts{}, fmt.Errorf("%s:%d: malformed task checkbox %q — expected \"- [ ]\", \"* [ ]\", or \"+ [ ]\" (checked or unchecked)", path, lineNo, strings.TrimSpace(line))
		}
	}
	if err := scanner.Err(); err != nil {
		return taskCounts{}, fmt.Errorf("%s could not be parsed: %w", path, err)
	}
	// An unterminated fence or comment means every line after it was skipped,
	// so the counters describe only part of the file. Returning them would let
	// a genuinely stranded change hide behind an unclosed ``` and be reported
	// clean — the exact silent miss this guard exists to prevent, reintroduced
	// through the skipping machinery itself. Fail closed instead.
	if inFence {
		return taskCounts{}, fmt.Errorf("%s: unterminated fenced code block opened at line %d — every later line was skipped, so the task counts cannot be trusted", path, fenceOpenLine)
	}
	if inComment {
		return taskCounts{}, fmt.Errorf("%s: unterminated HTML comment opened at line %d — every later line was skipped, so the task counts cannot be trusted", path, commentOpenLine)
	}
	return counts, nil
}

// fenceOpen reports whether line opens a fenced code block: three or more
// identical backtick or tilde characters after leading whitespace, with an
// optional info string (e.g. "bash"). It returns the fence character and how
// many were used, so the matching close can require at least that many.
func fenceOpen(line string) (ch byte, n int, ok bool) {
	// CommonMark allows at most three spaces of indentation before a fence;
	// four or more makes it an indented code block, not a fence opener.
	// Treating a deeply indented line as a fence would let a coincidental
	// open/close pair swallow real tasks between them.
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return 0, 0, false
	}
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return 0, 0, false
	}
	first := trimmed[0]
	if first != '`' && first != '~' {
		return 0, 0, false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == first {
		i++
	}
	if i < 3 {
		return 0, 0, false
	}
	return first, i, true
}

// isFenceClose reports whether line closes a fence that was opened with
// fenceChar repeated fenceLen times: the trimmed line must consist of
// nothing but fenceChar, repeated at least fenceLen times.
func isFenceClose(line string, fenceChar byte, fenceLen int) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == fenceChar {
		i++
	}
	return i >= fenceLen && i == len(trimmed)
}

// capabilityStatuses lists the delta capabilities of one change (the
// subdirectories of <changeDir>/specs/) and reports whether each has been
// promoted into openspec/specs/<name>/spec.md.
//
// A change with no specs/ subdirectory at all has zero delta capabilities,
// which is not an error: not every change necessarily carries spec deltas,
// and the change is still reported as stranded on task completion alone.
func capabilityStatuses(root, changeDir string) ([]capabilityStatus, error) {
	changeSpecsDir := filepath.Join(changeDir, "specs")
	entries, err := os.ReadDir(changeSpecsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read %s: %w", changeSpecsDir, err)
	}

	var caps []capabilityStatus
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specPath := filepath.Join(root, filepath.FromSlash(specsRelDir), e.Name(), specFileName)
		_, statErr := os.Stat(specPath)
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("could not check %s: %w", specPath, statErr)
		}
		caps = append(caps, capabilityStatus{Name: e.Name(), Promoted: statErr == nil})
	}
	sort.Slice(caps, func(i, j int) bool { return caps[i].Name < caps[j].Name })
	return caps, nil
}

// reportStranded writes one blocking diagnostic per stranded change. It names
// the change, its task count, and per-capability promotion status, and it
// states which of the two known-worse shapes applies: capabilities all
// promoted (only the folder move was skipped) versus one or more unpromoted
// (the canonical spec set is itself incomplete). The remediation is the exact
// action to take, not a vague "please archive."
func reportStranded(stderr io.Writer, findings []strandedChange) {
	fmt.Fprintf(stderr, "archive-reconcile: STRANDED change(s) — %d change(s) under %s are complete (every task checked) but were never archived.\n", len(findings), changesRelDir)

	for _, f := range findings {
		fmt.Fprintf(stderr, "\n  %s — %d/%d tasks checked, not archived.\n", f.Name, f.TotalTasks, f.TotalTasks)

		unpromoted := 0
		if len(f.Capabilities) == 0 {
			fmt.Fprint(stderr, "    no delta capabilities under specs/ — nothing to promote-check.\n")
		} else {
			for _, c := range f.Capabilities {
				if c.Promoted {
					fmt.Fprintf(stderr, "    - %s: promoted (%s/%s/%s)\n", c.Name, specsRelDir, c.Name, specFileName)
				} else {
					unpromoted++
					fmt.Fprintf(stderr, "    - %s: NOT promoted (%s/%s/%s is missing)\n", c.Name, specsRelDir, c.Name, specFileName)
				}
			}
			if unpromoted > 0 {
				fmt.Fprintf(stderr, "    worse: %d/%d capabilities are unpromoted — the canonical spec set is incomplete, not just the folder move.\n", unpromoted, len(f.Capabilities))
			} else {
				fmt.Fprint(stderr, "    all capabilities are promoted — only the archive move was skipped.\n")
			}
		}

		fmt.Fprint(stderr, "    fix:\n")
		step := 1
		if unpromoted > 0 {
			for _, c := range f.Capabilities {
				if !c.Promoted {
					fmt.Fprintf(stderr, "      %d. promote %s/%s/specs/%s/%s into %s/%s/%s\n", step, changesRelDir, f.Name, c.Name, specFileName, specsRelDir, c.Name, specFileName)
					step++
				}
			}
		}
		fmt.Fprintf(stderr, "      %d. archive the change: git mv %s/%s %s/%s/<dated-name>-%s\n", step, changesRelDir, f.Name, changesRelDir, archiveDirName, f.Name)
	}

	fmt.Fprint(stderr, strandedConsequence)
}

// reportUndetermined writes one diagnostic per change this guard could not
// classify, naming the shape actually observed so the operator can tell a
// change with an unreadable completion convention from one with no completion
// marker at all — different files, different fixes.
//
// strandedCount is passed only so the closing note can state plainly that the
// stranded list printed above it is incomplete. That sentence is the whole
// justification for exit 3 outranking exit 1, and an operator reading the
// output should not have to infer it.
func reportUndetermined(stderr io.Writer, undetermined []undeterminedChange, strandedCount int) {
	fmt.Fprintf(stderr, "\narchive-reconcile: UNDETERMINED change(s) — %d change(s) under %s have a tasks.md this guard could not read a completion signal from.\n", len(undetermined), changesRelDir)

	for _, u := range undetermined {
		fmt.Fprintf(stderr, "\n  %s — %s.\n", u.Name, u.Observed)
		fmt.Fprint(stderr, "    fix: either express this change's tasks as bullet checkboxes (\"- [ ]\" / \"- [x]\"), or confirm by hand whether it is complete and archive it.\n")
	}

	fmt.Fprint(stderr, undeterminedListNote)
	if strandedCount > 0 {
		fmt.Fprintf(stderr, "The %d stranded change(s) listed above are therefore an incomplete answer, not the\nwhole one, which is why this run exits 3 rather than 1.\n", strandedCount)
	}
}

// undeterminedListNote states the consequence of an unclassified change: the
// scan's stranded list is partial, so "no other stranded changes" is a claim
// this run cannot make.
const undeterminedListNote = `
An undetermined change is not a clean one: this guard reads completion only
from bullet checkboxes, so a change whose task breakdown lives in another
shape is invisible to it and could be stranded right now without any tool
noticing.
`

// strandedConsequence closes the report with the reason this is worth
// blocking on: a merged, complete change that stays active forever is not a
// cosmetic housekeeping gap. This is not hypothetical in this repository —
// deterministic-verification-evidence sat active for five days before a
// manual audit caught it, and this exact check would have caught it on day
// one.
const strandedConsequence = `
A stranded change is invisible to every other tool: it is not in
openspec/changes/archive/, and if its capabilities are unpromoted its
requirements are not in openspec/specs/ either. Nothing else in this
repository notices; it has previously been found only by manual audit.
`

// undeterminedNote closes every exit-3 path. Stating the fail-closed stance
// out loud matters because the alternative reading — "the guard errored, so
// the repository is probably fine" — reintroduces the exact blind spot this
// guard exists to close.
// undeterminedNote deliberately does NOT name openspec/changes/. It is
// printed for an unreadable ledger and an unresolvable repo root too, and
// neither lives there -- an earlier wording claimed that locality for all
// three, which is the guard asserting more than it observed in the one
// message whose whole job is to say it observed too little.
const undeterminedNote = `This guard fails closed: it could not read or parse everything it needed,
so it does not report that the repository is clean. The error above names
what it could not read.
Resolve it and re-run.
`
