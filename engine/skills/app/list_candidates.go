package app

import (
	"fmt"
	"sort"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// CandidateListInput selects which parsed candidates to display. Reconciliation always
// uses the complete source before these display filters are applied.
type CandidateListInput struct {
	Project       string
	Statuses      []skills.CandidateStatus
	Kind          string
	IncludeDrafts bool
}

// CandidateListResult is the sorted, parsed view and every record or cross-record anomaly.
type CandidateListResult struct {
	Candidates []skills.CandidateRecord
	Drafts     []skills.DraftRecord
	Anomalies  []skills.Anomaly
}

// ListCandidates reads Engram through the CandidateSource port, parses records without
// repair, reconciles all loaded records, and applies display filters to candidates.
func ListCandidates(source skills.CandidateSource, in CandidateListInput) (CandidateListResult, error) {
	if source == nil {
		return CandidateListResult{}, fmt.Errorf("skills candidates: candidate source is required")
	}
	raw, err := source.ListCandidateObservations(in.Project)
	if err != nil {
		return CandidateListResult{}, fmt.Errorf("skills candidates: list observations: %w", err)
	}

	candidates := make([]skills.CandidateRecord, 0, len(raw.Candidates))
	drafts := make([]skills.DraftRecord, 0, len(raw.Drafts))
	anomalies := make([]skills.Anomaly, 0)
	for _, observation := range raw.Candidates {
		record, found := skills.ParseCandidateRecord(observation.TopicKey, observation.Content)
		candidates = append(candidates, record)
		anomalies = append(anomalies, found...)
	}
	for _, observation := range raw.Drafts {
		record, found := skills.ParseDraftRecord(observation.TopicKey, observation.Content)
		drafts = append(drafts, record)
		anomalies = append(anomalies, found...)
	}
	anomalies = append(anomalies, skills.ReconcileCandidates(candidates, drafts)...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Key < candidates[j].Key })
	sort.Slice(drafts, func(i, j int) bool { return drafts[i].Key < drafts[j].Key })
	sort.Slice(anomalies, func(i, j int) bool {
		if anomalies[i].Key != anomalies[j].Key {
			return anomalies[i].Key < anomalies[j].Key
		}
		return anomalies[i].Message < anomalies[j].Message
	})

	filtered := make([]skills.CandidateRecord, 0, len(candidates))
	for _, candidate := range candidates {
		if in.Kind != "" && candidate.Kind != in.Kind {
			continue
		}
		if !matchesCandidateStatus(candidate.Status, in.Statuses) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	result := CandidateListResult{Candidates: filtered, Anomalies: anomalies}
	if in.IncludeDrafts {
		result.Drafts = drafts
	}
	return result, nil
}

func matchesCandidateStatus(status skills.CandidateStatus, statuses []skills.CandidateStatus) bool {
	if len(statuses) == 0 {
		return true
	}
	for _, wanted := range statuses {
		if status == wanted {
			return true
		}
	}
	return false
}
