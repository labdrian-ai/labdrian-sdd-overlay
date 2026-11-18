package roles

import "regexp"

// skillMention is the one form in which a role body names a skill: the id in backticks, then the
// word skill. The id is the form of a directory under skills/.
var skillMention = regexp.MustCompile("`([a-z0-9][a-z0-9-]*)` skill")

// BodyPath is where the source of a role is, as a slash path under the root of the repository.
func BodyPath(r Role) string { return "roles/" + string(r) + "/body.md" }

// SkillsNamed is the skill ids a role body names in the form "`<id>` skill", in the order of first
// mention, each once. A role on a runtime that denies it the skill tool reads each one's SKILL.md
// by the path the runtime gives it, so this is the one reading of a body that says which skills
// those are: the generator checks each exists and the OpenCode adapter injects each path.
func SkillsNamed(body string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range skillMention.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	return ids
}
