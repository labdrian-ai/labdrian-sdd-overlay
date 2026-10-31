package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
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
   "defect": {"file": "engine/z/z.go", "lines": "5-6", "text": "no summary case"}}
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
