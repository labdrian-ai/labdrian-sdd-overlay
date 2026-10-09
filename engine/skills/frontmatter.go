package skills

import (
	"bytes"
	"strings"
)

// FrontmatterFault says which fence of a SKILL.md or agent file is missing.
type FrontmatterFault int

const (
	// NoOpeningFence means the file does not start with a `---` line.
	NoOpeningFence FrontmatterFault = iota + 1
	// NoClosingFence means the file opens a frontmatter block and never closes it.
	NoClosingFence
)

func (f FrontmatterFault) String() string {
	switch f {
	case NoOpeningFence:
		return "file does not start with a `---` frontmatter fence"
	case NoClosingFence:
		return "file has no closing `---` frontmatter fence"
	}
	return "unknown frontmatter fault"
}

// FrontmatterError is the failure of ReadFrontmatter: a fence that is missing. It carries the
// fault so a caller can say it in its own words (the Pi package build and the Pi agent link
// each do) without matching on text.
type FrontmatterError struct {
	Fault FrontmatterFault
}

func (e *FrontmatterError) Error() string { return e.Fault.String() }

// FrontmatterEntry is one top-level key of a frontmatter block.
type FrontmatterEntry struct {
	// Key is the name before the first colon, trimmed.
	Key string
	// Value is what follows the colon, trimmed and as written: quotes kept, empty when the
	// value is a block that starts on the next line.
	Value string
	// Items is how many lines under the key start a list item (`- x`), indented or not.
	Items int
}

// Text is Value without one layer of matching single or double quotes.
func (e FrontmatterEntry) Text() string { return unquote(e.Value) }

// Frontmatter is the top-level reading of the block between the fences of a SKILL.md or of an
// agent file: the keys that start a line, in order, with the value written after the colon and
// the number of list items under each. It is the one shared reader of that shape (the Pi
// package build checks a skill's name with it, the Pi runtime checks the agent file it links).
//
// It is a line reader, not a YAML parser, and it is not the lint's reader: parseFrontmatterFields
// knows the five fields a skill must have, the metadata block and the shapes a description must
// not take, and the lint rules are written against it. It is also not the contract reader of
// engine/contract, which parses a different document (a managed contract's metadata, with
// strict inline lists). A key that is indented belongs to the key above it and is not read.
type Frontmatter struct {
	entries []FrontmatterEntry
}

// ReadFrontmatter reads the frontmatter of a whole file. The fences are those of SplitSkillFile:
// a leading byte-order mark and a carriage return are tolerated, as is white space after a fence,
// and a fence that is indented is not one. A file without a fence is a *FrontmatterError.
func ReadFrontmatter(data []byte) (Frontmatter, error) {
	block, _, fault := splitFences(data)
	if fault != 0 {
		return Frontmatter{}, &FrontmatterError{Fault: fault}
	}
	var fm Frontmatter
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// A line that starts with a dash is a list item of the key above it, whether it is
		// indented or not (YAML allows `tools:` followed by `- a` at the margin). A key
		// cannot start with a dash, so no key is lost to this branch.
		if strings.HasPrefix(strings.TrimSpace(line), "-") {
			if len(fm.entries) > 0 {
				fm.entries[len(fm.entries)-1].Items++
			}
			continue
		}
		// Any other indented line is the body of the key above it, which this reader does
		// not look into.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		fm.entries = append(fm.entries, FrontmatterEntry{Key: key, Value: value})
	}
	return fm, nil
}

// Entries are the top-level keys in the order of the file. The slice is a copy.
func (f Frontmatter) Entries() []FrontmatterEntry {
	return append([]FrontmatterEntry(nil), f.entries...)
}

// Entry is the first entry with the key, and whether there is one.
func (f Frontmatter) Entry(key string) (FrontmatterEntry, bool) {
	for _, e := range f.entries {
		if e.Key == key {
			return e, true
		}
	}
	return FrontmatterEntry{}, false
}

// splitFences cuts a file into the block between its two `---` lines and what follows. It is
// the one place that decides where a frontmatter is; SplitSkillFile and ReadFrontmatter both
// read it. fault is zero when both fences are there.
func splitFences(data []byte) (block, body string, fault FrontmatterFault) {
	data = bytes.TrimPrefix(data, utf8BOM)
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || !isFenceLine(lines[0]) {
		return "", "", NoOpeningFence
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if isFenceLine(lines[i]) {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return "", "", NoClosingFence
	}
	return strings.Join(lines[1:closeIdx], "\n"), strings.Join(lines[closeIdx+1:], "\n"), 0
}
