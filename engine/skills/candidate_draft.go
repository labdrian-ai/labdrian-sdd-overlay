package skills

import (
	"fmt"
	"strings"
)

// DraftRecord is the header of the Engram draft observation in section 8 of
// procedural-candidate-detection.md. Like CandidateRecord, it keeps the readable
// fields even when the parser finds anomalies. The Body is not record metadata.
type DraftRecord struct {
	// Key and Goal follow the same topic-prefix rule as CandidateRecord.
	Key, Goal   string
	Candidate   string
	Disposition string
	Status      DraftStatus
	// ForRevision is positive for a usable latch. Zero means absent or invalid;
	// reconciliation must not group such a draft under a revision.
	ForRevision int
	// History records candidate lifecycle events, not a second transition table
	// for the draft's separate Status vocabulary.
	History []HistoryEntry
}

const draftKeyPrefix = "procedural/drafts/"

// ParseDraftRecord reads the field block before Body, never the fenced skill or
// diff inside it. It reports anomalies without repairing the observation. The
// owner requires a positive ForRevision (C23 M10); missing or invalid latches
// remain zero and do not participate in duplicate-latch detection.
func ParseDraftRecord(topicKey, content string) (DraftRecord, []Anomaly) {
	goal, key := splitGoalPrefix(topicKey)
	rec := DraftRecord{Key: key, Goal: goal}
	var anomalies []Anomaly
	add := func(format string, args ...any) {
		anomalies = append(anomalies, Anomaly{Key: topicKey, Message: fmt.Sprintf(format, args...)})
	}
	if tail, ok := strings.CutPrefix(key, draftKeyPrefix); !ok {
		add("draft key %q must start with %q", key, draftKeyPrefix)
	} else if err := ValidateCandidateKey(candidateKeyPrefix + tail); err != nil {
		// A draft has exactly the candidate key's kind and slug-tail grammar.
		add("draft key: %v", err)
	}

	f := readRecordFields(content, "Body")
	for _, name := range f.twice {
		add("the field %s is given twice", name)
	}
	rec.Candidate = f.scalars["Candidate"]
	if rec.Candidate == "" {
		add("no Candidate")
	} else {
		_, candidate := splitGoalPrefix(rec.Candidate)
		if err := ValidateCandidateKey(candidate); err != nil {
			add("%v", err)
		}
	}
	rec.Disposition = f.scalars["Disposition"]
	if raw := f.scalars["Status"]; raw == "" {
		add("no Status")
	} else if status, err := ParseDraftStatus(raw); err != nil {
		add("%v", err)
	} else {
		rec.Status = status
	}

	badRevision := false
	rec.ForRevision = number(f.scalars, "ForRevision", func(format string, args ...any) {
		badRevision = true
		add(format, args...)
	})
	if rec.ForRevision == 0 && !badRevision {
		add("ForRevision %q is not a positive integer", f.scalars["ForRevision"])
	}
	rec.History = readHistory(f.lists["History"], add)
	// The shared history rules judge the candidate events and their chaining.
	// Do not compare their final state with the draft's different vocabulary.
	checkHistory(CandidateRecord{History: rec.History}, add)
	return rec, anomalies
}
