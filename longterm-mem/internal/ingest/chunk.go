package ingest

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Chunk is one deterministically-bounded piece of a normalized source
// document, produced by ChunkText. Index/Total/Start/End/Path/Split are
// wired into a Header's chunk-specific fields via Chunk.Header.
type Chunk struct {
	Text  string
	Index int // 1-based position among this document's chunks
	Total int // total chunk count for this document
	Start int // byte offset into the normalized (CRLF -> LF) source
	End   int // exclusive byte offset into the normalized source
	Path  string
	Split SplitKind
}

// Header returns base with this chunk's position, span, path, and split
// strategy applied, ready for Header.Render(). base should already carry
// the document-level fields (Source-*, Ingested-*).
func (c Chunk) Header(base Header) Header {
	base.IsManifest = false
	base.ChunkN = c.Index
	base.ChunkTotal = c.Total
	base.ChunkSpanStart = c.Start
	base.ChunkSpanEnd = c.End
	base.ChunkPath = c.Path
	base.Split = c.Split
	return base
}

// normalizeCRLF folds CRLF to LF. No other byte is altered.
func normalizeCRLF(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

type blockKind int

const (
	blockParagraph blockKind = iota
	blockHeading
	blockCode
	blockTable
)

type block struct {
	kind    blockKind
	start   int
	end     int // exclusive
	heading string
	level   int
}

func isATXHeading(line string) bool {
	i := 0
	for i < len(line) && i < 7 && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return false
	}
	return i == len(line) || line[i] == ' '
}

func headingLevelAndText(line string) (int, string) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	return i, strings.TrimSpace(line[i:])
}

func isFenceMarker(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "```") {
		return "```", true
	}
	if strings.HasPrefix(t, "~~~") {
		return "~~~", true
	}
	return "", false
}

func isTableLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "|")
}

// splitLinesKeepEnds splits s into lines, each retaining its trailing "\n"
// (the last line keeps none if s has no trailing newline), so line byte
// lengths sum exactly to len(s).
func splitLinesKeepEnds(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// segmentBlocks segments a normalized (LF-only) document into an ordered
// list of blocks: an ATX heading starts a new block; a fenced code block is
// atomic; a contiguous run of '|'-leading lines (table) is atomic;
// otherwise a block is a blank-line-delimited paragraph.
func segmentBlocks(text string) []block {
	lines := splitLinesKeepEnds(text)
	var blocks []block
	offset := 0
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimRight(lines[i], "\n")
		switch {
		case isATXHeading(trimmed):
			level, heading := headingLevelAndText(trimmed)
			start := offset
			offset += len(lines[i])
			i++
			blocks = append(blocks, block{kind: blockHeading, start: start, end: offset, heading: heading, level: level})
		case func() bool { _, ok := isFenceMarker(trimmed); return ok }():
			fence, _ := isFenceMarker(trimmed)
			start := offset
			offset += len(lines[i])
			i++
			for i < len(lines) {
				l := lines[i]
				offset += len(l)
				i++
				if f, ok := isFenceMarker(strings.TrimRight(l, "\n")); ok && f == fence {
					break
				}
			}
			blocks = append(blocks, block{kind: blockCode, start: start, end: offset})
		case isTableLine(trimmed):
			start := offset
			for i < len(lines) && isTableLine(strings.TrimRight(lines[i], "\n")) {
				offset += len(lines[i])
				i++
			}
			blocks = append(blocks, block{kind: blockTable, start: start, end: offset})
		case trimmed == "":
			offset += len(lines[i])
			i++
		default:
			start := offset
			for i < len(lines) {
				l := strings.TrimRight(lines[i], "\n")
				if l == "" || isATXHeading(l) || isTableLine(l) {
					break
				}
				if _, ok := isFenceMarker(l); ok {
					break
				}
				offset += len(lines[i])
				i++
			}
			blocks = append(blocks, block{kind: blockParagraph, start: start, end: offset})
		}
	}
	return closeBlockGaps(blocks, len(text))
}

// closeBlockGaps absorbs every inter-block gap (a blank-line run the
// segmentation switch above assigns to no block) into the following
// block, and the doc bounds into the first/last block, BEFORE any size
// decision runs -- the accumulation/force-split bounds below only see
// block lengths, so a gap still open at that point grows a chunk past
// maxBytes with nothing left to catch it.
func closeBlockGaps(blocks []block, textLen int) []block {
	if len(blocks) == 0 {
		return blocks
	}
	blocks[0].start = 0
	for i := 1; i < len(blocks); i++ {
		blocks[i].start = blocks[i-1].end
	}
	blocks[len(blocks)-1].end = textLen
	return blocks
}

// updateHeadingStack pushes/pops heading levels so the stack always holds
// the breadcrumb in effect after seeing a heading of the given level.
func updateHeadingStack(stack []string, level int, text string) []string {
	if level > len(stack) {
		for len(stack) < level-1 {
			stack = append(stack, "")
		}
		return append(stack[:level-1], text)
	}
	stack = stack[:level-1]
	return append(stack, text)
}

func pathOf(stack []string) string {
	var parts []string
	for _, s := range stack {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " > ")
}

// ErrTooManyChunks is returned when a source would produce more than
// MaxChunks chunks; per doc.go, such a source is refused rather than
// silently renumbered past the four-digit ordinal space.
var ErrTooManyChunks = errors.New("ingest: source would exceed MaxChunks")

// ChunkText splits normalized source text into deterministically-bounded
// chunks per the boundary algorithm: CRLF normalization, block
// segmentation, greedy accumulation respecting minBytes/maxBytes, and a
// forced split (sentence -> line -> byte, moved to the nearest rune
// boundary) for any block that alone exceeds maxBytes. It returns
// ErrTooManyChunks if the result would exceed MaxChunks.
func ChunkText(text string, maxBytes, minBytes int) ([]Chunk, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxChunkBytes
	}
	if minBytes <= 0 {
		minBytes = DefaultMinChunkBytes
	}
	text = normalizeCRLF(text)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	blocks := segmentBlocks(text)
	var pieces []rawChunk
	headingStack := []string{}
	currentPath := ""
	// Recomputed only when a heading changes headingStack -- pathOf
	// Joins the whole (untrusted, unbounded) breadcrumb, so calling it
	// per-block made total work blocks x breadcrumb length.
	headingPath := ""

	curStart, curEnd := -1, -1
	flush := func() {
		if curEnd < 0 || curEnd <= curStart {
			curStart, curEnd = -1, -1
			return
		}
		pieces = append(pieces, rawChunk{start: curStart, end: curEnd, path: currentPath, split: SplitNone})
		curStart, curEnd = -1, -1
	}

	for _, b := range blocks {
		if b.kind == blockHeading {
			headingStack = updateHeadingStack(headingStack, b.level, b.heading)
			headingPath = pathOf(headingStack)
		}
		blockPath := headingPath
		blockLen := b.end - b.start
		if blockLen > maxBytes {
			flush()
			currentPath = blockPath
			for _, fc := range forceSplit(text, b.start, b.end, maxBytes, minBytes) {
				fc.path = blockPath
				pieces = append(pieces, fc)
			}
			continue
		}
		if curEnd < 0 {
			curStart, curEnd = b.start, b.end
			currentPath = blockPath
			continue
		}
		if blockPath != currentPath {
			flush()
			curStart, curEnd = b.start, b.end
			currentPath = blockPath
			continue
		}
		if (curEnd-curStart)+blockLen <= maxBytes {
			curEnd = b.end
		} else {
			flush()
			curStart, curEnd = b.start, b.end
		}
	}
	flush()

	pieces = mergeUndersizedTrailing(pieces, minBytes, maxBytes)
	if len(pieces) > MaxChunks {
		return nil, fmt.Errorf("%w: %d chunks", ErrTooManyChunks, len(pieces))
	}

	chunks := make([]Chunk, len(pieces))
	for i, p := range pieces {
		chunks[i] = Chunk{
			Text:  text[p.start:p.end],
			Index: i + 1,
			Total: len(pieces),
			Start: p.start,
			End:   p.end,
			Path:  p.path,
			Split: p.split,
		}
	}
	return chunks, nil
}

type rawChunk struct {
	start, end int
	path       string
	split      SplitKind
}

// mergeUndersizedTrailing merges a final chunk under minBytes into the
// previous one, only when that stays at or under maxBytes -- the max
// bound is the harder invariant (doc.go), so an unmergeable remainder is
// left as its own short final chunk instead.
func mergeUndersizedTrailing(pieces []rawChunk, minBytes, maxBytes int) []rawChunk {
	if len(pieces) < 2 {
		return pieces
	}
	last := pieces[len(pieces)-1]
	if last.end-last.start >= minBytes {
		return pieces
	}
	prev := pieces[len(pieces)-2]
	merged := rawChunk{start: prev.start, end: last.end, path: prev.path, split: prev.split}
	if merged.end-merged.start > maxBytes {
		return pieces
	}
	out := make([]rawChunk, len(pieces)-1)
	copy(out, pieces[:len(pieces)-2])
	out[len(out)-1] = merged
	return out
}

// forceSplit splits text[start:end] (a single block larger than maxBytes)
// into pieces at or under maxBytes, in the documented fallback order: last
// sentence boundary at or before max, else last line boundary at or before
// max, else a hard byte cut moved back to the nearest UTF-8 rune boundary.
// A sub-minBytes trailing remainder is rebalanced into its predecessor
// (routine for a hard byte-count split), as long as that stays <= maxBytes.
func forceSplit(text string, start, end, maxBytes, minBytes int) []rawChunk {
	var out []rawChunk
	for start < end {
		remaining := end - start
		if remaining <= maxBytes {
			out = append(out, rawChunk{start: start, end: end, split: splitKindFor(out)})
			break
		}
		cut := start + maxBytes
		if remaining <= maxBytes+minBytes {
			// A full maxBytes cut leaves a sub-minBytes orphan remainder;
			// merging back isn't an option (remaining > maxBytes by
			// definition here), so balance the last two pieces instead.
			cut = start + (remaining - minBytes)
		}
		splitAt, kind := findSplitPoint(text, start, cut)
		if splitAt <= start {
			splitAt = cut
			kind = SplitForcedByte
		}
		out = append(out, rawChunk{start: start, end: splitAt, split: kind})
		start = splitAt
	}
	// Safety net: a natural sentence/line boundary can still leave a
	// sub-minBytes trailing fragment; merge it back within maxBytes.
	if n := len(out); n >= 2 && out[n-1].end-out[n-1].start < minBytes {
		merged := rawChunk{start: out[n-2].start, end: out[n-1].end, split: out[n-2].split}
		if merged.end-merged.start <= maxBytes {
			out[n-2] = merged
			out = out[:n-1]
		}
	}
	return out
}

// splitKindFor marks the final undersized remainder piece with the same
// forced classification as its siblings so a force-split block never
// reports Split: none for any of its pieces.
func splitKindFor(prior []rawChunk) SplitKind {
	if len(prior) == 0 {
		return SplitForcedByte
	}
	return prior[len(prior)-1].split
}

// findSplitPoint looks for the last sentence boundary, else the last line
// boundary, at or before cut (exclusive upper bound), within text[start:end
// search window]. It returns a byte offset moved to the nearest rune
// boundary and the SplitKind that produced it, or (start, "") if none found.
func findSplitPoint(text string, start, cut int) (int, SplitKind) {
	window := text[start:cut]
	if idx := lastSentenceBoundary(window); idx >= 0 {
		return moveToRuneBoundary(text, start+idx), SplitForcedSentence
	}
	if idx := strings.LastIndexByte(window, '\n'); idx >= 0 {
		return moveToRuneBoundary(text, start+idx+1), SplitForcedLine
	}
	return moveToRuneBoundary(text, cut), SplitForcedByte
}

// lastSentenceBoundary returns the byte offset just after the last
// '.'/'!'/'?' followed by whitespace within s, or -1 if none.
func lastSentenceBoundary(s string) int {
	best := -1
	for i := 0; i < len(s)-1; i++ {
		c := s[i]
		if (c == '.' || c == '!' || c == '?') && (s[i+1] == ' ' || s[i+1] == '\n' || s[i+1] == '\t') {
			best = i + 1
		}
	}
	return best
}

// moveToRuneBoundary moves offset back to the nearest UTF-8 rune boundary
// in text, so a hard byte cut never splits a multibyte rune.
func moveToRuneBoundary(text string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset >= len(text) {
		return len(text)
	}
	for offset > 0 && !utf8.RuneStart(text[offset]) {
		offset--
	}
	return offset
}
