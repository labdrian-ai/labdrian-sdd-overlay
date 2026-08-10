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

// usageText names the only flag this guard accepts. --repo lets it scan a
// repository other than the caller's own working directory, which is what
// makes it testable against a temp fixture directory instead of only the live
// repo it ships in.
const usageText = "usage: archive-reconcile [--repo <path>]\n"

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
	repoFlag, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}

	root, err := resolveRepoRoot(repoFlag)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	findings, scanned, err := scanChanges(root)
	if err != nil {
		fmt.Fprintf(stderr, "archive-reconcile: %v\n", err)
		fmt.Fprint(stderr, undeterminedNote)
		return outcomeUndetermined
	}

	if len(findings) == 0 {
		fmt.Fprintf(stdout, "archive-reconcile: clean — %d active change(s) checked under %s, none stranded.\n", scanned, changesRelDir)
		return outcomeClean
	}

	reportStranded(stderr, findings)
	return outcomeStranded
}

// parseArgs parses the guard's only flag, mirroring the sibling
// review-preflight's parseArgs shape (ContinueOnError plus a discarded
// FlagSet output, so run owns every byte written to stderr). Leftover
// positional arguments are an error rather than being ignored: this tool
// takes no subcommand, and silently accepting one would let an operator's
// typo run as a bare invocation and report a verdict about something they did
// not ask for.
func parseArgs(args []string) (string, error) {
	fs := flag.NewFlagSet("archive-reconcile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repo := fs.String("repo", "", "repository root to scan (default: the caller's working directory)")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected argument %q; this guard takes no subcommand", fs.Arg(0))
	}
	return *repo, nil
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

// scanChanges walks every non-archive entry under openspec/changes/ and
// returns the stranded ones, plus the total number of active entries scanned
// (used only for the clean-case stdout summary).
//
// Every error here is returned rather than skipped or defaulted, per the same
// reasoning as review-preflight's parseProjection: a tasks.md this guard
// cannot read or parse is a fact it never observed, and reporting the change
// it belongs to as "not stranded" would silently reproduce the exact blind
// spot this guard exists to close.
func scanChanges(root string) ([]strandedChange, int, error) {
	changesDir := filepath.Join(root, filepath.FromSlash(changesRelDir))
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read %s: %w", changesDir, err)
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
	for _, name := range names {
		changeDir := filepath.Join(changesDir, name)
		tasksPath := filepath.Join(changeDir, tasksFileName)

		total, unchecked, err := countTasks(tasksPath)
		if err != nil {
			return nil, 0, fmt.Errorf("change %q: %w", name, err)
		}
		if total == 0 {
			// Planning-only: no task breakdown yet, definitely not stranded.
			continue
		}
		if unchecked > 0 {
			// Still in progress.
			continue
		}

		caps, err := capabilityStatuses(root, changeDir)
		if err != nil {
			return nil, 0, fmt.Errorf("change %q: %w", name, err)
		}
		findings = append(findings, strandedChange{Name: name, TotalTasks: total, Capabilities: caps})
	}
	return findings, len(names), nil
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

// countTasks reads one tasks.md and returns the total number of task
// checkboxes and how many are unchecked.
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
func countTasks(path string) (total int, unchecked int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("%s could not be read: %w", path, err)
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
				return 0, 0, ambiguousLine(path, lineNo, line,
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
			return 0, 0, ambiguousLine(path, lineNo, line,
				"a checkbox indented four or more spaces is an indented code block in CommonMark, not a task")
		}

		if m := checkboxPattern.FindStringSubmatch(line); m != nil {
			total++
			if m[1] == " " {
				unchecked++
			}
			continue
		}
		// A bullet whose first token is a bracket is only a malformed checkbox
		// when the bracket plausibly attempts one. A markdown link such as
		// "- [see the design](design.md)" is ordinary prose and must not fail
		// the scan; anything else bracketed is genuinely undecidable here.
		if checkboxLikePattern.MatchString(line) {
			if markdownLinkBulletPattern.MatchString(line) {
				continue
			}
			return 0, 0, fmt.Errorf("%s:%d: malformed task checkbox %q — expected \"- [ ]\", \"* [ ]\", or \"+ [ ]\" (checked or unchecked)", path, lineNo, strings.TrimSpace(line))
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("%s could not be parsed: %w", path, err)
	}
	// An unterminated fence or comment means every line after it was skipped,
	// so the counters describe only part of the file. Returning them would let
	// a genuinely stranded change hide behind an unclosed ``` and be reported
	// clean — the exact silent miss this guard exists to prevent, reintroduced
	// through the skipping machinery itself. Fail closed instead.
	if inFence {
		return 0, 0, fmt.Errorf("%s: unterminated fenced code block opened at line %d — every later line was skipped, so the task counts cannot be trusted", path, fenceOpenLine)
	}
	if inComment {
		return 0, 0, fmt.Errorf("%s: unterminated HTML comment opened at line %d — every later line was skipped, so the task counts cannot be trusted", path, commentOpenLine)
	}
	return total, unchecked, nil
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
const undeterminedNote = `This guard fails closed: it could not read or parse everything it needed under
openspec/changes/, so it does not report that the repository is clean.
Resolve the error above and re-run.
`
