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

// ListCandidates reads memorycard through the CandidateSource port, parses records without
// repair, reconciles all loaded records, and applies display filters to candidates.
func ListCandidates(source skills.CandidateSource, resolver skills.LegacyOccurrenceResolver, in CandidateListInput) (CandidateListResult, error) {
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
	// Legacy `engram:<n>` ids count as the `memorycard:<m>` they were imported as: the records are
	// read once to learn which numbers to ask about, and read again with the answer. A listing with
	// no legacy id never asks and is read once, as it always was.
	numbers := legacyNumbers(raw.Candidates)
	var mapped map[string]int
	if len(numbers) > 0 {
		if resolver == nil {
			return CandidateListResult{}, fmt.Errorf("skills candidates: a candidate lists legacy engram ids and no legacy occurrence resolver is wired")
		}
		var err error
		if mapped, err = resolver.ResolveLegacyOccurrences(numbers); err != nil {
			return CandidateListResult{}, fmt.Errorf("skills candidates: resolve legacy occurrences: %w", err)
		}
	}
	for _, observation := range raw.Candidates {
		record, found := skills.ParseCandidateRecordTranslated(observation.TopicKey, observation.Content, mapped)
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

// legacyNumbers returns the distinct numbers of the legacy `engram:<n>` ids the candidates list, sorted
// as text so the resolver is asked the same question for the same listing.
func legacyNumbers(candidates []skills.CandidateObservation) []string {
	seen := map[string]bool{}
	var numbers []string
	for _, observation := range candidates {
		record, _ := skills.ParseCandidateRecord(observation.TopicKey, observation.Content)
		for _, id := range record.Occurrences {
			if number, ok := skills.LegacyOccurrenceNumber(id); ok && !seen[number] {
				seen[number] = true
				numbers = append(numbers, number)
			}
		}
	}
	sort.Strings(numbers)
	return numbers
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
