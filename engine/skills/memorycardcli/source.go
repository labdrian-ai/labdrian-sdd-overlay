// Package memorycardcli adapts memorycard's read-only `list` verb to the skills
// CandidateSource port. It reads no database itself and never writes memorycard.
package memorycardcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// CommandRunner is the process boundary the adapter needs. execrunner.Runner is
// the production implementation; tests provide a fixture-only runner.
type CommandRunner interface {
	Output(context.Context, []string, string, ...string) ([]byte, []byte, error)
}

// Source lists one project's candidate and draft memories.
type Source struct {
	Executable string
	Runner     CommandRunner
}

// ListCandidateObservations implements skills.CandidateSource. One `memorycard list` call
// returns both families and every goal-scoped topic; the verb orders rows by document key, part
// number and id, and the adapter keeps that order. A topic key listed once per scope stays one
// row per scope.
func (s Source) ListCandidateObservations(project string) (skills.CandidateObservations, error) {
	if s.Runner == nil {
		return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: command runner is required")
	}
	if strings.TrimSpace(project) == "" {
		return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: project is required")
	}
	bin := s.Executable
	if bin == "" {
		bin = "memorycard"
	}
	stdout, stderr, err := s.Runner.Output(context.Background(), nil, bin, "list", "--project", project,
		"--topic-prefix", "procedural/candidates/", "--topic-prefix", "procedural/drafts/", "--topic-prefix", "goal:", "--json")
	if err != nil {
		detail := strings.TrimSpace(string(stderr))
		if detail != "" {
			return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: list project %q: %s: %w", project, detail, err)
		}
		return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: list project %q: %w", project, err)
	}
	rows, err := decodeList(stdout)
	if err != nil {
		return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: decode list: %w", err)
	}
	var result skills.CandidateObservations
	for _, row := range rows {
		family := skills.CandidateTopicFamily(row.TopicKey)
		if family == skills.FamilyNone {
			continue
		}
		if splitPart.MatchString(row.TopicKey) {
			return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: %q is a split-part row; a record must be stored whole", row.TopicKey)
		}
		if row.Scope == nil {
			return skills.CandidateObservations{}, fmt.Errorf("memorycard candidates: row %q has no scope key; the installed memorycard does not match this adapter", row.TopicKey)
		}
		observation := skills.CandidateObservation{TopicKey: row.TopicKey, Content: row.Body}
		if family == skills.FamilyCandidate {
			result.Candidates = append(result.Candidates, observation)
		} else {
			result.Drafts = append(result.Drafts, observation)
		}
	}
	return result, nil
}

// splitPart matches the key memorycard gives parts 2..N of a memory it split.
var splitPart = regexp.MustCompile(`/part-\d+$`)

type listedMemory struct {
	TopicKey string  `json:"topic_key"`
	Scope    *string `json:"scope"`
	Body     string  `json:"body"`
}

// decodeList refuses an absent or null memories array rather than treating a changed output
// format as a successfully read empty project.
func decodeList(data []byte) ([]listedMemory, error) {
	var envelope struct {
		Memories json.RawMessage `json:"memories"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Memories) == 0 || string(envelope.Memories) == "null" {
		return nil, errors.New("list has no memories array")
	}
	var rows []listedMemory
	if err := json.Unmarshal(envelope.Memories, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
