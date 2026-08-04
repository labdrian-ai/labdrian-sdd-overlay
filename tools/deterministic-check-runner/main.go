package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// usageText names both permitted subcommands (R-009): normalize (mutating,
// pre-freeze only) and check (read-only, sole permitted post-freeze step).
const usageText = "usage: deterministic-check-runner <normalize|check> [--top-n N] [--out-dir DIR]\n"

// run dispatches to the normalize or check subcommand (Phase 4 mode split).
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "resolve working directory: %v\n", err)
		return outcomeProceduralToolingFailed
	}
	modules, err := discoverModules(root)
	if err != nil {
		fmt.Fprintf(stderr, "discover modules: %v\n", err)
		return outcomeProceduralToolingFailed
	}

	switch args[0] {
	case "check":
		return runCheckMode(modules, args[1:], root, stdout, stderr)
	case "normalize":
		return runNormalize(modules, stdout, stderr)
	default:
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}
}

// runCheckMode wires the read-only check subcommand (R-009/R-010);
// --out-dir inside root is a usage error (D5), checked against the raw flag
// value before any resolution or write — a log written inside root would
// mutate the candidate under review. Once past the guard, the flag value (or
// os.TempDir() when unset) is resolved exactly once and threaded to every
// check's log write, so the guard and the write always agree on the same
// directory (correction A3: previously the guard read the flag but the
// write always used os.TempDir() regardless).
func runCheckMode(modules []string, args []string, root string, stdout, stderr io.Writer) int {
	outDir := parseOutDir(args)
	if outDir != "" && pathInsideRoot(root, outDir) {
		fmt.Fprintf(stderr, "check: --out-dir %q must be outside the repository root %q\n", outDir, root)
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}
	if len(modules) == 0 {
		fmt.Fprintf(stderr, "check: no Go modules discovered under %q; nothing to verify\n", root)
		return outcomeProceduralToolingFailed
	}
	if outDir == "" {
		outDir = os.TempDir()
	}
	results := make([]result, len(registry))
	for i, c := range registry {
		results[i] = runCheck(c, modules, outDir)
	}
	emitRows(stdout, results, parseTopN(args))
	return selectOutcome(results)
}

// runNormalize runs each check's normalizeArgv fixer (nil = no fixer)
// across every module. The only mutating subcommand; pre-freeze only
// (R-011). Every module/fixer pair is attempted regardless of an earlier
// pair's outcome (correction B1's no-early-return rule, extended here):
// stopping early would leave later modules unformatted for no benefit,
// since normalize itself gates nothing — check, run afterward, is what
// decides pass/fail. Each fixer's own stdout/stderr is captured (runCheck's
// shape) and, on failure, surfaced on stderr behind a diagnostic naming the
// failing fixer and module (correction C2): before this fix runNormalize
// discarded the fixer's own text entirely, and its ExitError branch was a
// silent no-op that fell through to an unconditional outcomePassed — so a
// module gofmt -w could not parse (gofmt writes a parse error to stderr and
// exits non-zero, rewriting nothing) was indistinguishable from one that
// converged. normalize runs strictly pre-freeze
// (skills/sdd-verify/SKILL.md:69), so a silent failure here is strictly
// worse than a loud one: the cost lands on post-freeze check instead, where
// it can no longer be fixed without burning the single correction attempt
// (the obs #2668 dead end this change exists to avoid). A fixer that could
// not run at all and one that ran but exited non-zero are both reported
// this way and both fail the run: neither leaves the fixer's job actually
// done, and normalize has no separate "verification failed" concept to
// route a mere non-zero exit to instead.
//
// Zero discovered modules is deliberately NOT given C1's runCheckMode
// treatment here: an operator who runs normalize against the wrong cwd
// still runs check next per the documented ordering
// (skills/sdd-verify/SKILL.md:69), and check's own C1 guard already fails
// that run closed before freeze — duplicating the guard here would not
// close any additional gap, only add surface.
func runNormalize(modules []string, stdout, stderr io.Writer) int {
	failed := false
	for _, c := range registry {
		if c.normalizeArgv == nil {
			continue
		}
		for _, module := range modules {
			cmd := exec.Command(c.normalizeArgv[0], c.normalizeArgv[1:]...)
			cmd.Dir = module
			var out, errOut bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil {
				failed = true
				fmt.Fprintf(stderr, "normalize: %s failed in module %q: %v\n", c.name, module, err)
				if out.Len() > 0 {
					stdout.Write(out.Bytes())
				}
				if errOut.Len() > 0 {
					stderr.Write(errOut.Bytes())
				}
			}
		}
	}
	if failed {
		return outcomeProceduralToolingFailed
	}
	return outcomePassed
}

// runCheck runs c.checkArgv in every module dir and combines each module's
// own result into one row. Every module gets its own stdout/stderr buffer,
// analyzed independently via buildResult, never concatenated across modules
// (correction B1): a shared buffer let one module's toolchain-mismatch text
// or a bare 127 exit erase every other module's genuine findings (obs
// #2712-class bug, one level up — module isolation, not stream isolation).
// Combination rule, chosen to match what selectOutcome consumes:
//   - findings SUM across modules and their excerpts concatenate in module
//     order (discoverModules already sorts) — a module with 3 findings and
//     one with 2 is 5, never either alone;
//   - a module is excluded from the sum when the tool could not analyze it
//     at all: exit 127, an unrunnable process, or c.unavailableIf matches
//     its own output — that module contributed no genuine result, but the
//     others still can;
//   - the check as a whole is unavailable only when it could not run
//     ANYWHERE, i.e. every attempted module was excluded that way. Zero
//     discovered modules never reaches this loop: runCheckMode fails
//     closed before calling runCheck (correction C1: zero-module false
//     green);
//   - the row's exit code is the max across the modules that DID
//     contribute — the same max rule as before, now scoped to genuinely
//     usable modules instead of a run a single bad module could poison.
//
// No early return: every discovered module is attempted regardless of an
// earlier module's outcome. The combined, in-order findings stream is
// written once to the check's stable full-log path (D6) via writeLog; a
// failed write clears the returned result's logPath rather than failing the
// check itself (a full /tmp must never hold verification hostage).
func runCheck(c check, modules []string, outDir string) result {
	toolPath, lookErr := exec.LookPath(c.checkArgv[0])
	if lookErr != nil {
		return result{check: c, exitCode: unavailableExitCode, unavailable: true}
	}

	attempted, usable := 0, 0
	exitCode, totalCount := 0, 0
	var top []string
	var combined bytes.Buffer
	for _, module := range modules {
		attempted++
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(toolPath, c.checkArgv[1:]...)
		cmd.Dir = module
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		moduleExit := 0
		if runErr := cmd.Run(); runErr != nil {
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) {
				continue // this module's process could not run at all; excluded, not fatal to the whole check
			}
			moduleExit = exitErr.ExitCode()
		}
		if moduleExit == unavailableExitCode {
			continue // e.g. an unresolved "go run <module>@version"; this module only
		}
		findings := stdout.Bytes()
		if c.stderrFindings {
			findings = stderr.Bytes()
		}
		mr := buildResult(c, moduleExit, findings, outDir)
		if mr.unavailable {
			continue // e.g. a staticcheck toolchain mismatch; this module only
		}
		usable++
		if moduleExit > exitCode {
			exitCode = moduleExit
		}
		totalCount += mr.count
		top = append(top, mr.top...)
		combined.Write(findings)
	}

	unavailable := attempted > 0 && usable == 0
	if unavailable {
		exitCode = unavailableExitCode
	}
	r := result{check: c, exitCode: exitCode, unavailable: unavailable, count: totalCount, top: top, logPath: logPathFor(c.name, outDir), modulesAttempted: attempted, modulesUsable: usable}
	if !writeLog(r.logPath, combined.Bytes()) {
		r.logPath = ""
	}
	return r
}

// writeLog writes findings to path, creating its parent directory first, and
// overwrites any existing file (D6: the default out-dir path is stable and
// "overwritten"). It reports whether the write succeeded. The row that names
// this path is the evidence capture-evidence receives (skills/sdd-verify/
// SKILL.md:70), so a failed write must not be silently ignored — but it also
// must never become a verification failure or a procedural error (runCheck
// clears result.logPath on false, so renderSummary omits the full: clause
// instead of pointing at a file that does not exist).
func writeLog(path string, findings []byte) bool {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false
	}
	return os.WriteFile(path, findings, 0o644) == nil
}

// unavailableExitCode is the POSIX "command not found" convention: it is
// synthesized when exec.LookPath fails, and also honored when the process
// itself exits 127 (e.g. a "go run <module>@version" invocation whose
// module cannot be resolved) — a real subprocess exit this composes with
// the LookPath sites for rather than duplicating (4.10, amended R-016).
const unavailableExitCode = 127

// buildResult constructs a check's result from its captured stdout: count
// and top come from c.parse; unavailable is set when c.unavailableIf
// detects the tool could not analyze the code at all (e.g. a staticcheck
// toolchain mismatch), so selectOutcome routes it to
// procedural_tooling_failed rather than verification_failed (D4/R-016).
// Extracted from runCheck so this wiring is directly unit-testable without
// shelling out to a real toolchain-mismatched module.
func buildResult(c check, exitCode int, stdout []byte, outDir string) result {
	count, top := c.parse(exitCode, stdout)
	unavailable := c.unavailableIf != nil && c.unavailableIf(stdout)
	return result{check: c, exitCode: exitCode, unavailable: unavailable, count: count, top: top, logPath: logPathFor(c.name, outDir)}
}

// emitRows writes one "tool | exit_code | summary" row per result, in
// order, with summary rendered by renderSummary (D6); topN bounds how many
// excerpts each row shows. If the full block would exceed the
// capture-evidence input boundary (R-013), rows are re-rendered with
// excerpts dropped (topN 0) instead of truncating raw flattened bytes —
// every row still appears, keeping its tool, exit code, and count (D6,
// correction for obs #2719). capPayload remains only as a last-resort,
// line-safe backstop for the degraded block.
func emitRows(w io.Writer, results []result, topN int) {
	block := renderRowBlock(results, topN)
	if len(block) > payloadByteCap {
		block = renderRowBlock(results, 0)
	}
	w.Write(capPayload(block))
}

// renderRowBlock renders every result as one "tool | exit_code | summary"
// row, in order, never omitting a row: this is the structured form emitRows
// caps before flattening (obs #2719).
func renderRowBlock(results []result, topN int) []byte {
	var buf bytes.Buffer
	for _, r := range results {
		summary := renderRowSummary(r, topN)
		fmt.Fprintf(&buf, "%s | %d | %s\n", r.check.name, r.exitCode, summary)
	}
	return buf.Bytes()
}

// renderRowSummary extends renderSummary's rendered text with a
// "modules=usable/attempted" coverage note (correction B2) whenever
// runCheck excluded at least one discovered module (a 127 exit or an
// unavailableIf module), prepended so the existing "full: <path>" clause
// (parsed via a trailing-$ pattern in tests) stays the last thing in the
// row. Without this, a row like "staticcheck | 1 | count=2; ..." cannot
// tell a reader that one of four modules was never analysed — partial
// coverage was indistinguishable from full coverage
// (skills/sdd-verify/SKILL.md:70 treats these row bytes as the evidence
// itself). Full coverage (modulesUsable == modulesAttempted, including
// every result built outside runCheck's loop, which leaves both fields at
// their zero value) renders byte-identical to plain renderSummary, so
// existing rows and the golden fixture are unaffected — this is a
// report-only-when-partial choice, deliberately not a fifth row column,
// to keep the two-pipe "tool | exit_code | summary" contract
// capture-evidence parses unchanged (D6).
func renderRowSummary(r result, topN int) string {
	summary := renderSummary(r.count, r.top, topN, r.logPath)
	if r.modulesAttempted > r.modulesUsable {
		summary = fmt.Sprintf("modules=%d/%d; %s", r.modulesUsable, r.modulesAttempted, summary)
	}
	return summary
}

// check describes one verification tool. deterministic and blocking are
// declared independently; classify() is the sole place they are combined.
// parse turns a check's captured output into a finding count and excerpts;
// failed decides pass/fail from exit code and count together (D3), since a
// tool's own exit code is not always authoritative (gofmt -l exits 0 while
// listing violations). unavailableIf is optional (nil for most checks): when
// set, it detects that the tool ran but could not analyze the code at all
// (D4/R-016) — see buildResult. normalizeArgv is the mode-split fixer
// invocation (Phase 4), nil when the tool has no fixer; checkArgv must never
// contain a fixer flag (enforced by containsFixerFlag/init below).
// stderrFindings is true only for go vet, whose diagnostics land on stderr
// (runCheck picks the matching buffer before parse ever runs).
type check struct {
	name           string
	deterministic  bool
	blocking       bool
	checkArgv      []string
	normalizeArgv  []string
	parse          func(exit int, out []byte) (count int, top []string)
	failed         func(exit, count int) bool
	unavailableIf  func(out []byte) bool
	stderrFindings bool
}

// registry is the hardcoded v1 check set. It is not configurable: gofmt,
// go vet, and staticcheck are deterministic and blocking; deadcode is
// deterministic but WARNING-only (amended R-016). staticcheck and deadcode
// use the same pinned `go run <module>@<version>` invocation CI uses
// (.github/workflows/ci.yml) rather than resolving bare binaries from PATH,
// so both agree on tool availability and honor the same version pin
// (TestCheckArgvPinnedToCIInvocation enforces this parity).
var registry = []check{
	{name: "gofmt", deterministic: true, blocking: true, checkArgv: []string{"gofmt", "-l", "."}, normalizeArgv: []string{"gofmt", "-w", "."}, parse: parseGofmt, failed: failedGofmt},
	{name: "go vet", deterministic: true, blocking: true, checkArgv: []string{"go", "vet", "./..."}, parse: parseGoVet, failed: failedGoVet, stderrFindings: true},
	{name: "staticcheck", deterministic: true, blocking: true, checkArgv: []string{"go", "run", "honnef.co/go/tools/cmd/staticcheck@v0.7.0", "./..."}, parse: parseStaticcheck, failed: failedStaticcheck, unavailableIf: isStaticcheckToolchainMismatch},
	{name: "deadcode", deterministic: true, blocking: false, checkArgv: []string{"go", "run", "golang.org/x/tools/cmd/deadcode@v0.48.0", "./..."}, parse: parseDeadcode, failed: failedDeadcode},
}

// fixerFlags are argv tokens indicating a tool would mutate files; no
// checkArgv entry may contain one (R-009/R-010/D5).
var fixerFlags = []string{"-w", "-fix"}

// containsFixerFlag reports whether argv contains any known fixer flag.
func containsFixerFlag(argv []string) bool {
	for _, arg := range argv {
		for _, fixer := range fixerFlags {
			if arg == fixer {
				return true
			}
		}
	}
	return false
}

// init enforces the checkArgv allowlist as a real registry check: a fixer
// flag added to checkArgv fails loud at program start.
func init() {
	for _, c := range registry {
		if containsFixerFlag(c.checkArgv) {
			panic(fmt.Sprintf("registry invariant violated: check %q checkArgv %v contains fixer flag", c.name, c.checkArgv))
		}
	}
}

// goVetDiagnosticPattern matches go vet's diagnostic lines
// ("file.go:line:col: message"), distinguishing them from the "# package"
// headers go vet ./... prints ahead of each package's findings.
var goVetDiagnosticPattern = regexp.MustCompile(`^\S+\.go:\d+:\d+:`)

// parseGofmt counts gofmt -l's reported files from its stdout. gofmt -l
// exits 0 even when it lists unformatted files, so a caller must not treat
// a zero exit code as a clean result (D3) — failedGofmt trusts count.
func parseGofmt(exit int, out []byte) (count int, top []string) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}
	lines := strings.Split(trimmed, "\n")
	return len(lines), lines
}

// failedGofmt is D3's failure predicate for gofmt: any listed file fails
// the check regardless of exit code, and a non-zero exit (e.g. a genuine
// invocation error) fails it too.
func failedGofmt(exit, count int) bool {
	return count > 0 || exit != 0
}

// parseGoVet counts go vet's diagnostic lines from its captured output,
// skipping "# package" headers.
func parseGoVet(exit int, out []byte) (count int, top []string) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if goVetDiagnosticPattern.MatchString(strings.TrimSpace(line)) {
			top = append(top, line)
		}
	}
	return len(top), top
}

// failedGoVet trusts go vet's own exit code directly, unlike gofmt: go vet
// exits non-zero exactly when it has diagnostics to report.
func failedGoVet(exit, count int) bool {
	return exit != 0
}

// staticcheckFindingPattern matches staticcheck's finding lines
// ("path:line:col: message (CODE)"). CODE is not uniformly two letters —
// U1000 is one letter + four digits, ST1005/SA1006 are two + four — so the
// pattern accepts a variable-length uppercase-letter prefix. A pattern keyed
// to two letters silently undercounts U-series findings (audit finding, obs
// #2711). The toolchain-mismatch line ends in "(compile)", which this
// pattern does not match, but isStaticcheckToolchainMismatch is checked
// first regardless so that distinction never has to rely on this regex.
var staticcheckFindingPattern = regexp.MustCompile(`^\S+:\d+:\d+: .+\([A-Z]+\d+\)$`)

// staticcheckToolchainMismatchPattern matches staticcheck's stdout when the
// local Go build toolchain is older than a module's declared go directive
// (reproduced against tui/go.mod's go 1.26.1 with a go1.25 build). Confirmed
// via honnef.co/go/tools@v0.7.0's lintcmd/lint.go: this per-package load
// error becomes a "compile"-category diagnostic (lint.go's failed()),
// rendered through the same textFormatter{W: os.Stdout} as real findings
// (cmd.go) — never stderr. Not a verification finding: staticcheck could not
// analyze the module at all.
var staticcheckToolchainMismatchPattern = regexp.MustCompile(`requires newer Go version`)

// isStaticcheckToolchainMismatch reports whether out is staticcheck's
// toolchain-mismatch failure rather than real findings (audit finding, obs
// #2711). Both cases exit non-zero, so exit code alone cannot distinguish
// them — wired as the staticcheck registry entry's unavailableIf so
// buildResult marks the result unavailable and selectOutcome routes it to
// procedural_tooling_failed, never verification_failed (D4/R-016), never
// burning the single correction attempt on an environment problem instead of
// a real finding.
func isStaticcheckToolchainMismatch(out []byte) bool {
	return staticcheckToolchainMismatchPattern.Match(out)
}

// parseStaticcheck counts staticcheck's findings from its captured output,
// one per line. A toolchain mismatch is not counted as a finding — the tool
// could not analyze the code at all — so it is excluded before line parsing
// (see isStaticcheckToolchainMismatch).
func parseStaticcheck(exit int, out []byte) (count int, top []string) {
	if isStaticcheckToolchainMismatch(out) {
		return 0, nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if staticcheckFindingPattern.MatchString(line) {
			top = append(top, line)
		}
	}
	return len(top), top
}

// failedStaticcheck trusts staticcheck's own exit code, like failedGoVet:
// staticcheck exits non-zero exactly when it reports findings. Exit alone
// cannot distinguish a genuine finding from a toolchain-mismatch failure —
// both exit non-zero — which is why that distinction is not this
// predicate's job: isStaticcheckToolchainMismatch must be checked ahead of
// failed so a mismatch is classified unavailable/procedural before failed is
// ever consulted as a verification result (D4/R-016).
func failedStaticcheck(exit, count int) bool {
	return exit != 0
}

// deadcodeFindingPattern matches deadcode's finding lines
// ("path:line:col: unreachable func: Name").
var deadcodeFindingPattern = regexp.MustCompile(`^\S+:\d+:\d+: unreachable func: \S+$`)

// parseDeadcode counts deadcode's findings from its captured stdout only.
// deadcode exits 0 even when it reports findings (measured in this
// repository: 21 findings, exit code 0), so its exit code can never signal a
// clean run — only the count can, the mirror image of gofmt's D3 trap.
// Callers must pass stdout alone: when stdout and stderr are merged, the Go
// toolchain-switch message ("switching to goX.Y.Z") lands in the stream and
// would inflate the count by one if mistaken for a finding line (audit
// finding, obs #2712). deadcodeFindingPattern only matches genuine
// "unreachable func" lines, so a stray non-finding line never counts even if
// it appears in the parsed stream.
func parseDeadcode(exit int, out []byte) (count int, top []string) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if deadcodeFindingPattern.MatchString(line) {
			top = append(top, line)
		}
	}
	return len(top), top
}

// failedDeadcode trusts the count, never the exit code: deadcode exits 0
// even when it reports findings, so exit==0 must not be read as "clean"
// (D3-class trap, mirrored — gofmt fails despite exit 0 for the opposite
// reason: findings without a failing exit). deadcode is registered
// blocking: false (amended R-016), so classify() prevents this predicate
// from ever escalating the run outcome; failedDeadcode only decides the
// row's own pass/fail state.
func failedDeadcode(exit, count int) bool {
	return count > 0
}

// classify is the single enforcement point for effective blocking: a check
// only blocks the run when it is both declared blocking AND deterministic.
// No other code in this module may compute effective blocking.
func classify(c check) bool {
	return c.blocking && c.deterministic
}

// Process exit codes returned by selectOutcome (D4). outcomeUsage (2) is
// returned by run directly for a missing/unrecognized subcommand.
const (
	outcomePassed                  = 0
	outcomeVerificationFailed      = 1
	outcomeUsage                   = 2
	outcomeProceduralToolingFailed = 3
)

// result is the outcome of executing one check. unavailable (could not run)
// and a plain failing exitCode (ran, found problems) are distinct states;
// runnerErr marks a runner-internal fault. Rows keep the raw exit code.
// count, top, and logPath are populated by runCheck from c.parse's stdout
// parse and logPathFor; they stay zero-value when the tool never ran.
// modulesAttempted and modulesUsable record runCheck's per-module coverage
// (correction B2): modulesAttempted is every discovered module runCheck
// tried, modulesUsable is how many of those actually contributed to count/
// top (excludes a 127 exit or an unavailableIf module). Both stay zero-value
// for the two early-return results (LookPath failure, usage/procedural
// errors), which never entered runCheck's per-module loop.
type result struct {
	check            check
	exitCode         int
	unavailable      bool
	runnerErr        bool
	count            int
	top              []string
	logPath          string
	modulesAttempted int
	modulesUsable    int
}

// selectOutcome is the single place outcome precedence lives (amended
// R-016): (1) an unexecutable blocking-set check or a runner error →
// procedural_tooling_failed, outranking everything else; (2) otherwise a
// blocking-set check that ran and failed → verification_failed; (3)
// otherwise → passed. A WARNING-only check (e.g. deadcode) can never alone
// reach gate 1, and never suppresses a real blocking failure or
// unavailability elsewhere. Gate 2 consults each check's own failed(exit,
// count) predicate (D3) rather than the raw exit code directly: gofmt -l
// exits 0 while listing violations, so trusting exitCode alone here missed
// that finding (obs #2735) — failed is defined for every registry entry, but
// a nil check is kept as a defensive guard against a future entry omitting
// it. classify() remains the sole effective-blocking enforcement point;
// failed only decides whether a check that IS effectively blocking counts as
// red.
func selectOutcome(results []result) int {
	for _, r := range results {
		if r.runnerErr {
			return outcomeProceduralToolingFailed
		}
	}
	for _, r := range results {
		if r.unavailable && classify(r.check) {
			return outcomeProceduralToolingFailed
		}
	}
	for _, r := range results {
		if !r.unavailable && !r.runnerErr && classify(r.check) && r.check.failed != nil && r.check.failed(r.exitCode, r.count) {
			return outcomeVerificationFailed
		}
	}
	return outcomePassed
}

// Phase 3E: summary rendering, --top-n, payload cap, banned-literal guard
// (R-007, R-008, R-013); wired into run()/emitRows by 3F.
const (
	defaultTopN       = 5
	excerptCharCap    = 200
	payloadByteCap    = 4 * 1024 * 1024 // R-013 capture-evidence boundary
	defaultOutDirName = "labdrian-deterministic-checks"
)

// bannedSummaryLiterals are the status words upstream rejects by regex (R-008).
var bannedSummaryLiterals = []string{"PASS", "PASSED", "SUCCESS", "N/A", "NA", "NONE", "TODO", "TBD", "PLACEHOLDER"}

// isBannedSummaryLiteral reports whether summary, as a whole, is banned (case-insensitive).
func isBannedSummaryLiteral(summary string) bool {
	trimmed := strings.TrimSpace(summary)
	for _, literal := range bannedSummaryLiterals {
		if strings.EqualFold(trimmed, literal) {
			return true
		}
	}
	return false
}

// capExcerpt truncates s to excerptCharCap runes without splitting a rune.
func capExcerpt(s string) string {
	r := []rune(s)
	if len(r) <= excerptCharCap {
		return s
	}
	return string(r[:excerptCharCap])
}

// renderSummary renders D6's bounded summary "count=N; top: e1; …; full:
// <path>". Zero findings render the literal digit "0" — never a status
// word (R-008): upstream rejects narrated evidence by regex. An empty
// logPath (writeLog failed) omits the "; full: <path>" clause entirely
// rather than naming a file that was never written; the count and excerpts
// still render. The trailing guardAgainstBannedSummary call wires
// isBannedSummaryLiteral into the render path (3I.0): renderSummary is the
// sole place a row's summary text is constructed, so this is the one call
// site that catches a future edit letting a bare banned literal through
// construction before it reaches capture-evidence.
func renderSummary(count int, top []string, topN int, logPath string) string {
	if count == 0 {
		return "0"
	}
	if topN < 0 {
		topN = 0
	}
	shown := top
	if len(shown) > topN {
		shown = shown[:topN]
	}
	excerpts := make([]string, len(shown))
	for i, e := range shown {
		excerpts[i] = capExcerpt(e)
	}
	summary := fmt.Sprintf("count=%d; top: %s", count, strings.Join(excerpts, "; "))
	if logPath != "" {
		summary += fmt.Sprintf("; full: %s", logPath)
	}
	guardAgainstBannedSummary(summary)
	return summary
}

// guardAgainstBannedSummary panics if summary is itself a bare banned
// literal (R-008). Before this, isBannedSummaryLiteral was defined and
// unit-tested but never called from production code — deadcode flagged it
// as unreachable during 3H (obs: main.go:370:6) — so nothing prevented a
// banned literal from being emitted at runtime; the protection lived only
// in the test suite. Panicking on a broken invariant mirrors the existing
// precedent in engine/prespec/brief.go: a bare banned literal here is a
// programmer error in renderSummary's own construction, not a recoverable
// operational failure, so failing loud beats emitting rejected evidence.
func guardAgainstBannedSummary(summary string) {
	if isBannedSummaryLiteral(summary) {
		panic(fmt.Sprintf("renderSummary produced a banned literal %q (R-008): refusing to emit narrated evidence", summary))
	}
}

// logPathFor returns the stable full-log path for tool under outDir (D6);
// spaces become "-". Callers resolve outDir once (flag-or-default,
// runCheckMode) and pass it in; this function never reads os.TempDir()
// itself (correction A3).
func logPathFor(tool, outDir string) string {
	return filepath.Join(outDir, defaultOutDirName, strings.ReplaceAll(tool, " ", "-")+".log")
}

// parseTopN extracts --top-n from args (default defaultTopN); malformed
// falls back to the default rather than aborting.
func parseTopN(args []string) int {
	fs := flag.NewFlagSet("deterministic-check-runner", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	topN := fs.Int("top-n", defaultTopN, "maximum finding excerpts per row")
	if err := fs.Parse(args); err != nil {
		return defaultTopN
	}
	return *topN
}

// parseOutDir extracts --out-dir from check-mode args (empty = unset).
func parseOutDir(args []string) string {
	fs := flag.NewFlagSet("deterministic-check-runner", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int("top-n", defaultTopN, "maximum finding excerpts per row")
	outDir := fs.String("out-dir", "", "directory for full check logs")
	if err := fs.Parse(args); err != nil {
		return ""
	}
	return *outDir
}

// pathInsideRoot reports whether path is at or under root (R-010 guard).
func pathInsideRoot(root, path string) bool {
	absRoot, rootErr := filepath.Abs(root)
	absPath, pathErr := filepath.Abs(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	return absPath == absRoot || strings.HasPrefix(absPath, absRoot+string(filepath.Separator))
}

// capPayload is the last-resort byte-level backstop for R-013 (correction
// for obs #2719): emitRows already sheds excerpts row-by-row before
// flattening when the block is oversized, so this only guards a
// pathological remainder (e.g. an unbounded log path) that is still over
// payloadByteCap after that degrade. It cuts on the last complete line at or
// before the cap, never mid-row and never mid-rune — cutting immediately
// after '\n' is always rune-safe, since '\n' is a single ASCII byte. If no
// line boundary exists within the cap, it returns empty rather than emit a
// partial, possibly rune-split line.
func capPayload(payload []byte) []byte {
	if len(payload) <= payloadByteCap {
		return payload
	}
	if cut := bytes.LastIndexByte(payload[:payloadByteCap], '\n'); cut >= 0 {
		return payload[:cut+1]
	}
	return payload[:0]
}

// discoverModules walks root for go.mod files and returns their containing
// directories as absolute, sorted paths. root is normalized to absolute
// first, so a relative root resolves against the caller's cwd.
func discoverModules(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var modules []string
	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		modules = append(modules, filepath.Dir(path))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Strings(modules)
	return modules, nil
}
