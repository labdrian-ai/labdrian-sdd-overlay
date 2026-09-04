package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newChange creates <root>/openspec/changes/<name>/tasks.md with tasksContent,
// plus an empty specs/<capability>/spec.md delta for each named capability.
func newChange(t *testing.T, root, name, tasksContent string, capabilities ...string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "openspec", "changes", name, "tasks.md"), tasksContent)
	for _, cap := range capabilities {
		mustWriteFile(t, filepath.Join(root, "openspec", "changes", name, "specs", cap, "spec.md"), "# delta spec\n")
	}
}

// promoteCapability creates the canonical openspec/specs/<capability>/spec.md
// that marks a delta capability as already promoted.
func promoteCapability(t *testing.T, root, capability string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "openspec", "specs", capability, "spec.md"), "# canonical spec\n")
}

const allChecked = "# Tasks\n\n- [x] 1.1 do the thing\n- [x] 1.2 do the other thing\n"
const someUnchecked = "# Tasks\n\n- [x] 1.1 done\n- [ ] 1.2 not done yet\n"
const zeroCheckboxes = "# Tasks\n\nStill planning; no tasks written yet.\n"
const malformedTasks = "# Tasks\n\n- [x] 1.1 done\n- [oops] 1.2 what is this\n"

// TestRunSkipsChangeWithUncheckedTasks pins check 1's core rule: a change still
// in progress (at least one unchecked task) is not a stranded change, even
// though it already has some checked tasks.
func TestRunSkipsChangeWithUncheckedTasks(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "in-progress-change", someUnchecked)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeClean {
		t.Fatalf("run() with an in-progress change = exit %d, want %d (outcomeClean); stderr=%q", got, outcomeClean, stderr.String())
	}
	if strings.Contains(stderr.String(), "in-progress-change") {
		t.Errorf("stderr %q names the in-progress change, want it not reported", stderr.String())
	}
}

// TestRunReportsZeroCheckboxChangeAsUndeterminedNotStranded pins the
// distinction this guard exists to get right. Zero checkboxes is NOT the same
// fact as "planning-only": it only says no bullet checkbox was found, which is
// also true of a fully complete breakdown written in another shape. So such a
// change is neither reported as stranded (that would be a guess) nor passed
// over as clean (that was the old guess) — it is reported as undetermined.
func TestRunReportsZeroCheckboxChangeAsUndeterminedNotStranded(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "zero-checkbox-change", zeroCheckboxes)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a zero-checkbox change = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	if strings.Contains(stderr.String(), "STRANDED") {
		t.Errorf("stderr %q reports the change as stranded; the guard has no completion signal here and must not assert one", stderr.String())
	}
	if !strings.Contains(stderr.String(), "zero-checkbox-change") {
		t.Errorf("stderr %q does not name the change it could not classify", stderr.String())
	}
}

// TestRunReportsFullyCheckedChangeAsStranded pins the positive case: a change
// with at least one checkbox and zero unchecked ones is complete but still
// active, which is exactly the failure this guard exists to catch.
func TestRunReportsFullyCheckedChangeAsStranded(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "done-change", allChecked)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeStranded {
		t.Fatalf("run() with a fully checked change = exit %d, want %d (outcomeStranded); stderr=%q", got, outcomeStranded, stderr.String())
	}
	diagnostic := stderr.String()
	for _, want := range []string{"done-change", "2/2"} {
		if !strings.Contains(diagnostic, want) {
			t.Errorf("stranded diagnostic %q does not mention %q", diagnostic, want)
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("stranded run wrote %q to stdout, want the finding on stderr only", stdout.String())
	}
}

// TestRunDistinguishesPromotedFromUnpromotedCapability pins check 2: a
// stranded change whose capabilities are unpromoted is a strictly worse
// finding than one whose capabilities are already promoted, and the output
// must say which is which rather than reporting both the same way.
func TestRunDistinguishesPromotedFromUnpromotedCapability(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "promoted-change", allChecked, "promoted-capability")
	promoteCapability(t, root, "promoted-capability")
	newChange(t, root, "unpromoted-change", allChecked, "unpromoted-capability")

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeStranded {
		t.Fatalf("run() with two stranded changes = exit %d, want %d (outcomeStranded); stderr=%q", got, outcomeStranded, stderr.String())
	}
	diagnostic := stderr.String()
	for _, want := range []string{"promoted-change", "unpromoted-change", "promoted-capability", "unpromoted-capability"} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostic %q does not mention %q", diagnostic, want)
		}
	}

	promotedIdx := strings.Index(diagnostic, "promoted-change")
	promotedBlockEnd := strings.Index(diagnostic, "unpromoted-change")
	if promotedIdx < 0 || promotedBlockEnd < 0 || promotedBlockEnd < promotedIdx {
		t.Fatalf("could not locate both change blocks in diagnostic %q", diagnostic)
	}
	promotedBlock := diagnostic[promotedIdx:promotedBlockEnd]
	unpromotedBlock := diagnostic[promotedBlockEnd:]

	if !strings.Contains(strings.ToLower(promotedBlock), "promoted") || strings.Contains(strings.ToLower(promotedBlock), "not promoted") {
		t.Errorf("promoted-change block %q does not clearly say its capability is promoted", promotedBlock)
	}
	if !strings.Contains(strings.ToLower(unpromotedBlock), "not promoted") {
		t.Errorf("unpromoted-change block %q does not clearly say its capability is NOT promoted", unpromotedBlock)
	}
	// The two must be distinguished, not just both mentioned: an unpromoted
	// capability is strictly worse and the output must say so.
	if !strings.Contains(strings.ToLower(diagnostic), "incomplete") && !strings.Contains(strings.ToLower(diagnostic), "worse") {
		t.Errorf("diagnostic %q does not explain that the unpromoted case is a worse finding", diagnostic)
	}
}

// TestRunReportsStrandedChangeWithNoDeltaCapabilities covers a change with no
// specs/ subdirectory at all: it is still stranded (it has completed tasks),
// but there is nothing to promote-check, so this must be reported without
// treating the missing specs/ directory as a parse failure.
func TestRunReportsStrandedChangeWithNoDeltaCapabilities(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "no-capabilities-change", allChecked)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeStranded {
		t.Fatalf("run() with a capability-less stranded change = exit %d, want %d (outcomeStranded); stderr=%q", got, outcomeStranded, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no-capabilities-change") {
		t.Errorf("stderr %q does not name the stranded change", stderr.String())
	}
}

// TestRunExcludesArchiveDirectory pins that openspec/changes/archive/ is never
// scanned as if its entries were active changes, even though archived changes
// also carry fully-checked tasks.md files by construction.
func TestRunExcludesArchiveDirectory(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "openspec", "changes", "archive", "2026-01-01-old-change", "tasks.md"), allChecked)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeClean {
		t.Fatalf("run() with only an archived change = exit %d, want %d (outcomeClean); stderr=%q", got, outcomeClean, stderr.String())
	}
}

// TestRunFailsClosedWhenChangesDirIsMissing pins the fail-closed contract: a
// repository whose openspec/changes/ cannot be read must never report clean.
func TestRunFailsClosedWhenChangesDirIsMissing(t *testing.T) {
	root := t.TempDir() // no openspec/changes/ at all

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with no openspec/changes/ = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	if !strings.Contains(stderr.String(), "openspec/changes") {
		t.Errorf("undetermined diagnostic %q does not name openspec/changes", stderr.String())
	}
	if !strings.Contains(stderr.String(), "fails closed") {
		t.Errorf("undetermined diagnostic %q does not state the fail-closed stance", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("undetermined run wrote %q to stdout, want diagnostics on stderr only", stdout.String())
	}
}

// TestRunFailsClosedOnMalformedTasksFile pins the other fail-closed path: a
// tasks.md this guard cannot confidently parse must never be silently treated
// as zero checkboxes (which would hide a stranded change) or as fully checked
// (which would report a false positive). Either way exit 0 is forbidden.
func TestRunFailsClosedOnMalformedTasksFile(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "malformed-change", malformedTasks)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a malformed tasks.md = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	if !strings.Contains(stderr.String(), "malformed-change") {
		t.Errorf("undetermined diagnostic %q does not name the offending change", stderr.String())
	}
}

// TestRunFailsClosedWhenTasksFileIsMissing covers a change directory that
// exists but has no tasks.md at all: this guard has no data to compute
// completion from, so it must fail closed rather than guess "0 checkboxes,
// not stranded."
func TestRunFailsClosedWhenTasksFileIsMissing(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "openspec", "changes", "no-tasks-file"))

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a missing tasks.md = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no-tasks-file") {
		t.Errorf("undetermined diagnostic %q does not name the offending change", stderr.String())
	}
}

// TestRunFailsClosedWhenRepoRootDoesNotExist covers a bad --repo value: this
// is a different failure than a bad flag (exit 2), because the invocation
// itself is well-formed and only the target cannot be resolved.
func TestRunFailsClosedWhenRepoRootDoesNotExist(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does-not-exist")

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", missing}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a nonexistent --repo = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
}

// TestRunCleanFixtureExitsZero covers a repository whose active changes are
// all readable and all still in progress. A zero-checkbox change used to be
// part of this fixture, on the assumption it was "planning-only"; it now
// belongs to the undetermined tests instead, because the guard cannot in fact
// tell a planning-only file from an unreadable breakdown.
func TestRunCleanFixtureExitsZero(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "in-progress", someUnchecked)
	newChange(t, root, "also-in-progress", someUnchecked)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeClean {
		t.Fatalf("run() with a clean fixture = exit %d, want %d (outcomeClean); stderr=%q", got, outcomeClean, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("clean run wrote %q to stderr, want nothing", stderr.String())
	}
	if !strings.Contains(stdout.String(), "clean") {
		t.Errorf("clean stdout %q does not say clean", stdout.String())
	}
}

// TestRunRejectsBadInvocations pins exit 2 for usage errors, mirroring the
// review-preflight sibling's contract: a typo in a flag is the operator's to
// fix and must not be conflated with "could not determine."
func TestRunRejectsBadInvocations(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--repoo", "/tmp"}},
		{name: "unexpected positional argument", args: []string{"check"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tc.args, &stdout, &stderr)

			if got != outcomeUsage {
				t.Fatalf("run(%v) = exit %d, want %d (outcomeUsage); stderr=%q", tc.args, got, outcomeUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "usage:") {
				t.Errorf("usage error %q does not reprint the usage text", stderr.String())
			}
		})
	}
}

// TestRunDefaultsToTheCallerWorkingDirectory covers the no-flag path: with no
// --repo given, the guard must scan the caller's own working directory,
// exactly like the sibling review-preflight and deterministic-checks guards
// default to the caller's cwd rather than requiring an explicit path.
func TestRunDefaultsToTheCallerWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "done-change", allChecked)

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir into %s: %v", root, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeStranded {
		t.Fatalf("run() with no --repo from inside the fixture = exit %d, want %d (outcomeStranded); stderr=%q", got, outcomeStranded, stderr.String())
	}
	if !strings.Contains(stderr.String(), "done-change") {
		t.Errorf("stderr %q does not name the stranded change", stderr.String())
	}
}

// TestRunHandlesCheckboxesInsideFencesAndComments pins the R3-001/R4-001 fix:
// a checkbox-shaped line inside a fenced code block or an HTML comment must
// never count toward total or unchecked. Before the fix, an unchecked
// fenced/hidden example kept unchecked > 0 and hid a stranded change, while
// checked fenced examples inflated total and falsely flagged planning-only
// changes as stranded.
func TestRunHandlesCheckboxesInsideFencesAndComments(t *testing.T) {
	cases := []struct {
		name       string
		changeName string
		content    string
		wantExit   int
		wantNamed  bool
		// wantDiagnostic, when set, must appear in the report. It exists for
		// the fenced case: asserting only "exit 3, change named" cannot tell
		// "no completion marker of any shape" from "a marker in a shape I do
		// not read", so moving the other-marker accumulator above the fence
		// skip would count a fenced EXAMPLE as a real completion marker and
		// the suite would stay green while the diagnostic told the operator
		// the opposite of the truth.
		wantDiagnostic string
	}{
		{
			name:       "unchecked example inside fence still reported stranded",
			changeName: "fenced-example-change",
			content:    "# Tasks\n\n- [x] 1.1 real task one\n- [x] 1.2 real task two\n\nExample usage:\n\n```bash\n- [ ] example placeholder, not a real task\n```\n",
			wantExit:   outcomeStranded,
			wantNamed:  true,
		},
		{
			// Fenced examples must not be counted as tasks, which leaves this
			// file with zero real checkboxes — undetermined, not clean. The
			// assertion that matters here is still the fence rule: the
			// examples never inflate total and never make it look stranded.
			name:       "checked examples only inside fence never count as tasks",
			changeName: "fenced-no-real-tasks-change",
			content:    "# Tasks\n\nPlanning notes; example checkbox format:\n\n```\n- [x] example done marker\n- [x] another example marker\n```\n\nNo real tasks have been broken down yet.\n",
			wantExit:   outcomeUndetermined,
			wantNamed:  true,
			// Two checked markers sit in this file, both fenced. The report
			// must say the file carries no marker of ANY shape: a fenced
			// example is not a completion signal in an unread shape, it is
			// not a completion signal at all.
			wantDiagnostic: "any shape",
		},
		{
			name:       "hidden unchecked task inside HTML comment ignored",
			changeName: "html-comment-change",
			content:    "# Tasks\n\n- [x] 1.1 real task\n\n<!--\n- [ ] hidden example, ignore me\n-->\n",
			wantExit:   outcomeStranded,
			wantNamed:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			newChange(t, root, tc.changeName, tc.content)

			var stdout, stderr bytes.Buffer
			got := run([]string{"--repo", root}, &stdout, &stderr)

			if got != tc.wantExit {
				t.Fatalf("run() = exit %d, want %d; stderr=%q", got, tc.wantExit, stderr.String())
			}
			if named := strings.Contains(stderr.String(), tc.changeName); named != tc.wantNamed {
				t.Errorf("stderr %q names %q = %v, want %v", stderr.String(), tc.changeName, named, tc.wantNamed)
			}
			if tc.wantDiagnostic != "" && !strings.Contains(stderr.String(), tc.wantDiagnostic) {
				t.Errorf("stderr %q does not contain %q -- a fenced example must not be reported as a completion marker in an unread shape", stderr.String(), tc.wantDiagnostic)
			}
		})
	}
}

// TestRunReportsStrandedChangeUsingAlternateBulletMarkers pins the R3-002
// fix: CommonMark's "*" and "+" bullet markers must be accepted like "-".
// Before the fix, checkboxPattern matched neither, so total stayed 0 and the
// change was silently treated as planning-only forever.
func TestRunReportsStrandedChangeUsingAlternateBulletMarkers(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "asterisk bullets", content: "# Tasks\n\n* [x] 1.1 do the thing\n* [x] 1.2 do the other thing\n"},
		{name: "plus bullets", content: "# Tasks\n\n+ [x] 1.1 do the thing\n+ [x] 1.2 do the other thing\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			newChange(t, root, "alt-bullet-change", tc.content)

			var stdout, stderr bytes.Buffer
			got := run([]string{"--repo", root}, &stdout, &stderr)

			if got != outcomeStranded {
				t.Fatalf("run() = exit %d, want %d (outcomeStranded); stderr=%q", got, outcomeStranded, stderr.String())
			}
			if !strings.Contains(stderr.String(), "alt-bullet-change") {
				t.Errorf("stderr %q does not name the stranded change", stderr.String())
			}
		})
	}
}

// TestRunFailsClosedOnMalformedCheckboxOutsideFence is a regression guard:
// fence-awareness must not swallow a genuinely malformed checkbox outside
// any fence, nor report the fenced distraction line instead of the real one.
func TestRunFailsClosedOnMalformedCheckboxOutsideFence(t *testing.T) {
	root := t.TempDir()
	content := "# Tasks\n\nExample:\n\n```bash\n- [oops-in-example] ignore me\n```\n\n- [x] 1.1 done\n- [oops] 1.2 what is this\n"
	newChange(t, root, "malformed-outside-fence-change", content)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a malformed checkbox outside any fence = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, "what is this") {
		t.Errorf("undetermined diagnostic %q does not report the real malformed line outside the fence", diagnostic)
	}
	if strings.Contains(diagnostic, "oops-in-example") {
		t.Errorf("undetermined diagnostic %q reports a malformed checkbox found INSIDE a fenced code block, want it ignored", diagnostic)
	}
}

// TestRunAgainstLiveRepository is the guard's own dogfood test: run against
// this actual repository (via --repo, never by chdir-ing away from the module
// under test) and assert only what stays true as the repository evolves.
//
// It used to pin the live scan by change NAME — it asserted that
// anti-generic-design-runtime-wiring and gadu-portable-operator were reported
// stranded. Both have since been archived, which is exactly the outcome this
// guard exists to produce, and the test failed for it. That form was removed
// rather than re-pointed at today's names: a test that breaks every time the
// repository improves teaches its next reader to edit the expectation instead
// of reading the failure, and the previous version's own failure message
// invited precisely that ("update wantStranded above"). Names live in the
// fixture tests above, where a name means something because the test created
// it. What is left here is the part that does not rot: the guard runs to
// completion against a real corpus, emits a verdict a human can parse, and its
// exit code agrees with the lists it printed.
func TestRunAgainstLiveRepository(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "openspec", "changes")); err != nil {
		t.Skipf("live openspec/changes not found under %s, skipping dogfood test: %v", root, err)
	}

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	switch got {
	case outcomeClean, outcomeStranded, outcomeUndetermined:
	default:
		t.Fatalf("run() against the live repository = exit %d, which is not a scan verdict at all (want 0, 1, or 3); stdout=%q stderr=%q", got, stdout.String(), stderr.String())
	}

	diagnostic := stderr.String()
	namedStranded := strings.Contains(diagnostic, "STRANDED change(s)")
	namedUndetermined := strings.Contains(diagnostic, "UNDETERMINED change(s)")

	// The verdict must be legible: every non-clean run says which kind of
	// finding it made, and a clean run says clean on stdout and nothing on
	// stderr.
	if got == outcomeClean {
		if diagnostic != "" {
			t.Errorf("live run exited %d (outcomeClean) but wrote %q to stderr; a clean verdict must not carry findings", outcomeClean, diagnostic)
		}
		if !strings.Contains(stdout.String(), "clean") {
			t.Errorf("live clean stdout %q does not say clean", stdout.String())
		}
		return
	}
	if !namedStranded && !namedUndetermined {
		t.Fatalf("live run exited %d but its diagnostic %q names neither a stranded nor an undetermined list; the verdict is not parseable", got, diagnostic)
	}

	// The exit code must agree with what was printed. These are the two
	// directions that would let the guard lie about its own completeness.
	if namedUndetermined && got != outcomeUndetermined {
		t.Errorf("live run printed an undetermined list but exited %d, want %d (outcomeUndetermined): naming a change it could not classify while exiting under any other code claims a completeness it does not have; stderr=%q", got, outcomeUndetermined, diagnostic)
	}
	if namedStranded && got == outcomeClean {
		t.Errorf("live run printed a stranded list but exited %d (outcomeClean); stderr=%q", got, diagnostic)
	}
	if got == outcomeStranded && namedUndetermined {
		t.Errorf("live run exited %d (outcomeStranded) with undetermined changes named; stranded must never outrank undetermined; stderr=%q", got, diagnostic)
	}
}

// An unterminated fence or HTML comment means every line after it was skipped,
// so the task counters describe only part of the file. Returning them would let
// a genuinely stranded change hide behind an unclosed ``` and be reported clean
// — the exact silent miss this guard exists to prevent, reintroduced through the
// skipping machinery added to fix the fenced-example case. These pin the fix.
func TestRunFailsClosedOnUnterminatedFenceOrComment(t *testing.T) {
	for _, tt := range []struct {
		name  string
		tasks string
	}{
		{
			name:  "unterminated fence hides real completed tasks",
			tasks: "```\n- [ ] example, fence never closes\n\n- [x] real one\n- [x] real two\n",
		},
		{
			name:  "unterminated html comment hides a real pending task",
			tasks: "<!--\n- [x] commented example\n\n- [ ] real pending\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			newChange(t, root, "c", tt.tasks)
			var out, errOut bytes.Buffer
			if got := run([]string{"--repo", root}, &out, &errOut); got != outcomeUndetermined {
				t.Fatalf("exit %d, want %d (outcomeUndetermined) — an unterminated fence or comment "+
					"must fail closed, never report clean; stderr=%q", got, outcomeUndetermined, errOut.String())
			}
			if !strings.Contains(errOut.String(), "unterminated") {
				t.Fatalf("diagnostic must name the unterminated block so the operator can find it; got %q", errOut.String())
			}
		})
	}
}

// CommonMark allows at most three spaces before a fence; four or more is an
// indented code block. Treating a deeply indented line as a fence opener would
// let a coincidental open/close pair swallow the real tasks between them.
func TestRunTreatsDeeplyIndentedBackticksAsNotAFence(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "c", "    ```\n- [x] real one\n    ```\n- [x] real two\n")
	var out, errOut bytes.Buffer
	if got := run([]string{"--repo", root}, &out, &errOut); got != outcomeStranded {
		t.Fatalf("exit %d, want %d (outcomeStranded) — a 4-space-indented ``` is not a fence, "+
			"so both real tasks must still be counted; stdout=%q stderr=%q",
			got, outcomeStranded, out.String(), errOut.String())
	}
}

// Four separate silent misses shipped in this tool before the policy changed
// from "guess" to "refuse": unrastreado fences, unrecognised bullet markers, an
// unterminated fence, and the three vectors below. A line-oriented reader cannot
// resolve markdown block context, so every line it cannot classify must fail
// closed rather than be counted or dropped.
func TestRunRefusesToGuessOnAmbiguousLines(t *testing.T) {
	for _, tt := range []struct {
		name  string
		tasks string
		why   string
	}{
		{
			name:  "checkbox sharing a line with an inline HTML comment",
			tasks: "- [x] 1.1 done <!-- verified -->\n",
			why:   "silently counted as zero tasks, so a stranded change read as planning-only",
		},
		{
			name:  "checkbox indented into a CommonMark indented code block",
			tasks: "- [x] real one\n    - [ ] indented example\n",
			why:   "silently counted as a real unchecked task, so a stranded change read as in progress",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			newChange(t, root, "c", tt.tasks)
			var out, errOut bytes.Buffer
			if got := run([]string{"--repo", root}, &out, &errOut); got != outcomeUndetermined {
				t.Fatalf("exit %d, want %d (outcomeUndetermined) — before the refuse policy this %s; stderr=%q",
					got, outcomeUndetermined, tt.why, errOut.String())
			}
			if !strings.Contains(errOut.String(), "refuses to guess") {
				t.Fatalf("the diagnostic must say the guard refused rather than failed, so the operator knows to disambiguate the file; got %q", errOut.String())
			}
		})
	}
}

// A bullet whose first token is a markdown link is ordinary prose. Failing the
// scan on it would make the guard unusable on real task files, which is how a
// fail-closed policy turns into a guard nobody can keep enabled.
func TestRunAcceptsMarkdownLinkBullets(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "c", "- [see the design](design.md)\n- [x] 1.1 real task\n")
	var out, errOut bytes.Buffer
	if got := run([]string{"--repo", root}, &out, &errOut); got != outcomeStranded {
		t.Fatalf("exit %d, want %d (outcomeStranded) — a markdown-link bullet must not be mistaken for a malformed checkbox; stdout=%q stderr=%q",
			got, outcomeStranded, out.String(), errOut.String())
	}
}

// zeroBulletsOtherMarkers is the shape four live changes in this repository
// actually use: a real, complete task breakdown whose completion markers are
// not bullet checkboxes at all. skill-lifecycle carries ten
// "**Status**: [x] done" lines and zero bullet checkboxes.
const zeroBulletsOtherMarkers = "# Tasks\n\n## T-01 first task\n**Status**: [x] done\n\n## T-02 second task\n**Status**: [x] done\n"

// zeroCheckboxesOfAnyShape is the other unreadable shape: a full task
// breakdown expressed as headings, with no checkbox-shaped marker anywhere.
const zeroCheckboxesOfAnyShape = "# Tasks\n\n### T-01 first task\nDepends on: none.\n\n### T-02 second task\nDepends on: T-01.\n"

// TestRunReportsZeroBulletCheckboxesWithOtherMarkersAsUndetermined pins the
// core correction: total == 0 does not mean "planning-only", it means "no
// bullet checkbox was found", and this guard cannot tell those apart. It must
// report the change as undetermined and name the shape it actually observed
// so the operator knows which kind of unreadable it is.
func TestRunReportsZeroBulletCheckboxesWithOtherMarkersAsUndetermined(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "other-shape-change", zeroBulletsOtherMarkers)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a zero-bullet change carrying other [x] markers = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, "other-shape-change") {
		t.Errorf("undetermined diagnostic %q does not name the change", diagnostic)
	}
	if !strings.Contains(diagnostic, "2 other") {
		t.Errorf("undetermined diagnostic %q does not report the 2 other [x]-shaped markers it observed", diagnostic)
	}
}

// TestRunReportsNoCheckboxOfAnyShapeAsUndetermined pins the second observed
// shape: a tasks.md with a real breakdown and no checkbox-shaped marker at
// all is still unreadable to this parser, and still undetermined.
func TestRunReportsNoCheckboxOfAnyShapeAsUndetermined(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "no-shape-change", zeroCheckboxesOfAnyShape)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a change carrying no checkbox of any shape = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, "no-shape-change") {
		t.Errorf("undetermined diagnostic %q does not name the change", diagnostic)
	}
	if !strings.Contains(diagnostic, "any shape") {
		t.Errorf("undetermined diagnostic %q does not distinguish this from the other-marker shape", diagnostic)
	}
}

// TestRunReportsEveryUndeterminedChangeNotJustTheFirst pins that the guard
// collects undetermined changes instead of dying on the first: a guard that
// names all of them is worth more than one that reports one and stops.
func TestRunReportsEveryUndeterminedChangeNotJustTheFirst(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "undetermined-a", zeroBulletsOtherMarkers)
	newChange(t, root, "undetermined-b", zeroCheckboxesOfAnyShape)
	newChange(t, root, "undetermined-c", zeroCheckboxes)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with three undetermined changes = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	for _, name := range []string{"undetermined-a", "undetermined-b", "undetermined-c"} {
		if !strings.Contains(stderr.String(), name) {
			t.Errorf("undetermined diagnostic %q does not name %q; the guard stopped at the first instead of reporting all", stderr.String(), name)
		}
	}
}

// TestRunPrintsBothListsWhenUndeterminedAndStrandedCoexist pins that neither
// list is suppressed by the other: the operator needs the stranded changes it
// did find AND the ones it could not classify, in one run.
func TestRunPrintsBothListsWhenUndeterminedAndStrandedCoexist(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "stranded-change", allChecked)
	newChange(t, root, "undetermined-change", zeroBulletsOtherMarkers)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with one stranded and one undetermined = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, "stranded-change") {
		t.Errorf("diagnostic %q drops the stranded list when something was undetermined", diagnostic)
	}
	if !strings.Contains(diagnostic, "undetermined-change") {
		t.Errorf("diagnostic %q drops the undetermined list", diagnostic)
	}
	if !strings.Contains(diagnostic, "incomplete") {
		t.Errorf("diagnostic %q does not say the stranded list is incomplete, which is the whole reason exit 3 outranks exit 1", diagnostic)
	}
}

// TestRunUndeterminedOutranksStranded is the revert-proof assertion for the
// exit precedence itself. It is deliberately separate from the both-lists
// test above so that a later editor who "simplifies" the precedence back to
// stranded-wins cannot make the suite pass by adjusting a message: the only
// thing asserted here is that a scan carrying an undetermined change never
// exits 1, because exit 1 means "here are the stranded ones" and that claims
// a completeness an unclassified change denies.
func TestRunUndeterminedOutranksStranded(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "stranded-change", allChecked)
	newChange(t, root, "undetermined-change", zeroCheckboxesOfAnyShape)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got == outcomeStranded {
		t.Fatalf("run() exited %d (outcomeStranded) with an undetermined change present; the stranded list is incomplete, so exit %d (outcomeUndetermined) is the only honest code", got, outcomeUndetermined)
	}
	if got != outcomeUndetermined {
		t.Fatalf("run() = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
}

// TestUnreadableTasksFileStaysAHardErrorNotAnUndeterminedEntry is the twin
// check for the undetermined collection above. Both a missing tasks.md and an
// unclassifiable one now exit 3, so it would be easy to quietly fold the first
// into the second — and that would lose a real distinction. A missing file is
// a broken change directory the operator must repair; a zero-checkbox file is
// an intact change written in a shape this guard cannot read. They get
// different messages and different remediations, and the missing-file case
// still aborts the scan rather than joining a list.
func TestUnreadableTasksFileStaysAHardErrorNotAnUndeterminedEntry(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "openspec", "changes", "broken-change"))

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a missing tasks.md = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, "could not be read") {
		t.Errorf("diagnostic %q lost the read error; a missing tasks.md must still report that the file could not be read, not be listed as an unreadable-shape change", diagnostic)
	}
	if strings.Contains(diagnostic, "UNDETERMINED change(s)") {
		t.Errorf("diagnostic %q folded a missing tasks.md into the undetermined-shape list; it is a broken change directory, which is a different fact with a different fix", diagnostic)
	}
}

// TestEmptyTasksFileIsUndetermined pins the other half of the twin: a
// tasks.md that exists and is empty is readable, so it is not the hard error
// above — but it carries no completion marker either, so it is undetermined
// like any other zero-checkbox file rather than being waved through as clean.
func TestEmptyTasksFileIsUndetermined(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "empty-tasks-change", "")

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with an empty tasks.md = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	if !strings.Contains(stderr.String(), "any shape") {
		t.Errorf("diagnostic %q does not report that no checkbox of any shape was found", stderr.String())
	}
}

// TestRunCleanScanSaysSoWithoutOverclaiming pins the honest clean case: every
// active change was classifiable and none was stranded, so exit 0 and say so.
func TestRunCleanScanSaysSoWithoutOverclaiming(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "in-progress-one", someUnchecked)
	newChange(t, root, "in-progress-two", someUnchecked)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeClean {
		t.Fatalf("run() with a fully classifiable, unstranded fixture = exit %d, want %d (outcomeClean); stderr=%q", got, outcomeClean, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("clean run wrote %q to stderr, want nothing", stderr.String())
	}
	if !strings.Contains(stdout.String(), "clean") {
		t.Errorf("clean stdout %q does not say clean", stdout.String())
	}
}
