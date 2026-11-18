package skillregistry

import (
	"regexp"
	"strings"
)

// mappingKey is a line that opens a nested mapping, a lower-case key with nothing after its colon, such
// as `metadata:`: a quoted value never goes on into one. A key followed by text, such as `note: see the
// docs` or `Trigger: a`, reads as prose of the description and goes on.
var mappingKey = regexp.MustCompile(`^[a-z_][a-z0-9_-]*:\s*$`)

// Skill is what a SKILL.md says of itself.
type Skill struct {
	Name        string
	Description string
}

// ParseSkill reads the frontmatter of the SKILL.md text of the skill kept in the directory dir.
//
// The frontmatter is the lines between a first line `---` and the next one; a file that does not
// begin with one, or whose frontmatter never closes, has none. Only top-level `name:` and
// `description:` lines are read, the last of each winning. A value is its text with the white space
// and every quote, single or double, dropped from both ends; YAML escapes are not read. A skill with
// no name is named by its directory.
//
// A description may also be a block scalar: `>`, `|`, `>-` or `|-` followed by the indented lines,
// which are joined by single spaces (a blank line inside adds a space of its own). Any other
// indicator is the text itself. A plain value is its first line only. A quoted value that goes on
// over several indented lines up to the line that ends with the same quote is read whole, joined by
// single spaces; this is where the registry differs from gentle-ai, which keeps the first line. A
// quote that never closes, or that would go on into a line that sets a key (`metadata:`), leaves the
// first line.
func ParseSkill(dir, text string) Skill {
	name, description := readFrontmatter(text)
	if name == "" {
		name = dir
	}
	return Skill{Name: name, Description: description}
}

func readFrontmatter(text string) (name, description string) {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	if !isFence(lines[0]) {
		return "", ""
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if isFence(lines[i]) {
			end = i
			break
		}
	}
	if end < 0 {
		return "", ""
	}
	body := lines[1:end]
	for i := 0; i < len(body); i++ {
		if rest, ok := strings.CutPrefix(body[i], "name:"); ok {
			name = unquote(rest)
		} else if rest, ok := strings.CutPrefix(body[i], "description:"); ok {
			description, i = readDescription(body, i, rest)
		}
	}
	return name, description
}

// isFence says whether a line closes or opens the frontmatter: three bars and nothing else but
// white space after them.
func isFence(line string) bool { return strings.TrimRight(line, " \t") == "---" }

// unquote is a value without the white space and the quotes at its ends.
func unquote(value string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), `"'`))
}

// readDescription reads the description whose key line is body[at], with rest after the key, and
// returns it with the index of the last line it used.
func readDescription(body []string, at int, rest string) (string, int) {
	value := strings.TrimSpace(rest)
	switch value {
	case ">", "|", ">-", "|-":
		pieces, last := indented(body, at+1, true)
		return strings.TrimSpace(strings.Join(pieces, " ")), last
	}
	if quote, ok := openQuote(value); ok {
		var pieces []string
		for i := at + 1; i < len(body); i++ {
			line := body[i]
			if strings.TrimSpace(line) == "" {
				continue
			}
			if line[0] != ' ' && line[0] != '\t' {
				break
			}
			piece := strings.TrimSpace(line)
			if mappingKey.MatchString(piece) {
				break
			}
			pieces = append(pieces, piece)
			if strings.HasSuffix(piece, quote) {
				return unquote(strings.Join(append([]string{value}, pieces...), " ")), i
			}
		}
	}
	return unquote(value), at
}

// openQuote says whether value begins with a quote that it does not end with, so that the value
// goes on over the lines below, and which quote it is.
func openQuote(value string) (string, bool) {
	for _, quote := range []string{`"`, `'`} {
		if strings.HasPrefix(value, quote) {
			return quote, len(value) == 1 || !strings.HasSuffix(value, quote)
		}
	}
	return "", false
}

// indented returns the trimmed lines that go on from body[from]: the ones that begin with white
// space, and the blank ones when blanks says so, up to the first line that is neither; and the
// index of the last line taken. Blank lines are dropped from the pieces unless blanks says to keep
// them. The line that ended the run is not taken.
func indented(body []string, from int, blanks bool) ([]string, int) {
	var pieces []string
	last := from - 1
	for i := from; i < len(body); i++ {
		line := body[i]
		blank := strings.TrimSpace(line) == ""
		if !blank && line[0] != ' ' && line[0] != '\t' {
			break
		}
		last = i
		if blank && !blanks {
			continue
		}
		pieces = append(pieces, strings.TrimSpace(line))
	}
	return pieces, last
}
