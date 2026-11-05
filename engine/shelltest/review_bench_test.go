package shelltest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// benchFixture copies score.sh next to a fixture corpus and returns the script and a bench directory.
func benchFixture(t *testing.T, summaries map[string]string) (string, string) {
	t.Helper()
	overlayScript(t) // require bash
	src, err := filepath.Abs("../../tools/review-bench/score.sh")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	script := filepath.Join(tools, "score.sh")
	if err := os.WriteFile(script, body, 0700); err != nil {
		t.Fatal(err)
	}
	corpus := `{"real": [
  {"id": "R1", "head": "aaaa", "base": "bbbb", "range": "bbbb..aaaa", "bytes": 1, "fix": "cccc",
   "defect": {"file": "engine/x/x.go", "lines": "17-18", "text": "empty word counts as found"}},
  {"id": "R2", "head": "dddd", "base": "eeee", "range": "eeee..dddd", "bytes": 1, "fix": "ffff",
   "defect": {"file": "engine/y/y.go", "lines": "40", "text": "error dropped"}},
  {"id": "R3", "head": "gggg", "base": "hhhh", "range": "hhhh..gggg", "bytes": 1, "fix": "iiii",
   "defect": {"file": "engine/z/z.go", "lines": "5-6", "text": "no summary case"}},
  {"id": "R4", "head": "jjjj", "base": "kkkk", "range": "kkkk..jjjj", "bytes": 1, "fix": "llll",
   "defect": {"file": "engine/w/w.go", "lines": "89-", "text": "open range"}}
], "mutants": []}`
	if err := os.WriteFile(filepath.Join(tools, "corpus.json"), []byte(corpus), 0600); err != nil {
		t.Fatal(err)
	}
	bench := t.TempDir()
	for id, summary := range summaries {
		dir := filepath.Join(bench, "out", id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(summary), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return script, bench
}

func runScore(t *testing.T, script, bench string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = []string{"BENCH=" + bench, "PATH=" + os.Getenv("PATH")}
	out, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

func TestReviewBenchScore(t *testing.T) {
	r1 := `{"id": "R1", "state": "approved", "lenses": [
  {"lens": "review-risk", "n": 2, "findings": [
    {"location": "engine/x/x.go:20-22", "severity": "WARNING", "claim": "an empty word matches every command"},
    {"location": "engine/x/x.go:40", "severity": "SUGGESTION", "claim": "name is unclear"}]},
  {"lens": "review-reliability", "n": 1, "findings": [
    {"location": "engine/other.go:17", "severity": "WARNING", "claim": "another file"}]}],
 "runs": [{"in": "100", "out": "5", "exit": "0", "secs": "1.0"}, {"in": "250", "out": "7", "exit": "0", "secs": "2.0"}]}`
	r2 := `{"id": "R2", "state": "approved", "lenses": [], "analyzers": ["engine/y/y.go:41:2: error return value is not checked", "engine/y/y.go:90:1: far away"], "runs": []}`
	script, bench := benchFixture(t, map[string]string{"R1": r1, "R2": r2})

	t.Run("located finding within the lines plus three, others left out", func(t *testing.T) {
		out, code := runScore(t, script, bench, "R1")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		want := "R1 engine/x/x.go:17-18 prompt_bytes=350\n" +
			"  finding review-risk WARNING engine/x/x.go:20-22: an empty word matches every command\n"
		if out != want {
			t.Fatalf("got\n%s\nwant\n%s", out, want)
		}
	})

	t.Run("analyzer line located, a far one left out", func(t *testing.T) {
		out, code := runScore(t, script, bench, "R2")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		want := "R2 engine/y/y.go:40 prompt_bytes=0\n" +
			"  analyzer engine/y/y.go:41:2: error return value is not checked\n"
		if out != want {
			t.Fatalf("got\n%s\nwant\n%s", out, want)
		}
	})

	t.Run("a case with no out directory is reported and fails the run", func(t *testing.T) {
		out, code := runScore(t, script, bench, "R1", "R3")
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if !strings.Contains(out, "R1 engine/x/x.go:17-18 prompt_bytes=350\n") {
			t.Fatalf("R1 not scored before R3\n%s", out)
		}
		if !strings.HasSuffix(out, "R3 no summary at "+filepath.Join(bench, "out", "R3", "summary.json")+"\n") {
			t.Fatalf("missing summary not reported\n%s", out)
		}
	})

	t.Run("a case with nothing located says so", func(t *testing.T) {
		script, bench := benchFixture(t, map[string]string{"R1": `{"id": "R1", "lenses": [], "runs": [{"in": "9"}]}`})
		out, code := runScore(t, script, bench, "R1")
		if code != 0 || out != "R1 engine/x/x.go:17-18 prompt_bytes=9\n  none located\n" {
			t.Fatalf("exit %d\n%s", code, out)
		}
	})

	t.Run("an open range names its first line", func(t *testing.T) {
		script, bench := benchFixture(t, map[string]string{"R4": `{"id": "R4", "lenses": [{"lens": "review-risk", "n": 2, "findings": [
  {"location": "engine/w/w.go:92", "severity": "WARNING", "claim": "near the first line"},
  {"location": "engine/w/w.go:93", "severity": "WARNING", "claim": "past the first line plus three"}]}], "runs": []}`})
		out, code := runScore(t, script, bench, "R4")
		want := "R4 engine/w/w.go:89- prompt_bytes=0\n  finding review-risk WARNING engine/w/w.go:92: near the first line\n"
		if code != 0 || out != want {
			t.Fatalf("exit %d\ngot\n%s\nwant\n%s", code, out, want)
		}
	})

	t.Run("an unknown id and no id are refused", func(t *testing.T) {
		out, code := runScore(t, script, bench, "R9")
		if code != 2 || out != "unknown case R9\n" {
			t.Fatalf("exit %d\n%s", code, out)
		}
		out, code = runScore(t, script, bench)
		if code != 2 || out != "usage: score.sh <id>...\n" {
			t.Fatalf("exit %d\n%s", code, out)
		}
	})

	t.Run("OUT selects another runner's summaries", func(t *testing.T) {
		dir := filepath.Join(bench, "out", "ours", "R1")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"id": "R1", "lenses": [], "runs": [{"in": "4"}]}`), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script, "R1")
		cmd.Env = []string{"BENCH=" + bench, "OUT=" + filepath.Join(bench, "out", "ours"), "PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err != nil || string(out) != "R1 engine/x/x.go:17-18 prompt_bytes=4\n  none located\n" {
			t.Fatalf("%v\n%s", err, out)
		}
	})
}

// stubLabdrian logs each call, answers sweep and bind, and makes a launch pipe its brief through the claude on the
// PATH (the bench shim) and store a result as the next <head12>-<n>.json, unless STUB_FAIL_ROW names its row.
const stubLabdrian = `#!/usr/bin/env bash
echo "$*" >> "$STUB_DIR/calls.log"
verb="$1 $2"
shift 2
while [ $# -gt 0 ]; do
  case "$1" in --base) b=$2; shift;; --head) h=$2; shift;; --rows) r=$2; shift;; --cwd) c=$2; shift;; esac
  shift
done
case "$verb" in
"review sweep")
  [ -z "${STUB_SWEEP_FAIL:-}" ] || { echo "sweep: staticcheck did not run" >&2; exit 2; }
  echo '{"outcome":"fired","path":"p","sha256":"s","candidates":["T1 path: gone.go","T2 staticcheck engine: x/x.go:18:2: error dropped","T2 shellcheck .: tools/a.sh:3:1: SC2086"]}';;
"review bind")
  echo "{\"base\":\"$b\",\"head\":\"$h\",\"diff_sha256\":\"d1ff\",\"worktree\":\"/wt/review-${h:0:12}\"}";;
"review launch")
  claude --print > /dev/null
  [ "${STUB_FAIL_ROW:-}" != "$r" ] || { echo "launch: claude exited 1" >&2; exit 1; }
  dir="$c/odd/review-results/review-bench"
  mkdir -p "$dir"
  n=$(( $(compgen -G "$dir/${h:0:12}-*.json" | wc -l) + 1 ))
  echo "{\"version\":1,\"rows\":[\"$r\"],\"reply\":{\"findings\":[{\"location\":\"engine/x/x.go:17\",\"severity\":\"WARNING\",\"claim\":\"claim of $r\"}]}}" > "$dir/${h:0:12}-$n.json"
  echo '{"payload_kind":"review-report"}';;
esac
`

// oursFixture builds a bench: a clone of two commits, a corpus whose R1 is the second, run-ours.sh and the shim beside
// it, a rows file, a stub labdrian on the PATH and a stub claude at $HOME/bin/claude that saves the brief it reads.
func oursFixture(t *testing.T) (script, bench, head string, env []string) {
	t.Helper()
	overlayScript(t) // require bash
	tools := t.TempDir()
	for _, f := range []string{"run-ours.sh", "shim/claude"} {
		body, err := os.ReadFile(filepath.Join("../../tools/review-bench", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(tools, f)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tools, f), body, 0700); err != nil {
			t.Fatal(err)
		}
	}
	rows := `{"rows": [{"id": "default", "sentence": "Inspect behavior."}, {"id": "hunt", "sentence": "Hunt four classes."}]}`
	if err := os.WriteFile(filepath.Join(tools, "rows.json"), []byte(rows), 0600); err != nil {
		t.Fatal(err)
	}
	bench = t.TempDir()
	repo := filepath.Join(bench, "repo")
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "base")
	git("commit", "-q", "--allow-empty", "-m", "head")
	head = git("rev-parse", "HEAD")
	corpus := `{"real": [{"id": "R1", "head": "` + head[:8] + `", "base": "` + head[:8] + `^", "range": "` + head[:8] + `^..` + head[:8] + `",
  "bytes": 1, "fix": "cccc", "defect": {"file": "engine/x/x.go", "lines": "17", "text": "t"}}], "mutants": []}`
	if err := os.WriteFile(filepath.Join(tools, "corpus.json"), []byte(corpus), 0600); err != nil {
		t.Fatal(err)
	}
	stubs := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "labdrian"), []byte(stubLabdrian), 0700); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	claude := "#!/usr/bin/env bash\ncat > \"$STUB_DIR/brief-$(date +%s%N)\"\necho reply\n"
	if err := os.WriteFile(filepath.Join(home, "bin", "claude"), []byte(claude), 0700); err != nil {
		t.Fatal(err)
	}
	env = []string{"BENCH=" + bench, "HOME=" + home, "STUB_DIR=" + stubs, "ROWS_FILE=" + filepath.Join(tools, "rows.json"),
		"PATH=" + stubs + string(os.PathListSeparator) + os.Getenv("PATH")}
	return filepath.Join(tools, "run-ours.sh"), bench, head, env
}

// stubDir is the STUB_DIR of a fixture's environment, found by its key so the order of env does not matter.
func stubDir(t *testing.T, env []string) string {
	t.Helper()
	for _, kv := range env {
		if dir, ok := strings.CutPrefix(kv, "STUB_DIR="); ok {
			return dir
		}
	}
	t.Fatal("the fixture environment has no STUB_DIR")
	return ""
}

// runScript runs one bench script (run-ours.sh or run-gentle.sh) under env and returns its stdout, stderr and exit code.
func runScript(t *testing.T, script string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), code
}

func readSummary(t *testing.T, bench string) string {
	t.Helper()
	out, err := exec.Command("jq", "-c", "del(.secs)", filepath.Join(bench, "out", "ours", "R1", "summary.json")).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestReviewBenchRunOurs(t *testing.T) {
	t.Run("sweep, bind, then one launch per row in order, and the summary of them", func(t *testing.T) {
		script, bench, head, env := oursFixture(t)
		// A result of an earlier review of the same head is not this run's.
		old := filepath.Join(bench, "repo", "odd", "review-results", "review-bench")
		if err := os.MkdirAll(old, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(old, head[:12]+"-1.json"), []byte(`{"reply":{"findings":[]}}`), 0600); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, code := runScript(t, script, env, "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		stubs := stubDir(t, env)
		calls := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(stubs, "calls.log"))), "\n")
		repo := filepath.Join(bench, "repo")
		parent, err := exec.Command("git", "-C", repo, "rev-parse", head+"^").Output()
		if err != nil {
			t.Fatal(err)
		}
		b := strings.TrimSpace(string(parent))
		want := []string{
			"review sweep --feature review-bench --base " + b + " --head " + head + " --cwd " + repo,
			"review bind --base " + b + " --head " + head + " --cwd " + repo,
			"review launch --runtime claude --feature review-bench --base " + b + " --head " + head + " --rows default --model claude-sonnet-5-5 --cwd " + repo,
			"review launch --runtime claude --feature review-bench --base " + b + " --head " + head + " --rows hunt --model claude-sonnet-5-5 --cwd " + repo,
		}
		if strings.Join(calls, "\n") != strings.Join(want, "\n") {
			t.Fatalf("calls\n%s\nwant\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
		}
		brief := readFile(t, filepath.Join(bench, "out", "ours", "R1", "brief-hunt.txt"))
		wantBrief := "Lens: Hunt four classes.\nbase: " + b + "\nhead: " + head +
			"\ndiff_sha256: d1ff\nworktree: /wt/review-" + head[:12] + "\n"
		if brief != wantBrief {
			t.Fatalf("brief\n%s\nwant\n%s", brief, wantBrief)
		}
		briefs, err := filepath.Glob(filepath.Join(stubs, "brief-*"))
		if err != nil || len(briefs) != 2 {
			t.Fatalf("claude ran %d times: %v", len(briefs), err)
		}
		got := readSummary(t, bench)
		wantSummary := `{"id":"R1","state":"recorded","lenses":[` +
			`{"lens":"default","n":1,"findings":[{"location":"engine/x/x.go:17","severity":"WARNING","claim":"claim of default"}]},` +
			`{"lens":"hunt","n":1,"findings":[{"location":"engine/x/x.go:17","severity":"WARNING","claim":"claim of hunt"}]}],` +
			`"analyzers":["engine/x/x.go:18:2: error dropped [staticcheck]","tools/a.sh:3:1: SC2086 [shellcheck]"],` +
			`"runs":[{"in":"` + strconv.Itoa(len(readFile(t, filepath.Join(bench, "out", "ours", "R1", "brief-default.txt")))) + `","out":"6","exit":"0","secs":`
		if !strings.HasPrefix(got, wantSummary) || strings.Count(got, `"in":`) != 2 {
			t.Fatalf("summary\n%s\nwant prefix\n%s", got, wantSummary)
		}
		if stdout != `{"id":"R1","state":"recorded","secs":`+jqField(t, bench, ".secs")+`,"lenses":[{"lens":"default","n":1},{"lens":"hunt","n":1}],"analyzers":2,"runs":2}`+"\n" {
			t.Fatalf("stdout %q", stdout)
		}
	})

	t.Run("ROWS picks the rows, and a row the rows file lacks is refused before anything runs", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		_, stderr, code := runScript(t, script, append(env, "ROWS=default"), "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if got := jqField(t, bench, "[.lenses[].lens] | join(\",\")"); got != `"default"` {
			t.Fatalf("lenses %s", got)
		}
		stubs := stubDir(t, env)
		if err := os.Remove(filepath.Join(stubs, "calls.log")); err != nil {
			t.Fatal(err)
		}
		_, stderr, code = runScript(t, script, append(env, "ROWS=default,nope"), "R1")
		if code != 2 || stderr != "unknown row nope\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(stubs, "calls.log")); !os.IsNotExist(err) {
			t.Fatalf("labdrian ran: %v", err)
		}
	})

	t.Run("the default rows file is the repository's, read by its rows", func(t *testing.T) {
		overlayScript(t) // require bash
		script, err := filepath.Abs("../../tools/review-bench/run-ours.sh")
		if err != nil {
			t.Fatal(err)
		}
		// An empty bench has no clone: a known row passes the row check and the case stops at the checkout.
		env := []string{"BENCH=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}
		if _, stderr, code := runScript(t, script, append(env, "ROWS=default,readability"), "R1"); code != 2 || strings.Contains(stderr, "unknown row") {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if _, stderr, code := runScript(t, script, append(env, "ROWS=default,nope"), "R1"); code != 2 || stderr != "unknown row nope\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
	})

	t.Run("ROWS=recall,hunt over the repository's rows file writes the recall row's sentence in its brief", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		rows, err := filepath.Abs("../../roles/reviewer/rows.json")
		if err != nil {
			t.Fatal(err)
		}
		_, stderr, code := runScript(t, script, append(env, "ROWS_FILE="+rows, "ROWS=recall,hunt"), "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		brief := readFile(t, filepath.Join(bench, "out", "ours", "R1", "brief-recall.txt"))
		first, _, _ := strings.Cut(brief, "\n")
		if !strings.HasPrefix(first, "Lens: Inspect behavior, tests, boundaries,") ||
			!strings.HasSuffix(first, "a concrete failure mode for each finding; report a failure whose trigger you cannot show realistic input reaches as a WARNING or a SUGGESTION rather than leaving it out.") ||
			strings.Contains(first, "do not report speculation") {
			t.Fatalf("the recall brief's lens line is %q, want the default row's sentence with S2 (b)'s substitution", first)
		}
		if got := jqField(t, bench, "[.lenses[].lens] | join(\",\")"); got != `"recall,hunt"` {
			t.Fatalf("lenses %s", got)
		}
	})

	t.Run("a rows file that cannot be read is named, not taken for an unknown row", func(t *testing.T) {
		script, _, _, env := oursFixture(t)
		missing := filepath.Join(t.TempDir(), "rows.json")
		_, stderr, code := runScript(t, script, append(env, "ROWS_FILE="+missing), "R1")
		if code != 2 || stderr != "cannot read the rows of "+missing+"\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
	})

	t.Run("a failed launch ends the case non-zero with a summary that says so", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		_, stderr, code := runScript(t, script, append(env, "STUB_FAIL_ROW=default"), "R1")
		if code != 4 || stderr != "case R1 ended without a review result: launch failed: default\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if got := jqField(t, bench, "[.state, (.lenses | length)]"); got != `["launch failed: default",0]` {
			t.Fatalf("summary %s", got)
		}
		stubs := stubDir(t, env)
		if calls := readFile(t, filepath.Join(stubs, "calls.log")); strings.Contains(calls, "--rows hunt") {
			t.Fatalf("the next row ran after a failure:\n%s", calls)
		}
	})

	t.Run("a failed sweep ends the case before any launch", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		_, stderr, code := runScript(t, script, append(env, "STUB_SWEEP_FAIL=1"), "R1")
		if code != 4 || stderr != "case R1 ended without a review result: sweep failed\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if got := jqField(t, bench, "[.state, .analyzers, .runs]"); got != `["sweep failed",[],[]]` {
			t.Fatalf("summary %s", got)
		}
	})

	t.Run("RUN=2 writes under ours/run2/<id> and leaves an earlier run1 untouched", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		run1 := filepath.Join(bench, "out", "ours", "run1", "R1", "summary.json")
		if err := os.MkdirAll(filepath.Dir(run1), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(run1, []byte("run one\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runScript(t, script, append(env, "RUN=2"), "R1"); code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		out, err := exec.Command("jq", "-r", ".state", filepath.Join(bench, "out", "ours", "run2", "R1", "summary.json")).Output()
		if err != nil || strings.TrimSpace(string(out)) != "recorded" {
			t.Fatalf("run2 summary state %q: %v", out, err)
		}
		if got := readFile(t, run1); got != "run one\n" {
			t.Fatalf("run1 summary changed: %q", got)
		}
		if _, err := os.Stat(filepath.Join(bench, "out", "ours", "R1")); !os.IsNotExist(err) {
			t.Fatalf("RUN=2 also wrote out/ours/R1: %v", err)
		}
	})

	t.Run("a RUN that is not a positive integer exits 2 before anything runs", func(t *testing.T) {
		for _, run := range []string{"0", "x", "-1"} {
			script, bench, _, env := oursFixture(t)
			_, stderr, code := runScript(t, script, append(env, "RUN="+run), "R1")
			if code != 2 || stderr != "RUN must be a positive integer, not "+run+"\n" {
				t.Fatalf("RUN=%s: exit %d\n%s", run, code, stderr)
			}
			stubs := stubDir(t, env)
			if _, err := os.Stat(filepath.Join(stubs, "calls.log")); !os.IsNotExist(err) {
				t.Fatalf("RUN=%s: labdrian ran: %v", run, err)
			}
			if _, err := os.Stat(filepath.Join(bench, "out")); !os.IsNotExist(err) {
				t.Fatalf("RUN=%s: an output directory was made: %v", run, err)
			}
		}
	})

	t.Run("an unknown id and a missing id are refused", func(t *testing.T) {
		script, _, _, env := oursFixture(t)
		if _, stderr, code := runScript(t, script, env, "R9"); code != 2 || stderr != "unknown case R9\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if _, stderr, code := runScript(t, script, env); code != 2 || stderr != "usage: run-ours.sh <id>\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
	})
}

// stubGentle answers the three calls run-gentle.sh makes on a fresh case: the first STATUS names a START, the START a
// bound STATUS, and the bound STATUS stops, so the case ends without a review result and still writes its summary.
const stubGentle = `#!/usr/bin/env bash
echo "$*" >> "$STUB_DIR/gentle.log"
case "$1 $2" in
"review status")
  case " $* " in
  *" --bound "*) echo '{"next_transition":{"kind":"stop","reason_code":"stub_stop"}}';;
  *) echo '{"next_transition":{"execute":{"command":"gentle-ai review start"}}}';;
  esac;;
"review start") echo '{"action":"started","next_transition":{"execute":{"command":"gentle-ai review status --bound"}}}';;
esac
`

// gentleFixture is oursFixture with run-gentle.sh beside run-ours.sh, a stub gentle-ai on the PATH and an empty shim.log.
func gentleFixture(t *testing.T) (script, bench string, env []string) {
	t.Helper()
	ours, bench, _, env := oursFixture(t)
	body, err := os.ReadFile("../../tools/review-bench/run-gentle.sh")
	if err != nil {
		t.Fatal(err)
	}
	script = filepath.Join(filepath.Dir(ours), "run-gentle.sh")
	if err := os.WriteFile(script, body, 0700); err != nil {
		t.Fatal(err)
	}
	stubs := stubDir(t, env)
	if err := os.WriteFile(filepath.Join(stubs, "gentle-ai"), []byte(stubGentle), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bench, "scratch"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bench, "scratch", "shim.log"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return script, bench, env
}

func TestReviewBenchRunGentle(t *testing.T) {
	stopped := "case R1 ended without a review result: status:stop:stub_stop\n"

	t.Run("RUN unset writes out/<id> as before", func(t *testing.T) {
		script, bench, env := gentleFixture(t)
		if _, stderr, code := runScript(t, script, env, "R1"); code != 4 || stderr != stopped {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		out, err := exec.Command("jq", "-c", "[.id, .state]", filepath.Join(bench, "out", "R1", "summary.json")).Output()
		if err != nil || strings.TrimSpace(string(out)) != `["R1","status:stop:stub_stop"]` {
			t.Fatalf("summary %q: %v", out, err)
		}
	})

	t.Run("RUN=2 writes under run2/<id> and leaves an earlier run1 untouched", func(t *testing.T) {
		script, bench, env := gentleFixture(t)
		run1 := filepath.Join(bench, "out", "run1", "R1", "summary.json")
		if err := os.MkdirAll(filepath.Dir(run1), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(run1, []byte("run one\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runScript(t, script, append(env, "RUN=2"), "R1"); code != 4 || stderr != stopped {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		out, err := exec.Command("jq", "-r", ".id", filepath.Join(bench, "out", "run2", "R1", "summary.json")).Output()
		if err != nil || strings.TrimSpace(string(out)) != "R1" {
			t.Fatalf("run2 summary %q: %v", out, err)
		}
		if got := readFile(t, run1); got != "run one\n" {
			t.Fatalf("run1 summary changed: %q", got)
		}
		if _, err := os.Stat(filepath.Join(bench, "out", "R1")); !os.IsNotExist(err) {
			t.Fatalf("RUN=2 also wrote out/R1: %v", err)
		}
	})

	t.Run("a RUN that is not a positive integer exits 2 before anything runs", func(t *testing.T) {
		for _, run := range []string{"0", "x", "-1"} {
			script, bench, env := gentleFixture(t)
			_, stderr, code := runScript(t, script, append(env, "RUN="+run), "R1")
			if code != 2 || stderr != "RUN must be a positive integer, not "+run+"\n" {
				t.Fatalf("RUN=%s: exit %d\n%s", run, code, stderr)
			}
			stubs := stubDir(t, env)
			if _, err := os.Stat(filepath.Join(stubs, "gentle.log")); !os.IsNotExist(err) {
				t.Fatalf("RUN=%s: gentle-ai ran: %v", run, err)
			}
			if _, err := os.Stat(filepath.Join(bench, "out")); !os.IsNotExist(err) {
				t.Fatalf("RUN=%s: an output directory was made: %v", run, err)
			}
		}
	})
}

func jqField(t *testing.T, bench, filter string) string {
	t.Helper()
	out, err := exec.Command("jq", "-c", filter, filepath.Join(bench, "out", "ours", "R1", "summary.json")).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// frozenFinding is one entry of the array findings.sh prints (phase9-c28-review-precision S2 b).
type frozenFinding struct {
	Key               string   `json:"key"`
	Runner            string   `json:"runner"`
	ID                string   `json:"id"`
	Run               int      `json:"run"`
	Row               string   `json:"row"`
	Finding           string   `json:"finding"`
	Severity          string   `json:"severity"`
	Location          string   `json:"location"`
	Claim             string   `json:"claim"`
	ProofRefs         []string `json:"proof_refs"`
	EvidenceClass     *string  `json:"evidence_class"`
	CausalDisposition *string  `json:"causal_disposition"`
	C27Control        bool     `json:"c27_control"`
}

// findingsFixture copies findings.sh next to a corpus of R1 and R2 and an empty scores directory, and returns the
// script, its tools directory and a bench directory with no output yet.
func findingsFixture(t *testing.T) (script, tools, bench string) {
	t.Helper()
	overlayScript(t) // require bash
	body, err := os.ReadFile("../../tools/review-bench/findings.sh")
	if err != nil {
		t.Fatal(err)
	}
	tools = t.TempDir()
	script = filepath.Join(tools, "findings.sh")
	if err := os.WriteFile(script, body, 0700); err != nil {
		t.Fatal(err)
	}
	corpus := `{"real": [{"id": "R1", "defect": {"file": "a.go", "lines": "1"}}, {"id": "R2", "defect": {"file": "b.go", "lines": "2"}}], "mutants": []}`
	if err := os.WriteFile(filepath.Join(tools, "corpus.json"), []byte(corpus), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tools, "scores"), 0700); err != nil {
		t.Fatal(err)
	}
	return script, tools, t.TempDir()
}

// plantSummary writes a summary.json for one runner, run and case: ours under out/ours/run<k>/<id>, gentle under
// out/run<k>/<id>, as run-ours.sh and run-gentle.sh write them.
func plantSummary(t *testing.T, bench, runner string, run int, id, summary string) {
	t.Helper()
	dir := filepath.Join(bench, "out", "run"+strconv.Itoa(run), id)
	if runner == "ours" {
		dir = filepath.Join(bench, "out", "ours", "run"+strconv.Itoa(run), id)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(summary), 0600); err != nil {
		t.Fatal(err)
	}
}

func runFindings(t *testing.T, script, bench string, env []string, args ...string) (string, string, int) {
	t.Helper()
	return runScript(t, script, append([]string{"BENCH=" + bench, "PATH=" + os.Getenv("PATH")}, env...), args...)
}

func parseFindings(t *testing.T, stdout string) []frozenFinding {
	t.Helper()
	var got []frozenFinding
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not a JSON array of findings: %v\n%s", err, stdout)
	}
	return got
}

func findingKey(runner, id string, run int, row, finding string) string {
	sum := sha256.Sum256([]byte(runner + "/" + id + "/" + strconv.Itoa(run) + "/" + row + "/" + finding))
	return hex.EncodeToString(sum[:])[:12]
}

const oursR1Run1 = `{"id": "R1", "lenses": [
  {"lens": "recall", "n": 2, "findings": [
    {"id": "F1", "severity": "WARNING", "location": "a.go:1-3", "claim": "c1", "evidence_class": "inferential",
     "causal_disposition": "introduced", "proof_refs": ["a.go:1"]},
    {"id": "F2", "severity": "SUGGESTION", "location": "a.go:9", "claim": "c2", "evidence_class": "deterministic",
     "causal_disposition": "activated", "proof_refs": []}]},
  {"lens": "hunt", "n": 0, "findings": []}]}`

const oursR1Run2 = `{"id": "R1", "lenses": [
  {"lens": "recall", "n": 0, "findings": []},
  {"lens": "hunt", "n": 1, "findings": [
    {"id": "F1", "severity": "WARNING", "location": "a.go:1", "claim": "c3", "evidence_class": "inferential",
     "causal_disposition": "introduced", "proof_refs": ["a.go:1"]}]}]}`

func TestReviewBenchFindings(t *testing.T) {
	t.Run("ours: every finding of every run and row, in order, with its key", func(t *testing.T) {
		script, _, bench := findingsFixture(t)
		plantSummary(t, bench, "ours", 1, "R1", oursR1Run1)
		plantSummary(t, bench, "ours", 2, "R1", oursR1Run2)
		stdout, stderr, code := runFindings(t, script, bench, []string{"RUNNER=ours", "RUNS=1 2"}, "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		got := parseFindings(t, stdout)
		want := []struct {
			run          int
			row, finding string
			severity     string
			location     string
			claim        string
		}{
			{1, "recall", "F1", "WARNING", "a.go:1-3", "c1"},
			{1, "recall", "F2", "SUGGESTION", "a.go:9", "c2"},
			{2, "hunt", "F1", "WARNING", "a.go:1", "c3"},
		}
		if len(got) != len(want) {
			t.Fatalf("%d entries, want %d\n%s", len(got), len(want), stdout)
		}
		for i, w := range want {
			g := got[i]
			if g.Runner != "ours" || g.ID != "R1" || g.Run != w.run || g.Row != w.row || g.Finding != w.finding ||
				g.Severity != w.severity || g.Location != w.location || g.Claim != w.claim || g.C27Control {
				t.Fatalf("entry %d: %+v", i, g)
			}
			if g.Key != findingKey("ours", "R1", w.run, w.row, w.finding) {
				t.Fatalf("entry %d key %q", i, g.Key)
			}
		}
		if got[0].EvidenceClass == nil || *got[0].EvidenceClass != "inferential" || got[0].CausalDisposition == nil ||
			*got[0].CausalDisposition != "introduced" || len(got[0].ProofRefs) != 1 || got[0].ProofRefs[0] != "a.go:1" {
			t.Fatalf("entry 0 members: %+v", got[0])
		}
	})

	t.Run("gentle: a finding without evidence_class and causal_disposition is written with both null", func(t *testing.T) {
		script, _, bench := findingsFixture(t)
		plantSummary(t, bench, "gentle", 1, "R2", `{"id": "R2", "lenses": [
  {"lens": "review-risk", "n": 1, "findings": [
    {"id": "R1-001", "lens": "risk", "severity": "WARNING", "location": "b.go:2", "claim": "g1", "proof_refs": ["b.go:2"]}]},
  {"lens": "review-reliability", "n": 0, "findings": []}]}`)
		stdout, stderr, code := runFindings(t, script, bench, []string{"RUNNER=gentle", "RUNS=1"}, "R2")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if !strings.Contains(stdout, `"evidence_class": null`) || !strings.Contains(stdout, `"causal_disposition": null`) {
			t.Fatalf("missing members not written as null:\n%s", stdout)
		}
		got := parseFindings(t, stdout)
		if len(got) != 1 || got[0].Runner != "gentle" || got[0].Row != "review-risk" || got[0].Finding != "R1-001" ||
			got[0].Key != findingKey("gentle", "R2", 1, "review-risk", "R1-001") {
			t.Fatalf("entries: %+v", got)
		}
	})

	t.Run("the same case, run, row and finding id under the two runners give two distinct keys", func(t *testing.T) {
		script, _, bench := findingsFixture(t)
		plantSummary(t, bench, "ours", 1, "R1", oursR1Run1)
		plantSummary(t, bench, "gentle", 1, "R1", oursR1Run1)
		ours, _, code := runFindings(t, script, bench, []string{"RUNNER=ours", "RUNS=1"}, "R1")
		if code != 0 {
			t.Fatalf("ours exit %d", code)
		}
		gentle, _, code := runFindings(t, script, bench, []string{"RUNNER=gentle", "RUNS=1"}, "R1")
		if code != 0 {
			t.Fatalf("gentle exit %d", code)
		}
		o, g := parseFindings(t, ours), parseFindings(t, gentle)
		if len(o) != 2 || len(g) != 2 || o[0].Key == g[0].Key || o[1].Key == g[1].Key || g[0].Runner != "gentle" {
			t.Fatalf("ours %+v\ngentle %+v", o, g)
		}
	})

	t.Run("a scored quote marks c27_control on the one finding it names, and an ambiguous quote on none", func(t *testing.T) {
		script, tools, bench := findingsFixture(t)
		// F1 of the hunt row shares its id with the quoted F1 of the recall row, and must stay unmarked.
		plantSummary(t, bench, "ours", 1, "R1", strings.Replace(oursR1Run1, `{"lens": "hunt", "n": 0, "findings": []}`,
			`{"lens": "hunt", "n": 1, "findings": [{"id": "F1", "severity": "WARNING", "location": "a.go:2", "claim": "h", "proof_refs": []}]}`, 1))
		plantSummary(t, bench, "ours", 1, "R2", `{"id": "R2", "lenses": [
  {"lens": "recall", "n": 2, "findings": [
    {"id": "F1", "severity": "WARNING", "location": "b.go:2", "claim": "x", "proof_refs": []},
    {"id": "F2", "severity": "WARNING", "location": "b.go:2", "claim": "y", "proof_refs": []}]},
  {"lens": "hunt", "n": 0, "findings": []}]}`)
		scores := `[{"id": "R1", "run": 1, "score": 1, "quote": "recall WARNING a.go:1-3: c1 ..."},
 {"id": "R1", "run": 2, "score": 1, "quote": "recall SUGGESTION a.go:9: other run"},
 {"id": "R2", "run": 1, "score": 0.5, "quote": "recall WARNING b.go:2: x"},
 {"id": "R2", "run": 1, "score": 1, "quote": "hunt WARNING b.go:2: no such finding"},
 {"id": "R1", "run": 1, "score": 0, "quote": "recall SUGGESTION a.go:9: c2"}]`
		if err := os.WriteFile(filepath.Join(tools, "scores", "ours-c27.json"), []byte(scores), 0600); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, code := runFindings(t, script, bench, []string{"RUNNER=ours", "RUNS=1", "SCORES=" + filepath.Join(tools, "scores", "ours-c27.json")}, "R1", "R2")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if stderr != "quote of R2 run 1 names 2 findings; none is marked\nquote of R2 run 1 names no finding; none is marked\n" {
			t.Fatalf("stderr %q", stderr)
		}
		var marked []string
		for _, f := range parseFindings(t, stdout) {
			if f.C27Control {
				marked = append(marked, f.ID+"/"+f.Row+"/"+f.Finding)
			}
		}
		if strings.Join(marked, ",") != "R1/recall/F1" {
			t.Fatalf("marked %v\n%s", marked, stdout)
		}
	})

	t.Run("with SCORES unset no finding is marked, even beside a scores file", func(t *testing.T) {
		script, tools, bench := findingsFixture(t)
		plantSummary(t, bench, "ours", 1, "R1", oursR1Run1)
		scores := `[{"id": "R1", "run": 1, "score": 1, "quote": "recall WARNING a.go:1-3: c1"}]`
		if err := os.WriteFile(filepath.Join(tools, "scores", "ours-c27.json"), []byte(scores), 0600); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, code := runFindings(t, script, bench, []string{"RUNNER=ours", "RUNS=1"}, "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		for _, f := range parseFindings(t, stdout) {
			if f.C27Control {
				t.Fatalf("marked without SCORES: %+v", f)
			}
		}
	})

	t.Run("an unknown runner, an unknown id or a missing summary exits 2 and prints no findings", func(t *testing.T) {
		script, _, bench := findingsFixture(t)
		plantSummary(t, bench, "ours", 1, "R1", oursR1Run1)
		cases := []struct {
			env    []string
			args   []string
			stderr string
		}{
			{[]string{"RUNNER=both", "RUNS=1"}, []string{"R1"}, "RUNNER must be ours or gentle, not both\n"},
			{[]string{"RUNS=1"}, []string{"R1"}, "RUNNER must be ours or gentle, not \n"},
			{[]string{"RUNNER=ours", "RUNS=1"}, nil, "usage: RUNNER=ours|gentle findings.sh <id>...\n"},
			{[]string{"RUNNER=ours", "RUNS=1"}, []string{"R1", "R9"}, "unknown case R9\n"},
			{[]string{"RUNNER=ours", "RUNS=1 2"}, []string{"R1"}, "R1 run 2: no summary at " + filepath.Join(bench, "out", "ours", "run2", "R1", "summary.json") + "\n"},
			{[]string{"RUNNER=ours", "RUNS=0"}, []string{"R1"}, "RUNS must hold positive integers, not 0\n"},
			{[]string{"RUNNER=ours", "RUNS= "}, []string{"R1"}, "RUNS names no run\n"},
			{[]string{"RUNNER=ours", "RUNS=1 1"}, []string{"R1"}, "RUNS names run 1 twice\n"},
			{[]string{"RUNNER=ours", "RUNS=1"}, []string{"R1", "R1"}, "case R1 is named twice\n"},
			{[]string{"RUNNER=ours", "RUNS=1", "SCORES=" + filepath.Join(bench, "none.json")}, []string{"R1"}, "no scores file at " + filepath.Join(bench, "none.json") + "\n"},
		}
		for _, c := range cases {
			stdout, stderr, code := runFindings(t, script, bench, c.env, c.args...)
			if code != 2 || stdout != "" || stderr != c.stderr {
				t.Fatalf("%v %v: exit %d stdout %q stderr %q", c.env, c.args, code, stdout, stderr)
			}
		}
	})
}

// Planted findings of both runners for precision.sh (phase9-c28-review-precision S6): ours o1-o5 over R1 and R2 and
// rows recall and hunt, gentle g1-g2 over rows review-risk and review-reliability.
const precisionOurs = `[
  {"key": "o1", "runner": "ours", "id": "R1", "row": "recall", "severity": "WARNING"},
  {"key": "o2", "runner": "ours", "id": "R1", "row": "recall", "severity": "SUGGESTION"},
  {"key": "o3", "runner": "ours", "id": "R1", "row": "hunt", "severity": "WARNING"},
  {"key": "o4", "runner": "ours", "id": "R2", "row": "hunt", "severity": "WARNING"},
  {"key": "o5", "runner": "ours", "id": "R2", "row": "recall", "severity": "WARNING"}]`

const precisionGentle = `[
  {"key": "g1", "runner": "gentle", "id": "R1", "row": "review-risk", "severity": "WARNING"},
  {"key": "g2", "runner": "gentle", "id": "R2", "row": "review-reliability", "severity": "SUGGESTION"}]`

// precisionLabels leaves o5 unlabeled (not-judged); o2 is real but states no failure.
const precisionLabels = `{"labels": [
  {"key": "o1", "label": "real", "failure_stated": true, "citation": "head/a.go:1", "reason": "r"},
  {"key": "o2", "label": "real", "failure_stated": false, "citation": "head/a.go:2", "reason": "r"},
  {"key": "o3", "label": "false", "failure_stated": true, "citation": "head/a.go:3", "reason": "r"},
  {"key": "o4", "label": "unsure", "failure_stated": true, "citation": "", "reason": "r"},
  {"key": "g1", "label": "real", "failure_stated": true, "citation": "head/b.go:1", "reason": "r"},
  {"key": "g2", "label": "false", "failure_stated": true, "citation": "head/b.go:2", "reason": "r"}]}`

// runPrecision writes the findings and labels into a temporary directory, runs precision.sh on them (plus a repro
// file when repro is not empty) and returns its stdout, stderr and exit code.
func runPrecision(t *testing.T, ours, gentle, labels, repro string) (string, string, int) {
	t.Helper()
	overlayScript(t) // require bash
	script, err := filepath.Abs("../../tools/review-bench/precision.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	args := []string{}
	for _, f := range []struct{ name, body string }{{"ours.json", ours}, {"gentle.json", gentle}, {"labels.json", labels}, {"repro.json", repro}} {
		if f.body == "" {
			continue
		}
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte(f.body), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	return runScript(t, script, []string{"PATH=" + os.Getenv("PATH")}, args...)
}

// precisionTable parses the output of precision.sh into "<group> <metric>" -> [ours, gentle].
func precisionTable(t *testing.T, stdout string) map[string][2]string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) == 0 || strings.Join(strings.Fields(lines[0]), " ") != "group metric ours gentle" {
		t.Fatalf("no header line:\n%s", stdout)
	}
	table := map[string][2]string{}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) != 4 {
			t.Fatalf("line %q has %d fields, not 4", line, len(f))
		}
		table[f[0]+" "+f[1]] = [2]string{f[2], f[3]}
	}
	return table
}

func assertPrecision(t *testing.T, table map[string][2]string, want map[string][2]string) {
	t.Helper()
	for k, w := range want {
		got, ok := table[k]
		if !ok {
			t.Errorf("%s: missing", k)
		} else if got != w {
			t.Errorf("%s: got ours %s gentle %s, want ours %s gentle %s", k, got[0], got[1], w[0], w[1])
		}
	}
}

func TestReviewBenchPrecision(t *testing.T) {
	t.Run("known counts give the precision and both bounds per runner, overall and per severity, row and case", func(t *testing.T) {
		stdout, stderr, code := runPrecision(t, precisionOurs, precisionGentle, precisionLabels, "")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		table := precisionTable(t, stdout)
		if first := strings.Fields(strings.Split(stdout, "\n")[1]); first[0] != "headline" || first[1] != "precision" {
			t.Errorf("the first row is %v, not the headline precision", first)
		}
		assertPrecision(t, table, map[string][2]string{
			// headline: o2 (real, no failure stated) counts false; unsure left out, then counted false, then real.
			"headline precision":             {"0.333", "0.5"},
			"headline unsure":                {"1", "0"},
			"headline low":                   {"0.25", "0.5"},
			"headline high":                  {"0.5", "0.5"},
			"overall real":                   {"2", "1"},
			"overall false":                  {"1", "1"},
			"overall unsure":                 {"1", "0"},
			"overall not-judged":             {"1", "0"},
			"overall precision":              {"0.667", "0.5"},
			"overall low":                    {"0.5", "0.5"},
			"overall high":                   {"0.75", "0.5"},
			"failure_stated=true real":       {"1", "1"},
			"failure_stated=true precision":  {"0.5", "0.5"},
			"failure_stated=false real":      {"1", "-"},
			"failure_stated=false precision": {"1", "-"},
			"severity=WARNING precision":     {"0.5", "1"},
			"severity=WARNING low":           {"0.333", "1"},
			"severity=WARNING high":          {"0.667", "1"},
			"severity=SUGGESTION precision":  {"1", "0"},
			"row=recall real":                {"2", "-"},
			"row=recall not-judged":          {"1", "-"},
			"row=hunt precision":             {"0", "-"},
			"row=review-risk precision":      {"-", "1"},
			"row=review-reliability false":   {"-", "1"},
			"case=R1 precision":              {"0.667", "1"},
			"case=R2 unsure":                 {"1", "0"},
		})
	})

	t.Run("a label joins its runner by key only", func(t *testing.T) {
		labels := strings.Replace(precisionLabels, `{"key": "o1",`, `{"key": "o1", "runner": "gentle", "id": "R2",`, 1)
		stdout, _, code := runPrecision(t, precisionOurs, precisionGentle, labels, "")
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertPrecision(t, precisionTable(t, stdout), map[string][2]string{
			"overall real": {"2", "1"},
			"case=R1 real": {"2", "1"},
			"case=R2 real": {"0", "0"},
		})
	})

	t.Run("not-judged stays out of judged", func(t *testing.T) {
		stdout, _, code := runPrecision(t, precisionOurs, precisionGentle, precisionLabels, "")
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		// row=recall holds o1 and o2 (real) and o5 (not-judged): both bounds are 2/2, not 2/3.
		assertPrecision(t, precisionTable(t, stdout), map[string][2]string{
			"row=recall low":  {"1", "-"},
			"row=recall high": {"1", "-"},
		})
	})

	t.Run("no real + false prints the precision as null, not a division error", func(t *testing.T) {
		stdout, stderr, code := runPrecision(t, precisionOurs, precisionGentle, precisionLabels, "")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		// case=R2 of ours holds o4 (unsure) and o5 (not-judged).
		assertPrecision(t, precisionTable(t, stdout), map[string][2]string{
			"case=R2 precision": {"null", "0"},
			"case=R2 low":       {"0", "0"},
			"case=R2 high":      {"1", "0"},
		})
	})

	t.Run("a repro label replaces the reading label of its key", func(t *testing.T) {
		repro := `{"labels": [{"key": "o3", "label": "real", "failure_stated": true, "citation": "head/a.go:3", "reason": "reproduced on head"}]}`
		stdout, _, code := runPrecision(t, precisionOurs, precisionGentle, precisionLabels, repro)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertPrecision(t, precisionTable(t, stdout), map[string][2]string{
			"overall real":  {"3", "1"},
			"overall false": {"0", "1"},
		})
	})

	t.Run("a label whose key is in neither findings file exits 2", func(t *testing.T) {
		labels := strings.Replace(precisionLabels, `"labels": [`, `"labels": [{"key": "zz", "label": "real", "failure_stated": true, "citation": "head/c.go:1", "reason": "r"},`, 1)
		stdout, stderr, code := runPrecision(t, precisionOurs, precisionGentle, labels, "")
		if code != 2 || stdout != "" || stderr != "label key in neither findings file: zz\n" {
			t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
		}
	})
}
