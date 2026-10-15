package overlaycli

// The adapter's other two ports: Run carries out one invocation of the backend
// and says how it ended, and Report reads the cached-only sync-check the launch
// probe is made of. What the real backend does for each is pinned by the
// *_backend_test.go files; these prove the process contract over fakes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordedRun reads the one invocation a stub backend recorded: the working
// directory it ran in and its arguments.
func recordedRun(t *testing.T, recorder string) (dir, args string) {
	t.Helper()
	data, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatalf("the backend was never invoked: %v", err)
	}
	dir, args, _ = strings.Cut(strings.TrimSpace(string(data)), "|")
	dir, _ = filepath.EvalSymlinks(dir)
	return dir, args
}

// TestOverlayCLI_Run_RunsOneInvocationFromTheRepoRoot: the arguments reach the
// script as they are, in the repo root, and the output comes back.
func TestOverlayCLI_Run_RunsOneInvocationFromTheRepoRoot(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	writeStubBackend(t, root, recorder, "applied", 0)

	run := New(root).Run([]string{"apply", "--target", "claude"})

	if run.Err != nil || run.ExitCode != 0 || strings.TrimSpace(run.Output) != "applied" {
		t.Errorf("Run = %+v, want a clean run that printed applied", run)
	}
	dir, args := recordedRun(t, recorder)
	if args != "apply --target claude" {
		t.Errorf("backend invoked with %q, want exactly %q", args, "apply --target claude")
	}
	wantDir, _ := filepath.EvalSymlinks(root)
	if dir != wantDir {
		t.Errorf("backend ran in %q, want the repo root %q", dir, root)
	}
}

// TestOverlayCLI_Run_SaysHowTheBackendEnded: the exit status is what tells a
// hard failure (1) from a degraded run (2); what the backend wrote on both
// streams is kept, in the order it wrote it.
func TestOverlayCLI_Run_SaysHowTheBackendEnded(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		output   string
		exitCode int
		failed   bool
	}{
		{"a clean run", `echo done`, "done\n", 0, false},
		{"a hard failure", `echo boom; exit 1`, "boom\n", 1, true},
		{"a degraded run", `echo engine DEGRADED; exit 2`, "engine DEGRADED\n", 2, true},
		{"standard output and standard error together, in order", `echo one; echo two >&2; echo three; exit 3`, "one\ntwo\nthree\n", 3, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeScriptBackend(t, root, tc.script)

			run := New(root).Run([]string{"status"})

			if run.Output != tc.output || run.ExitCode != tc.exitCode || (run.Err != nil) != tc.failed {
				t.Errorf("Run = {output %q, exit %d, err %v}, want {output %q, exit %d, failed %t}",
					run.Output, run.ExitCode, run.Err, tc.output, tc.exitCode, tc.failed)
			}
		})
	}

	t.Run("a backend that cannot be started has no exit status of its own", func(t *testing.T) {
		run := New(t.TempDir()).Run([]string{"status"})
		if run.Err == nil || run.ExitCode != -1 {
			t.Errorf("Run = %+v, want a failure with exit -1", run)
		}
	})
}

// TestOverlayCLI_Report_RunsACachedOnlySyncCheckFromTheRepoRoot proves the
// exec contract of the launch probe (D4): `sync-check` alone, no
// --fetch/--check-origin (a cached-only probe, R-001), from the repo root,
// and the output is the report even when the process exits non-zero -- the
// probe's job is reading a cached ref, not requiring a clean exit.
func TestOverlayCLI_Report_RunsACachedOnlySyncCheckFromTheRepoRoot(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	const sample = "\n=== sync-check: claude ===\n" +
		"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=7\n" +
		"ACTION:claude: run 'git pull' to update your local clone\n"
	writeStubBackend(t, root, recorder, sample, 1) // nonzero exit on purpose

	verdicts := New(root).Report()

	if len(verdicts) != 1 || verdicts[0].Target != "claude" || verdicts[0].RepoBehindOrigin != 7 {
		t.Errorf("Report = %+v, want the claude verdict of the sync-check even though the stub exits nonzero", verdicts)
	}
	dir, args := recordedRun(t, recorder)
	if args != "sync-check" {
		t.Errorf("backend invoked with %q, want exactly sync-check: no --fetch/--check-origin (cached-only probe per R-001)", args)
	}
	wantDir, _ := filepath.EvalSymlinks(root)
	if dir != wantDir {
		t.Errorf("backend ran in %q, want the repo root %q", dir, root)
	}
}

// TestOverlayCLI_Report_WithNothingToReadHasNoVerdicts: a probe that cannot be
// run, or whose output holds no verdict, says nothing, which the domain
// degrades to "unknown" (BehindOf) rather than to "up to date".
func TestOverlayCLI_Report_WithNothingToReadHasNoVerdicts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{"a repo root without a backend", func(*testing.T, string) {}},
		{"a backend that prints nothing", func(t *testing.T, root string) { writeScriptBackend(t, root, `:`) }},
		{"a target skipped, so no verdict", func(t *testing.T, root string) {
			writeScriptBackend(t, root, `echo "SYNC_CHECK:claude: target dir not found -- skipping"`)
		}},
		{"output that is no report at all", func(t *testing.T, root string) {
			writeScriptBackend(t, root, `printf 'not even close to a sync-check line\n\000\377 binary noise'`)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)

			if verdicts := New(root).Report(); len(verdicts) != 0 {
				t.Errorf("Report = %+v, want no verdict", verdicts)
			}
		})
	}
}
