package main

// prespec subcommand: 'prespec rank|lint|readiness|brief'. The discovery rules
// (the grid, the lint, the readiness score, the brief and its ID) live in
// engine/prespec and are pure; this file is the CLI around them: it reads JSON
// from stdin, dispatches on the verb, and writes JSON to stdout. It is also where
// the process's clock and entropy source are chosen, because a brief needs a
// time and a random ID and the domain takes both as values.

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
)

// prespecEnv is what the brief verb takes from the process around it: the clock
// that stamps a brief and the entropy source behind its ID. Production uses
// realPrespecEnv; a test passes its own, so a brief is reproducible.
type prespecEnv struct {
	now     func() time.Time
	entropy io.Reader
}

// realPrespecEnv is the production environment: the wall clock and crypto/rand.
func realPrespecEnv() prespecEnv {
	return prespecEnv{now: time.Now, entropy: rand.Reader}
}

// runPrespec implements the 'prespec <verb>' subcommand.
// Requires exactly one verb argument; fails LOUD on missing or unknown verb (ADR-4).
func runPrespec(p process, args []string) {
	runPrespecCore(verbFromArgs(args), p.stdin, p.stdout, p.stderr, p.exit, realPrespecEnv())
}

// runPrespecCore is the testable core of the `engine prespec <verb>` subcommand.
// It reads JSON from stdin, dispatches to the appropriate pure function, and
// writes JSON to stdout. Fails LOUD (calls exit(1)) on malformed input or
// unknown verb (ADR-4).
//
// Supported verbs:
//
//	rank       {cells}                                          → {ranked}
//	lint       {question}                                       → {accepted, rule, reason}
//	readiness  {cells}                                          → {value, passes, clear, total}
//	brief      {project, job, sections[6], transcript, cells}  → {id, markdown, path}
func runPrespecCore(verb string, stdin io.Reader, stdout io.Writer, stderr io.Writer, exit func(int), env prespecEnv) {
	if verb == "" {
		fmt.Fprintln(stderr, "error: prespec requires a verb: rank, lint, readiness, brief")
		exit(1)
		return
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "prespec: cannot read stdin: %v\n", err)
		exit(1)
		return
	}

	switch verb {
	case "rank":
		prespecRank(raw, stdout, stderr, exit)
	case "lint":
		prespecLint(raw, stdout, stderr, exit)
	case "readiness":
		prespecReadiness(raw, stdout, stderr, exit)
	case "brief":
		prespecBrief(raw, stdout, stderr, exit, env)
	default:
		fmt.Fprintf(stderr, "prespec: unknown verb %q; valid verbs: rank, lint, readiness, brief\n", verb)
		exit(1)
	}
}

// prespecCellJSON is the JSON wire shape for a single cell.
type prespecCellJSON struct {
	Key         string `json:"key"`
	Impact      int    `json:"impact"`
	Uncertainty int    `json:"uncertainty"`
	State       string `json:"state"` // "missing" | "partial" | "clear"
}

// parsePrespecCells decodes a []prespecCellJSON into []prespec.Cell.
func parsePrespecCells(raw []prespecCellJSON) []prespec.Cell {
	cells := make([]prespec.Cell, len(raw))
	for i, cj := range raw {
		var state prespec.CellState
		switch strings.ToLower(cj.State) {
		case "partial":
			state = prespec.Partial
		case "clear":
			state = prespec.Clear
		default:
			state = prespec.Missing
		}
		cells[i] = prespec.Cell{
			Key:         cj.Key,
			Impact:      cj.Impact,
			Uncertainty: cj.Uncertainty,
			State:       state,
		}
	}
	return cells
}

// prespecRank implements the rank verb.
func prespecRank(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Cells []prespecCellJSON `json:"cells"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec rank: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	cells := parsePrespecCells(input.Cells)
	g := prespec.Grid{Cells: cells}
	ranked := g.RankUncovered()

	type rankedCell struct {
		Key         string `json:"key"`
		Impact      int    `json:"impact"`
		Uncertainty int    `json:"uncertainty"`
		State       string `json:"state"`
	}
	result := make([]rankedCell, len(ranked))
	for i, c := range ranked {
		state := "missing"
		if c.State == prespec.Partial {
			state = "partial"
		}
		result[i] = rankedCell{
			Key:         c.Key,
			Impact:      c.Impact,
			Uncertainty: c.Uncertainty,
			State:       state,
		}
	}
	writePrespecJSON(stdout, stderr, exit, map[string]interface{}{"ranked": result})
}

// prespecLint implements the lint verb.
func prespecLint(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec lint: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	r := prespec.Lint(input.Question)
	writePrespecJSON(stdout, stderr, exit, map[string]interface{}{
		"accepted": r.Accepted,
		"rule":     r.Rule,
		"reason":   r.Reason,
	})
}

// prespecReadiness implements the readiness verb.
func prespecReadiness(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Cells []prespecCellJSON `json:"cells"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec readiness: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	cells := parsePrespecCells(input.Cells)
	score := prespec.Readiness(cells)
	writePrespecJSON(stdout, stderr, exit, map[string]interface{}{
		"value":  score.Value,
		"passes": score.Passes(),
		"clear":  score.ClearCount,
		"total":  score.Total,
	})
}

// prespecBrief implements the brief verb. The clock is read once, so the ID's
// timestamp and the brief's creation time are the same instant.
func prespecBrief(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int), env prespecEnv) {
	var input struct {
		Project    string            `json:"project"`
		Job        string            `json:"job"`
		Sections   [6]string         `json:"sections"`
		Transcript string            `json:"transcript"`
		Cells      []prespecCellJSON `json:"cells"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec brief: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	cells := parsePrespecCells(input.Cells)
	now := env.now().UTC()
	id := prespec.NewIDFrom(now, env.entropy)
	b := prespec.Brief{
		DiscoveryID: id,
		Project:     input.Project,
		CreatedAt:   now,
		Job:         input.Job,
		Sections:    input.Sections,
		Transcript:  input.Transcript,
	}

	if err := b.Validate(cells); err != nil {
		fmt.Fprintf(stderr, "prespec brief: %v\n", err)
		exit(1)
		return
	}

	markdown := prespec.RenderBrief(b)
	path := b.TopicKey()

	writePrespecJSON(stdout, stderr, exit, map[string]interface{}{
		"id":       id,
		"markdown": markdown,
		"path":     path,
	})
}

// writePrespecJSON marshals v to stdout. Fails loud on marshal error (should never
// happen with our controlled output shapes).
func writePrespecJSON(stdout io.Writer, stderr io.Writer, exit func(int), v interface{}) {
	enc := json.NewEncoder(stdout)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "prespec: JSON encode error: %v\n", err)
		exit(1)
	}
}
