// Package engramexport adapts Engram's read-only export command to the skills
// CandidateSource port. It reads no database itself and never writes Engram.
package engramexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// CommandRunner is the process boundary the adapter needs. execrunner.Runner is
// the production implementation; tests provide a fixture-only runner.
type CommandRunner interface {
	Output(context.Context, []string, string, ...string) ([]byte, []byte, error)
}

// Source exports one Engram project and returns only candidate and draft topics.
type Source struct {
	Executable string
	TempDir    string
	Runner     CommandRunner
}

// ListCandidateObservations implements skills.CandidateSource. Export is written
// to a private scratch file because `engram search` truncates observation content.
func (s Source) ListCandidateObservations(project string) (skills.CandidateObservations, error) {
	if s.Runner == nil {
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: command runner is required")
	}
	if strings.TrimSpace(project) == "" {
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: project is required")
	}
	bin := s.Executable
	if bin == "" {
		bin = "engram"
	}
	file, err := os.CreateTemp(s.TempDir, "engram-candidates-*.json")
	if err != nil {
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: create export file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: close export file: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: prepare export file: %w", err)
	}
	defer os.Remove(path)

	_, stderr, err := s.Runner.Output(context.Background(), nil, bin, "export", path, "--project", project)
	if err != nil {
		detail := strings.TrimSpace(string(stderr))
		if detail != "" {
			return skills.CandidateObservations{}, fmt.Errorf("engram candidates: export project %q: %s: %w", project, detail, err)
		}
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: export project %q: %w", project, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: read export: %w", err)
	}
	observations, err := decodeExport(data)
	if err != nil {
		return skills.CandidateObservations{}, fmt.Errorf("engram candidates: decode export: %w", err)
	}
	var result skills.CandidateObservations
	for _, observation := range observations {
		family := topicFamily(observation.TopicKey)
		switch family {
		case "procedural/candidates/":
			result.Candidates = append(result.Candidates, skills.CandidateObservation{TopicKey: observation.TopicKey, Content: observation.Content})
		case "procedural/drafts/":
			result.Drafts = append(result.Drafts, skills.CandidateObservation{TopicKey: observation.TopicKey, Content: observation.Content})
		}
	}
	return result, nil
}

type exportedObservation struct {
	TopicKey string `json:"topic_key"`
	Content  string `json:"content"`
}

// decodeExport refuses an absent or null observations array rather than treating a
// changed export format as a successfully read empty project.
func decodeExport(data []byte) ([]exportedObservation, error) {
	var envelope struct {
		Observations json.RawMessage `json:"observations"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Observations) == 0 || string(envelope.Observations) == "null" {
		return nil, errors.New("export has no observations array")
	}
	var observations []exportedObservation
	if err := json.Unmarshal(envelope.Observations, &observations); err != nil {
		return nil, err
	}
	if observations == nil {
		return nil, errors.New("export observations are null")
	}
	return observations, nil
}

// topicFamily strips the optional goal prefix from a topic for family matching.
// It returns only one of the two record namespaces the adapter owns.
func topicFamily(topic string) string {
	if after, ok := strings.CutPrefix(topic, "goal:"); ok {
		_, rest, slash := strings.Cut(after, "/")
		if !slash {
			return ""
		}
		topic = rest
	}
	for _, prefix := range []string{"procedural/candidates/", "procedural/drafts/"} {
		if strings.HasPrefix(topic, prefix) {
			return prefix
		}
	}
	return ""
}
