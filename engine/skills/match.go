package skills

import "strings"

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

	out := strings.Trim(b.String(), "-")
	return truncateSlug(out, maxSlugBytes)
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
