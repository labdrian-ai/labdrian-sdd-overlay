package skills

import (
	"path"
	"strings"
)

// maxSlugBytes is the maximum length of a normalized slug segment, per the
// procedural-candidate-detection contract document.
const maxSlugBytes = 48

// NormalizeSlug returns the canonical identity form of s: lowercase ASCII,
// every run of characters outside [a-z0-9] collapsed to a single '-', leading
// and trailing '-' trimmed, truncated to at most 48 bytes at the last '-'
// boundary at or before 48. It is the single definition of identity
// normalization for procedural promotion candidates; the contract document
// cites it rather than restating the rule. NormalizeSlug performs no
// filesystem or network access.
func NormalizeSlug(s string) string {
	return truncateSlug(normalizeSlugUntruncated(s), maxSlugBytes)
}

// normalizeSlugUntruncated applies NormalizeSlug's lowercase/collapse/trim
// rule without the 48-byte truncation step. MatchCandidate compares this
// untruncated form so that two distinct identities longer than maxSlugBytes
// which merely share the same truncated prefix are never treated as the
// same identity (see MatchCandidate's doc comment).
func normalizeSlugUntruncated(s string) string {
	var b strings.Builder
	inRun := false

	for _, r := range s {
		var lower rune
		switch {
		case r >= 'A' && r <= 'Z':
			lower = r - 'A' + 'a'
		case r >= 'a' && r <= 'z':
			lower = r
		case r >= '0' && r <= '9':
			lower = r
		default:
			lower = 0
		}

		if lower != 0 {
			b.WriteRune(lower)
			inRun = false
			continue
		}

		if !inRun {
			b.WriteByte('-')
			inRun = true
		}
	}

	return strings.Trim(b.String(), "-")
}

// truncateSlug truncates s to at most maxLen bytes, backtracking to the last
// '-' boundary at or before maxLen so a word is never split mid-token. If no
// such boundary exists, s is hard-truncated to maxLen and any resulting
// trailing '-' is trimmed.
func truncateSlug(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	prefix := s[:maxLen]
	if idx := strings.LastIndexByte(prefix, '-'); idx >= 0 {
		return prefix[:idx]
	}

	return strings.Trim(prefix, "-")
}

// MatchCandidate reports whether a registered skill already covers candidate,
// and returns that skill's path when it does. Matching is exact identity
// match on the UNTRUNCATED normalized form (normalizeSlugUntruncated), never
// on NormalizeSlug's 48-byte-truncated output, against each entry's ID, Path,
// and the base segment of Path, in registry order; the first match wins.
// Comparing the untruncated form matters because two distinct identities
// longer than maxSlugBytes can share the same truncated prefix without being
// the same identity; truncated comparison would treat them as a false
// duplicate. There is no substring, prefix, or similarity matching: a false
// duplicate silently discards knowledge, a miss only defers it. An empty or
// all-punctuation candidate never matches. MatchCandidate performs no
// filesystem or Engram access; the caller parses skills.registry.yaml with
// ParseRegistry and passes the resulting Registry in.
//
// Extension point (documented, not built): when the registry schema grows a
// trigger/keyword field, add MatchCandidateBy(reg, candidate, fields
// ...MatchField) beside MatchCandidate. MatchCandidate's own contract stays
// "exact identity match after normalization" forever, so existing rejection
// records keep their meaning.
func MatchCandidate(reg Registry, candidate string) (matched bool, skillPath string) {
	normalizedCandidate := normalizeSlugUntruncated(candidate)
	if normalizedCandidate == "" {
		return false, ""
	}

	for _, entry := range reg.Skills {
		if normalizedCandidate == normalizeSlugUntruncated(entry.ID) ||
			normalizedCandidate == normalizeSlugUntruncated(entry.Path) ||
			normalizedCandidate == normalizeSlugUntruncated(path.Base(entry.Path)) {
			return true, entry.Path
		}
	}

	return false, ""
}
