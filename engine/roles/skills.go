package roles

import (
	"regexp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillid"
)

// skillMention is the one form in which a role body names a skill: the id in backticks, then the
// word skill. The id is the form of a directory under skills/. SkillsNamed also requires that the word
// ends there (RE2 has no lookahead, so it reads the byte after the match).
var skillMention = regexp.MustCompile("`(" + skillid.Pattern + ")` skill")

// ReadsOnlyItsCandidate is whether the role named agent reads only the candidate it is given, so a skill its
// brief names in prose is a quotation or a negation (a criterion, a "do not use") and not a skill to load:
// the reviewer and the polisher (roles/reviewer/body.md:8, roles/polisher/body.md:9). On OpenCode a launch of
// such a role gets no skill added from prose (labdrian-orchestrator-opencode T60 correction B, S8 L173).
func ReadsOnlyItsCandidate(agent string) bool {
	return agent == "reviewer" || agent == "polisher"
}

// BodyPath is where the source of a role is, as a slash path under the root of the repository.
func BodyPath(r Role) string { return "roles/" + string(r) + "/body.md" }

// SkillsNamed is the skill ids a role body names in the form "`<id>` skill", in the order of first
// mention, each once. The word skill ends there: "`<id>` skills" and "`<id>` skill-set" name nothing.
// A role on a runtime that denies it the skill tool reads each one's SKILL.md
// by the path the runtime gives it, so this is the one reading of a body that says which skills
// those are: the generator checks each exists and the OpenCode adapter injects each path.
func SkillsNamed(body string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range skillMention.FindAllStringSubmatchIndex(body, -1) {
		if m[1] < len(body) && skillid.WordChar(body[m[1]]) {
			continue
		}
		id := body[m[2]:m[3]]
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
