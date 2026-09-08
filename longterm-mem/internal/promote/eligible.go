// Package promote implements longterm-mem's promotion writer: which
// Engram observations are eligible (R-007) and how an eligible observation
// becomes a contract-conformant vault page (R-027).
package promote

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// excludedTopicPrefixes are the first path segments curatedTopicKey treats
// as noise, never as a curated eligibility signal (R-007). Observation type
// and revision count are retired as automatic eligibility criteria (R-003);
// this map replaces eligibleTypes, mirroring its shape.
var excludedTopicPrefixes = map[string]bool{
	"sdd":      true,
	"review":   true,
	"delivery": true,
}

// Eligible reports whether obs is eligible for promotion (R-007): pinned,
// OR carrying a curated topic_key, OR explicitly targeted by a promote
// call, which overrides every other criterion.
func Eligible(obs engram.Observation, explicit bool) bool {
	return explicit || obs.Pinned || curatedTopicKey(obs.TopicKey)
}

// curatedTopicKey reports whether key is a non-empty topic_key whose first
// path segment (the substring before the first "/", or the whole value
// when it contains no "/") is not exactly one of excludedTopicPrefixes.
// Matching is exact and case-sensitive: "sdd-init/..." and "sddx/..." are
// NOT excluded, since their first segments are "sdd-init" and "sddx", not
// "sdd".
func curatedTopicKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	head, _, _ := strings.Cut(key, "/")
	return !excludedTopicPrefixes[head]
}
