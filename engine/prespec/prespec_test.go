package prespec

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// capturePrespec runs PrespecCore with the given verb and stdin JSON,
// returning stdout and stderr as strings plus the captured exit code.
func capturePrespec(verb, stdinJSON string) (stdout, stderr string, exitCode int) {
	var outBuf, errBuf bytes.Buffer
	code := 0
	PrespecCore(
		verb,
		strings.NewReader(stdinJSON),
		&outBuf,
		&errBuf,
		func(c int) { code = c },
	)
	return outBuf.String(), errBuf.String(), code
}

// TestPrespecRankVerb verifies the rank verb returns ranked uncovered cells.
func TestPrespecRankVerb(t *testing.T) {
	// Use the default 10-cell grid with all Missing state.
	cells := DefaultCells()
	inputCells := make([]map[string]interface{}, len(cells))
	for i, c := range cells {
		inputCells[i] = map[string]interface{}{
			"key":         c.Key,
			"impact":      c.Impact,
			"uncertainty": c.Uncertainty,
			"state":       "missing",
		}
	}
	input := map[string]interface{}{"cells": inputCells}
	inJSON, _ := json.Marshal(input)

	stdout, stderr, code := capturePrespec("rank", string(inJSON))
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
	_, stderr, code := capturePrespec("rank", "not-json")
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
	stdout, stderr, code := capturePrespec("lint", input)
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
	stdout, _, code := capturePrespec("lint", input)
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
	cells := DefaultCells()
	// Mark first 6 as Clear.
	for i := 0; i < 6; i++ {
		cells[i].State = Clear
	}
	inputCells := marshalCells(cells)
	input := map[string]interface{}{"cells": inputCells}
	inJSON, _ := json.Marshal(input)

	stdout, stderr, code := capturePrespec("readiness", string(inJSON))
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

// TestPrespecBriefVerb verifies the brief verb renders markdown and returns path template.
func TestPrespecBriefVerb(t *testing.T) {
	cells := DefaultCells()
	for i := 0; i < 6; i++ {
		cells[i].State = Clear
	}
	inputCells := marshalCells(cells)

	input := map[string]interface{}{
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
		"cells":      inputCells,
	}
	inJSON, _ := json.Marshal(input)

	stdout, stderr, code := capturePrespec("brief", string(inJSON))
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

// TestPrespecBriefVerbFailsGate verifies brief verb exits 1 when readiness is below gate.
func TestPrespecBriefVerbFailsGate(t *testing.T) {
	cells := DefaultCells() // all Missing → score 0.0
	inputCells := marshalCells(cells)
	input := map[string]interface{}{
		"project":    "my-project",
		"job":        "Do something",
		"sections":   [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		"transcript": "Q: What?",
		"cells":      inputCells,
	}
	inJSON, _ := json.Marshal(input)
	_, stderr, code := capturePrespec("brief", string(inJSON))
	if code != 1 {
		t.Errorf("brief below gate: want exit 1; got %d", code)
	}
	if stderr == "" {
		t.Error("brief below gate: expected stderr error message")
	}
}

// TestPrespecUnknownVerb verifies an unknown verb exits 1.
func TestPrespecUnknownVerb(t *testing.T) {
	_, stderr, code := capturePrespec("unknown-verb", "{}")
	if code != 1 {
		t.Errorf("unknown verb: want exit 1; got %d", code)
	}
	if stderr == "" {
		t.Error("unknown verb: expected stderr diagnostic")
	}
}

// marshalCells converts []Cell into the JSON-serialisable shape the dispatch expects.
func marshalCells(cells []Cell) []map[string]interface{} {
	result := make([]map[string]interface{}, len(cells))
	for i, c := range cells {
		state := "missing"
		switch c.State {
		case Partial:
			state = "partial"
		case Clear:
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

