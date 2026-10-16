package propagator

import "strings"

// registryLine is one line of a registry, with where it stands among the marker blocks of the
// registry. A marker block is whatever lies from a line that starts with "<!-- BEGIN:" to the next
// line that starts with "<!-- END:", the markers of any contract, ours or a foreign one's: a
// scan that must leave a block alone does not need to know whose it is.
type registryLine struct {
	// Index is the position of the line among the lines of the registry, from 0.
	Index int
	// Text is the line as it is, including any space around it and a CR before the line feed.
	Text string
	// Trimmed is the line with the space around it taken off.
	Trimmed string
	// Opens says that the line is an opening marker, Closes that it is a closing one.
	Opens, Closes bool
	// InBlock says that the line is inside a block once it has been read: an opening marker is
	// inside it, a closing marker is not.
	InBlock bool
}

// lineScanner walks the lines of a registry, in order, and keeps the one thing every walk over a
// registry needs to know: whether a line is inside a marker block. It does not nest: a block is
// closed by the first closing marker, and a closing marker with no block open closes nothing.
// A block that is never closed holds the rest of the registry.
type lineScanner struct {
	lines   []string
	next    int
	inBlock bool
	line    registryLine
}

func newLineScanner(lines []string) *lineScanner { return &lineScanner{lines: lines} }

// Next moves to the next line and says whether there was one. Line is the line it moved to.
func (s *lineScanner) Next() bool {
	if s.next >= len(s.lines) {
		return false
	}
	text := s.lines[s.next]
	trimmed := strings.TrimSpace(text)
	opens := strings.HasPrefix(trimmed, beginMarkerPrefix)
	closes := strings.HasPrefix(trimmed, endMarkerPrefix)
	switch {
	case opens:
		s.inBlock = true
	case closes:
		s.inBlock = false
	}
	s.line = registryLine{Index: s.next, Text: text, Trimmed: trimmed, Opens: opens, Closes: closes, InBlock: s.inBlock}
	s.next++
	return true
}

// Line is the line Next moved to.
func (s *lineScanner) Line() registryLine { return s.line }
