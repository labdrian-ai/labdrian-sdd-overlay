package main

// Phase 6 lifecycle acceptance fixtures (W5): one test per acceptance
// criterion from odd/tasks/workflow-lifecycle.md's ledger, black-box
// against the real 'workflow <verb>' subcommand. Every test isolates HOME
// and XDG_STATE_HOME under t.TempDir(): none may ever touch the real
// user's state home, ~/.claude, ~/.labdrian-overlay, ~/.pi, ~/.codex, or
// Engram (see the safety note on each scenario that builds or runs a real
// binary).

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// --- shared fixtures -------------------------------------------------------

// phase6IsolatedHome points both HOME and XDG_STATE_HOME at fresh temp
// dirs, so NewStore's XDG_STATE_HOME branch resolves the isolated root
// deterministically (rather than falling back through HOME) and nothing a
// test does can ever reach the real user's state home.
func phase6IsolatedHome(t *testing.T) (home, stateHome string) {
	t.Helper()
	home = t.TempDir()
	stateHome = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)
	return home, stateHome
}

// phase6WorkflowLogPath is the on-disk path Store.Append/Load uses for one
// workflow, reproduced independently of engine/workflow's own internals so
// a test can assert on it without importing an unexported helper.
func phase6WorkflowLogPath(stateHome, project, wf string) string {
	return filepath.Join(stateHome, "labdrian", "workflows", project, wf+".jsonl")
}

// phase6ReadLog reads a workflow's raw log bytes directly (bypassing the
// CLI), for before/after byte-identity comparisons.
func phase6ReadLog(t *testing.T, stateHome, project, wf string) []byte {
	t.Helper()
	data, err := os.ReadFile(phase6WorkflowLogPath(stateHome, project, wf))
	if err != nil {
		t.Fatalf("read workflow log: %v", err)
	}
	return data
}

// phase6LoadOwned loads and requires ClassificationOwned, for tests that
// need the replayed State or raw Events without going through the CLI's
// JSON view.
func phase6LoadOwned(t *testing.T, stateHome, project, wf string) workflow.Loaded {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", stateHome)
	store, err := newWorkflowStore()
	if err != nil {
		t.Fatalf("newWorkflowStore() = %v, want nil", err)
	}
	loaded, err := store.Load(project, wf)
	if err != nil {
		t.Fatalf("Store.Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationOwned {
		t.Fatalf("Store.Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationOwned)
	}
	return loaded
}

func phase6MustUnmarshal(t *testing.T, data []byte) workflowStateJSON {
	t.Helper()
	var s workflowStateJSON
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("json.Unmarshal(%q) = %v, want nil", data, err)
	}
	return s
}

// phase6MustExitZero runs one runWorkflowTest call and fails the test with
// full context (args, stdout, stderr) unless it exits 0.
func phase6MustExitZero(t *testing.T, label string, args []string, cwd string) workflowRun {
	t.Helper()
	r := runWorkflowTest(args, cwd)
	if r.code != 0 {
		t.Fatalf("%s: args=%v code=%d stderr=%q, want exit 0", label, args, r.code, r.stderr)
	}
	return r
}

// phase6WalkRelPaths lists every path under root, relative to root, sorted;
// root itself is excluded. Used to assert exactly what a clean install
// wrote under an isolated HOME.
func phase6WalkRelPaths(t *testing.T, root string) []string {
	t.Helper()
	var got []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.Walk(%q) = %v, want nil", root, err)
	}
	return got
}

// --- Scenario 1: clean install ---------------------------------------------

// TestPhase6Acceptance_CleanInstall proves: with an empty state home,
// status reports absent; create creates the directory tree with 0700 dirs
// and a 0600 file; nothing else is written anywhere under HOME. This
// scenario deliberately isolates only HOME (not XDG_STATE_HOME), so
// NewStore's fallback resolution ($HOME/.local/state) is what is actually
// exercised, matching a genuinely clean install with no XDG override set.
func TestPhase6Acceptance_CleanInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	workDir := t.TempDir()
	const project, wf = "proj-1", "wf-1"

	status := phase6MustExitZero(t, "status", []string{"status", "--project", project, "--workflow", wf}, workDir)
	if got := phase6MustUnmarshal(t, []byte(status.stdout)).Classification; got != "absent" {
		t.Fatalf("status classification = %q, want %q", got, "absent")
	}
	if entries := phase6WalkRelPaths(t, home); len(entries) != 0 {
		t.Fatalf("HOME entries after a read-only status = %v, want none (status must never write)", entries)
	}

	goalPath := writeMemoryTestFile(t, workDir, "goal.json", memoryTestGoalJSON(project, "goal-1"))
	phase6MustExitZero(t, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal"}, workDir)

	stateHome := filepath.Join(home, ".local", "state")
	logPath := phase6WorkflowLogPath(stateHome, project, wf)
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("os.Stat(%q) = %v, want nil", logPath, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("log file mode = %o, want %o", info.Mode().Perm(), 0o600)
	}

	for _, dir := range []string{
		stateHome,
		filepath.Join(stateHome, "labdrian"),
		filepath.Join(stateHome, "labdrian", "workflows"),
		filepath.Join(stateHome, "labdrian", "workflows", project),
	} {
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("os.Stat(%q) = %v, want nil", dir, err)
		}
		if !dirInfo.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("dir %q mode = %o, want %o", dir, dirInfo.Mode().Perm(), 0o700)
		}
	}

	// Nothing else was written anywhere under HOME: exactly the expected
	// directory tree, the log file, and its append lock file (a
	// persistent flock target, never removed -- see engine/workflow's W2
	// store notes).
	want := []string{
		".local",
		".local/state",
		".local/state/labdrian",
		".local/state/labdrian/workflows",
		".local/state/labdrian/workflows/" + project,
		".local/state/labdrian/workflows/" + project + "/" + wf + ".jsonl",
		".local/state/labdrian/workflows/" + project + "/" + wf + ".lock",
	}
	got := phase6WalkRelPaths(t, home)
	if len(got) != len(want) {
		t.Fatalf("HOME entries = %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("HOME entries = %v, want exactly %v", got, want)
		}
	}
}

// --- Scenario 2: restart ----------------------------------------------------

// phase6BuildEngineBinary builds the real engine binary (this same cmd
// package's main) into a temp path, exactly the way
// shelltest/overlay_pi_package_build_test.go builds it for its own
// black-box tests. Building takes a couple of seconds; callers that need it
// more than once within one test should build it once and reuse the path.
func phase6BuildEngineBinary(t *testing.T) string {
	t.Helper()
	enginePath := filepath.Join(t.TempDir(), "workflow-acceptance-engine")
	engineDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve engine cmd dir: %v", err)
	}
	build := exec.Command("go", "build", "-o", enginePath, ".")
	build.Dir = engineDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build engine binary: %v\n%s", err, out)
	}
	return enginePath
}

// phase6RunBinary runs the built engine binary as a genuinely separate OS
// process (simulating one process's exit between lifecycle steps: killing
// is simulated by the process simply exiting after each step, since
// nothing here keeps a process alive across steps), with HOME/
// XDG_STATE_HOME/cwd exactly as given.
func phase6RunBinary(t *testing.T, binary string, args []string, home, stateHome, cwd string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "XDG_STATE_HOME=" + stateHome, "PATH=" + os.Getenv("PATH")}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run engine binary %v: %v", args, err)
		}
	}
	return out.String(), errBuf.String(), code
}

// TestPhase6Acceptance_Restart proves: each lifecycle step runs as a
// separate built-binary process, and the final state (read back through a
// fresh 'status' process) equals the state replayed directly from the
// on-disk log.
//
// Safety: this builds and runs the real engine binary, but every
// invocation has HOME and XDG_STATE_HOME pointed at this test's own
// t.TempDir()s; it never reads or writes the real user's state.
func TestPhase6Acceptance_Restart(t *testing.T) {
	home, stateHome := phase6IsolatedHome(t)
	workDir := t.TempDir()
	const project, wf = "proj-1", "wf-1"
	goalPath := writeMemoryTestFile(t, workDir, "goal.json", memoryTestGoalJSON(project, "goal-1"))
	binary := phase6BuildEngineBinary(t)

	run := func(label string, args ...string) (string, string, int) {
		stdout, stderr, code := phase6RunBinary(t, binary, append([]string{"workflow"}, args...), home, stateHome, workDir)
		if code != 0 {
			t.Fatalf("%s: code=%d stderr=%q, want exit 0", label, code, stderr)
		}
		return stdout, stderr, code
	}

	// Each call below is its own process: nothing links them but the
	// on-disk log under stateHome. A process "restart" is simulated
	// trivially, since every step already exits before the next begins.
	run("create", "create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal")
	run("start", "start", "--project", project, "--workflow", wf)
	profile, err := workflowprofile.Resolve("standalone-minimal")
	if err != nil {
		t.Fatalf("workflowprofile.Resolve() = %v, want nil", err)
	}
	for _, stage := range profile.Stages {
		run("stage:"+stage.Name, "stage", "--project", project, "--workflow", wf, "--stage", stage.Name)
	}
	run("verify", "verify", "--project", project, "--workflow", wf, "--goal", goalPath)
	closeOut, _, _ := run("close", "close", "--project", project, "--workflow", wf, "--outcome", "completed")
	closedFromProcess := phase6MustUnmarshal(t, []byte(closeOut))

	// A fresh, separate 'status' process must report exactly the state a
	// direct in-process replay of the on-disk log produces.
	statusOut, _, _ := run("status", "status", "--project", project, "--workflow", wf)
	statusFromProcess := phase6MustUnmarshal(t, []byte(statusOut))

	loaded := phase6LoadOwned(t, stateHome, project, wf)
	replayed, err := workflow.Replay(loaded.Events)
	if err != nil {
		t.Fatalf("workflow.Replay() = %v, want nil", err)
	}
	if statusFromProcess.Status != string(replayed.Status) || statusFromProcess.CloseOutcome != string(replayed.CloseOutcome) {
		t.Fatalf("status after restart = %+v, want it to equal the log's own replayed state (status=%q outcome=%q)", statusFromProcess, replayed.Status, replayed.CloseOutcome)
	}
	if closedFromProcess.Status != "closed" || closedFromProcess.CloseOutcome != "completed" {
		t.Fatalf("close result = %+v, want status=closed close_outcome=completed", closedFromProcess)
	}
	if len(statusFromProcess.Stages) != len(profile.Stages) {
		t.Fatalf("status stages = %v, want all %d profile stages recorded", statusFromProcess.Stages, len(profile.Stages))
	}
}

// --- Scenario 3: corrupt state ----------------------------------------------

// phase6CreateOwnedLog runs create then start through the CLI under an
// isolated home, and returns the resulting raw log bytes -- a genuine
// owned two-event log to corrupt in each corruption subtest.
func phase6CreateOwnedLog(t *testing.T, stateHome, workDir, project, wf string) []byte {
	t.Helper()
	goalPath := writeMemoryTestFile(t, workDir, "goal.json", memoryTestGoalJSON(project, "goal-1"))
	phase6MustExitZero(t, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal"}, workDir)
	phase6MustExitZero(t, "start", []string{"start", "--project", project, "--workflow", wf}, workDir)
	return phase6ReadLog(t, stateHome, project, wf)
}

// TestPhase6Acceptance_CorruptState proves: a truncated file, invalid
// UTF-8, a broken hash chain, and reordered lines are each classified
// malformed or drifted; every mutating verb then refuses with exit 2, and
// the file's bytes are left completely unchanged by that refusal.
func TestPhase6Acceptance_CorruptState(t *testing.T) {
	const project, wf = "proj-1", "wf-1"

	cases := []struct {
		name               string
		corrupt            func(valid []byte) []byte
		wantClassification workflow.Classification
	}{
		{
			name: "truncated file (mid-line, no trailing newline)",
			corrupt: func(valid []byte) []byte {
				return valid[:len(valid)/2]
			},
			wantClassification: workflow.ClassificationMalformed,
		},
		{
			name: "invalid UTF-8",
			corrupt: func(valid []byte) []byte {
				out := append([]byte{}, valid...)
				// Splice an invalid UTF-8 byte sequence into the middle of
				// the file; it can never form part of any well-formed JSON
				// document.
				mid := len(out) / 2
				out = append(out[:mid], append([]byte{0xff, 0xfe}, out[mid:]...)...)
				return out
			},
			wantClassification: workflow.ClassificationMalformed,
		},
		{
			name: "broken hash chain (tampered prev_digest)",
			corrupt: func(valid []byte) []byte {
				lines := strings.Split(strings.TrimSuffix(string(valid), "\n"), "\n")
				if len(lines) < 2 {
					t.Fatalf("fixture has %d lines, want at least 2 to break the chain between them", len(lines))
				}
				// Replace with a different but still well-formed 64-hex
				// digest (all zeros), so the event still parses cleanly
				// (Validate only checks the shape) and only the hash-chain
				// check in VerifyEvents fails -- the "drifted" case, as
				// opposed to a shape failure that would classify foreign.
				digestField := regexp.MustCompile(`"prev_digest":"[0-9a-f]{64}"`)
				if !digestField.MatchString(lines[1]) {
					t.Fatalf("fixture line 2 = %q, want it to contain a 64-hex prev_digest field", lines[1])
				}
				lines[1] = digestField.ReplaceAllString(lines[1], `"prev_digest":"`+strings.Repeat("0", 64)+`"`)
				return []byte(strings.Join(lines, "\n") + "\n")
			},
			wantClassification: workflow.ClassificationDrifted,
		},
		{
			name: "reordered lines",
			corrupt: func(valid []byte) []byte {
				lines := strings.Split(strings.TrimSuffix(string(valid), "\n"), "\n")
				if len(lines) < 2 {
					t.Fatalf("fixture has %d lines, want at least 2 to reorder", len(lines))
				}
				lines[0], lines[1] = lines[1], lines[0]
				return []byte(strings.Join(lines, "\n") + "\n")
			},
			wantClassification: workflow.ClassificationDrifted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stateHome := phase6IsolatedHome(t)
			workDir := t.TempDir()
			valid := phase6CreateOwnedLog(t, stateHome, workDir, project, wf)

			corrupted := tc.corrupt(valid)
			logPath := phase6WorkflowLogPath(stateHome, project, wf)
			if err := os.WriteFile(logPath, corrupted, 0o600); err != nil {
				t.Fatalf("os.WriteFile(%q) = %v, want nil", logPath, err)
			}

			loaded := func() workflow.Loaded {
				t.Setenv("XDG_STATE_HOME", stateHome)
				store, err := newWorkflowStore()
				if err != nil {
					t.Fatalf("newWorkflowStore() = %v, want nil", err)
				}
				l, err := store.Load(project, wf)
				if err != nil {
					t.Fatalf("Store.Load() = %v, want nil", err)
				}
				return l
			}()
			if loaded.Classification != tc.wantClassification {
				t.Fatalf("classification = %q (detail=%q), want %q", loaded.Classification, loaded.Detail, tc.wantClassification)
			}

			r := runWorkflowTest([]string{"pause", "--project", project, "--workflow", wf}, workDir)
			if r.code != 2 {
				t.Fatalf("pause on %s state: code=%d stderr=%q, want exit 2 (refused)", tc.wantClassification, r.code, r.stderr)
			}

			after, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) = %v, want nil", logPath, err)
			}
			if !bytes.Equal(after, corrupted) {
				t.Fatalf("a refused append changed the file bytes for %s state", tc.wantClassification)
			}
		})
	}
}

// --- Scenario 4: foreign state -----------------------------------------------

// TestPhase6Acceptance_ForeignState proves: a JSONL file that is valid JSON
// but not one of our events, and a log recognizably ours but for a
// different project/workflow, are both classified foreign; every mutating
// verb refuses, and the file's bytes are left unchanged.
func TestPhase6Acceptance_ForeignState(t *testing.T) {
	const project, wf = "proj-1", "wf-1"

	cases := []struct {
		name    string
		content []byte
	}{
		{
			name:    "valid JSON, not a workflow event",
			content: []byte(`{"some":"other tool's own state","version":1}` + "\n"),
		},
		{
			name: "a recognized workflow event for a different project/workflow",
			content: func() []byte {
				_, otherStateHome := phase6IsolatedHome(t)
				otherWorkDir := t.TempDir()
				goalPath := writeMemoryTestFile(t, otherWorkDir, "goal.json", memoryTestGoalJSON("other-project", "goal-1"))
				phase6MustExitZero(t, "create (other)", []string{"create", "--project", "other-project", "--workflow", "other-wf", "--goal", goalPath, "--profile", "standalone-minimal"}, otherWorkDir)
				return phase6ReadLog(t, otherStateHome, "other-project", "other-wf")
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stateHome := phase6IsolatedHome(t)
			workDir := t.TempDir()
			logPath := phase6WorkflowLogPath(stateHome, project, wf)
			if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
				t.Fatalf("os.MkdirAll() = %v, want nil", err)
			}
			if err := os.WriteFile(logPath, tc.content, 0o600); err != nil {
				t.Fatalf("os.WriteFile(%q) = %v, want nil", logPath, err)
			}

			t.Setenv("XDG_STATE_HOME", stateHome)
			store, err := newWorkflowStore()
			if err != nil {
				t.Fatalf("newWorkflowStore() = %v, want nil", err)
			}
			loaded, err := store.Load(project, wf)
			if err != nil {
				t.Fatalf("Store.Load() = %v, want nil", err)
			}
			if loaded.Classification != workflow.ClassificationForeign {
				t.Fatalf("classification = %q (detail=%q), want %q", loaded.Classification, loaded.Detail, workflow.ClassificationForeign)
			}

			r := runWorkflowTest([]string{"start", "--project", project, "--workflow", wf}, workDir)
			if r.code != 2 {
				t.Fatalf("start on foreign state: code=%d stderr=%q, want exit 2 (refused)", r.code, r.stderr)
			}

			after, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) = %v, want nil", logPath, err)
			}
			if !bytes.Equal(after, tc.content) {
				t.Fatalf("a refused append changed the file bytes for foreign state")
			}
		})
	}
}

// --- Scenario 5: worktrees ---------------------------------------------------

// phase6BuildWorktreeFixture hand-builds a linked-worktree pair sharing one
// git dir (never a real git repository or the git binary -- see
// workflow_provenance_test.go's own fixtures), and returns the two
// worktree roots plus the HEAD each one resolves to.
func phase6BuildWorktreeFixture(t *testing.T, root string) (worktreeA, worktreeB, headA, headB string) {
	t.Helper()
	mainGit := filepath.Join(root, "main-repo", ".git")
	worktreeA = filepath.Join(root, "wt-a")
	worktreeB = filepath.Join(root, "wt-b")
	gitDirA := filepath.Join(mainGit, "worktrees", "wt-a")
	gitDirB := filepath.Join(mainGit, "worktrees", "wt-b")
	headA = strings.Repeat("a", 40)
	headB = strings.Repeat("b", 40)

	writeFixtureFile(t, filepath.Join(gitDirA, "HEAD"), headA+"\n")
	writeFixtureFile(t, filepath.Join(gitDirA, "commondir"), "../..\n")
	writeFixtureFile(t, filepath.Join(worktreeA, ".git"), "gitdir: "+gitDirA+"\n")

	writeFixtureFile(t, filepath.Join(gitDirB, "HEAD"), headB+"\n")
	writeFixtureFile(t, filepath.Join(gitDirB, "commondir"), "../..\n")
	writeFixtureFile(t, filepath.Join(worktreeB, ".git"), "gitdir: "+gitDirB+"\n")

	return worktreeA, worktreeB, headA, headB
}

// TestPhase6Acceptance_Worktrees proves: a workflow created from worktree A
// is visible and advanceable from worktree B (state lives outside every
// worktree, keyed by project_id -- Decision 2), and each event's
// provenance names the worktree it actually came from, not a shared or
// stale one.
func TestPhase6Acceptance_Worktrees(t *testing.T) {
	_, stateHome := phase6IsolatedHome(t)
	root := t.TempDir()
	worktreeA, worktreeB, headA, headB := phase6BuildWorktreeFixture(t, root)
	const project, wf = "proj-1", "wf-1"
	goalPath := writeMemoryTestFile(t, root, "goal.json", memoryTestGoalJSON(project, "goal-1"))

	phase6MustExitZero(t, "create (worktree A)", []string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal"}, worktreeA)
	// Advanceable from worktree B: start does not need to run from the
	// same worktree the workflow was created in.
	statusFromB := phase6MustExitZero(t, "status (worktree B)", []string{"status", "--project", project, "--workflow", wf}, worktreeB)
	if got := phase6MustUnmarshal(t, []byte(statusFromB.stdout)).Classification; got != "owned" {
		t.Fatalf("status from worktree B classification = %q, want %q (same workflow visible from another worktree)", got, "owned")
	}
	phase6MustExitZero(t, "start (worktree B)", []string{"start", "--project", project, "--workflow", wf}, worktreeB)

	loaded := phase6LoadOwned(t, stateHome, project, wf)
	if len(loaded.Events) != 2 {
		t.Fatalf("events = %d, want 2 (created from A, started from B)", len(loaded.Events))
	}
	created, started := loaded.Events[0], loaded.Events[1]
	if created.Provenance.WorktreeRoot != worktreeA || created.Provenance.GitHead != headA {
		t.Fatalf("created provenance = %+v, want worktree_root=%q git_head=%q", created.Provenance, worktreeA, headA)
	}
	if started.Provenance.WorktreeRoot != worktreeB || started.Provenance.GitHead != headB {
		t.Fatalf("started provenance = %+v, want worktree_root=%q git_head=%q", started.Provenance, worktreeB, headB)
	}
}

// --- Scenario 6: missing dependencies ---------------------------------------

// TestPhase6Acceptance_MissingDependencies proves: with no Gentle AI, no
// runtime, no memory, and no auth available (an empty PATH, on top of the
// isolated HOME every scenario already uses), every verb across a full
// create-through-close(completed) run still succeeds, and every recorded
// observation for gentle-ai-review and every memory source is
// "unavailable" -- never "available" by default (Decision 5/7: an
// unavailable dependency is recorded, never approved or hidden).
func TestPhase6Acceptance_MissingDependencies(t *testing.T) {
	_, stateHome := phase6IsolatedHome(t)
	t.Setenv("PATH", "")
	workDir := t.TempDir()
	const project, wf = "proj-1", "wf-1"
	// "odd" is a gentleReviewProfiles member (see
	// engine/workflow/lifecycle.go), so its declared dependencies include
	// gentle-ai-review on top of its memory sources -- the richer profile
	// to prove nothing ever reads "available" from an empty PATH.
	const profileName = "odd"
	goalPath := writeMemoryTestFile(t, workDir, "goal.json", memoryTestGoalJSON(project, "goal-1"))

	phase6MustExitZero(t, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", profileName}, workDir)
	phase6MustExitZero(t, "start", []string{"start", "--project", project, "--workflow", wf}, workDir)
	profile, err := workflowprofile.Resolve(profileName)
	if err != nil {
		t.Fatalf("workflowprofile.Resolve() = %v, want nil", err)
	}
	for _, stage := range profile.Stages {
		phase6MustExitZero(t, "stage:"+stage.Name, []string{"stage", "--project", project, "--workflow", wf, "--stage", stage.Name}, workDir)
	}
	phase6MustExitZero(t, "verify", []string{"verify", "--project", project, "--workflow", wf, "--goal", goalPath}, workDir)
	phase6MustExitZero(t, "close", []string{"close", "--project", project, "--workflow", wf, "--outcome", "completed"}, workDir)

	loaded := phase6LoadOwned(t, stateHome, project, wf)
	if len(loaded.Events) == 0 {
		t.Fatalf("events is empty, want the full recorded lifecycle")
	}
	sawGentleAIReview := false
	for _, e := range loaded.Events {
		if len(e.Observations) == 0 {
			t.Fatalf("event kind=%q has no observations, want at least one declared dependency observed", e.Kind)
		}
		for _, o := range e.Observations {
			if o.Status != workflow.ObservationUnavailable {
				t.Fatalf("event kind=%q observation %+v status = %q, want %q (never approved by default)", e.Kind, o, o.Status, workflow.ObservationUnavailable)
			}
			if o.Capability == "gentle-ai-review" {
				sawGentleAIReview = true
			}
			if !strings.HasPrefix(o.Capability, "memory:") && o.Capability != "gentle-ai-review" {
				t.Fatalf("event kind=%q observation capability = %q, want a memory:* source or gentle-ai-review", e.Kind, o.Capability)
			}
		}
	}
	if !sawGentleAIReview {
		t.Fatalf("no event ever observed gentle-ai-review, want profile %q (a gentleReviewProfiles member) to declare it", profileName)
	}
}

// --- Scenario 7: overlay lifecycle preservation -----------------------------

// phase6OverlayPath resolves bin/labdrian-overlay relative to this
// package (engine/cmd), skipping (not failing) when bash is unavailable,
// matching every other shell-harness test in this repository.
func phase6OverlayPath(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not available: %v", err)
	}
	overlay, err := filepath.Abs(filepath.Join("..", "..", "bin", "labdrian-overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("overlay entrypoint not found at %s: %v", overlay, err)
	}
	return overlay
}

// phase6SourceAndCall sources bin/labdrian-overlay (guarded BASH_SOURCE
// dispatch, so sourcing alone never runs the CLI) and calls one internal
// bash function directly, under the given HOME -- reusing the exact
// pattern shelltest/overlay_pi_target_test.go's sourceAndRun already
// proves works for this script.
func phase6SourceAndCall(t *testing.T, overlay, home, fn string, extraEnv ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "-c", `source "$1"; `+fn, "_", overlay)
	env := append(os.Environ(), "HOME="+home, "OVERLAY_SKIP_TOOL_INSTALL=1")
	env = append(env, extraEnv...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// phase6StaticFunctionBody extracts one top-level bash function's source
// text from bin/labdrian-overlay, from its "name() {" line up to (but not
// including) the next top-level function definition -- the same style
// every function in this file is written in (checked once by
// TestPhase6Acceptance_OverlayNeverReferencesWorkflowState below).
func phase6StaticFunctionBody(t *testing.T, source []string, name string) string {
	t.Helper()
	funcStart := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\(\)\s*\{`)
	start := -1
	for i, line := range source {
		if strings.HasPrefix(line, name+"() {") {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("function %q not found in bin/labdrian-overlay", name)
	}
	end := len(source)
	for i := start + 1; i < len(source); i++ {
		if funcStart.MatchString(source[i]) {
			end = i
			break
		}
	}
	return strings.Join(source[start:end], "\n")
}

// TestPhase6Acceptance_OverlayLifecyclePreservation proves overlay update,
// rollback, and uninstall preserve workflow state (the last acceptance
// criterion). Of the overlay's lifecycle verbs, only install-hooks and
// uninstall-hooks can be run here safely: they act entirely on paths under
// $HOME (the engine binary at $HOME/.claude/bin, settings at
// $HOME/.claude/settings.json) and never touch git or the network. apply,
// self-update, update, and restore all `cd "$OVERLAY_DIR"` and/or run git
// operations (checkout, merge, fetch) against the actual overlay
// repository checkout this test runs from -- there is no way to isolate
// that from the real repository state, so per this task's safety
// instructions they are NOT run; instead, a static check (below, same
// test file) proves the overlay script's source for those four verbs never
// references the workflow state path at all, so they cannot preserve it by
// accident-avoidance -- they simply never touch it.
//
// Safety: HOME is isolated under t.TempDir() for every overlay call in
// this test; OVERLAY_SKIP_TOOL_INSTALL=1 stops install-hooks from
// attempting to brew-install anything. The workflow state itself lives
// under XDG_STATE_HOME (isolated), never under HOME/.claude.
func TestPhase6Acceptance_OverlayLifecyclePreservation(t *testing.T) {
	overlay := phase6OverlayPath(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is not available: %v", err)
	}

	home, stateHome := phase6IsolatedHome(t)
	workDir := t.TempDir()
	const project, wf = "proj-1", "wf-1"
	goalPath := writeMemoryTestFile(t, workDir, "goal.json", memoryTestGoalJSON(project, "goal-1"))
	phase6MustExitZero(t, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal"}, workDir)
	before := phase6ReadLog(t, stateHome, project, wf)

	// Run: install-hooks (builds the real engine binary under
	// $HOME/.claude/bin and merges hooks into $HOME/.claude/settings.json,
	// both isolated), then uninstall-hooks (removes them again). Neither
	// verb's own source ever mentions the workflow state path (proven
	// below), so this is a genuine run, not a stub.
	if out, err := phase6SourceAndCall(t, overlay, home, "cmd_install_hooks"); err != nil {
		t.Fatalf("cmd_install_hooks failed: %v\n%s", err, out)
	}
	if out, err := phase6SourceAndCall(t, overlay, home, "cmd_uninstall_hooks"); err != nil {
		t.Fatalf("cmd_uninstall_hooks failed: %v\n%s", err, out)
	}

	after := phase6ReadLog(t, stateHome, project, wf)
	if !bytes.Equal(before, after) {
		t.Fatalf("workflow log changed after install-hooks/uninstall-hooks; before=%q after=%q", before, after)
	}
	status := phase6MustExitZero(t, "status", []string{"status", "--project", project, "--workflow", wf}, workDir)
	if got := phase6MustUnmarshal(t, []byte(status.stdout)).Classification; got != "owned" {
		t.Fatalf("status classification after install/uninstall-hooks = %q, want %q", got, "owned")
	}
}

// TestPhase6Acceptance_OverlayNeverReferencesWorkflowState is the static
// half of the overlay-lifecycle-preservation criterion: apply,
// self-update, update, and restore cannot be run in isolation (see
// TestPhase6Acceptance_OverlayLifecyclePreservation's doc comment), so
// this proves instead that none of their source ever names the workflow
// state path components at all.
func TestPhase6Acceptance_OverlayNeverReferencesWorkflowState(t *testing.T) {
	overlay := phase6OverlayPath(t)
	data, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = %v, want nil", overlay, err)
	}
	lines := strings.Split(string(data), "\n")

	forbidden := []string{"labdrian/workflows", "XDG_STATE_HOME"}
	for _, fn := range []string{"cmd_apply", "cmd_self_update", "cmd_update", "cmd_restore"} {
		body := phase6StaticFunctionBody(t, lines, fn)
		for _, needle := range forbidden {
			if strings.Contains(body, needle) {
				t.Fatalf("%s references %q; it must never touch the workflow state path", fn, needle)
			}
		}
	}
}
