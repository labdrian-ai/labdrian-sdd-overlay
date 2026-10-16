// Package gitobject is the one place the engine says what a full git object name
// looks like: 40 lowercase hexadecimal characters for a SHA-1 repository, 64 for
// a SHA-256 one. The workflow event that records the commit a command ran in, the
// observation of a worktree by the git binary and the reader of a repository's
// files all decide whether a string is a full name, and they decide it here, so
// no one of them has a rule of its own for what git calls an object.
//
// It is a domain package: pure standard library, no file, no process.
package gitobject

// The number of characters of a full object name written as hex, for the two hash
// functions a git repository can use.
const (
	sha1Length   = 40
	sha256Length = 64
)

// IsName reports whether s is a full object name as git writes one: exactly 40
// lowercase hexadecimal characters or exactly 64, nothing before, after or
// between them (so no uppercase, no abbreviation, no prefix and no trailing
// newline).
func IsName(s string) bool {
	if len(s) != sha1Length && len(s) != sha256Length {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
