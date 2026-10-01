package main

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
)

// ulidRe matches the Crockford base32 uppercase ULID format (R-014).
var ulidRe = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

// prespecFixedTime is the instant the injected clock reports.
var prespecFixedTime = time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

// zeroEntropy is an io.Reader that only returns zero bytes, which gives a brief
// a predictable ID.
type zeroEntropy struct{}

func (zeroEntropy) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// fixedPrespecEnv is the environment of a test run: a clock that stands still and
// an entropy source that never varies, so a brief is the same every time.
func fixedPrespecEnv() prespecEnv {
	return prespecEnv{
		now:     func() time.Time { return prespecFixedTime },
		entropy: zeroEntropy{},
	}
}

// capturePrespec runs the prespec subcommand core with the given verb and stdin
// JSON under env, returning stdout and stderr as strings plus the exit code.
func capturePrespec(env prespecEnv, verb, stdinJSON string) (stdout, stderr string, exitCode int) {
	var outBuf, errBuf bytes.Buffer
	code := 0
	runPrespecCore(verb, strings.NewReader(stdinJSON), &outBuf, &errBuf, func(c int) { code = c }, env)
	return outBuf.String(), errBuf.String(), code
}

// marshalCells converts []prespec.Cell into the JSON shape the verbs read.
func marshalCells(cells []prespec.Cell) []map[string]interface{} {
	result := make([]map[string]interface{}, len(cells))
	for i, c := range cells {
		state := "missing"
		switch c.State {
		case prespec.Partial:
			state = "partial"
		case prespec.Clear:
			state = "clear"
		}
		result[i] = map[string]interface{}{
			"key":         c.Key,
			"impact":      c.Impact,
			"uncertainty": c.Uncertainty,
			"state":       state,
		}
	}
	return result
}

// briefInput is the stdin of a brief that passes the readiness gate.
func briefInput(t *testing.T) string {
	t.Helper()
	cells := prespec.DefaultCells()
	for i := 0; i < 6; i++ {
		cells[i].State = prespec.Clear
	}
	inJSON, err := json.Marshal(map[string]interface{}{
		"project": "my-project",
		"job":     "Ship faster without breaking things",
		"sections": [6]string{
			"Problem statement here.",
			"Outcome gap here.",
			"Constraints here.",
			"Hypothesis here.",
			"Context here.",
			"Success signal here.",
		},
		"transcript": "Q: What? A: This.",
		"cells":      marshalCells(cells),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(inJSON)
}

// TestPrespecRankVerb verifies the rank verb returns ranked uncovered cells.
func TestPrespecRankVerb(t *testing.T) {
	cells := prespec.DefaultCells() // every cell Missing
	inJSON, _ := json.Marshal(map[string]interface{}{"cells": marshalCells(cells)})

	stdout, stderr, code := capturePrespec(fixedPrespecEnv(), "rank", string(inJSON))
	if code != 0 {
		t.Fatalf("rank: exit %d; stderr=%q", code, stderr)
	}

	var out struct {
		Ranked []struct {
			Key string `json:"key"`
		} `json:"ranked"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("rank: invalid JSON output: %v\nout=%q", err, stdout)
	}
	if len(out.Ranked) == 0 {
		t.Fatal("rank: ranked list is empty; expected 10 cells")
	}
	// First ranked cell must be jtbd-job (highest I×U = 25).
	if out.Ranked[0].Key != "jtbd-job" {
		t.Errorf("rank: first cell = %q; want jtbd-job", out.Ranked[0].Key)
	}
}

// TestPrespecRankVerbMalformedJSON verifies rank exits 1 on bad input.
func TestPrespecRankVerbMalformedJSON(t *testing.T) {
	_, stderr, code := capturePrespec(fixedPrespecEnv(), "rank", "not-json")
	if code != 1 {
		t.Errorf("rank malformed JSON: want exit 1; got %d", code)
	}
	if stderr == "" {
		t.Error("rank malformed JSON: expected stderr diagnostic")
	}
}

// TestPrespecLintVerb verifies the lint verb accepts a clean question.
func TestPrespecLintVerb(t *testing.T) {
	input := `{"question": "What is the main obstacle to shipping faster?"}`
	stdout, stderr, code := capturePrespec(fixedPrespecEnv(), "lint", input)
	if code != 0 {
		t.Fatalf("lint: exit %d; stderr=%q", code, stderr)
	}
	var out struct {
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("lint: invalid JSON output: %v\nout=%q", err, stdout)
	}
	if !out.Accepted {
		t.Errorf("lint: expected accepted=true; got reason=%q", out.Reason)
	}
}

// TestPrespecLintVerbRejects verifies the lint verb rejects a leading question.
func TestPrespecLintVerbRejects(t *testing.T) {
	input := `{"question": "Would you like a dashboard?"}`
	stdout, _, code := capturePrespec(fixedPrespecEnv(), "lint", input)
	if code != 0 {
		t.Fatalf("lint: unexpected exit %d", code)
	}
	var out struct {
		Accepted bool   `json:"accepted"`
		Rule     string `json:"rule"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("lint: invalid JSON output: %v\nout=%q", err, stdout)
	}
	if out.Accepted {
		t.Error("lint: expected accepted=false for leading question")
	}
	if out.Rule == "" {
		t.Error("lint: expected non-empty rule on rejection")
	}
}

// TestPrespecReadinessVerb verifies the readiness verb computes and returns the score.
func TestPrespecReadinessVerb(t *testing.T) {
	cells := prespec.DefaultCells()
	for i := 0; i < 6; i++ { // mark the first 6 Clear
		cells[i].State = prespec.Clear
	}
	inJSON, _ := json.Marshal(map[string]interface{}{"cells": marshalCells(cells)})

	stdout, stderr, code := capturePrespec(fixedPrespecEnv(), "readiness", string(inJSON))
	if code != 0 {
		t.Fatalf("readiness: exit %d; stderr=%q", code, stderr)
	}

	var out struct {
		Value  float64 `json:"value"`
		Passes bool    `json:"passes"`
		Clear  int     `json:"clear"`
		Total  int     `json:"total"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("readiness: invalid JSON output: %v\nout=%q", err, stdout)
	}
	if out.Value != 0.6 {
		t.Errorf("readiness: value = %.2f; want 0.60", out.Value)
	}
	if !out.Passes {
		t.Error("readiness: passes should be true at 0.6")
	}
	if out.Clear != 6 {
		t.Errorf("readiness: clear = %d; want 6", out.Clear)
	}
}

// TestPrespecBriefVerb verifies the brief verb renders markdown and returns the
// path template.
func TestPrespecBriefVerb(t *testing.T) {
	stdout, stderr, code := capturePrespec(fixedPrespecEnv(), "brief", briefInput(t))
	if code != 0 {
		t.Fatalf("brief: exit %d; stderr=%q", code, stderr)
	}

	var out struct {
		ID       string `json:"id"`
		Markdown string `json:"markdown"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("brief: invalid JSON output: %v\nout=%q", err, stdout)
	}
	if !ulidRe.MatchString(out.ID) {
		t.Errorf("brief: id = %q; does not match ULID regex", out.ID)
	}
	if !strings.Contains(out.Markdown, "# Discovery Brief") {
		t.Errorf("brief: markdown missing header; got %q", out.Markdown[:min(80, len(out.Markdown))])
	}
	wantPathPrefix := "project/my-project/prespec/"
	if !strings.HasPrefix(out.Path, wantPathPrefix) {
		t.Errorf("brief: path = %q; want prefix %q", out.Path, wantPathPrefix)
	}
}

// The ID and the creation time of a brief come from the injected clock and entropy
// source, not from the wall clock and crypto/rand: under a fixed environment the
// brief is exactly the same on every run, and both read the one instant.
func TestPrespecBriefTakesItsIDAndTimeFromTheInjectedEnvironment(t *testing.T) {
	stdout, stderr, code := capturePrespec(fixedPrespecEnv(), "brief", briefInput(t))
	if code != 0 {
		t.Fatalf("brief: exit %d; stderr=%q", code, stderr)
	}
	var out struct {
		ID       string `json:"id"`
		Markdown string `json:"markdown"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("brief: invalid JSON output: %v\nout=%q", err, stdout)
	}

	wantID := prespec.NewIDFrom(prespecFixedTime, zeroEntropy{})
	if out.ID != wantID {
		t.Errorf("id = %q, want %q (clock and entropy as injected)", out.ID, wantID)
	}
	if want := "project/my-project/prespec/" + wantID; out.Path != want {
		t.Errorf("path = %q, want %q", out.Path, want)
	}
	if want := "**Created**: 2024-01-15T10:30:00Z"; !strings.Contains(out.Markdown, want) {
		t.Errorf("markdown does not carry %q (the injected clock's time)", want)
	}

	again, _, _ := capturePrespec(fixedPrespecEnv(), "brief", briefInput(t))
	if again != stdout {
		t.Errorf("a second run under the same environment differs:\n%s\n%s", stdout, again)
	}
}

// Production wiring: the real clock and the real entropy source give a valid ID
// and never the same one twice.
func TestRealPrespecEnvIssuesValidDistinctIDs(t *testing.T) {
	env := realPrespecEnv()
	var ids [2]string
	for i := range ids {
		stdout, stderr, code := capturePrespec(env, "brief", briefInput(t))
		if code != 0 {
			t.Fatalf("brief: exit %d; stderr=%q", code, stderr)
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("brief: invalid JSON output: %v\nout=%q", err, stdout)
		}
		if !ulidRe.MatchString(out.ID) {
			t.Fatalf("brief: id = %q; does not match ULID regex", out.ID)
		}
		ids[i] = out.ID
	}
	if ids[0] == ids[1] {
		t.Errorf("two briefs got the same id %q", ids[0])
	}
}

// TestPrespecBriefVerbFailsGate verifies brief exits 1 when readiness is below the gate.
func TestPrespecBriefVerbFailsGate(t *testing.T) {
	cells := prespec.DefaultCells() // all Missing → score 0.0
	inJSON, _ := json.Marshal(map[string]interface{}{
		"project":    "my-project",
		"job":        "Do something",
		"sections":   [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		"transcript": "Q: What?",
		"cells":      marshalCells(cells),
	})
	stdout, stderr, code := capturePrespec(fixedPrespecEnv(), "brief", string(inJSON))
	if code != 1 {
		t.Errorf("brief below gate: want exit 1; got %d", code)
	}
	if stderr == "" {
		t.Error("brief below gate: expected stderr error message")
	}
	if stdout != "" {
		t.Errorf("brief below gate: stdout = %q, want nothing", stdout)
	}
}

// TestPrespecUnknownVerb verifies an unknown verb exits 1.
func TestPrespecUnknownVerb(t *testing.T) {
	_, stderr, code := capturePrespec(fixedPrespecEnv(), "unknown-verb", "{}")
	if code != 1 {
		t.Errorf("unknown verb: want exit 1; got %d", code)
	}
	if !strings.Contains(stderr, "unknown verb") {
		t.Errorf("unknown verb: stderr = %q, want a diagnostic naming it", stderr)
	}
}

// A missing verb is refused before stdin is read: failingReader fails every read,
// so a core that read stdin first would report the read error instead.
func TestPrespecMissingVerbExitsOneWithoutReadingStdin(t *testing.T) {
	var errBuf bytes.Buffer
	code := -1
	runPrespecCore("", failingReader{}, io.Discard, &errBuf, func(c int) { code = c }, fixedPrespecEnv())
	if code != 1 || !strings.Contains(errBuf.String(), "requires a verb") {
		t.Errorf("missing verb: exit %d, stderr %q; want exit 1 naming the verb requirement", code, errBuf.String())
	}
}
