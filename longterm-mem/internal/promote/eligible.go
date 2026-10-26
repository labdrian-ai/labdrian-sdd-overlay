// Package promote implements longterm-mem's promotion writer: which
// Engram observations are eligible (R-007) and how an eligible observation
// becomes a contract-conformant vault page (R-027).
package promote

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// excludedTopicPrefixes are the first path segments curatedTopicKey treats
// as noise, never as a curated eligibility signal (R-007): SDD, review, and
// delivery bookkeeping topics that observations accumulate as a side
// effect of process, not because a human curated them for the vault.
var excludedTopicPrefixes = map[string]bool{
	"sdd":      true,
	"review":   true,
	"delivery": true,
}

// Eligible reports whether obs is eligible for promotion (R-007): pinned,
// OR carrying a curated topic_key, OR explicitly targeted by a promote
// call, which overrides every other criterion.
func Eligible(obs memory.Observation, explicit bool) bool {
	return explicit || obs.Pinned || curatedTopicKey(obs.TopicKey)
}

// IneligibleReason says why obs is not eligible for promotion on its own, with no explicit promote call: it is
// the reason Eligible(obs, false) is false, worded as the predicate of "observation N", and empty when
// Eligible(obs, false) is true. It lives beside the rule so that a change to the rule cannot leave the reason it
// gives behind: both read the same pinned flag and the same topic_key.
//
// An observation that is not pinned is refused for one of two reasons, and the reason says which: it has no
// topic_key (an empty one, or one of white space), or it has one that is not curated -- its first segment, past
// a goal's prefix, is empty or is one of the excluded ones (sdd, review, delivery). A topic_key is quoted as it
// is, trimmed of the white space around it.
func IneligibleReason(obs memory.Observation) string {
	if Eligible(obs, false) {
		return ""
	}
	key := strings.TrimSpace(obs.TopicKey)
	if key == "" {
		return "is not pinned and has no topic_key"
	}
	return fmt.Sprintf("is not pinned and its topic_key %q is not a curated topic", key)
}

// PageObservationID is the id of the Engram observation page was promoted from: the engram_id of its
// frontmatter, read from the page's content as the vault holds it (the whole file, or its frontmatter block
// alone). It reports false for a page that names none -- no frontmatter block, no engram_id, one that is not a
// number -- and never observation 0, so that no page is joined to an observation it does not name.
func PageObservationID(page Page) (int64, bool) {
	block, ok := frontmatterBlock(page.Frontmatter)
	if !ok {
		return 0, false
	}
	id, named := frontmatterEngramID(block)
	return id, named && id > 0
}

// curatedTopicKey reports whether key is a non-empty topic_key whose first
// path segment (the substring before the first "/", or the whole value
// when it contains no "/") is neither empty nor exactly one of
// excludedTopicPrefixes. Matching is exact and case-sensitive:
// "sdd-init/..." and "sddx/..." are NOT excluded, since their first
// segments are "sdd-init" and "sddx", not "sdd". A key starting with "/"
// (e.g. "/sdd/auth" or "/") splits into an empty first segment, which is
// treated as uncurated rather than falling through the exclusion map by
// accident. A goal's topic (`goal:<id>/...`, ADR C23 M5) is judged by what
// follows its goal prefix.
func curatedTopicKey(key string) bool {
	key = memory.TopicWithoutGoal(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	head, _, _ := strings.Cut(key, "/")
	if head == "" {
		return false
	}
	return !excludedTopicPrefixes[head]
}
