package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/workflow-v1-golden record what the program does with a workflow
// whose log it did not write in its current form: the logs under testdata/workflow-v1-logs were
// written by the program as it was before a workflow's created event recorded a snapshot of its
// profile (Phase 9 unit H33), the event version was 1 and the event named its profile and nothing
// else. Each log is a workflow of one profile either just created or running with its first two
// stages recorded. The transcripts continue them through every verb that does not create: the stage
// it admits and the ones it refuses, a pause and a resume, the verification and both closes, the
// status after each, and the log they leave, with the timestamps and the digests that chain over
// them written as placeholders. They were recorded from the program before that change, so a
// version 1 workflow that behaves differently, or whose log grows by a byte the program did not
// write before, fails here. Rewrite them deliberately with
//
//	go test ./cmd -run TestWorkflowV1Golden -update-profile-golden
//
// and read the diff before committing it.

// v1LogPath is where the log of the workflow proj-1/wf-1 lives under a world's state home.
func (w *profileWorld) v1LogPath() string {
	return filepath.Join(w.state, "labdrian", "workflows", "proj-1", "wf-1.jsonl")
}

// installV1Log puts the committed log of the profile in the shape given (created, or running) in the
// state home stateHome as the workflow proj-1/wf-1.
func installV1Log(t *testing.T, stateHome, profile, shape string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "workflow-v1-logs", profile+"-"+shape+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateHome, "labdrian", "workflows", "proj-1", "wf-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

var (
	v1LogAt      = regexp.MustCompile(`"at":"[^"]*"`)
	v1LogDigests = regexp.MustCompile(`"(prev_digest|chain_digest)":"[0-9a-f]{64}"`)
)

// log records the log of the workflow as it is, line by line: the version, the sequence number and
// the kind of each event, and the SHA-256 of the line with the timestamps and the digests that chain
// over them written as placeholders, so that a byte the program writes differently shows here. The
// lines themselves are not written out: they are about a kilobyte each. The lines that were in the
// log before the transcript began are those of testdata/workflow-v1-logs, and the ones after them
// are what the program appended.
func (w *profileWorld) log() {
	w.t.Helper()
	data, err := os.ReadFile(w.v1LogPath())
	if err != nil {
		w.t.Fatal(err)
	}
	w.write("the log, line by line (version, seq, kind, SHA-256 of the line with the timestamp and chain digests masked):\n")
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		masked := v1LogAt.ReplaceAllString(line, `"at":"<AT>"`)
		masked = v1LogDigests.ReplaceAllString(masked, `"$1":"<DIGEST>"`)
		var head struct {
			Version int    `json:"version"`
			Seq     int    `json:"seq"`
			Kind    string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &head); err != nil {
			w.t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(masked))
		w.write("  v%d %d %s %s\n", head.Version, head.Seq, head.Kind, hex.EncodeToString(sum[:]))
	}
	w.write("\n")
}

// v1Transcript continues a version 1 workflow of the profile in two worlds: from its running log
// (two stages recorded) through the rest of its stages to a completed close, and from its created
// log through a start, a pause, a resume and an abandoned close.
func v1Transcript(t *testing.T, bin, profile string, stages []string) string {
	var transcript strings.Builder
	for _, shape := range []string{"running", "created"} {
		w := newProfileWorld(t, bin, false)
		goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
		w.places[goal] = "<GOAL>"
		flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
		with := func(verb string, more ...string) []string {
			return append(append([]string{"workflow", verb}, flags...), more...)
		}
		w.write("== %s, from its %s log ==\n\n", profile, shape)
		installV1Log(t, w.state, profile, shape)
		w.run(with("status")...)
		last := stages[len(stages)-1]
		if shape == "running" {
			// Two stages are recorded: the first again, one that is not declared and the last one
			// are refused, and the others are admitted in order.
			w.runBrief(with("stage", "--stage", stages[0])...)
			w.runBrief(with("stage", "--stage", "no-such-stage")...)
			w.runBrief(with("stage", "--stage", last)...)
			for _, stage := range stages[2:] {
				w.runBrief(with("stage", "--stage", stage)...)
			}
			w.runBrief(with("stage", "--stage", stages[0])...)
			w.run(with("verify", "--goal", goal)...)
			w.run(with("close", "--outcome", "completed")...)
			w.run(with("status")...)
		} else {
			w.runBrief(with("stage", "--stage", stages[0])...)
			w.runBrief(with("pause")...)
			w.runBrief(with("start")...)
			w.runBrief(with("start")...)
			w.runBrief(with("pause")...)
			w.runBrief(with("stage", "--stage", stages[0])...)
			w.runBrief(with("resume")...)
			w.runBrief(with("stage", "--stage", stages[0])...)
			w.run(with("verify", "--goal", goal)...)
			w.run(with("close", "--outcome", "abandoned", "--reason", "no longer needed")...)
			w.run(with("status")...)
		}
		w.log()
		transcript.WriteString(w.text())
	}
	return transcript.String()
}

// TestWorkflowV1Golden continues the version 1 log of every profile and compares the transcript
// with its golden file.
func TestWorkflowV1Golden(t *testing.T) {
	for _, p := range profileStages {
		p := p
		name := "workflow-v1-" + p.profile
		t.Run(name, func(t *testing.T) {
			checkProfileGolden(t, "workflow-v1-golden", name, v1Transcript(t, engineBinary(t), p.profile, p.stages))
		})
	}
}
