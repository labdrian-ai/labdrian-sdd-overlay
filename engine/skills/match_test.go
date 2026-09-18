package skills

import (
	"strings"
	"testing"
)

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

// literalMatchRegistry builds a Registry directly (not via ParseRegistry) with
// entries whose ID and Path are deliberately distinct, so exact-ID hits,
// exact-Path hits, and path.Base hits can be tested independently.
func literalMatchRegistry() Registry {
	return Registry{
		Version: "1",
		Skills: []Entry{
			{ID: "sdd-spec", Path: "sdd-spec"},
			{ID: "internal-nested-alias", Path: "tools/actual-skill-name"},
		},
	}
}

func TestMatchCandidate(t *testing.T) {
	t.Run("exact ID hit", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "sdd-spec")
		if !matched || path != "sdd-spec" {
			t.Fatalf("MatchCandidate(sdd-spec) = (%v, %q), want (true, %q)", matched, path, "sdd-spec")
		}
	})

	t.Run("exact Path hit", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "tools/actual-skill-name")
		if !matched || path != "tools/actual-skill-name" {
			t.Fatalf("MatchCandidate(tools/actual-skill-name) = (%v, %q), want (true, %q)", matched, path, "tools/actual-skill-name")
		}
	})

	t.Run("path.Base hit", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "actual-skill-name")
		if !matched || path != "tools/actual-skill-name" {
			t.Fatalf("MatchCandidate(actual-skill-name) = (%v, %q), want (true, %q)", matched, path, "tools/actual-skill-name")
		}
	})

	t.Run("case underscore space variant normalizes to a hit", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "SDD Spec")
		if !matched || path != "sdd-spec" {
			t.Fatalf("MatchCandidate(SDD Spec) = (%v, %q), want (true, %q)", matched, path, "sdd-spec")
		}
	})

	t.Run("no match on unrelated candidate", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "totally-unrelated-thing")
		if matched || path != "" {
			t.Fatalf("MatchCandidate(totally-unrelated-thing) = (%v, %q), want (false, \"\")", matched, path)
		}
	})

	t.Run("no match on substring near-miss", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "sdd-spec-review")
		if matched || path != "" {
			t.Fatalf("MatchCandidate(sdd-spec-review) = (%v, %q), want (false, \"\") — substring near-miss must not match", matched, path)
		}
	})

	t.Run("empty candidate string", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "")
		if matched || path != "" {
			t.Fatalf("MatchCandidate(\"\") = (%v, %q), want (false, \"\")", matched, path)
		}
	})

	t.Run("all-punctuation candidate string", func(t *testing.T) {
		matched, path := MatchCandidate(literalMatchRegistry(), "!!!___...")
		if matched || path != "" {
			t.Fatalf("MatchCandidate(!!!___...) = (%v, %q), want (false, \"\")", matched, path)
		}
	})

	t.Run("empty registry", func(t *testing.T) {
		matched, path := MatchCandidate(Registry{}, "sdd-spec")
		if matched || path != "" {
			t.Fatalf("MatchCandidate over empty registry = (%v, %q), want (false, \"\")", matched, path)
		}
	})

	t.Run("deterministic first-match-wins when two entries both match", func(t *testing.T) {
		reg := Registry{Skills: []Entry{
			{ID: "shared-slug", Path: "first-entry"},
			{ID: "shared-slug", Path: "second-entry"},
		}}
		matched, path := MatchCandidate(reg, "shared-slug")
		if !matched || path != "first-entry" {
			t.Fatalf("MatchCandidate(shared-slug) = (%v, %q), want (true, %q) — first registry entry must win", matched, path, "first-entry")
		}
	})

	t.Run("built via ParseRegistry over a fixture YAML", func(t *testing.T) {
		reg, err := ParseRegistry(strings.NewReader(readTestFixture(t, "valid_core_and_custom")))
		if err != nil {
			t.Fatalf("ParseRegistry: %v", err)
		}

		matched, path := MatchCandidate(reg, "sdd-spec")
		if !matched || path != "sdd-spec" {
			t.Fatalf("MatchCandidate(sdd-spec) over parsed registry = (%v, %q), want (true, %q)", matched, path, "sdd-spec")
		}

		matched, path = MatchCandidate(reg, "sdd-spec-review")
		if matched || path != "" {
			t.Fatalf("MatchCandidate(sdd-spec-review) over parsed registry = (%v, %q), want (false, \"\") — substring near-miss must not match", matched, path)
		}
	})
}
