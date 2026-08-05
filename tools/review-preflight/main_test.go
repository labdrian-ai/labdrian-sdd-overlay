package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// emptyCandidateStatus is the real `gentle-ai review status` payload observed
// in this repository against a clean working tree. It is the exact shape this
// guard exists to catch: the workspace projection resolves base_tree and
// current_candidate_tree to the same tree and covers zero paths, so a review
// started against it inspects nothing and still produces an approved receipt.
const emptyCandidateStatus = `{
  "schema": "gentle-ai.review-integration.status/v3",
  "contract": "gentle-ai.review-integration/v2",
  "operation": "review.status",
  "action": "start",
  "projection": {
    "schema": "gentle-ai.review-integration.projection/v1",
    "kind": "current-changes",
    "projection": "workspace",
    "base_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
    "current_candidate_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
    "paths": [],
    "intended_untracked": []
  },
  "candidates": []
}`

// healthyCandidateStatus is the real payload observed while the same two files
// were still uncommitted: the trees differ and the projection names the paths
// the review would actually inspect. This is the case that must pass.
const healthyCandidateStatus = `{
  "schema": "gentle-ai.review-integration.status/v3",
  "contract": "gentle-ai.review-integration/v2",
  "operation": "review.status",
  "action": "start",
  "projection": {
    "schema": "gentle-ai.review-integration.projection/v1",
    "kind": "current-changes",
    "projection": "workspace",
    "base_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
    "current_candidate_tree": "9773630db7b612668160b438a4415ed65ae12e67",
    "paths": ["tools/deterministic-check-runner/main.go", "tools/deterministic-check-runner/main_test.go"],
    "intended_untracked": []
  },
  "candidates": []
}`

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

// stubStatusProbe replaces the status seam for the duration of one test and
// restores it afterwards. Every run()-level test goes through this seam
// because CI has no gentle-ai installed: a guard that could only be exercised
// against a real binary would be untested exactly where it is expected to run.
func stubStatusProbe(t *testing.T, probe statusProbe) {
	t.Helper()
	original := runReviewStatus
	runReviewStatus = probe
	t.Cleanup(func() { runReviewStatus = original })
}

// staticStatusProbe returns a probe that always answers with payload.
func staticStatusProbe(payload string) statusProbe {
	return func(dir, baseRef string) ([]byte, error) { return []byte(payload), nil }
}

// failingStatusProbe returns a probe that always fails with message.
func failingStatusProbe(message string) statusProbe {
	return func(dir, baseRef string) ([]byte, error) { return nil, fmt.Errorf("%s", message) }
}

// projectionStatus wraps a projection body in the surrounding status document
// so tests exercise the same decode path the real payload takes.
func projectionStatus(body string) string {
	return `{"schema": "gentle-ai.review-integration.status/v3", "projection": ` + body + `}`
}

func mustWriteFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// shadowPATH points PATH at dir alone for the duration of one test, so the
// real exec path can be exercised without a gentle-ai installed anywhere.
func shadowPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir)
}

// fakeGentleAI installs an executable named gentle-ai in a fresh directory and
// PREPENDS that directory to PATH. body is the shell script the fake runs.
//
// Prepending rather than replacing: any real gentle-ai is still shadowed, but
// the fake script keeps the ordinary utilities it needs on PATH. Replacing PATH
// outright made a fake that shells out fail with "not found" instead of
// exercising the behavior under test.
func fakeGentleAI(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "gentle-ai"), "#!/bin/sh\n"+body, 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// TestModuleShape asserts parity with the sibling deterministic-check-runner
// module's shape, and that this module has zero external dependencies.
func TestModuleShape(t *testing.T) {
	root := repoRoot(t)
	preflightDir := filepath.Join(root, "tools", "review-preflight")
	runnerDir := filepath.Join(root, "tools", "deterministic-check-runner")

	for _, name := range []string{"go.mod", "main.go", "main_test.go"} {
		if _, err := os.Stat(filepath.Join(runnerDir, name)); err != nil {
			t.Fatalf("sibling deterministic-check-runner/%s missing: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(preflightDir, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(preflightDir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	modContent := string(data)
	const wantModule = "module github.com/labdrian-ai/labdrian-sdd-overlay/tools/review-preflight"
	if !strings.Contains(modContent, wantModule) {
		t.Errorf("go.mod module path = %q, want it to contain %q", modContent, wantModule)
	}
	if strings.Contains(modContent, "require") {
		t.Errorf("go.mod declares a dependency, want zero external dependencies: %q", modContent)
	}
}

// TestRunBlocksTheRealEmptyWorkspaceCandidate drives the exact payload observed
// in this repository with a clean tree. Exit 1 alone is not enough: an operator
// who has never hit this must be able to act on the diagnostic without reading
// any other document, because nothing in the repo documents --base-ref today.
func TestRunBlocksTheRealEmptyWorkspaceCandidate(t *testing.T) {
	stubStatusProbe(t, staticStatusProbe(emptyCandidateStatus))

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeEmptyCandidate {
		t.Fatalf("run() against the observed empty workspace candidate = exit %d, want %d (outcomeEmptyCandidate); stderr=%q", got, outcomeEmptyCandidate, stderr.String())
	}
	diagnostic := stderr.String()
	// A slice, not a map: the assertions report in a fixed order, so a failing
	// run is reproducible rather than reshuffled by map iteration.
	wants := []struct {
		fragment  string
		complaint string
	}{
		{"0 changed paths", "does not state that the candidate covers zero paths"},
		{"base_tree and current_candidate_tree are the same tree", "does not state that the two trees are identical"},
		{"APPROVED RECEIPT over nothing", "does not state that a review here would approve nothing"},
		{"--base-ref <remote>/<branch>", "does not name --base-ref <remote>/<branch> as the fix"},
		{"uncommitted", "does not offer reviewing before committing as the alternative"},
	}
	for _, want := range wants {
		if !strings.Contains(diagnostic, want.fragment) {
			t.Errorf("empty-candidate diagnostic %q %s (missing %q)", diagnostic, want.complaint, want.fragment)
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("blocked run wrote %q to stdout, want the blocking verdict on stderr only", stdout.String())
	}
}

// TestRunAcceptsTheRealHealthyCandidate is the other half of the pair: the same
// repository with the two files still uncommitted must not be blocked, and the
// pass line must name what the review would actually cover.
func TestRunAcceptsTheRealHealthyCandidate(t *testing.T) {
	stubStatusProbe(t, staticStatusProbe(healthyCandidateStatus))

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeCovered {
		t.Fatalf("run() against the observed healthy candidate = exit %d, want %d (outcomeCovered); stderr=%q", got, outcomeCovered, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2") {
		t.Errorf("pass line %q does not report the 2 paths the review would cover", stdout.String())
	}
}

// TestRunBlocksIdenticalTreesEvenWhenPathsArePresent pins the fail-closed half
// of the emptiness rule: either condition alone blocks. A projection that names
// paths but resolves both trees to the same object describes no change at all,
// and trusting the path list there would let exactly the hazard through.
func TestRunBlocksIdenticalTreesEvenWhenPathsArePresent(t *testing.T) {
	stubStatusProbe(t, staticStatusProbe(projectionStatus(`{
		"base_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
		"current_candidate_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
		"paths": ["tools/review-preflight/main.go"],
		"intended_untracked": []
	}`)))

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeEmptyCandidate {
		t.Fatalf("run() with identical trees but a non-empty path list = exit %d, want %d (outcomeEmptyCandidate); stderr=%q", got, outcomeEmptyCandidate, stderr.String())
	}
}

// TestRunBlocksZeroPathsEvenWhenTreesDiffer pins the mirror condition: differing
// trees do not license a review whose projection covers no path at all.
func TestRunBlocksZeroPathsEvenWhenTreesDiffer(t *testing.T) {
	stubStatusProbe(t, staticStatusProbe(projectionStatus(`{
		"base_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
		"current_candidate_tree": "9773630db7b612668160b438a4415ed65ae12e67",
		"paths": [],
		"intended_untracked": []
	}`)))

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeEmptyCandidate {
		t.Fatalf("run() with differing trees but zero paths = exit %d, want %d (outcomeEmptyCandidate); stderr=%q", got, outcomeEmptyCandidate, stderr.String())
	}
}

// TestRunAcceptsIntendedUntrackedOnlyCandidate covers the AND in the path rule:
// a candidate made entirely of intended-untracked files still covers something,
// so blocking it would be a false positive that teaches operators to ignore the
// guard.
func TestRunAcceptsIntendedUntrackedOnlyCandidate(t *testing.T) {
	stubStatusProbe(t, staticStatusProbe(projectionStatus(`{
		"base_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9",
		"current_candidate_tree": "9773630db7b612668160b438a4415ed65ae12e67",
		"paths": [],
		"intended_untracked": ["tools/review-preflight/main.go"]
	}`)))

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeCovered {
		t.Fatalf("run() with an intended-untracked-only candidate = exit %d, want %d (outcomeCovered); stderr=%q", got, outcomeCovered, stderr.String())
	}
}

// TestRunReportsUndeterminedForMalformedPayload covers every payload this guard
// cannot read. None of them may reach exit 0, and none may be reported as an
// empty candidate either: "the review would cover nothing" and "I could not
// tell" are different facts and send the operator to different fixes.
func TestRunReportsUndeterminedForMalformedPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "unparseable JSON",
			payload: `{"schema": "gentle-ai.review-integration.status/v3", "projection":`,
			want:    "JSON",
		},
		{
			name:    "no projection key",
			payload: `{"schema": "gentle-ai.review-integration.status/v3", "candidates": []}`,
			want:    "projection",
		},
		{
			name:    "projection without trees",
			payload: projectionStatus(`{"paths": [], "intended_untracked": []}`),
			want:    "base_tree",
		},
		{
			name:    "projection missing the candidate tree",
			payload: projectionStatus(`{"base_tree": "9555e76b8438276f98ea9dbc81197aa9093680c9", "paths": ["a.go"], "intended_untracked": []}`),
			want:    "current_candidate_tree",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubStatusProbe(t, staticStatusProbe(tc.payload))

			var stdout, stderr bytes.Buffer
			got := run(nil, &stdout, &stderr)

			if got != outcomeUndetermined {
				t.Fatalf("run() with %s = exit %d, want %d (outcomeUndetermined); stderr=%q", tc.name, got, outcomeUndetermined, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("undetermined diagnostic %q does not name %q, so the operator cannot tell what was unreadable", stderr.String(), tc.want)
			}
		})
	}
}

// TestRunReportsUndeterminedWhenTheStatusProbeFails covers the tool-absent and
// command-failed paths at the run() boundary: a guard that cannot reach
// gentle-ai must never answer "safe to start a review here".
func TestRunReportsUndeterminedWhenTheStatusProbeFails(t *testing.T) {
	stubStatusProbe(t, failingStatusProbe("gentle-ai is not on PATH"))

	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)

	if got != outcomeUndetermined {
		t.Fatalf("run() with a failing status probe = exit %d, want %d (outcomeUndetermined); stderr=%q", got, outcomeUndetermined, stderr.String())
	}
	if !strings.Contains(stderr.String(), "gentle-ai") {
		t.Errorf("undetermined diagnostic %q does not name gentle-ai, the tool that could not be reached", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("undetermined run wrote %q to stdout, want diagnostics on stderr only", stdout.String())
	}
}

// TestRunRejectsBadInvocations pins exit 2 for usage errors, separately from
// exit 3: a typo in a flag is the operator's to fix, and collapsing it into
// "could not determine" would hide that.
func TestRunRejectsBadInvocations(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--base-reff", "origin/main"}},
		{name: "unexpected positional argument", args: []string{"check"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubStatusProbe(t, func(dir, baseRef string) ([]byte, error) {
				t.Fatalf("status probe was reached with a %s; the invocation must be rejected before any subprocess runs", tc.name)
				return nil, nil
			})

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

// TestRunForwardsBaseRefToTheStatusProbe proves the flag is not merely accepted:
// --base-ref is the whole documented workaround, so a run that silently dropped
// it would keep projecting the workspace and re-approve nothing.
func TestRunForwardsBaseRefToTheStatusProbe(t *testing.T) {
	var gotDir, gotBaseRef string
	stubStatusProbe(t, func(dir, baseRef string) ([]byte, error) {
		gotDir, gotBaseRef = dir, baseRef
		return []byte(healthyCandidateStatus), nil
	})

	var stdout, stderr bytes.Buffer
	if got := run([]string{"--base-ref", "origin/main"}, &stdout, &stderr); got != outcomeCovered {
		t.Fatalf("run(--base-ref origin/main) = exit %d, want %d (outcomeCovered); stderr=%q", got, outcomeCovered, stderr.String())
	}
	if gotBaseRef != "origin/main" {
		t.Errorf("status probe received base ref %q, want %q", gotBaseRef, "origin/main")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	if gotDir != wd {
		t.Errorf("status probe received dir %q, want the caller's working directory %q", gotDir, wd)
	}
}

// TestEmptyBaseRefProjectionDiagnosticAddressesTheBaseRef covers the operator
// who already took the documented workaround and still got an empty candidate.
// Repeating "pass --base-ref" verbatim there would be advice they have already
// followed; the diagnostic has to address the ref itself.
func TestEmptyBaseRefProjectionDiagnosticAddressesTheBaseRef(t *testing.T) {
	stubStatusProbe(t, staticStatusProbe(emptyCandidateStatus))

	var stdout, stderr bytes.Buffer
	got := run([]string{"--base-ref", "origin/main"}, &stdout, &stderr)

	if got != outcomeEmptyCandidate {
		t.Fatalf("run(--base-ref origin/main) against an empty candidate = exit %d, want %d (outcomeEmptyCandidate)", got, outcomeEmptyCandidate)
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, "origin/main") {
		t.Errorf("empty base-ref diagnostic %q does not name the ref %q that produced the empty projection", diagnostic, "origin/main")
	}
	if !strings.Contains(diagnostic, "--base-ref <remote>/<branch>") {
		t.Errorf("empty base-ref diagnostic %q does not tell the operator to repoint --base-ref <remote>/<branch>", diagnostic)
	}
}

// TestExecReviewStatusFailsClosedWhenGentleAIIsAbsent exercises the real exec
// path, not the seam, with PATH shadowed by an empty directory. This is the CI
// condition: gentle-ai is not installed there, and the tool must say so rather
// than return an empty payload that would decode into a blocked-or-passed
// verdict it never actually observed.
func TestExecReviewStatusFailsClosedWhenGentleAIIsAbsent(t *testing.T) {
	shadowPATH(t, t.TempDir())

	payload, err := execReviewStatus(t.TempDir(), "")

	if err == nil {
		t.Fatalf("execReviewStatus with no gentle-ai on PATH returned payload %q and no error, want a failure", payload)
	}
	if !strings.Contains(err.Error(), "gentle-ai") {
		t.Errorf("absent-tool error %q does not name gentle-ai", err)
	}
	if payload != nil {
		t.Errorf("absent-tool call returned payload %q, want nil", payload)
	}
}

// TestExecReviewStatusInvokesTheContractedStatusCommand pins the exact argv the
// real exec path builds, through a fake gentle-ai that records what it was
// called with. The contract token is what makes the projection field names this
// guard reads meaningful, and --base-ref is the fix the guard recommends, so
// both have to actually reach the binary.
func TestExecReviewStatusInvokesTheContractedStatusCommand(t *testing.T) {
	dir := fakeGentleAI(t, "printf '%s\\n' \"$@\" > \"$FAKE_ARGV_FILE\"\nprintf '%s' '"+healthyCandidateStatus+"'\n")
	argvFile := filepath.Join(dir, "argv.txt")
	t.Setenv("FAKE_ARGV_FILE", argvFile)

	payload, err := execReviewStatus("/some/repo", "origin/main")
	if err != nil {
		t.Fatalf("execReviewStatus against a fake gentle-ai: %v", err)
	}
	if !strings.Contains(string(payload), "current_candidate_tree") {
		t.Errorf("execReviewStatus returned %q, want the fake's stdout payload", payload)
	}

	recorded, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("read recorded argv: %v", err)
	}
	argv := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	want := []string{"review", "status", "--cwd", "/some/repo", "--contract", "gentle-ai.review-integration/v2", "--base-ref", "origin/main"}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Errorf("gentle-ai was invoked with %v, want %v", argv, want)
	}
}

// TestExecReviewStatusOmitsBaseRefWhenUnset is the negative of the above: an
// empty --base-ref value must not be passed through, because gentle-ai would
// then have to interpret an empty ref rather than fall back to the workspace
// projection this guard's default case is written against.
func TestExecReviewStatusOmitsBaseRefWhenUnset(t *testing.T) {
	dir := fakeGentleAI(t, "printf '%s\\n' \"$@\" > \"$FAKE_ARGV_FILE\"\nprintf '%s' '"+emptyCandidateStatus+"'\n")
	argvFile := filepath.Join(dir, "argv.txt")
	t.Setenv("FAKE_ARGV_FILE", argvFile)

	if _, err := execReviewStatus("/some/repo", ""); err != nil {
		t.Fatalf("execReviewStatus against a fake gentle-ai: %v", err)
	}

	recorded, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("read recorded argv: %v", err)
	}
	if strings.Contains(string(recorded), "--base-ref") {
		t.Errorf("gentle-ai was invoked with %q, want no --base-ref when none was requested", recorded)
	}
}

// TestExecReviewStatusReportsANonZeroStatusCommand covers a gentle-ai that runs
// but refuses: its own stderr is the only thing that can explain why, so it has
// to survive into the diagnostic instead of being swallowed.
func TestExecReviewStatusReportsANonZeroStatusCommand(t *testing.T) {
	fakeGentleAI(t, "echo 'not a git repository' >&2\nexit 1\n")

	payload, err := execReviewStatus("/some/repo", "")

	if err == nil {
		t.Fatalf("execReviewStatus with a failing gentle-ai returned payload %q and no error, want a failure", payload)
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("failed-status error %q discards gentle-ai's own stderr", err)
	}
}

// TestExecReviewStatusEnforcesItsDeadline proves the probe is bounded. A guard
// that hangs forever is a guard nobody keeps in their loop, and the sibling
// runner already had to bound every tool subprocess for the same reason.
func TestExecReviewStatusEnforcesItsDeadline(t *testing.T) {
	originalTimeout, originalWaitDelay := statusExecTimeout, statusWaitDelay
	statusExecTimeout = 200 * time.Millisecond
	statusWaitDelay = 20 * time.Millisecond
	t.Cleanup(func() {
		statusExecTimeout, statusWaitDelay = originalTimeout, originalWaitDelay
	})
	fakeGentleAI(t, "sleep 30\n")

	start := time.Now()
	payload, err := execReviewStatus("/some/repo", "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("execReviewStatus against a hanging gentle-ai returned payload %q and no error, want a deadline failure", payload)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("execReviewStatus took %s against a hanging gentle-ai, want it bounded by the %s deadline", elapsed, statusExecTimeout)
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Errorf("deadline error %q does not say the probe was killed by its deadline", err)
	}
}
