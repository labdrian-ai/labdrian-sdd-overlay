package projection

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// These tests reach the helpers behind Project directly, so each rule of the
// sanitizer, the bound, and the capability line is pinned on its own, with the
// awkward inputs that Project's tests only sample. Characters that the test
// layer would decode from an escape are built from their code points.

func TestSanitizeLine(t *testing.T) {
	r := func(code rune) string { return string(code) }
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is unchanged", "goal-1", "goal-1"},
		{"the empty string", "", ""},
		{"non-ASCII text is kept", "línea-日本語-" + r(0x1F600), "línea-日本語-" + r(0x1F600)},
		{"combining marks are kept", "e" + r(0x0301), "e" + r(0x0301)},

		// Line breaks and other white space become one space, so a field can
		// never start a new line of the projected text.
		{"line breaks collapse to one space", "a\nb\r\nc", "a b c"},
		{"tabs and vertical white space collapse", "a\t\v\f b", "a b"},
		{"non-breaking and ideographic spaces collapse", "a" + r(0x00A0) + "b" + r(0x3000) + "c", "a b c"},
		{"next line and the Unicode separators collapse", "a" + r(0x0085) + "b" + r(0x2028) + "c" + r(0x2029) + "d", "a b c d"},
		{"leading and trailing white space is trimmed", "  \n a b \t ", "a b"},
		{"runs of white space become a single space", "a \n\t \r  b", "a b"},

		// Characters that draw nothing, or steer how text is drawn, are removed.
		{"control characters are removed", "a\x00b\x1bc\x7fd", "abcd"},
		{"a terminal escape loses its escape", "\x1b[31mred\x1b[0m", "[31mred[0m"},
		{"C1 control characters are removed", "a" + r(0x009B) + "b" + r(0x0090) + "c", "abc"},
		{"zero-width characters are removed", "a" + r(0x200B) + "b" + r(0x200D) + "c" + r(0xFEFF) + "d", "abcd"},
		{"bidirectional controls are removed", "a" + r(0x202E) + "b" + r(0x2066) + "c" + r(0x2069) + "d", "abcd"},
		{"a soft hyphen is removed", "a" + r(0x00AD) + "b", "ab"},
		{"tag characters are removed", "a" + r(0xE0041) + "b", "ab"},
		{"private-use characters are removed", "a" + r(0xE000) + "b", "ab"},
		{"invalid UTF-8 is removed", "a\xffb\xc3", "ab"},
		{"nothing printable is left", "\x00\n\t" + r(0x200B), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeLine(tt.in); got != tt.want {
				t.Errorf("sanitizeLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestClip(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip under the limit = %q, want it unchanged", got)
	}
	if got := clip("exactly-ten", 11); got != "exactly-ten" {
		t.Errorf("clip at the limit = %q, want it unchanged", got)
	}
	if got := clip("abcdefghij", 4); got != "abcd..." {
		t.Errorf("clip over the limit = %q, want the first 4 characters and an ellipsis", got)
	}
	// Whole characters only: a cut never lands inside a multi-byte one.
	got := clip(strings.Repeat(string(rune(0x1F600)), 10), 3)
	if got != strings.Repeat(string(rune(0x1F600)), 3)+"..." || !utf8.ValidString(got) {
		t.Errorf("clip of four-byte characters = %q, want three whole characters and an ellipsis", got)
	}
}

func TestBoundContext(t *testing.T) {
	cut := MaxContextBytes - len(truncationMarker)

	t.Run("text within the bound is unchanged", func(t *testing.T) {
		for _, size := range []int{0, 1, MaxContextBytes - 1, MaxContextBytes} {
			in := strings.Repeat("a", size)
			if got := boundContext(in); got != in {
				t.Errorf("boundContext of %d bytes changed the text", size)
			}
		}
	})

	t.Run("text over the bound is cut and marked", func(t *testing.T) {
		in := strings.Repeat("a", MaxContextBytes+1)
		got := boundContext(in)
		if len(got) != MaxContextBytes {
			t.Fatalf("boundContext = %d bytes, want exactly the bound %d", len(got), MaxContextBytes)
		}
		if want := strings.Repeat("a", cut) + truncationMarker; got != want {
			t.Errorf("boundContext did not keep the longest prefix that leaves room for the marker")
		}
	})

	t.Run("the marker says what happened", func(t *testing.T) {
		for _, want := range []string{"labdrian", "truncated", "16384", "bytes"} {
			if !strings.Contains(truncationMarker, want) {
				t.Errorf("truncationMarker %q does not mention %q", truncationMarker, want)
			}
		}
		if !strings.HasPrefix(truncationMarker, "\n") {
			t.Errorf("truncationMarker %q does not start on a line of its own", truncationMarker)
		}
	})

	// The cut lands inside a four-byte character wherever the character starts
	// within the last three bytes before it: the whole character must go.
	for offset := 1; offset <= 3; offset++ {
		t.Run("a character straddling the cut, starting "+string(rune('0'+offset))+" bytes before it", func(t *testing.T) {
			emoji := string(rune(0x1F600))
			prefix := strings.Repeat("a", cut-offset)
			in := prefix + emoji + strings.Repeat("b", 500)
			got := boundContext(in)
			if !utf8.ValidString(got) {
				t.Fatalf("boundContext cut inside a character: %q", got[len(got)-len(truncationMarker)-8:])
			}
			if got != prefix+truncationMarker {
				t.Errorf("boundContext kept part of the straddling character")
			}
			if len(got) > MaxContextBytes {
				t.Errorf("boundContext = %d bytes, over the bound", len(got))
			}
		})
	}

	t.Run("a character that ends exactly at the cut is kept", func(t *testing.T) {
		emoji := string(rune(0x1F600))
		prefix := strings.Repeat("a", cut-len(emoji)) + emoji
		in := prefix + strings.Repeat("b", 500)
		if got := boundContext(in); got != prefix+truncationMarker {
			t.Errorf("boundContext dropped a character that fits")
		}
	})
}

func TestCapabilityLimitsLine(t *testing.T) {
	declaration := capability.Declaration{Target: capability.TargetClaude, Claims: []capability.Claim{
		{Capability: capability.Installation, Status: capability.Supported},
		{Capability: capability.Projection, Status: capability.Partial},
		{Capability: capability.Dispatch, Status: capability.Supported},
		{Capability: capability.Cancellation, Status: capability.Unsupported},
	}}
	if got, want := capabilityLimitsLine(declaration), "capability limits (claude): projection=partial, cancellation=unsupported"; got != want {
		t.Errorf("capabilityLimitsLine() = %q, want %q", got, want)
	}

	// When a claim is upgraded the line follows the declaration.
	declaration.Claims[1].Status = capability.Supported
	if got, want := capabilityLimitsLine(declaration), "capability limits (claude): cancellation=unsupported"; got != want {
		t.Errorf("after an upgrade capabilityLimitsLine() = %q, want %q", got, want)
	}

	// With nothing left to declare, the line says so instead of vanishing.
	declaration.Claims[3].Status = capability.Supported
	if got := capabilityLimitsLine(declaration); !strings.Contains(got, "none") {
		t.Errorf("capabilityLimitsLine() with every claim supported = %q, want it to say there is no limit", got)
	}
}
