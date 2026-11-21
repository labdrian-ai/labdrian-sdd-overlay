// Package skillregistry is the table of skills in a project's .atl/skill-registry.md: which
// directories hold skills, how a SKILL.md says what it is called and what it is for, which skills
// the table lists, and how the file is written around the rows without touching any line that the
// table does not own. The rules are the ones `gentle-ai skill-registry refresh` 4.2.0 follows,
// recorded from its output in a sandbox (engine/cmd's TestRegistryRefreshWritesWhatGentleWroteForTheSameSkills), with one stated
// difference: a description written as a quoted value over several lines is read whole, where
// gentle-ai keeps only its first line.
//
// It reads and writes no file, reads no clock and prints nothing; where the skills are, what day it
// is and where the registry lives belong to the use case (engine/skillregistry/app) and its adapter.
package skillregistry

import (
	"slices"
	"strings"
)

// Scopes of a row: where the skill was found.
const (
	ScopeProject = "project"
	ScopeUser    = "user"
)

// noDescription is what a row says of a skill that says nothing of itself.
const noDescription = "—"

// projectSources are the directories of a project that may hold skills, in the order they are
// scanned, relative to the project.
var projectSources = []string{
	"skills", ".opencode/skills", ".claude/skills", ".gemini/skills", ".cursor/skills",
	".github/skills", ".codex/skills", ".qwen/skills", ".kiro/skills", ".openclaw/skills",
	".pi/skills", ".agent/skills", ".agents/skills", ".atl/skills", ".hermes/skills",
}

// userSources are the directories of a home that may hold skills, in the order they are scanned,
// relative to the home. Which of them exist is for the adapter to find out.
var userSources = []string{
	".pi/agent/skills", ".config/agents/skills", ".agents/skills", ".kimi/skills",
	".config/opencode/skills", ".config/kilo/skills", ".claude/skills", ".gemini/skills",
	".gemini/antigravity-cli/skills", ".cursor/skills", ".copilot/skills", ".codex/skills",
	".codeium/windsurf/skills", ".qwen/skills", ".kiro/skills", ".openclaw/skills",
	".hermes/skills", ".trae/skills",
}

// Source is a directory that may hold skills.
type Source struct {
	Scope string
	// Label is how the registry names the source under "Sources scanned": the path relative to
	// the project for a project's directory, the whole path for one of the home.
	Label string
	// Dir is the path of the directory.
	Dir string
}

// Sources lists the directories scanned for the project at project, in the order they are scanned:
// those of the project, then those of home. A home that is empty has none. The directory of a
// source that is not there is listed too; the adapter skips it.
func Sources(project, home string) []Source {
	project = strings.TrimRight(project, "/")
	home = strings.TrimRight(home, "/")
	var out []Source
	for _, rel := range projectSources {
		out = append(out, Source{Scope: ScopeProject, Label: rel, Dir: project + "/" + rel})
	}
	if home == "" {
		return out
	}
	for _, rel := range userSources {
		path := home + "/" + rel
		out = append(out, Source{Scope: ScopeUser, Label: path, Dir: path})
	}
	return out
}

// Candidate is a SKILL.md found in a source. Candidates are handed over in the order the sources
// are scanned, and in the order of the directory names inside one source.
type Candidate struct {
	// Scope is the scope of the source it was found in.
	Scope string
	// DirName is the name of the directory the skill is kept in.
	DirName string
	// Path is the path of the SKILL.md.
	Path string
	// Text is the content of the SKILL.md.
	Text string
}

// Row is one line of the skills table.
type Row struct {
	Name, Description, Scope, Path string
}

// Line is the row as the table writes it. A bar in the name or the description is escaped so that
// it does not end a cell, and a skill with no description says a dash.
func (r Row) Line() string {
	description := r.Description
	if description == "" {
		description = noDescription
	}
	return "| `" + escapeBars(r.Name) + "` | " + escapeBars(description) + " | " + r.Scope + " | `" + r.Path + "` |"
}

func escapeBars(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// Rows is the table of the candidates: each skill keyed by the name its frontmatter gives it (the
// directory's name when it gives none), the first one found winning, `_shared` and `skill-registry`
// left out by their name, sorted by name in byte order.
func Rows(candidates []Candidate) []Row {
	seen := make(map[string]bool)
	var rows []Row
	for _, c := range candidates {
		skill := ParseSkill(c.DirName, c.Text)
		if skipped(skill.Name) || seen[skill.Name] {
			continue
		}
		seen[skill.Name] = true
		rows = append(rows, Row{Name: skill.Name, Description: skill.Description, Scope: c.Scope, Path: c.Path})
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return strings.Compare(a.Name, b.Name) })
	return rows
}

// skipped says whether a skill of this name is left out of the table: the shared files are not
// skills and the registry's own skill is not listed. A skill named sdd-something is listed like any
// other: the executor rows a repair used to put back are gone with the repair.
func skipped(name string) bool {
	return name == "_shared" || name == "skill-registry"
}
