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

// run wires discoverModules x registry x exec x classify/selectOutcome x
// emitRows into the working end-to-end command. Row summaries use the
// Phase-3 bounded renderer (D6), driven by --top-n (parseTopN).
func run(args []string, stdout, stderr io.Writer) int {
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

	results := make([]result, len(registry))
	for i, c := range registry {
		results[i] = runCheck(c, modules)
	}

	emitRows(stdout, results, parseTopN(args))
	return selectOutcome(results)
}

// runCheck runs c.checkArgv in every module dir and aggregates to one
// result: unavailable if the tool is missing from PATH, else the max exit
// code observed across modules. stdout and stderr are captured into
// distinct buffers, never merged: c.parse trusts stdout alone (per-parse-func
// docs), and merging streams has already inflated a finding count once by
// letting a Go toolchain-switch message land in the parsed stream (obs
// #2712). stderr is captured but not yet consumed by any decision here.
func runCheck(c check, modules []string) result {
	toolPath, lookErr := exec.LookPath(c.checkArgv[0])
	if lookErr != nil {
		return result{check: c, exitCode: 127, unavailable: true}
	}

	exitCode := 0
	var stdout, stderr bytes.Buffer
	for _, module := range modules {
		cmd := exec.Command(toolPath, c.checkArgv[1:]...)
		cmd.Dir = module
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if runErr := cmd.Run(); runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				if code := exitErr.ExitCode(); code > exitCode {
					exitCode = code
				}
				continue
			}
			return result{check: c, exitCode: 127, unavailable: true}
		}
	}
	return buildResult(c, exitCode, stdout.Bytes())
}

// buildResult constructs a check's result from its captured stdout: count
// and top come from c.parse; unavailable is set when c.unavailableIf
// detects the tool could not analyze the code at all (e.g. a staticcheck
// toolchain mismatch), so selectOutcome routes it to
// procedural_tooling_failed rather than verification_failed (D4/R-016).
// Extracted from runCheck so this wiring is directly unit-testable without
// shelling out to a real toolchain-mismatched module.
func buildResult(c check, exitCode int, stdout []byte) result {
	count, top := c.parse(exitCode, stdout)
	unavailable := c.unavailableIf != nil && c.unavailableIf(stdout)
	return result{check: c, exitCode: exitCode, unavailable: unavailable, count: count, top: top, logPath: logPathFor(c.name)}
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
		summary := renderSummary(r.count, r.top, topN, r.logPath)
		fmt.Fprintf(&buf, "%s | %d | %s\n", r.check.name, r.exitCode, summary)
	}
	return buf.Bytes()
}

// check describes one verification tool. deterministic and blocking are
// declared independently; classify() is the sole place they are combined.
// parse turns a check's captured output into a finding count and excerpts;
// failed decides pass/fail from exit code and count together (D3), since a
// tool's own exit code is not always authoritative (gofmt -l exits 0 while
// listing violations). unavailableIf is optional (nil for most checks): when
// set, it detects that the tool ran but could not analyze the code at all
// (D4/R-016) — see buildResult. All four registry entries now declare
// parse/failed. normalizeArgv is deferred to runner-mode-separation and is
// intentionally absent here to avoid rework.
type check struct {
	name          string
	deterministic bool
	blocking      bool
	checkArgv     []string
	parse         func(exit int, out []byte) (count int, top []string)
	failed        func(exit, count int) bool
	unavailableIf func(out []byte) bool
}

// registry is the hardcoded v1 check set. It is not configurable: gofmt,
// go vet, and staticcheck are deterministic and blocking; deadcode is
// deterministic but WARNING-only (amended R-016). staticcheck and deadcode
// use the same pinned `go run <module>@<version>` invocation CI uses
// (.github/workflows/ci.yml) rather than resolving bare binaries from PATH,
// so both agree on tool availability and honor the same version pin
// (TestCheckArgvPinnedToCIInvocation enforces this parity).
var registry = []check{
	{name: "gofmt", deterministic: true, blocking: true, checkArgv: []string{"gofmt", "-l", "."}, parse: parseGofmt, failed: failedGofmt},
	{name: "go vet", deterministic: true, blocking: true, checkArgv: []string{"go", "vet", "./..."}, parse: parseGoVet, failed: failedGoVet},
	{name: "staticcheck", deterministic: true, blocking: true, checkArgv: []string{"go", "run", "honnef.co/go/tools/cmd/staticcheck@v0.7.0", "./..."}, parse: parseStaticcheck, failed: failedStaticcheck, unavailableIf: isStaticcheckToolchainMismatch},
	{name: "deadcode", deterministic: true, blocking: false, checkArgv: []string{"go", "run", "golang.org/x/tools/cmd/deadcode@v0.48.0", "./..."}, parse: parseDeadcode, failed: failedDeadcode},
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

// Process exit codes returned by selectOutcome (D4).
const (
	outcomePassed                  = 0
	outcomeVerificationFailed      = 1
	outcomeProceduralToolingFailed = 3
)

// result is the outcome of executing one check. unavailable (could not run)
// and a plain failing exitCode (ran, found problems) are distinct states;
// runnerErr marks a runner-internal fault. Rows keep the raw exit code.
// count, top, and logPath are populated by runCheck from c.parse's stdout
// parse and logPathFor; they stay zero-value when the tool never ran.
type result struct {
	check       check
	exitCode    int
	unavailable bool
	runnerErr   bool
	count       int
	top         []string
	logPath     string
}

// selectOutcome is the single place outcome precedence lives (amended
// R-016): (1) an unexecutable blocking-set check or a runner error →
// procedural_tooling_failed, outranking everything else; (2) otherwise a
// blocking-set check that ran and failed → verification_failed; (3)
// otherwise → passed. A WARNING-only check (e.g. deadcode) can never alone
// reach gate 1, and never suppresses a real blocking failure or
// unavailability elsewhere.
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
		if !r.unavailable && !r.runnerErr && classify(r.check) && r.exitCode != 0 {
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
// word (R-008): upstream rejects narrated evidence by regex.
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
	return fmt.Sprintf("count=%d; top: %s; full: %s", count, strings.Join(excerpts, "; "), logPath)
}

// logPathFor returns the stable full-log path for tool (D6); spaces become "-".
func logPathFor(tool string) string {
	return filepath.Join(os.TempDir(), defaultOutDirName, strings.ReplaceAll(tool, " ", "-")+".log")
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
