// Package sha256hex is the one place the engine says what a SHA-256 digest looks
// like on the wire and how one is made: 64 lowercase hexadecimal characters, the
// hash of the exact bytes it names. The records that carry a digest (a goal, a
// workflow event, a role handoff, a clearance, an approval, a project lock, a
// binding) validate it with IsValid and make it with Sum, so no record has a
// rule of its own for what a digest is.
//
// It is a domain package: pure standard library, no file, no clock, no entropy.
package sha256hex

import (
	"crypto/sha256"
	"encoding/hex"
)

// Length is the number of characters of a SHA-256 digest written as hex.
const Length = 2 * sha256.Size

// Sum returns the lowercase hex SHA-256 of data.
func Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// IsValid reports whether s is a SHA-256 digest as every record writes it:
// exactly 64 lowercase hexadecimal characters, nothing before, after or between
// them (so no uppercase, no prefix and no trailing newline).
func IsValid(s string) bool {
	if len(s) != Length {
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
