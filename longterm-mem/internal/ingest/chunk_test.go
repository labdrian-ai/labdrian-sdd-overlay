package ingest

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// repeatToBytes returns a paragraph of prose padded to exactly n bytes,
// using ASCII filler so byte length equals rune length.
func repeatToBytes(n int) string {
	const word = "lorem ipsum dolor sit amet consectetur adipiscing elit "
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(word)
	}
	return b.String()[:n]
}

// mustChunk calls ChunkText and fails the test on error, so call sites
// that only care about the happy path don't each need their own check.
func mustChunk(t *testing.T, text string, maxBytes, minBytes int) []Chunk {
	t.Helper()
	chunks, err := ChunkText(text, maxBytes, minBytes)
	if err != nil {
		t.Fatalf("ChunkText returned an error: %v", err)
	}
	return chunks
}

func TestChunkTextBoundarySizes(t *testing.T) {
	sizes := []int{479, 480, 481, 1599, 1600, 1601}
	for _, n := range sizes {
		chunks := mustChunk(t, repeatToBytes(n), DefaultMaxChunkBytes, DefaultMinChunkBytes)
		if len(chunks) == 0 {
			t.Fatalf("size %d: got zero chunks", n)
		}
		for i, c := range chunks {
			if len(c.Text) > DefaultMaxChunkBytes {
				t.Errorf("size %d chunk %d: len %d exceeds max %d", n, i, len(c.Text), DefaultMaxChunkBytes)
			}
		}
		// A single paragraph at or under max must not be split.
		if n <= DefaultMaxChunkBytes && len(chunks) != 1 {
			t.Errorf("size %d: expected 1 chunk, got %d", n, len(chunks))
		}
	}
}

func TestChunkTextAtomicCodeBlockLargerThanMax(t *testing.T) {
	body := "```\n" + repeatToBytes(DefaultMaxChunkBytes+500) + "\n```\n"
	chunks := mustChunk(t, body, DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected the oversized fenced block to be force-split, got %d chunks", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > DefaultMaxChunkBytes {
			t.Errorf("chunk %d: len %d exceeds max %d", i, len(c.Text), DefaultMaxChunkBytes)
		}
		if c.Split == SplitNone {
			t.Errorf("chunk %d: forced piece of an oversized block must not record Split: none", i)
		}
	}
}

func TestChunkTextTableLargerThanMax(t *testing.T) {
	var b strings.Builder
	row := "| col1 | col2 | col3 | some filler text to widen the row |\n"
	for b.Len() < DefaultMaxChunkBytes+300 {
		b.WriteString(row)
	}
	chunks := mustChunk(t, b.String(), DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected the oversized table to be force-split, got %d chunks", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > DefaultMaxChunkBytes {
			t.Errorf("chunk %d: len %d exceeds max %d", i, len(c.Text), DefaultMaxChunkBytes)
		}
	}
}

func TestChunkTextNoBlankLines(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("this is a line of prose with no blank lines between them at all\n")
	}
	chunks := mustChunk(t, b.String(), DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for a long no-blank-line document, got %d", len(chunks))
	}
	assertContiguousCoverage(t, b.String(), chunks)
}

func TestChunkTextCRLFNormalized(t *testing.T) {
	text := "line one\r\nline two\r\nline three\r\n"
	chunks := mustChunk(t, text, DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if strings.Contains(chunks[0].Text, "\r") {
		t.Errorf("CRLF must be normalized to LF, got %q", chunks[0].Text)
	}
	want := "line one\nline two\nline three\n"
	if chunks[0].Text != want {
		t.Errorf("body must otherwise be verbatim: got %q want %q", chunks[0].Text, want)
	}
}

func TestChunkTextMultibyteRuneAtForcedCut(t *testing.T) {
	// Build a single oversized paragraph (no sentence/line boundary) that
	// straddles a multibyte rune right around the max-byte cut point.
	var b strings.Builder
	b.WriteString(strings.Repeat("x", DefaultMaxChunkBytes-1))
	b.WriteString("é") // 2-byte rune straddling the cut at DefaultMaxChunkBytes
	b.WriteString(strings.Repeat("y", 200))
	text := b.String()
	chunks := mustChunk(t, text, DefaultMaxChunkBytes, DefaultMinChunkBytes)
	for i, c := range chunks {
		if !isValidUTF8Chunk(c.Text) {
			t.Errorf("chunk %d is not valid UTF-8 at a rune boundary: %q", i, c.Text)
		}
	}
	assertContiguousCoverage(t, text, chunks)
}

func TestChunkTextEmptyInput(t *testing.T) {
	chunks := mustChunk(t, "", DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) != 0 {
		t.Errorf("expected zero chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkTextWhitespaceOnlyInput(t *testing.T) {
	chunks := mustChunk(t, "   \n\n\t\n  ", DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) != 0 {
		t.Errorf("expected zero chunks for whitespace-only input, got %d", len(chunks))
	}
}

func TestChunkTextHeadingWithLongProseNotOneOversizedUnit(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Heading\n\n")
	for b.Len() < 40*1024 {
		b.WriteString("This is a sentence of ordinary prose that keeps the document flowing. ")
	}
	chunks := mustChunk(t, b.String(), DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for 40KB of prose, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > DefaultMaxChunkBytes {
			t.Errorf("chunk %d: len %d exceeds max %d", i, len(c.Text), DefaultMaxChunkBytes)
		}
	}
}

func TestChunkTextMetadataMonotonicAndSharedIdentity(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Top\n\n")
	for b.Len() < 6000 {
		b.WriteString("Paragraph text that will eventually be split across several chunks.\n\n")
	}
	chunks := mustChunk(t, b.String(), DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	assertContiguousCoverage(t, b.String(), chunks)
	for i, c := range chunks {
		if c.Index != i+1 {
			t.Errorf("chunk %d: Index = %d, want %d", i, c.Index, i+1)
		}
		if c.Total != len(chunks) {
			t.Errorf("chunk %d: Total = %d, want %d", i, c.Total, len(chunks))
		}
		// This shape (blank-line-separated paragraphs, absorbed gaps) once
		// let the max-byte bound slip silently.
		if len(c.Text) > DefaultMaxChunkBytes {
			t.Errorf("chunk %d: len %d exceeds max %d", i, len(c.Text), DefaultMaxChunkBytes)
		}
	}
}

func TestChunkTextPathReflectsHeadingBreadcrumb(t *testing.T) {
	text := "# A\n\nintro\n\n## B\n\n" + repeatToBytes(2000)
	chunks := mustChunk(t, text, DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	sawA := false
	sawAB := false
	for _, c := range chunks {
		switch c.Path {
		case "A":
			sawA = true
		case "A > B":
			sawAB = true
		}
	}
	if !sawA || !sawAB {
		t.Errorf("expected chunk paths to include both %q and %q, got: %+v", "A", "A > B", pathsOf(chunks))
	}
}

func pathsOf(chunks []Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Path
	}
	return out
}

func isValidUTF8Chunk(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// TestChunkHeader_WiresPositionSpanPathSplit: confirm every chunk-specific
// field lands on Header, and document-level fields pass through unchanged.
func TestChunkHeader_WiresPositionSpanPathSplit(t *testing.T) {
	c := Chunk{Text: "body", Index: 2, Total: 5, Start: 10, End: 14, Path: "A > B", Split: SplitForcedLine}
	base := Header{SourceID: "shared-id", SourceSHA256: "shared-sha", IsManifest: true}
	got := c.Header(base)

	if got.IsManifest {
		t.Error("Chunk.Header must clear IsManifest -- a chunk record is never a manifest")
	}
	if got.ChunkN != 2 || got.ChunkTotal != 5 {
		t.Errorf("ChunkN/ChunkTotal = %d/%d, want 2/5", got.ChunkN, got.ChunkTotal)
	}
	if got.ChunkSpanStart != 10 || got.ChunkSpanEnd != 14 {
		t.Errorf("ChunkSpanStart/End = %d/%d, want 10/14", got.ChunkSpanStart, got.ChunkSpanEnd)
	}
	if got.ChunkPath != "A > B" {
		t.Errorf("ChunkPath = %q, want %q", got.ChunkPath, "A > B")
	}
	if got.Split != SplitForcedLine {
		t.Errorf("Split = %q, want %q", got.Split, SplitForcedLine)
	}
	if got.SourceID != "shared-id" || got.SourceSHA256 != "shared-sha" {
		t.Errorf("document-level fields must pass through unchanged: SourceID=%q SourceSHA256=%q", got.SourceID, got.SourceSHA256)
	}
}

// TestChunkText_WhitespacePaddingCannotDefeatMaxBound: a huge blank-line
// run between two paragraphs must not smuggle uncounted bytes into a chunk.
func TestChunkText_WhitespacePaddingCannotDefeatMaxBound(t *testing.T) {
	text := "para\n" + strings.Repeat("\n", 50000) + "tail para\n"
	chunks := mustChunk(t, text, DefaultMaxChunkBytes, DefaultMinChunkBytes)
	for i, c := range chunks {
		if len(c.Text) > DefaultMaxChunkBytes {
			t.Errorf("chunk %d: len %d exceeds max %d -- whitespace padding defeated the size bound", i, len(c.Text), DefaultMaxChunkBytes)
		}
	}
	assertContiguousCoverage(t, text, chunks)
}

// TestForceSplit_NoSubMinBytesTrailingFragment: a block just over maxBytes
// (e.g. 1601) must not leave a near-empty final fragment (1600 + 1).
func TestForceSplit_NoSubMinBytesTrailingFragment(t *testing.T) {
	chunks := mustChunk(t, repeatToBytes(DefaultMaxChunkBytes+1), DefaultMaxChunkBytes, DefaultMinChunkBytes)
	// A block of exactly maxBytes+1 must balance into two chunks both
	// within [minBytes, maxBytes], not maxBytes + a 1-byte orphan.
	for i, c := range chunks {
		if len(c.Text) < DefaultMinChunkBytes || len(c.Text) > DefaultMaxChunkBytes {
			t.Errorf("chunk %d: len %d outside [%d, %d]", i, len(c.Text), DefaultMinChunkBytes, DefaultMaxChunkBytes)
		}
	}
}

// TestChunkText_RefusesBeyondMaxChunks: a source over MaxChunks is refused.
func TestChunkText_RefusesBeyondMaxChunks(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= MaxChunks+10; i++ {
		// Unique heading text per section so blocks never share a path
		// and merge into fewer, larger chunks.
		fmt.Fprintf(&b, "## H%d\n\nshort body\n\n", i)
	}
	_, err := ChunkText(b.String(), DefaultMaxChunkBytes, DefaultMinChunkBytes)
	if err == nil {
		t.Fatal("expected an error for a source producing more than MaxChunks chunks, got nil")
	}
	if !errors.Is(err, ErrTooManyChunks) {
		t.Errorf("expected errors.Is(err, ErrTooManyChunks), got %v", err)
	}
}

// assertContiguousCoverage asserts chunk spans, ordered by Start, are
// contiguous and together cover the whole normalized source with no gap or
// overlap.
func assertContiguousCoverage(t *testing.T, normalizedSource string, chunks []Chunk) {
	t.Helper()
	want := normalizeCRLF(normalizedSource)
	if len(chunks) == 0 {
		return
	}
	if chunks[0].Start != 0 {
		t.Errorf("first chunk must start at 0, got %d", chunks[0].Start)
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Start != chunks[i-1].End {
			t.Errorf("gap/overlap between chunk %d (end=%d) and chunk %d (start=%d)", i-1, chunks[i-1].End, i, chunks[i].Start)
		}
	}
	last := chunks[len(chunks)-1]
	if last.End != len(want) {
		t.Errorf("last chunk must end at %d (source length), got %d", len(want), last.End)
	}
}
