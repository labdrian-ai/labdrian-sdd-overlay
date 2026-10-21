package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// skillsCandidates is the read-only `skills candidates` adapter. A failure to resolve or read
// Engram is exit 2; it is never represented as a successful empty list.
func skillsCandidates(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsCandidatesSpec.parse(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	statuses := make([]skills.CandidateStatus, 0, len(parsed.allValues(flagCandidateStatus)))
	for _, raw := range parsed.allValues(flagCandidateStatus) {
		status, err := skills.ParseCandidateStatus(raw)
		if err != nil {
			fmt.Fprintf(stderr, "error: skills candidates: %v\n", err)
			exit(1)
			return
		}
		statuses = append(statuses, status)
	}
	if deps.Candidates == nil {
		fmt.Fprintln(stderr, "error: skills candidates: no candidate source is wired")
		exit(2)
		return
	}
	root, err := candidateProjectRoot(deps, parsed.value(flagProjectRoot, ""))
	if err != nil {
		var usage *skillsUsageError
		if errors.As(err, &usage) {
			refuseSkillsUsage(usage, stderr, exit)
			return
		}
		fmt.Fprintf(stderr, "error: skills candidates: %v\n", err)
		exit(2)
		return
	}
	if deps.Identity == nil {
		fmt.Fprintln(stderr, "error: skills candidates: no project identity is wired")
		exit(2)
		return
	}
	project, ok, err := deps.Identity.Identify(skills.ProjectQuery{Dir: root})
	if err != nil {
		fmt.Fprintf(stderr, "error: skills candidates: identify project %q: %v\n", root, err)
		exit(2)
		return
	}
	if !ok || project == "" {
		fmt.Fprintf(stderr, "error: skills candidates: could not identify project %q\n", root)
		exit(2)
		return
	}
	result, err := app.ListCandidates(deps.Candidates, app.CandidateListInput{
		Project: string(project), Statuses: statuses, Kind: parsed.value(flagCandidateKind, ""),
		IncludeDrafts: parsed.switches[flagCandidateDrafts],
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(2)
		return
	}
	if parsed.switches[flagCandidateJSON] {
		writeCandidatesJSON(stdout, result)
		exit(0)
		return
	}
	writeCandidatesText(stdout, result)
	exit(0)
}

func candidateProjectRoot(deps skills.Deps, requested string) (string, error) {
	root := requested
	if root == "" {
		if deps.Cwd == nil {
			return "", fmt.Errorf("no project root was supplied and no working directory is wired")
		}
		var err error
		root, err = deps.Cwd()
		if err != nil {
			return "", fmt.Errorf("read working directory: %w", err)
		}
	}
	if !filepath.IsAbs(root) {
		return "", &skillsUsageError{fmt.Sprintf("skills candidates: project root %q is not absolute", root)}
	}
	return root, nil
}

type candidateJSON struct {
	Status      string `json:"status"`
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Count       int    `json:"count"`
	Threshold   int    `json:"threshold"`
	Disposition string `json:"disposition"`
	Draft       string `json:"draft"`
}

type draftJSON struct {
	Status      string `json:"status"`
	Key         string `json:"key"`
	Candidate   string `json:"candidate"`
	Disposition string `json:"disposition"`
	ForRevision int    `json:"for_revision"`
}

type anomalyJSON struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

type candidatesJSON struct {
	Candidates []candidateJSON `json:"candidates"`
	Drafts     []draftJSON     `json:"drafts"`
	Anomalies  []anomalyJSON   `json:"anomalies"`
}

func writeCandidatesJSON(out io.Writer, result app.CandidateListResult) {
	payload := candidatesJSON{
		Candidates: make([]candidateJSON, 0, len(result.Candidates)),
		Drafts:     make([]draftJSON, 0, len(result.Drafts)),
		Anomalies:  make([]anomalyJSON, 0, len(result.Anomalies)),
	}
	for _, record := range result.Candidates {
		payload.Candidates = append(payload.Candidates, candidateJSON{
			Status: string(record.Status), Key: record.Key, Kind: record.Kind,
			Count: record.OccurrenceCount, Threshold: record.Threshold,
			Disposition: record.Disposition, Draft: record.Draft,
		})
	}
	for _, record := range result.Drafts {
		payload.Drafts = append(payload.Drafts, draftJSON{
			Status: string(record.Status), Key: record.Key, Candidate: record.Candidate,
			Disposition: record.Disposition, ForRevision: record.ForRevision,
		})
	}
	for _, anomaly := range result.Anomalies {
		payload.Anomalies = append(payload.Anomalies, anomalyJSON{Key: anomaly.Key, Message: anomaly.Message})
	}
	_ = json.NewEncoder(out).Encode(payload)
}

func writeCandidatesText(out io.Writer, result app.CandidateListResult) {
	for _, record := range result.Candidates {
		fmt.Fprintf(out, "%s\t%s\t%d/%d\t%s\t%s\n", record.Status, record.Key,
			record.OccurrenceCount, record.Threshold, record.Disposition, record.Draft)
	}
	for _, record := range result.Drafts {
		fmt.Fprintf(out, "draft\t%s\t%s\tfor-revision=%d\n", record.Status, record.Key, record.ForRevision)
	}
	if len(result.Anomalies) > 0 {
		fmt.Fprintln(out, "Anomalies:")
		for _, anomaly := range result.Anomalies {
			fmt.Fprintf(out, "%s\t%s\n", anomaly.Key, anomaly.Message)
		}
	}
}
