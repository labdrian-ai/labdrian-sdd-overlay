package shelltest

import (
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

func runOurs(t *testing.T, script string, env []string, args ...string) (string, string, int) {
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
		stdout, stderr, code := runOurs(t, script, env, "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		stubs := strings.TrimPrefix(env[2], "STUB_DIR=")
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
		wantBrief := "Review the committed candidate below as the reviewer role.\nbase: " + b + "\nhead: " + head +
			"\ndiff_sha256: d1ff\nworktree: /wt/review-" + head[:12] + "\nLens row hunt: Hunt four classes.\n" +
			"No tool is available to you: the frozen context below is the whole of what you can inspect; set inspection.status to completed when it covers every changed path.\n"
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
		_, stderr, code := runOurs(t, script, append(env, "ROWS=default"), "R1")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if got := jqField(t, bench, "[.lenses[].lens] | join(\",\")"); got != `"default"` {
			t.Fatalf("lenses %s", got)
		}
		stubs := strings.TrimPrefix(env[2], "STUB_DIR=")
		if err := os.Remove(filepath.Join(stubs, "calls.log")); err != nil {
			t.Fatal(err)
		}
		_, stderr, code = runOurs(t, script, append(env, "ROWS=default,nope"), "R1")
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
		if _, stderr, code := runOurs(t, script, append(env, "ROWS=default,readability"), "R1"); code != 2 || strings.Contains(stderr, "unknown row") {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if _, stderr, code := runOurs(t, script, append(env, "ROWS=default,nope"), "R1"); code != 2 || stderr != "unknown row nope\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
	})

	t.Run("a rows file that cannot be read is named, not taken for an unknown row", func(t *testing.T) {
		script, _, _, env := oursFixture(t)
		missing := filepath.Join(t.TempDir(), "rows.json")
		_, stderr, code := runOurs(t, script, append(env, "ROWS_FILE="+missing), "R1")
		if code != 2 || stderr != "cannot read the rows of "+missing+"\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
	})

	t.Run("a failed launch ends the case non-zero with a summary that says so", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		_, stderr, code := runOurs(t, script, append(env, "STUB_FAIL_ROW=default"), "R1")
		if code != 4 || stderr != "case R1 ended without a review result: launch failed: default\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if got := jqField(t, bench, "[.state, (.lenses | length)]"); got != `["launch failed: default",0]` {
			t.Fatalf("summary %s", got)
		}
		stubs := strings.TrimPrefix(env[2], "STUB_DIR=")
		if calls := readFile(t, filepath.Join(stubs, "calls.log")); strings.Contains(calls, "--rows hunt") {
			t.Fatalf("the next row ran after a failure:\n%s", calls)
		}
	})

	t.Run("a failed sweep ends the case before any launch", func(t *testing.T) {
		script, bench, _, env := oursFixture(t)
		_, stderr, code := runOurs(t, script, append(env, "STUB_SWEEP_FAIL=1"), "R1")
		if code != 4 || stderr != "case R1 ended without a review result: sweep failed\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if got := jqField(t, bench, "[.state, .analyzers, .runs]"); got != `["sweep failed",[],[]]` {
			t.Fatalf("summary %s", got)
		}
	})

	t.Run("an unknown id and a missing id are refused", func(t *testing.T) {
		script, _, _, env := oursFixture(t)
		if _, stderr, code := runOurs(t, script, env, "R9"); code != 2 || stderr != "unknown case R9\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
		if _, stderr, code := runOurs(t, script, env); code != 2 || stderr != "usage: run-ours.sh <id>\n" {
			t.Fatalf("exit %d\n%s", code, stderr)
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
