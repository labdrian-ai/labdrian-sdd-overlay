package main

import (
	"bytes"
	"context"
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
	"time"
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

// runCheckMode wires the read-only check subcommand (R-009/R-010). The
// effective out-dir is resolved to its final value (flag, or os.TempDir()
// when unset) BEFORE the guard runs, and the guard then runs exactly once
// against that resolved value — see resolveOutDir. The previous order ran
// the guard against the raw flag and substituted the default afterwards,
// so exactly one of the two possible out-dirs was never checked at all.
// A log written inside root would mutate the candidate under review, and
// correction A3 (which made the write actually honor the flag) is what made
// every hole in this guard reachable.
func runCheckMode(modules []string, args []string, root string, stdout, stderr io.Writer) int {
	topN, outDirFlag, err := parseCheckArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		fmt.Fprint(stderr, usageText)
		return outcomeUsage
	}
	outDir, rejection, ok := resolveOutDir(root, outDirFlag, stderr)
	if !ok {
		return rejection
	}
	if len(modules) == 0 {
		fmt.Fprintf(stderr, "check: no Go modules discovered under %q; nothing to verify\n", root)
		return outcomeProceduralToolingFailed
	}
	results := make([]result, len(registry))
	for i, c := range registry {
		results[i] = runCheck(c, modules, outDir)
	}
	emitRows(stdout, results, topN)
	return selectOutcome(results)
}

// resolveOutDir resolves the effective out-dir (flag, or os.TempDir() when
// the flag is unset) and validates that one final value. ok=false means a
// diagnostic was already written to stderr and rejection is the exit code
// to return.
//
// The two rejections deliberately do NOT share an exit code, because they
// are not the same kind of problem and the operator's fix differs:
//   - an operator-supplied --out-dir inside root is a bad invocation, so it
//     stays outcomeUsage (2) and reprints the usage text;
//   - a resolved DEFAULT inside root is an environment misconfiguration —
//     the invocation named no directory at all — so it is
//     outcomeProceduralToolingFailed (3) and the diagnostic names TMPDIR,
//     the only thing an operator can actually change to fix it. Reporting
//     that as a usage error would point at a flag that was never passed.
//
// skills/sdd-verify/SKILL.md maps runner exit 2 and 3 to the same
// procedural_tooling_failed outcome, so this split costs the caller nothing
// and buys the operator a diagnosis.
//
// The control-byte check lives here, at the boundary where the flag is
// validated, rather than as an escape in renderSummary. --out-dir is the
// only caller-controlled input that reaches the emitted row block unsplit
// (it flows verbatim through logPathFor into result.logPath into
// renderSummary's "; full: %s" clause), so a newline in it appends a line
// that satisfies the same "tool | exit_code | summary" contract the capture
// parser reads. Tool output cannot do this — parsers split on '\n' first.
// Closing the single door is narrower and provable; escaping in the
// renderer would put the burden on every future caller of a shared
// formatting path and would still leave result.logPath itself poisoned.
func resolveOutDir(root, outDirFlag string, stderr io.Writer) (string, int, bool) {
	outDir, fromFlag := outDirFlag, true
	if outDir == "" {
		outDir, fromFlag = os.TempDir(), false
	}
	if b, bad := firstControlByte(outDir); bad {
		if fromFlag {
			fmt.Fprintf(stderr, "check: --out-dir %q contains control character %q; it would be copied verbatim into the emitted evidence rows\n", outDir, b)
			fmt.Fprint(stderr, usageText)
			return "", outcomeUsage, false
		}
		fmt.Fprintf(stderr, "check: the default out-dir %q (from TMPDIR) contains control character %q; set TMPDIR to a path without control characters\n", outDir, b)
		return "", outcomeProceduralToolingFailed, false
	}
	if pathInsideRoot(root, outDir) {
		if fromFlag {
			fmt.Fprintf(stderr, "check: --out-dir %q must be outside the repository root %q\n", outDir, root)
			fmt.Fprint(stderr, usageText)
			return "", outcomeUsage, false
		}
		fmt.Fprintf(stderr, "check: the default out-dir %q (from TMPDIR) is inside the repository root %q; set TMPDIR to a directory outside the repository\n", outDir, root)
		return "", outcomeProceduralToolingFailed, false
	}
	return outDir, outcomePassed, true
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
//
// A fixer killed by toolExecTimeout is reported by reportNormalizeKill rather
// than by the plain "failed" line above: unlike a fixer that exited on its own
// terms, a SIGKILLed in-place rewriter can leave the module's source bytes
// corrupted, and that has to be said out loud with the recovery artifacts
// named. It is still one of the two failure shapes this function reports and
// still returns outcomeProceduralToolingFailed.
func runNormalize(modules []string, stdout, stderr io.Writer) int {
	failed := false
	for _, c := range registry {
		if c.normalizeArgv == nil {
			continue
		}
		for _, module := range modules {
			ctx, cancel := context.WithTimeout(context.Background(), toolExecTimeout)
			cmd := exec.CommandContext(ctx, c.normalizeArgv[0], c.normalizeArgv[1:]...)
			cmd.Dir = module
			cmd.WaitDelay = toolWaitDelay
			var out, errOut bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			// Taken immediately before the fixer is launched, so every
			// backup this fixer writes is necessarily newer. It is the
			// provenance cutoff reportNormalizeKill uses to separate the
			// artifacts THIS kill produced from ones an earlier interrupted
			// run left behind (correction R4-01).
			fixerStart := time.Now()
			err := cmd.Run()
			timedOut := ctx.Err() == context.DeadlineExceeded
			cancel()
			if err != nil {
				failed = true
				if timedOut {
					reportNormalizeKill(stderr, c.name, module, fixerStart)
				} else {
					fmt.Fprintf(stderr, "normalize: %s failed in module %q: %v\n", c.name, module, err)
				}
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

// reportNormalizeKill writes the timeout diagnostic for a fixer the deadline
// killed. Correction D1 gave every tool subprocess a deadline without asking
// what killing one MEANS per path, and the normalize path rewrites files in
// place: exec.CommandContext's default cancel action is Process.Kill
// (SIGKILL), and gofmt -w — the only registry normalizeArgv — does not write
// atomically. Verified in GOROOT src/cmd/gofmt/gofmt.go:468-540 (Go 1.26.5):
// writeFile backs the original bytes up via backupFile, then opens the source
// O_WRONLY *without* O_TRUNC and writes the formatted bytes over it, calling
// Truncate only afterwards and removing the backup only on full success. A
// SIGKILL between the write and the truncate leaves a new head and a stale
// tail — real byte corruption, not merely an unformatted module — and gofmt
// processes files concurrently (fdSem), so several can be mid-write at once.
// gofmt installs no signal handler, so switching the cancel action to
// SIGTERM/SIGINT would buy nothing; the deadline stays and the diagnostic is
// what has to change.
//
// The pre-fix diagnostic named only the check and the module, enumerated
// nothing, and left recovery to the module happening to sit in a VCS working
// tree — which is exactly the case with no other way back, since runNormalize
// walks from the caller's arbitrary cwd and so covers modules outside any
// repo, plus ignored and untracked files. The one thing that IS recoverable
// without VCS is the backup the kill prevented gofmt from removing, so this
// names every one it can find. Honesty is the constraint: it reports the shape
// it scanned for, never claims artifacts it did not look for, and when the
// scan itself fails it says so instead of implying the module is clean.
//
// Correction R4-01 adds the provenance the restore instruction always needed.
// normalizeBackupPattern matches by shape alone, so a backup an EARLIER
// interrupted run left behind looks exactly like one this kill just wrote; the
// pre-fix diagnostic told the operator to restore both. Following it for the
// earlier one silently reverts that file to its pre-fixer bytes, discarding
// whatever repair happened since — this tool's own recovery guidance destroying
// already-corrected data. fixerStart, taken in runNormalize just before the
// subprocess starts, is the cutoff: only artifacts written at or after it can be
// this kill's, and only those get the bare restore instruction. Anything older,
// or whose mtime cannot be read at all, gets wording that says restoring it
// would revert later changes and must be inspected first — unreadable provenance
// fails toward not destroying data, never toward a confident instruction.
func reportNormalizeKill(stderr io.Writer, name, module string, fixerStart time.Time) {
	fmt.Fprintf(stderr, "normalize: %s timed out after %s in module %q and was killed (SIGKILL) mid-run; a fixer that rewrites sources in place rather than via an atomic rename — which is what gofmt -w does — leaves a new head and a stale tail behind when killed mid-write, so this module may now hold partially rewritten, byte-corrupted source files\n", name, toolExecTimeout, module)
	artifacts, scanErr := scanNormalizeRecoveryArtifacts(module)
	for _, artifact := range artifacts {
		original := strings.TrimSuffix(artifact.path, filepath.Ext(artifact.path))
		if artifact.modTimeErr == nil && !artifact.modTime.Before(fixerStart) {
			fmt.Fprintf(stderr, "normalize: recovery artifact holding the pre-fixer bytes: %s (restore it over %s)\n", artifact.path, original)
			continue
		}
		fmt.Fprintf(stderr, "normalize: recovery artifact %s cannot be attributed to this kill (%s), so it was most likely left by an earlier interrupted run; do NOT restore it blind over %s — that would revert every change made to that file since, including any repair already completed; inspect and diff the two first\n", artifact.path, artifact.provenance(fixerStart), original)
	}
	if scanErr != nil {
		fmt.Fprintf(stderr, "normalize: the scan of %q for recovery artifacts did not complete: %v; the %d listed above are only what it reached, not necessarily all of them\n", module, scanErr, len(artifacts))
		return
	}
	if len(artifacts) == 0 {
		fmt.Fprintf(stderr, "normalize: scanned %q for recovery artifacts matching %q and found none; this is not evidence the module is intact — the fixer may have been killed before writing a backup, or after removing one — so verify this module's source bytes by other means before running check over them\n", module, normalizeBackupPattern)
	}
}

// normalizeBackupPattern matches the sibling backup a killed in-place fixer
// leaves behind. It is gofmt -w's own shape, read off GOROOT
// src/cmd/gofmt/gofmt.go:545-568 (Go 1.26.5), where backupFile creates
// filepath.Join(dir, base+"."+strconv.Itoa(rand.Int())) holding the ORIGINAL
// bytes. ".go" before the digits is part of the shape, not an approximation:
// gofmt only ever rewrites files isGoFilename accepts (gofmt.go:93), so base
// always ends in ".go". gofmt is currently the only registry entry with a
// normalizeArgv; a future fixer leaving a differently shaped artifact would
// need this pattern extended, which is why reportNormalizeKill prints the
// pattern it scanned for instead of asserting a bare "none found".
var normalizeBackupPattern = regexp.MustCompile(`\.go\.\d+$`)

// normalizeArtifact is one recovery artifact the scan found, carrying the
// modification time reportNormalizeKill needs to decide whether this run's kill
// produced it. modTimeErr non-nil means the provenance is unknown, which is
// treated as "not this kill's" rather than guessed either way.
type normalizeArtifact struct {
	path       string
	modTime    time.Time
	modTimeErr error
}

// provenance renders why an artifact is not attributable to the kill being
// reported, for the operator who has to decide whether restoring it is safe.
func (a normalizeArtifact) provenance(fixerStart time.Time) string {
	if a.modTimeErr != nil {
		return fmt.Sprintf("its modification time could not be read: %v", a.modTimeErr)
	}
	return fmt.Sprintf("last modified %s, before this run's fixer started at %s", a.modTime.Format(time.RFC3339Nano), fixerStart.Format(time.RFC3339Nano))
}

// scanNormalizeRecoveryArtifacts walks module for normalizeBackupPattern
// matches whose original file still exists, returning them sorted by path with
// each one's modification time attached (correction R4-01: shape alone cannot
// tell this run's artifacts from an earlier run's). It requires
// the original sibling because that is what makes a match a recovery artifact
// rather than a coincidentally named file: gofmt opens the source O_WRONLY
// without O_TRUNC, so after a mid-write kill the file it was rewriting is
// always still there (corrupted), never missing. Any walk error is returned
// alongside whatever was found first, so the caller can report a partial scan
// as partial (never as a clean module). .git is skipped for the same reason
// discoverModules skips it: object files are not module sources.
func scanNormalizeRecoveryArtifacts(module string) ([]normalizeArtifact, error) {
	var artifacts []normalizeArtifact
	walkErr := filepath.WalkDir(module, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !normalizeBackupPattern.MatchString(d.Name()) {
			return nil
		}
		if _, statErr := os.Lstat(strings.TrimSuffix(path, filepath.Ext(path))); statErr != nil {
			return nil
		}
		artifact := normalizeArtifact{path: path}
		info, infoErr := d.Info()
		if infoErr != nil {
			artifact.modTimeErr = infoErr
		} else {
			artifact.modTime = info.ModTime()
		}
		artifacts = append(artifacts, artifact)
		return nil
	})
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].path < artifacts[j].path })
	return artifacts, walkErr
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
		ctx, cancel := context.WithTimeout(context.Background(), toolExecTimeout)
		cmd := exec.CommandContext(ctx, toolPath, c.checkArgv[1:]...)
		cmd.Dir = module
		cmd.WaitDelay = toolWaitDelay
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			// The deadline killed the process before it could analyze this
			// module; excluded exactly like an unrunnable process (D1), never
			// a verification failure. This module's exclusion already makes
			// modulesAttempted > modulesUsable, so renderRowSummary's
			// "modules=usable/attempted" note (correction B2) surfaces it on
			// the row instead of leaving it silent (same defect class C1/C2
			// fixed).
			continue
		}
		moduleExit := 0
		if runErr != nil {
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

// toolExecTimeout bounds every tool subprocess run by check and normalize
// (correction D1). The repo already solved this exact problem for its own
// network fetch: bin/labdrian-overlay:823-828 wraps `git fetch` in `timeout
// 10` specifically so a stalled network path cannot hang verification
// indefinitely, and buffers no partial evidence until the fetch resolves.
// This follows that shape, scoped per module rather than per whole check
// (matching correction B1's per-module isolation, so one stuck module never
// also consumes the budget every other module needs) and applied to each
// exec.CommandContext call site in runCheck and runNormalize. The duration
// itself cannot reuse the shell precedent's 10s: staticcheck and deadcode
// are invoked as "go run <module>@<version> ./..." (registry above), which
// on a cold module-proxy cache downloads and compiles the tool before it can
// analyze anything, not a lightweight network probe. 5 minutes is chosen as
// generous headroom above a plausible slow-but-legitimate cold run (module
// download + compile), so the deadline bounds a genuine hang without making
// the runner flaky on an ordinary slow network — flakiness from too short a
// deadline would be worse than the hang this exists to bound. A var, not a
// const, so tests can shrink it and prove real enforcement without waiting
// out a multi-minute deadline (see TestRunCheckEnforcesTimeout).
//
// That cold-download-and-compile rationale describes staticcheck and deadcode,
// which are check-only: neither has a normalizeArgv. The normalize path runs
// gofmt, a GOROOT binary resolved from PATH with nothing to fetch or build, so
// the rationale above does not justify 5 minutes there. The value is shared
// anyway, deliberately, because on the mutating path the deadline's length is
// not the safety lever — a shorter deadline still SIGKILLs gofmt mid-write
// (see reportNormalizeKill), it only shortens the wait before the same
// corruption. Tightening it would make that kill MORE likely on a large but
// legitimately slow tree, i.e. buy a faster failure by causing more of the
// damage this correction exists to disclose. Generous headroom on the path
// that rewrites files in place is the safer end of that trade; what makes the
// remaining risk survivable is the diagnostic, not the duration.
var toolExecTimeout = 5 * time.Minute

// toolWaitDelay bounds Wait() itself once toolExecTimeout's context is done
// (Go 1.20+ exec.Cmd.WaitDelay): killing a shell-invoked tool's direct
// process does not kill an orphaned grandchild (e.g. a subprocess a shell
// wrapper spawned) that still holds the stdout/stderr pipes open, so without
// WaitDelay a "killed" process can still leave Run() blocked reading those
// pipes until the orphan exits on its own — silently reintroducing the exact
// hang toolExecTimeout exists to bound. Kept short and separate from
// toolExecTimeout itself: once the deadline has already fired, this is only
// cleanup time for an already-cancelled run, not more budget for legitimate
// work. A var, alongside toolExecTimeout, so tests can shrink both and prove
// enforcement without waiting out either delay.
var toolWaitDelay = 5 * time.Second

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

// renderRowSummary extends renderSummary's rendered text with two
// independent coverage notes, each prepended so the existing "full: <path>"
// clause (parsed via a trailing-$ pattern in tests) stays the last thing in
// the row:
//   - "unavailable; " (D3, this correction) whenever r.unavailable is true.
//     renderSummary alone cannot make this distinction: it renders the bare
//     literal "0" both for a check that ran and found nothing and for a
//     check that could not run at all (LookPath failure, a toolchain
//     mismatch, or a killed-by-deadline module) — r.unavailable is what
//     selectOutcome actually consults to route to procedural_tooling_failed
//     (D4/R-016), yet it never reached the row bytes capture-evidence
//     receives. "unavailable" is deliberately not one of bannedSummaryLiterals
//     (R-008 bans narrated PASS/N/A-style words, not a factual runner state),
//     and guardAgainstBannedSummary only ever sees renderSummary's own bare
//     "0"/"count=N; ..." text, never this prefix, so the guard's invariant is
//     unaffected.
//   - "modules=usable/attempted" (correction B2) whenever runCheck excluded
//     at least one discovered module (a 127 exit or an unavailableIf
//     module). Without this, a row like "staticcheck | 1 | count=2; ..."
//     cannot tell a reader that one of four modules was never analysed —
//     partial coverage was indistinguishable from full coverage
//     (skills/sdd-verify/SKILL.md:70 treats these row bytes as the evidence
//     itself).
//
// Both notes can apply to the same row (a check unavailable across every
// attempted module: modulesAttempted > 0, modulesUsable == 0) and compose in
// a fixed order — "modules=0/N; unavailable; 0" — modules= first since it is
// the coarser coverage fact, unavailable second since it explains why
// coverage was zero. Full coverage and a check that ran (modulesUsable ==
// modulesAttempted, r.unavailable == false, including every result built
// outside runCheck's loop, which leaves both module fields at their zero
// value) renders byte-identical to plain renderSummary, so existing clean
// rows are unaffected — this is a report-only-when-applicable choice,
// deliberately not new row columns, to keep the two-pipe
// "tool | exit_code | summary" contract capture-evidence parses unchanged
// (D6).
func renderRowSummary(r result, topN int) string {
	summary := renderSummary(r.count, r.top, topN, r.logPath)
	if r.unavailable {
		summary = "unavailable; " + summary
	}
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
// otherwise a blocking-set check with partial module coverage →
// procedural_tooling_failed; (4) otherwise → passed. A WARNING-only check
// (e.g. deadcode) can never alone reach gate 1 or gate 3, and never
// suppresses a real blocking failure or unavailability elsewhere. Gate 2
// consults each check's own failed(exit, count) predicate (D3) rather than
// the raw exit code directly: gofmt -l exits 0 while listing violations, so
// trusting exitCode alone here missed that finding (obs #2735) — failed is
// defined for every registry entry, but a nil check is kept as a defensive
// guard against a future entry omitting it. classify() remains the sole
// effective-blocking enforcement point; failed only decides whether a check
// that IS effectively blocking counts as red.
//
// Gate 3 exists because correction B1 narrowed result.unavailable to "every
// attempted module was excluded" without teaching this function about the
// per-module coverage B1 introduced: a blocking check that could not analyze
// SOME modules and found nothing in the ones it could analyze reached gate 4
// and exited 0, which skills/sdd-verify/SKILL.md maps to `passed`. That is
// the live tui/go.mod (go 1.26.1) vs staticcheck@v0.7.0 case — one module
// never statically analyzed, gate still recording a clean verification.
// Before B1 that same condition exited 3, so the correction had converted a
// fail-closed into a pass. Exit 0 must mean "everything was verified and
// clean"; partial coverage with nothing found is "we could not verify",
// which is procedural_tooling_failed.
//
// Its placement is load-bearing in both directions. It sits AFTER gate 2
// because a run with genuine blocking findings is already a failing run: the
// operator fixes those and re-runs, and the coverage gap then surfaces as
// exit 3 on the re-run, so rewriting a real verification_failed into a
// tooling error here would only hide the findings the operator must act on
// (pinned by TestRunCheckToolchainMismatchInOneModuleDoesNotEraseFindings-
// InAnother). It sits BEFORE gate 4 because that is the only place a
// silently partial run can still be caught.
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
	for _, r := range results {
		if classify(r.check) && r.modulesAttempted > r.modulesUsable {
			return outcomeProceduralToolingFailed
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

// parseCheckArgs parses every check-mode flag from one shared FlagSet, so
// --top-n and --out-dir can never diverge on which flags they recognize
// (correction D2: two separate FlagSets used to exist, and the --top-n one
// never registered --out-dir, so "--out-dir D --top-n 20" silently returned
// defaultTopN with no diagnostic). Any parse error — malformed value or
// unrecognized flag — now fails loud (outcomeUsage) instead of guessing a
// default, matching the existing usage-error precedent for a bad
// subcommand and an out-dir-inside-root value.
func parseCheckArgs(args []string) (int, string, error) {
	fs := flag.NewFlagSet("deterministic-check-runner check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	topN := fs.Int("top-n", defaultTopN, "maximum finding excerpts per row")
	outDir := fs.String("out-dir", "", "directory for full check logs")
	if err := fs.Parse(args); err != nil {
		return 0, "", err
	}
	return *topN, *outDir, nil
}

// pathInsideRoot reports whether path is at or under root (R-010 guard).
// Both sides are symlink-resolved first: the previous version compared
// filepath.Abs results, and Abs only Cleans — it never resolves symlinks —
// so a path outside root that is, or traverses, a symlink resolving inside
// it passed the guard and the log write landed in the frozen candidate.
// Only filepath.EvalSymlinks closes that, via resolveNearestExisting.
//
// Resolution failure for any reason other than the out-dir simply not
// existing yet fails CLOSED (reports inside): an unverifiable path is not
// a safe one, and the cost of a false rejection is a re-run with an
// explicit --out-dir, while the cost of a false acceptance is a dirtied
// candidate and an invalidated receipt.
func pathInsideRoot(root, path string) bool {
	resolvedRoot, rootErr := resolveNearestExisting(root)
	resolvedPath, pathErr := resolveNearestExisting(path)
	if rootErr != nil || pathErr != nil {
		return true
	}
	return pathContainedIn(resolvedRoot, resolvedPath)
}

// resolveNearestExisting returns path made absolute with every symlink
// resolved. filepath.EvalSymlinks fails outright on a path that does not
// exist, and the out-dir legitimately may not exist yet (writeLog does the
// MkdirAll), so this walks up to the nearest existing ancestor, resolves
// that, and re-appends the unresolved remainder. Only a non-existence
// error walks up; anything else (a permission failure, an ancestor that is
// not a directory) is returned so the caller can fail closed.
func resolveNearestExisting(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	remainder := ""
	for {
		resolved, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			return filepath.Join(resolved, remainder), nil
		}
		if !errors.Is(evalErr, fs.ErrNotExist) {
			return "", evalErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", evalErr
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

// pathContainedIn reports whether the resolved path is at or under the
// resolved root, matching byte-exactly OR case-insensitively. The
// case-insensitive arm exists because strings.HasPrefix is byte-exact
// while a case-insensitive filesystem is not: on macOS — a documented host
// for this repository — a case-shifted path denotes the very directory a
// byte-exact comparison just refused to match. Over-rejecting a path that
// differs from root only by case on a case-sensitive filesystem is a
// deliberate fail-closed trade: this is a guard, so a spurious rejection
// costs one re-run with a different --out-dir, while a missed containment
// costs the frozen candidate. Both arms require a full path segment (the
// separator is part of the compared prefix), so a mere shared name prefix
// such as "<root>Sibling" is never treated as inside.
func pathContainedIn(root, path string) bool {
	if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
		return true
	}
	lowerRoot, lowerPath := strings.ToLower(root), strings.ToLower(path)
	return lowerPath == lowerRoot || strings.HasPrefix(lowerPath, lowerRoot+string(filepath.Separator))
}

// firstControlByte returns the first control byte in s (anything below
// 0x20, plus DEL) and whether one was found. Byte-wise, not rune-wise:
// the injection that matters is a raw 0x0A, and every byte in this range
// is its own rune in valid UTF-8 anyway, so scanning bytes is both exact
// and immune to invalid-UTF-8 input.
func firstControlByte(s string) (byte, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return s[i], true
		}
	}
	return 0, false
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
