package memory_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The budget is a derived number (design: about 480 characters per row so that the response ceiling
// binds only on unusual shapes); a change to it changes every snippet a person reads, so it is pinned.
func TestSnippetBudgetIsFourHundredEighty(t *testing.T) {
	if memory.SnippetBudget != 480 {
		t.Fatalf("SnippetBudget = %d, want 480", memory.SnippetBudget)
	}
}

func TestSnippetAtLeavesABodyThatFitsWhole(t *testing.T) {
	for _, content := range []string{"", "short", strings.Repeat("x", 100)} {
		got, truncated := memory.SnippetAt(content, 50, 100)
		if got != content || truncated {
			t.Errorf("SnippetAt(%d bytes, 50, 100) = %q, %v, want the body whole and not truncated", len(content), got, truncated)
		}
	}
}

func TestSnippetAtCentresTheWindowOnTheOffsetAndMarksBothCuts(t *testing.T) {
	content := strings.Repeat("a", 500) + "NEEDLE" + strings.Repeat("b", 500)
	got, truncated := memory.SnippetAt(content, 500, 100)
	if !truncated {
		t.Fatal("a window cut out of a longer body must say it is a fragment")
	}
	want := memory.TruncationMark + content[450:550] + memory.TruncationMark
	if got != want {
		t.Fatalf("SnippetAt = %q, want %q", got, want)
	}
	if !strings.Contains(got, "NEEDLE") {
		t.Errorf("the window %q does not hold the match it was centred on", got)
	}
}

func TestSnippetAtPullsTheWindowBackInsideTheBody(t *testing.T) {
	content := strings.Repeat("x", 1000)
	t.Run("at the head there is no mark before the window", func(t *testing.T) {
		got, truncated := memory.SnippetAt(content, 0, 100)
		if want := content[:100] + memory.TruncationMark; got != want || !truncated {
			t.Fatalf("SnippetAt = %q, %v, want %q, true", got, truncated, want)
		}
	})
	t.Run("a near-head offset starts at the head", func(t *testing.T) {
		got, _ := memory.SnippetAt(content, 20, 100)
		if want := content[:100] + memory.TruncationMark; got != want {
			t.Fatalf("SnippetAt = %q, want %q", got, want)
		}
	})
	t.Run("at the tail there is no mark after the window", func(t *testing.T) {
		got, truncated := memory.SnippetAt(content, 1000, 100)
		if want := memory.TruncationMark + content[900:]; got != want || !truncated {
			t.Fatalf("SnippetAt = %q, %v, want %q, true", got, truncated, want)
		}
	})
	t.Run("an offset past the end is treated like the tail", func(t *testing.T) {
		got, _ := memory.SnippetAt(content, 5000, 100)
		if want := memory.TruncationMark + content[900:]; got != want {
			t.Fatalf("SnippetAt = %q, want %q", got, want)
		}
	})
}

// One byte left out is still a cut: the mark appears as soon as any part of the body is missing at
// that edge, not only when a lot is.
func TestSnippetAtMarksACutOfASingleByte(t *testing.T) {
	content := strings.Repeat("x", 1000)
	t.Run("one byte missing before the window", func(t *testing.T) {
		got, _ := memory.SnippetAt(content, 51, 100) // the window is content[1:101]
		if want := memory.TruncationMark + content[1:101] + memory.TruncationMark; got != want {
			t.Fatalf("SnippetAt = %q, want %q", got, want)
		}
	})
	t.Run("one byte missing after the window", func(t *testing.T) {
		got, _ := memory.SnippetAt(content, 949, 100) // the window is content[899:999]
		if want := memory.TruncationMark + content[899:999] + memory.TruncationMark; got != want {
			t.Fatalf("SnippetAt = %q, want %q", got, want)
		}
	})
}

// The mark is the one character a reader and a client both see in the text; it is pinned so a change
// of it is a decision, not a side effect.
func TestTruncationMarkIsAnEllipsis(t *testing.T) {
	if memory.TruncationMark != "…" {
		t.Fatalf("TruncationMark = %q, want the single ellipsis character", memory.TruncationMark)
	}
}

// A snippet is text a person reads: a window that begins or ends inside a multi-byte character must
// move to the character's edge, not render a replacement glyph.
func TestSnippetAtNeverCutsACharacterInHalf(t *testing.T) {
	content := strings.Repeat("é", 1000) // two bytes each: every rune starts on an even index
	got, truncated := memory.SnippetAt(content, 1001, 100)
	if !truncated {
		t.Fatal("the window is a fragment")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("SnippetAt cut a character in half: %q", got)
	}
	// start = 1001 - 50 = 951 (inside a rune) moves back to 950; end = 951 + 100 = 1051 (inside a
	// rune) moves forward to 1052.
	want := memory.TruncationMark + content[950:1052] + memory.TruncationMark
	if got != want {
		t.Fatalf("SnippetAt = %q, want the window widened to the characters' edges %q", got, want)
	}
}
