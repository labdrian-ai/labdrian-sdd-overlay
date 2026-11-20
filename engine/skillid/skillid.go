// Package skillid is the form of a skill id and of the characters of a word that holds one (labdrian-
// orchestrator-opencode T60 commit A, S8 L173 F5): the one definition the gate and the roles share. A skill id
// is the name of a directory of skills/.
package skillid

import "regexp"

// Pattern is the form of a skill id, as a regular expression source with no anchors.
const Pattern = `[a-z0-9][a-z0-9-]*`

var whole = regexp.MustCompile(`^` + Pattern + `$`)

// Valid is whether s, whole, has the form of a skill id.
func Valid(s string) bool { return whole.MatchString(s) }

// WordChar is whether b continues a word that may hold a skill id: the characters of an id, uppercase
// letters and the underscore.
func WordChar(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
