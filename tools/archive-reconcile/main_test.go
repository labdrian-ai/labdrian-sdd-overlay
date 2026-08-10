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

// TestRunSkipsPlanningOnlyChangeWithZeroCheckboxes pins the distinction this
// guard exists to get right: zero checkboxes means the change has not started
// task breakdown at all, which is a completely different fact from "complete
// but not archived." Four real changes in this repository
// (skill-lifecycle, skill-manifest-gen, skill-package-manager,
// skill-project-scope) are in exactly this state and must never be reported.
func TestRunSkipsPlanningOnlyChangeWithZeroCheckboxes(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "planning-only-change", zeroCheckboxes)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	if got != outcomeClean {
		t.Fatalf("run() with a zero-checkbox change = exit %d, want %d (outcomeClean); stderr=%q", got, outcomeClean, stderr.String())
	}
	if strings.Contains(stderr.String(), "planning-only-change") {
		t.Errorf("stderr %q names the planning-only change, want it not reported", stderr.String())
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

// TestRunCleanFixtureExitsZero covers a repository with a healthy mix: an
// in-progress change and a planning-only change, neither of which is
// stranded.
func TestRunCleanFixtureExitsZero(t *testing.T) {
	root := t.TempDir()
	newChange(t, root, "in-progress", someUnchecked)
	newChange(t, root, "planning-only", zeroCheckboxes)

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
	}{
		{
			name:       "unchecked example inside fence still reported stranded",
			changeName: "fenced-example-change",
			content:    "# Tasks\n\n- [x] 1.1 real task one\n- [x] 1.2 real task two\n\nExample usage:\n\n```bash\n- [ ] example placeholder, not a real task\n```\n",
			wantExit:   outcomeStranded,
			wantNamed:  true,
		},
		{
			name:       "checked examples only inside fence stay planning-only",
			changeName: "fenced-planning-only-change",
			content:    "# Tasks\n\nPlanning notes; example checkbox format:\n\n```\n- [x] example done marker\n- [x] another example marker\n```\n\nNo real tasks have been broken down yet.\n",
			wantExit:   outcomeClean,
			wantNamed:  false,
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
// under test) and assert it reports exactly the two known stranded changes
// documented at the time this guard was built, and none of the four
// zero-checkbox planning-only changes.
//
// NOTE for future maintainers: this test WILL start failing the moment either
// anti-generic-design-runtime-wiring or gadu-portable-operator is archived and
// its capabilities promoted — that is progress, not a regression. When that
// happens, update the "wantStranded" list below (removing the archived
// change) rather than treating a failure here as a bug in the guard itself.
func TestRunAgainstLiveRepository(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "openspec", "changes")); err != nil {
		t.Skipf("live openspec/changes not found under %s, skipping dogfood test: %v", root, err)
	}

	var stdout, stderr bytes.Buffer
	got := run([]string{"--repo", root}, &stdout, &stderr)

	wantStranded := []string{"anti-generic-design-runtime-wiring", "gadu-portable-operator"}
	notStranded := []string{"skill-lifecycle", "skill-manifest-gen", "skill-package-manager", "skill-project-scope"}

	if got != outcomeStranded {
		t.Fatalf("run() against the live repository = exit %d, want %d (outcomeStranded); this test's expectations are stale if %v have already been archived — update wantStranded above rather than assuming a guard bug; stderr=%q",
			got, outcomeStranded, wantStranded, stderr.String())
	}
	diagnostic := stderr.String()
	for _, name := range wantStranded {
		if !strings.Contains(diagnostic, name) {
			t.Errorf("live-repository diagnostic %q does not mention %q; if this change was archived since this test was written, remove it from wantStranded above instead of treating this as a guard bug", diagnostic, name)
		}
	}
	for _, name := range notStranded {
		if strings.Contains(diagnostic, name) {
			t.Errorf("live-repository diagnostic %q mentions planning-only change %q, want it never reported; if %q has since gained checked tasks this expectation is stale and must be updated, not silenced", diagnostic, name, name)
		}
	}
}
