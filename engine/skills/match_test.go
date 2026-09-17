package skills

import "testing"

func TestNormalizeSlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"mixed case", "MixedCase", "mixedcase"},
		{"spaces", "hello world", "hello-world"},
		{"underscores", "hello_world", "hello-world"},
		{"punctuation runs", "hello!!!world???", "hello-world"},
		{"leading trailing separators", "  --hello world--  ", "hello-world"},
		{"unicode input", "café société", "caf-soci-t"},
		{"empty string", "", ""},
		{"all punctuation input", "!!!___...", ""},
		{
			"48-byte truncation at a - boundary",
			"this-is-a-very-long-slug-that-exceeds-the-forty-eight-byte-limit-by-a-lot",
			"this-is-a-very-long-slug-that-exceeds-the-forty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeSlug(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) > 48 {
				t.Fatalf("NormalizeSlug(%q) exceeded 48 bytes: %q (%d bytes)", tc.in, got, len(got))
			}
		})
	}
}

func TestNormalizeSlugIdempotence(t *testing.T) {
	inputs := []string{
		"MixedCase",
		"hello world",
		"hello_world",
		"hello!!!world???",
		"  --hello world--  ",
		"café société",
		"",
		"!!!___...",
		"this-is-a-very-long-slug-that-exceeds-the-forty-eight-byte-limit-by-a-lot",
	}

	for _, in := range inputs {
		first := NormalizeSlug(in)
		second := NormalizeSlug(first)
		if first != second {
			t.Fatalf("NormalizeSlug not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}

func TestNormalizeSlugCharsetInvariant(t *testing.T) {
	inputs := []string{
		"MixedCase",
		"hello world",
		"café société",
		"this-is-a-very-long-slug-that-exceeds-the-forty-eight-byte-limit-by-a-lot",
	}

	for _, in := range inputs {
		got := NormalizeSlug(in)
		for i, r := range got {
			isLower := r >= 'a' && r <= 'z'
			isDigit := r >= '0' && r <= '9'
			isDash := r == '-'
			if !isLower && !isDigit && !isDash {
				t.Fatalf("NormalizeSlug(%q)[%d] = %q, not in [a-z0-9-]", in, i, r)
			}
		}
		if len(got) > 0 && (got[0] == '-' || got[len(got)-1] == '-') {
			t.Fatalf("NormalizeSlug(%q) = %q has leading/trailing '-'", in, got)
		}
	}
}
