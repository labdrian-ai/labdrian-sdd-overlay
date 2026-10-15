// Package sddregistry is the part of a project's .atl/skill-registry.md that belongs to the SDD
// phase skills. gentle-ai writes that registry and, on purpose, leaves the sdd-* skills out of it:
// they are executor contracts, not skills a person invokes. SDD delegation still needs their exact
// paths, so the overlay puts a row for each installed one back, with the scope "executor", and
// says why in a note above the skills table. This package is that rule over text: which skills
// are executors, how their rows read, what a registry looks like that has none yet, and how the
// rows of an earlier repair are replaced by the rows of this one.
//
// It reads and writes no file. Where the registry and the skills are is the application's
// (engine/sddregistry/app), through ports; what it does with the machine is the adapter's
// (engine/sddregistry/fsstore); the lock and the words a person reads are the command's.
//
// The rules are the ones the overlay script followed when it edited the text with awk and grep,
// kept as they were, the ones that look like defects included, because the script's behavior is
// pinned (engine/installer/testdata/registry_pins.golden): the rows land before the heading of the
// loading protocol, after whatever blank line precedes it, and a second repair adds a blank line
// below the note and one below the rows, because the ones the first left are kept.
package sddregistry

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// The headings of the registry the repair works between.
const (
	skillsHeading   = "## Skills"
	protocolHeading = "## Loading protocol"
)

// Note is the sentence written above the skills heading. It is how a repair finds, and drops, the
// note an earlier repair left.
const Note = "`sdd-*` phase skills are executor contracts. Gentle may skip them as user-invocable skills, but this registry keeps their paths visible so SDD delegation can resolve the existing installed files."

// rowPrefix is how a line that counts as an executor row begins.
const rowPrefix = "| `sdd-"

// executors are the SDD phase skills the overlay keeps rows for, in the order of their rows.
var executors = []string{
	"sdd-apply",
	"sdd-archive",
	"sdd-design",
	"sdd-explore",
	"sdd-init",
	"sdd-onboard",
	"sdd-propose",
	"sdd-spec",
	"sdd-tasks",
	"sdd-time-estimation",
	"sdd-verify",
}

// Executors returns the SDD phase skills the overlay keeps rows for, in the order of their rows.
func Executors() []string { return slices.Clone(executors) }

// staleRow is a line an earlier repair may have written: the row of a skill named sdd-something,
// whatever else it says. A repair drops every one of them before it writes its own.
var staleRow = regexp.MustCompile("^\\| `sdd-[^`]+` \\|")

// whitespace is what a description may be indented with: the white space of awk's [[:space:]].
const whitespace = " \t\n\v\f\r"

// Executor is one row of the registry: an installed executor skill, where it is, and what it says
// of itself.
type Executor struct {
	Name        string
	Path        string
	Description string
}

// NewExecutor makes the row of the skill called name, whose file is at skillPath and holds
// skillText. A skill that has no description of its own is described by its name.
func NewExecutor(name, skillPath, skillText string) Executor {
	description := FrontmatterDescription(skillText)
	if description == "" {
		description = "SDD phase executor: " + name + "."
	}
	return Executor{Name: name, Path: skillPath, Description: description}
}

// Row is the line of the registry that lists the executor. The description is written as it is
// (the bars in it are escaped by FrontmatterDescription).
func (e Executor) Row() string {
	return fmt.Sprintf("| `%s` | %s | executor | `%s` |", e.Name, e.Description, e.Path)
}

// FrontmatterDescription is what a skill file says of itself: the text after "description:" on the
// first line that begins with it, wherever that line is in the file, with the white space before
// the text dropped, one double quote dropped from each end, and every bar escaped so that it does
// not end a table cell. A file with no such line says nothing, and so does a line with no text.
func FrontmatterDescription(skillText string) string {
	for _, line := range records(skillText) {
		rest, ok := strings.CutPrefix(line, "description:")
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, whitespace)
		rest = strings.TrimPrefix(rest, `"`)
		rest = strings.TrimSuffix(rest, `"`)
		return strings.ReplaceAll(rest, "|", `\|`)
	}
	return ""
}

// ProjectName is the name a registry made at registryPath gives its project: the directory that
// holds the .atl directory the registry is in.
func ProjectName(registryPath string) string {
	return path.Base(path.Dir(path.Dir(registryPath)))
}

// NewRegistry is the registry of a project that has none: the two sections the repair works
// between, the skills table with no row, and the loading protocol.
func NewRegistry(project string) string {
	return "# Skill Registry — " + project + "\n" +
		"\n" +
		skillsHeading + "\n" +
		"\n" +
		"| Skill | Trigger / description | Scope | Path |\n" +
		"| --- | --- | --- | --- |\n" +
		"\n" +
		protocolHeading + "\n" +
		"\n" +
		"1. Match task context and target files against the `Trigger / description` column.\n" +
		"2. Pass only the matching `Path` values to the subagent under `## Skills to load before work`.\n"
}

// MissingSectionError is the refusal of a registry that lacks a heading the repair works between.
type MissingSectionError struct {
	// Heading is the line the registry does not have, such as "## Skills".
	Heading string
}

func (e *MissingSectionError) Error() string {
	return fmt.Sprintf("registry has no '%s' section", e.Heading)
}

// Repair returns registry with its executor rows replaced by rows: the note of an earlier repair
// and every row of a skill named sdd-something are dropped, the note is written above the skills
// heading with a blank line after it, and the rows are written above the loading protocol heading
// with a blank line after them. Every other line stays as it was, in its place. A registry without
// the skills heading or without the loading protocol heading is refused, saying which, and nothing
// is returned.
func Repair(registry string, rows []Executor) (string, error) {
	lines := records(registry)
	for _, heading := range []string{skillsHeading, protocolHeading} {
		if !slices.Contains(lines, heading) {
			return "", &MissingSectionError{Heading: heading}
		}
	}
	var out strings.Builder
	emit := func(line string) {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	for _, line := range lines {
		switch {
		case line == Note, staleRow.MatchString(line):
			// An earlier repair wrote this one; this repair writes it again.
		case line == skillsHeading:
			emit(Note)
			emit("")
			emit(line)
		case line == protocolHeading:
			for _, row := range rows {
				emit(row.Row())
			}
			emit("")
			emit(line)
		default:
			emit(line)
		}
	}
	return out.String(), nil
}

// CountRows is how many lines of registry begin like an executor row: the rows a repair wrote and
// any other line that begins the same way.
func CountRows(registry string) int {
	count := 0
	for _, line := range records(registry) {
		if strings.HasPrefix(line, rowPrefix) {
			count++
		}
	}
	return count
}

// records splits text into its lines as awk and grep read them: by line end, with a last line that
// has no line end counted and no empty line after the last line end.
func records(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
