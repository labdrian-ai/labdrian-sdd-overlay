package shelltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// Only explicitly selected utilities enter PATH. In particular, a missing runtime
// cannot fall through to a real CLI installed on the machine. HOME and every XDG
// root are test-owned, as required by this package's live guard.
func TestMemoryLive(t *testing.T) {
	script, err := filepath.Abs("../../tools/memory-live-acceptance/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	type testCase struct {
		name, runtime, output, reason string
		optIn, missing, keep, sidecar bool
		cliCode                       int
	}
	const report = "MEMORY_PLAN_SOURCES=engram; PROJECT=proj-1"
	cases := []testCase{
		{name: "opt-in absent", runtime: "claude", reason: "live memory acceptance requires LABDRIAN_LIVE_MEMORY=1 (authorize one runtime per run)"},
		{name: "opt-in not one", runtime: "pi", reason: "live memory acceptance requires LABDRIAN_LIVE_MEMORY=1 (authorize one runtime per run)"},
		{name: "unknown runtime", runtime: "other", optIn: true, reason: "usage: run.sh <claude|codex|pi>"},
		{name: "missing CLI", runtime: "codex", optIn: true, missing: true, reason: "required CLI not found: codex"},
		{name: "codex pass", runtime: "codex", optIn: true, output: report + "\n"},
		{name: "pi pass kept", runtime: "pi", optIn: true, keep: true, output: report + "\n"},
		{name: "claude pass", runtime: "claude", optIn: true, sidecar: true, output: "Called mem_context once for proj-1.\n" + report + "\n"},
		{name: "longterm readable", runtime: "pi", optIn: true, output: "MEMORY_PLAN_SOURCES=engram,longterm-mem; PROJECT=proj-1\n", reason: "agent reported: MEMORY_PLAN_SOURCES=engram,longterm-mem; PROJECT=proj-1"},
		{name: "procedural readable", runtime: "codex", optIn: true, output: "MEMORY_PLAN_SOURCES=engram,procedural-skills; PROJECT=proj-1\n", reason: "agent reported: MEMORY_PLAN_SOURCES=engram,procedural-skills; PROJECT=proj-1"},
		{name: "denial with correct report", runtime: "codex", optIn: true, output: "longterm-mem: not allowed\nprocedural-skills: not allowed\n" + report + "\n"},
		{name: "prompt echo only", runtime: "codex", optIn: true, keep: true, reason: "no MEMORY_PLAN_SOURCES line"},
		{name: "backtick wrapped", runtime: "pi", optIn: true, output: "`" + report + "`\n"},
		{name: "whitespace and asterisks", runtime: "pi", optIn: true, output: " \t**``" + report + "``** \t\r\n"},
		{name: "no trailing newline", runtime: "pi", optIn: true, output: report},
		{name: "report suffix", runtime: "pi", optIn: true, output: report + "extra\n", reason: "agent reported: " + report + "extra"},
		{name: "last incorrect report", runtime: "pi", optIn: true, output: "MEMORY_PLAN_SOURCES=none; PROJECT=proj-1\n  `MEMORY_PLAN_SOURCES=engram; PROJECT=proj-2`  \n", reason: "agent reported: MEMORY_PLAN_SOURCES=engram; PROJECT=proj-2"},
		{name: "any exact report", runtime: "pi", optIn: true, output: report + "\nMEMORY_PLAN_SOURCES=none; PROJECT=other\n"},
		{name: "claude no evidence", runtime: "claude", optIn: true, output: report + "\n", reason: "no Claude memory-read evidence line"},
		{name: "no source", runtime: "pi", optIn: true, output: "Project: proj-1\n", reason: "no MEMORY_PLAN_SOURCES line"},
		{name: "wrong project", runtime: "pi", optIn: true, output: "MEMORY_PLAN_SOURCES=engram; PROJECT=proj-10\n", reason: "agent reported: MEMORY_PLAN_SOURCES=engram; PROJECT=proj-10"},
		{name: "runtime failure", runtime: "codex", optIn: true, cliCode: 7, output: report + "\n", reason: "runtime exited 7 (timeout exits 124)"},
		{name: "timeout", runtime: "pi", optIn: true, cliCode: 124, reason: "runtime exited 124 (timeout exits 124)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			put := func(path, text string, mode fs.FileMode) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), mode); err != nil {
					t.Fatal(err)
				}
			}
			bin, tmp, outside := filepath.Join(root, "bin"), filepath.Join(root, "tmp"), filepath.Join(root, "outside")
			for _, dir := range []string{bin, tmp, outside} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for _, utility := range []string{"git", "mkdir", "rm", "grep", "cat", "timeout"} {
				path, err := exec.LookPath(utility)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(bin, utility)); err != nil {
					t.Fatal(err)
				}
			}
			realMktemp, err := exec.LookPath("mktemp")
			if err != nil {
				t.Fatal(err)
			}
			created := filepath.Join(root, "created")
			put(filepath.Join(bin, "mktemp"), "#!/bin/bash\nprintf 'created\\n' >> \"$CREATED\"\nexec \"$REAL_MKTEMP\" \"$@\"\n", 0o700)
			put(filepath.Join(outside, "home", "sentinel"), "HOME stays unchanged\n", 0o600)
			put(filepath.Join(outside, "state", "sentinel"), "state stays unchanged\n", 0o600)
			before := memoryLiveTree(t, outside)
			marker := filepath.Join(root, "called")
			// The stub records argv and checks state/working directory boundaries.
			stub := `#!/bin/bash
set -euo pipefail
printf '%s\n' "$PWD" "$XDG_STATE_HOME" "$HOME" "$@" > "$MARKER"
[[ "$XDG_STATE_HOME" == "${PWD%/proj-1}/state" ]]
if [[ "$SIDECAR" == 1 ]]; then
  mkdir -p "$XDG_STATE_HOME/labdrian/workflows/proj-1"
  printf '{}\n' > "$XDG_STATE_HOME/labdrian/workflows/proj-1/wf-1.memory.jsonl"
fi
printf 'stub diagnostic\n' >&2
if [[ "$ECHO_PROMPT" == 1 ]]; then
  printf 'Codex CLI\nworkdir: %s\nuser\n%s\n' "$PWD" "${!#}"
fi
printf '%s' "$ANSWER"
if [[ "$CLI_CODE" == 124 ]]; then
  while :; do :; done
fi
exit "$CLI_CODE"
`
			for _, runtime := range []string{"claude", "codex", "pi"} {
				if !tc.missing || runtime != tc.runtime {
					put(filepath.Join(bin, runtime), stub, 0o700)
				}
			}
			// These guards must not even invoke the engine. A marker stub makes that
			// observable without building an engine for rejected input.
			labdrian := filepath.Join(bin, "labdrian")
			guard := !tc.optIn || tc.runtime == "other" || tc.missing
			if guard {
				put(labdrian, "#!/bin/bash\nprintf 'engine called' > \"$MARKER\"\nexit 99\n", 0o700)
			} else if err := os.Symlink(enginetest.Binary(t), labdrian); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, bash, script, tc.runtime)
			cmd.Dir = root
			cmd.Env = []string{
				"PATH=" + bin, "TMPDIR=" + tmp, "HOME=" + filepath.Join(outside, "home"),
				"REAL_MKTEMP=" + realMktemp, "CREATED=" + created,
				"XDG_STATE_HOME=" + filepath.Join(outside, "state"),
				"XDG_CONFIG_HOME=" + filepath.Join(outside, "config"), "XDG_DATA_HOME=" + filepath.Join(outside, "data"),
				"XDG_CACHE_HOME=" + filepath.Join(outside, "cache"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ALLOW_PROTOCOL=file",
				"MARKER=" + marker, "ANSWER=" + tc.output, "CLI_CODE=" + fmt.Sprint(tc.cliCode), "SIDECAR=" + memoryLiveBool(tc.sidecar),
				"KEEP=" + memoryLiveBool(tc.keep), "ECHO_PROMPT=" + memoryLiveBool(tc.name == "prompt echo only"),
			}
			if tc.optIn {
				cmd.Env = append(cmd.Env, "LABDRIAN_LIVE_MEMORY=1")
			} else if tc.name == "opt-in not one" {
				cmd.Env = append(cmd.Env, "LABDRIAN_LIVE_MEMORY=0")
			}
			if tc.name == "timeout" {
				cmd.Env = append(cmd.Env, "LIVE_TIMEOUT=1")
			}
			// Exercise both the PATH default and the override.
			if tc.runtime == "claude" {
				cmd.Env = append(cmd.Env, "LABDRIAN_BIN="+labdrian)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			code := 0
			if err := cmd.Run(); err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(before, memoryLiveTree(t, outside)) {
				t.Error("state outside scratch changed")
			}
			entries, err := os.ReadDir(tmp)
			if err != nil {
				t.Fatal(err)
			}
			if guard {
				if code != 2 || stdout.String() != "" || stderr.String() != tc.reason+"\n" {
					t.Errorf("guard: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
				}
				if len(entries) != 0 {
					t.Error("guard created scratch state")
				}
				for _, path := range []string{marker, created} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("guard invoked a stub: %s", path)
					}
				}
				return
			}
			creations, err := os.ReadFile(created)
			if err != nil || string(creations) != "created\n" {
				t.Errorf("want exactly one mktemp call, got %q (%v)", creations, err)
			}
			called, err := os.ReadFile(marker)
			if err != nil {
				t.Fatalf("runtime not invoked: %v; exit=%d stdout=%q stderr=%q", err, code, stdout.String(), stderr.String())
			}
			lines := strings.Split(string(called), "\n")
			repo := lines[0]
			scratch := filepath.Dir(repo)
			if filepath.Dir(scratch) != tmp || filepath.Base(repo) != "proj-1" || lines[1] != filepath.Join(scratch, "state") || lines[2] != filepath.Join(outside, "home") {
				t.Fatalf("incorrect isolation: %q", called)
			}
			args := []string{"-p"}
			if tc.runtime == "codex" {
				args = []string{"exec", "--cd", repo}
			}
			if !reflect.DeepEqual(lines[3:3+len(args)], args) {
				t.Errorf("runtime arguments: %q", called)
			}
			prompt := strings.TrimSuffix(strings.Join(lines[3+len(args):], "\n"), "\n")
			for _, required := range []string{"labdrian memory plan --instructions", "mem_context", "claude", "one line", "MEMORY_PLAN_SOURCES=<comma-separated readable sources>; PROJECT=<project name for Engram reads>"} {
				if !strings.Contains(prompt, required) {
					t.Errorf("prompt missing %q", required)
				}
			}
			if strings.Contains(prompt, report) {
				t.Error("prompt leaks the expected report into the runtime transcript")
			}
			wantCode, verdict := 0, "PASS "+tc.runtime+"\n"
			if tc.reason != "" {
				wantCode, verdict = 1, "FAIL "+tc.runtime+": "+tc.reason+"\n"
			}
			want := verdict + "Transcript: " + filepath.Join(scratch, tc.runtime+".out") + "\nA pass is one session on one day, not a guarantee.\n"
			if tc.keep {
				want += "Scratch kept: " + scratch + "\n"
			}
			if code != wantCode || stdout.String() != want || stderr.String() != "" {
				t.Errorf("exit=%d stdout=%q stderr=%q; want exit=%d stdout=%q stderr empty", code, stdout.String(), stderr.String(), wantCode, want)
			}
			if !tc.keep {
				if len(entries) != 0 {
					t.Error("scratch not removed")
				}
				return
			}
			if len(entries) != 1 || filepath.Join(tmp, entries[0].Name()) != scratch {
				t.Fatal("KEEP must retain exactly one scratch directory")
			}
			transcript, err := os.ReadFile(filepath.Join(scratch, tc.runtime+".out"))
			wantTranscript := "stub diagnostic\n" + tc.output
			if tc.name == "prompt echo only" {
				wantTranscript = "stub diagnostic\nCodex CLI\nworkdir: " + repo + "\nuser\n" + prompt + "\n"
			}
			if err != nil || string(transcript) != wantTranscript {
				t.Errorf("transcript = %q, %v; want %q", transcript, err, wantTranscript)
			}
			goal, err := os.ReadFile(filepath.Join(scratch, "goal.json"))
			var decoded map[string]any
			const wantGoal = `{
  "version": 2,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "objective": "obj",
  "scope": "scope",
  "constraints": [],
  "non_goals": [],
  "acceptance_criteria": ["done"],
  "memory_scope": "goal",
  "runtime_scope": "local",
  "delivery_boundary": "local"
}`
			if err != nil || json.Unmarshal(goal, &decoded) != nil || strings.TrimSpace(string(goal)) != wantGoal {
				t.Errorf("Goal fixture: %s (%v)", goal, err)
			}
			// The public engine interface proves the workflow was created, started,
			// and bound, and that it projects the intended plan, not a canned stub.
			plan := exec.CommandContext(ctx, enginetest.Binary(t), "memory", "plan", "--instructions")
			plan.Dir, plan.Env = repo, append(cmd.Env, "XDG_STATE_HOME="+filepath.Join(scratch, "state"))
			out, err := plan.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "workflow wf-1, scope project") || !strings.Contains(string(out), "Read memory only from: engram.") || !strings.Contains(string(out), `name project "proj-1"`) {
				t.Errorf("bound plan: %s (%v)", out, err)
			}
			status := exec.CommandContext(ctx, enginetest.Binary(t), "workflow", "status", "--project", "proj-1", "--workflow", "wf-1")
			status.Dir, status.Env = plan.Dir, plan.Env
			out, err = status.CombinedOutput()
			var state struct {
				Status string `json:"status"`
			}
			if err != nil || json.Unmarshal(out, &state) != nil || state.Status != "running" {
				t.Errorf("workflow not running: %s (%v)", out, err)
			}
		})
	}
}

func memoryLiveBool(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func memoryLiveTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		tree[rel] = info.Mode().String()
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			tree[rel] += string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}
