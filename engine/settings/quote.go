package settings

import "strings"

// shellQuote returns s as one word of a POSIX shell command. A path made only of characters the
// shell never treats specially is returned as it is, so a command written for such a path is the
// same text it always was and no installed settings.json drifts; anything else is single-quoted,
// with each embedded single quote written as quote, backslash, quote, quote.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !isShellSafe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("_@%+=:,./-", r)
}
