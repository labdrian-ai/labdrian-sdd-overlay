package skills

import (
	"fmt"
	"sort"
	"strings"
)

// ReconcileCandidates reports cross-record anomalies without changing either
// input: an orphan draft, a candidate's missing draft reference, and more than
// one open draft for a candidate revision. References use the normalized topic
// keys of ADR C23 M10; the goal prefix does not become part of a candidate key.
// Invalid latches (ForRevision <= 0) are excluded only from duplicate detection,
// not from the link checks. Results are ordered by topic key, then message,
// independently of the order in which the source returned its records.
func ReconcileCandidates(candidates []CandidateRecord, drafts []DraftRecord) []Anomaly {
	candidateKeys := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		candidateKeys[candidate.Key] = true
	}
	draftKeys := make(map[string]bool, len(drafts))
	type latch struct {
		candidate string
		revision  int
	}
	open := map[latch][]string{}
	var anomalies []Anomaly
	for _, draft := range drafts {
		draftKeys[draft.Key] = true
		_, candidate := splitGoalPrefix(draft.Candidate)
		if !candidateKeys[candidate] {
			anomalies = append(anomalies, Anomaly{
				Key:     recordTopic(draft.Goal, draft.Key),
				Message: fmt.Sprintf("orphan draft: candidate %q is missing", draft.Candidate),
			})
		}
		if draft.Status == DraftOpen && draft.ForRevision > 0 {
			key := latch{candidate: candidate, revision: draft.ForRevision}
			open[key] = append(open[key], recordTopic(draft.Goal, draft.Key))
		}
	}
	for _, candidate := range candidates {
		if candidate.Draft == "" {
			continue
		}
		_, draft := splitGoalPrefix(candidate.Draft)
		if !draftKeys[draft] {
			anomalies = append(anomalies, Anomaly{
				Key:     recordTopic(candidate.Goal, candidate.Key),
				Message: fmt.Sprintf("missing draft: %q", candidate.Draft),
			})
		}
	}
	for key, topics := range open {
		if len(topics) > 1 {
			sort.Strings(topics)
			anomalies = append(anomalies, Anomaly{
				Key: key.candidate,
				Message: fmt.Sprintf("multiple open drafts for candidate %q revision %d: %s",
					key.candidate, key.revision, strings.Join(topics, ", ")),
			})
		}
	}
	sort.Slice(anomalies, func(i, j int) bool {
		if anomalies[i].Key != anomalies[j].Key {
			return anomalies[i].Key < anomalies[j].Key
		}
		return anomalies[i].Message < anomalies[j].Message
	})
	return anomalies
}

// recordTopic restores the observation's label for a diagnostic, not its identity.
func recordTopic(goal, key string) string {
	if goal != "" {
		return goalTopicPrefix + goal + "/" + key
	}
	return key
}
