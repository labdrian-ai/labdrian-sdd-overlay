package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/telemetry"
)

var update = flag.Bool("update", false, "update human golden under testdata")

const data = "../../testdata"
const stats = "claude: files=2 lines=5 calls=2 skipped=2 duplicates=1 unreadable_files=0\npi: files=1 lines=2 calls=2 skipped=0 duplicates=0 unreadable_files=0\n"

func invoke(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := run(args, &out, &err, t.TempDir())
	return code, out.String(), err.String()
}
func fixtures(t *testing.T, extra ...string) string {
	t.Helper()
	args := append([]string{"report", "--claude-root", data + "/claude", "--pi-root", data + "/pi"}, extra...)
	code, out, err := invoke(t, args...)
	if code != 0 || err != stats {
		t.Fatalf("code=%d stderr=%q, want 0 / %q", code, err, stats)
	}
	return out
}
func TestHumanGolden(t *testing.T) {
	got := fixtures(t)
	path := data + "/golden/report.txt"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("human output differs from %s:\n%s", path, got)
	}
}
func TestJSONGolden(t *testing.T) {
	var a telemetry.Aggregator
	if _, err := telemetry.ReadClaude(data+"/claude", a.Add); err != nil {
		t.Fatal(err)
	}
	if _, err := telemetry.ReadPi(data+"/pi", a.Add); err != nil {
		t.Fatal(err)
	}
	want, err := a.Report().JSON()
	if err != nil {
		t.Fatal(err)
	}
	if got := fixtures(t, "--json"); got != string(want) {
		t.Fatalf("JSON differs from Report.JSON(): %s", got)
	}
}
func TestDeterministic(t *testing.T) {
	for _, args := range [][]string{nil, {"--json"}} {
		if a, b := fixtures(t, args...), fixtures(t, args...); a != b {
			t.Fatal("output changed")
		}
	}
}
func TestStatsAndPrivacy(t *testing.T) {
	for _, args := range [][]string{nil, {"--json"}} {
		out := fixtures(t, args...)
		for _, forbidden := range []string{"files=", "lines=", "skipped=", "PRIVATE_SENTINEL", "synthetic", "malformed", "session"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("stdout contains %q", forbidden)
			}
		}
	}
}
func TestMissingRoots(t *testing.T) {
	home := t.TempDir()
	var out, err bytes.Buffer
	code := run([]string{"report", "--claude-root", data + "/claude"}, &out, &err, home)
	note := fmt.Sprintf("pi: not found: %s (skipped default root)\n", filepath.Join(home, ".pi/agent/sessions"))
	if code != 0 || out.Len() == 0 || err.String() != strings.SplitAfter(stats, "\n")[0]+note {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	out.Reset()
	err.Reset()
	code = run([]string{"report", "--pi-root", data + "/pi"}, &out, &err, home)
	note = fmt.Sprintf("claude: not found: %s (skipped default root)\n", filepath.Join(home, ".claude/projects"))
	if code != 0 || out.Len() == 0 || err.String() != note+strings.SplitAfter(stats, "\n")[1] {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	missing := filepath.Join(home, "missing")
	for _, source := range []string{"claude", "pi"} {
		out.Reset()
		err.Reset()
		code = run([]string{"report", "--" + source + "-root", missing, "--" + map[string]string{"claude": "pi", "pi": "claude"}[source] + "-root", data + "/" + map[string]string{"claude": "pi", "pi": "claude"}[source]}, &out, &err, home)
		if code != 1 || out.Len() != 0 || !strings.Contains(err.String(), source+": ") || !strings.Contains(err.String(), missing) || strings.Contains(err.String(), "skipped default") {
			t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
		}
	}
}
func TestNoReadableSource(t *testing.T) {
	for _, empty := range []bool{false, true} {
		home := t.TempDir()
		args := []string{"report"}
		want := fmt.Sprintf("claude: not found: %s (skipped default root)\npi: not found: %s (skipped default root)\n", filepath.Join(home, ".claude/projects"), filepath.Join(home, ".pi/agent/sessions"))
		if empty {
			args = append(args, "--claude-root", home, "--pi-root", home)
			want = "claude: files=0 lines=0 calls=0 skipped=0 duplicates=0 unreadable_files=0\npi: files=0 lines=0 calls=0 skipped=0 duplicates=0 unreadable_files=0\n"
		}
		var out, err bytes.Buffer
		code := run(args, &out, &err, home)
		if code != 1 || out.Len() != 0 || err.String() != want+"no readable source\n" {
			t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
		}
	}
}
func TestUsageErrors(t *testing.T) {
	before := hashes(t)
	for _, args := range [][]string{nil, {"unknown"}, {"report", "--bad"}, {"report", "--top", "bad"}, {"report", "--top", "0"}, {"report", "--top", "-1"}, {"report", "extra"}, {"report", "--claude-root"}} {
		code, out, err := invoke(t, args...)
		if code != 2 || out != "" || !strings.Contains(err, "Usage: telemetry report") {
			t.Fatalf("args=%v code=%d out=%q err=%q", args, code, out, err)
		}
	}
	if !reflect.DeepEqual(before, hashes(t)) {
		t.Fatal("rejected arguments modified fixtures")
	}
}
func TestTop(t *testing.T) {
	out := fixtures(t, "--top", "1")
	for _, want := range []string{"By model\n", "… and 3 more", "… and 1 more", "2026-10-22", "main", "sub", "claude", "pi"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"2026-10-20", "2026-10-21"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("not last day: %s", out)
		}
	}
	// Context ties must break by key; only a-haiku survives top 1.
	models := strings.Split(strings.Split(out, "By model\n")[1], "By agent type\n")[0]
	if !strings.Contains(models, "a-haiku") || strings.Contains(models, "b-gpt") {
		t.Fatalf("model ordering: %s", models)
	}
}
func TestCost(t *testing.T) {
	if out := fixtures(t); !strings.Contains(out, "$3.75 (2 calls)") || !strings.Contains(out, "n/a") {
		t.Fatal(out)
	}
	code, out, err := invoke(t, "report", "--claude-root", data+"/claude")
	if code != 0 || !strings.Contains(out, "n/a") || strings.Contains(out, "$") || !strings.Contains(err, "skipped default root") {
		t.Fatalf("%d %q %q", code, out, err)
	}
}
func TestFormatting(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{{999, "999"}, {1000, "1k"}, {1234, "1.23k"}, {45600000, "45.6M"}, {1230000000, "1.23B"}} {
		t.Run(fmt.Sprint(tc.n), func(t *testing.T) {
			if got := compact(tc.n); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	for _, percent := range []bool{false, true} {
		if got := ratio(nil, percent); got != "-" {
			t.Fatalf("nil ratio=%q", got)
		}
	}
}
func hashes(t *testing.T) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	for _, root := range []string{data + "/claude", data + "/pi"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(path)
			result[path] = sha256.Sum256(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}
func TestPricesHumanColumn(t *testing.T) {
	before := hashes(t)
	code, out, err := invoke(t, "report", "--claude-root", data+"/claude", "--pi-root", data+"/pi", "--prices", data+"/prices.example.json")
	if code != 0 || err != stats+"prices: models=4 estimated_calls=2 unpriced_calls=0\n" {
		t.Fatalf("code=%d stderr=%q", code, err)
	}
	summary := strings.Split(out, "By role\n")[0]
	if !strings.Contains(summary, "Cost") || !strings.Contains(summary, "Est. cost") || !strings.Contains(summary, "$3.75 (2 calls)") || !strings.Contains(summary, "~$6.40 (2 calls)") {
		t.Fatalf("summary: %s", summary)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "b-gpt ") && !strings.HasSuffix(strings.TrimSpace(line), "-") {
			t.Fatalf("unestimated row missing dash: %s", line)
		}
	}
	for _, forbidden := range []string{"PRIVATE_SENTINEL", "synthetic", "malformed", "session"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("content leak: %q", forbidden)
		}
	}
	if !reflect.DeepEqual(before, hashes(t)) {
		t.Fatal("pricing modified logs")
	}
}

func TestPricesJSONRecordedFixture(t *testing.T) {
	code, out, stderr := invoke(t, "report", "--claude-root", data+"/claude", "--pi-root", data+"/pi", "--prices", data+"/prices.example.json", "--json")
	var r telemetry.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if code != 0 || stderr != stats+"prices: models=4 estimated_calls=2 unpriced_calls=0\n" || r.Summary.EstimatedCalls != 2 || r.Summary.EstimatedCostUSD == nil || *r.Summary.EstimatedCostUSD != 6.4 || r.Summary.CostUSD == nil || *r.Summary.CostUSD != 3.75 || r.Summary.CostCalls != 2 {
		t.Fatalf("code=%d stderr=%q summary=%+v", code, stderr, r.Summary)
	}
	for _, g := range r.BySource {
		if g.Key == "pi" && (g.EstimatedCalls != 0 || g.EstimatedCostUSD != nil || g.CostUSD == nil || *g.CostUSD != 3.75) {
			t.Fatalf("Pi re-estimated: %+v", g)
		}
	}
}

func TestPricesCoverageStats(t *testing.T) {
	for _, tc := range []struct {
		model               string
		estimated, unpriced int
	}{
		{"z-sonnet", 1, 1}, {"b-gpt", 0, 2},
	} {
		t.Run(tc.model, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "prices.json")
			table := fmt.Sprintf(`{"unit":"usd_per_million_tokens","models":{%q:{"input":1000,"cache_read":2000,"cache_write":3000,"output":4000}}}`, tc.model)
			if err := os.WriteFile(path, []byte(table), 0444); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := invoke(t, "report", "--claude-root", data+"/claude", "--pi-root", data+"/pi", "--prices", path)
			wantStats := stats + fmt.Sprintf("prices: models=1 estimated_calls=%d unpriced_calls=%d\n", tc.estimated, tc.unpriced)
			if code != 0 || stderr != wantStats {
				t.Fatalf("code=%d stderr=%q want=%q", code, stderr, wantStats)
			}
			if tc.estimated == 0 && out != fixtures(t) {
				t.Fatal("price table without matches changed human output")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != table {
				t.Fatal("price table modified")
			}
		})
	}
}

func TestPricesFileErrors(t *testing.T) {
	before := hashes(t)
	path := filepath.Join(t.TempDir(), "prices.json")
	for _, invalid := range []bool{false, true} {
		if invalid {
			if err := os.WriteFile(path, []byte(`{"unit":"wrong","models":{}}`), 0444); err != nil {
				t.Fatal(err)
			}
		}
		code, out, stderr := invoke(t, "report", "--claude-root", data+"/claude", "--pi-root", data+"/pi", "--prices", path)
		if code != 1 || out != "" || !strings.HasPrefix(stderr, "prices: ") || !strings.HasSuffix(stderr, "\n") || strings.Contains(stderr, "files=") {
			t.Fatalf("code=%d out=%q stderr=%q", code, out, stderr)
		}
	}
	if !reflect.DeepEqual(before, hashes(t)) {
		t.Fatal("rejected price table modified fixtures")
	}
}

func TestReadOnly(t *testing.T) {
	before := hashes(t)
	fixtures(t)
	fixtures(t, "--json")
	if !reflect.DeepEqual(before, hashes(t)) {
		t.Fatal("fixture files changed")
	}
}
