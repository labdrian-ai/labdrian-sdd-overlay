package prespec

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// cellJSON is the JSON wire shape for a single cell.
type cellJSON struct {
	Key         string `json:"key"`
	Impact      int    `json:"impact"`
	Uncertainty int    `json:"uncertainty"`
	State       string `json:"state"` // "missing" | "partial" | "clear"
}

// parseCells decodes a []cellJSON into []Cell.
func parseCells(raw []cellJSON) []Cell {
	cells := make([]Cell, len(raw))
	for i, cj := range raw {
		var state CellState
		switch strings.ToLower(cj.State) {
		case "partial":
			state = Partial
		case "clear":
			state = Clear
		default:
			state = Missing
		}
		cells[i] = Cell{
			Key:         cj.Key,
			Impact:      cj.Impact,
			Uncertainty: cj.Uncertainty,
			State:       state,
		}
	}
	return cells
}

// PrespecCore is the testable core of the `engine prespec <verb>` subcommand.
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
func PrespecCore(verb string, stdin io.Reader, stdout io.Writer, stderr io.Writer, exit func(int)) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "prespec: cannot read stdin: %v\n", err)
		exit(1)
		return
	}

	switch verb {
	case "rank":
		handleRank(raw, stdout, stderr, exit)
	case "lint":
		handleLint(raw, stdout, stderr, exit)
	case "readiness":
		handleReadiness(raw, stdout, stderr, exit)
	case "brief":
		handleBrief(raw, stdout, stderr, exit)
	default:
		fmt.Fprintf(stderr, "prespec: unknown verb %q; valid verbs: rank, lint, readiness, brief\n", verb)
		exit(1)
	}
}

// handleRank implements the rank verb.
func handleRank(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Cells []cellJSON `json:"cells"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec rank: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	cells := parseCells(input.Cells)
	g := Grid{Cells: cells}
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
		if c.State == Partial {
			state = "partial"
		}
		result[i] = rankedCell{
			Key:         c.Key,
			Impact:      c.Impact,
			Uncertainty: c.Uncertainty,
			State:       state,
		}
	}
	writeJSON(stdout, stderr, exit, map[string]interface{}{"ranked": result})
}

// handleLint implements the lint verb.
func handleLint(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec lint: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	r := Lint(input.Question)
	writeJSON(stdout, stderr, exit, map[string]interface{}{
		"accepted": r.Accepted,
		"rule":     r.Rule,
		"reason":   r.Reason,
	})
}

// handleReadiness implements the readiness verb.
func handleReadiness(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Cells []cellJSON `json:"cells"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec readiness: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	cells := parseCells(input.Cells)
	score := Readiness(cells)
	writeJSON(stdout, stderr, exit, map[string]interface{}{
		"value":  score.Value,
		"passes": score.Passes(),
		"clear":  score.ClearCount,
		"total":  score.Total,
	})
}

// handleBrief implements the brief verb.
func handleBrief(raw []byte, stdout io.Writer, stderr io.Writer, exit func(int)) {
	var input struct {
		Project    string     `json:"project"`
		Job        string     `json:"job"`
		Sections   [6]string  `json:"sections"`
		Transcript string     `json:"transcript"`
		Cells      []cellJSON `json:"cells"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintf(stderr, "prespec brief: malformed JSON input: %v\n", err)
		exit(1)
		return
	}

	cells := parseCells(input.Cells)
	id := NewID()
	b := Brief{
		DiscoveryID: id,
		Project:     input.Project,
		CreatedAt:   time.Now().UTC(),
		Job:         input.Job,
		Sections:    input.Sections,
		Transcript:  input.Transcript,
	}

	if err := b.Validate(cells); err != nil {
		fmt.Fprintf(stderr, "prespec brief: %v\n", err)
		exit(1)
		return
	}

	markdown := RenderBrief(b)
	path := b.TopicKey()

	writeJSON(stdout, stderr, exit, map[string]interface{}{
		"id":       id,
		"markdown": markdown,
		"path":     path,
	})
}

// writeJSON marshals v to stdout. Fails loud on marshal error (should never happen
// with our controlled output shapes).
func writeJSON(stdout io.Writer, stderr io.Writer, exit func(int), v interface{}) {
	enc := json.NewEncoder(stdout)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "prespec: JSON encode error: %v\n", err)
		exit(1)
	}
}
