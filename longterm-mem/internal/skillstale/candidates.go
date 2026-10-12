package skillstale

import (
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// candidateRecord is what the newest observation of a candidate says of it.
type candidateRecord struct {
	status       string
	lastObserved string
	updatedAt    string
	id           int64
}

var (
	statusField       = regexp.MustCompile(`(?m)^\s*\*\*Status\*\*:\s*(\S+)\s*$`)
	lastObservedField = regexp.MustCompile(`(?m)^\s*\*\*LastObserved\*\*:\s*(\S+)\s*$`)
)

// latestCandidates reads the Status and LastObserved of the newest observation of each topic key: the one
// updated last, and among those updated together the one with the highest id.
func latestCandidates(observations []memory.Observation) map[string]candidateRecord {
	out := make(map[string]candidateRecord)
	for _, observation := range observations {
		if observation.TopicKey == "" {
			continue
		}
		candidate := candidateRecord{
			status:       firstCapture(statusField, observation.Content),
			lastObserved: firstCapture(lastObservedField, observation.Content),
			updatedAt:    observation.UpdatedAt,
			id:           observation.ID,
		}
		if previous, ok := out[observation.TopicKey]; ok && !newerCandidate(candidate, previous) {
			continue
		}
		out[observation.TopicKey] = candidate
	}
	return out
}

func newerCandidate(candidate, previous candidateRecord) bool {
	if candidate.updatedAt != previous.updatedAt {
		return candidate.updatedAt > previous.updatedAt
	}
	return candidate.id > previous.id
}

func firstCapture(pattern *regexp.Regexp, content string) string {
	match := pattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func parseTimestamp(raw string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}
