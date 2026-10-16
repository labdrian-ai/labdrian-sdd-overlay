package gitobject

import (
	"regexp"
	"strings"
	"testing"
)

// IsName is the one rule the three readers of a git object name decide by, so its boundary is
// pinned at the function itself: exactly 40 or exactly 64 lowercase hexadecimal characters.
func TestIsName(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want bool
	}{
		{"40 lowercase hex digits", strings.Repeat("a1", 20), true},
		{"64 lowercase hex digits", strings.Repeat("a1", 32), true},
		{"40 digits", strings.Repeat("0", 40), true},
		{"64 digits", strings.Repeat("9", 64), true},
		{"every hex digit, 64 of them", strings.Repeat("0123456789abcdef", 4), true},
		{"empty", "", false},
		{"an abbreviated name", "0123456", false},
		{"one short of 40", strings.Repeat("a", 39), false},
		{"one over 40", strings.Repeat("a", 41), false},
		{"between 40 and 64", strings.Repeat("a", 50), false},
		{"one short of 64", strings.Repeat("a", 63), false},
		{"one over 64", strings.Repeat("a", 65), false},
		{"twice 40", strings.Repeat("a", 80), false},
		{"uppercase, 40", strings.Repeat("A", 40), false},
		{"uppercase, 64", strings.Repeat("A", 64), false},
		{"one uppercase among 40 lowercase", strings.Repeat("a", 39) + "F", false},
		{"one uppercase among 64 lowercase", "F" + strings.Repeat("a", 63), false},
		{"the character after f", strings.Repeat("a", 39) + "g", false},
		{"the character before 0", strings.Repeat("a", 39) + "/", false},
		{"the character after 9", strings.Repeat("a", 39) + ":", false},
		{"the character before a", strings.Repeat("a", 39) + "`", false},
		{"trailing newline after 40", strings.Repeat("a", 40) + "\n", false},
		{"trailing newline after 64", strings.Repeat("a", 64) + "\n", false},
		{"trailing newline in place of the last of 40", strings.Repeat("a", 39) + "\n", false},
		{"trailing space after 40", strings.Repeat("a", 40) + " ", false},
		{"leading space before 39", " " + strings.Repeat("a", 39), false},
		{"0x prefix", "0x" + strings.Repeat("a", 38), false},
		{"a multi-byte character that makes 40 bytes", strings.Repeat("a", 38) + string(rune(0xE9)), false},
		{"a NUL byte", strings.Repeat("a", 39) + "\x00", false},
		{"a SHA-256 digest with a prefix", "sha256:" + strings.Repeat("a", 64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsName(tc.in); got != tc.want {
				t.Errorf("IsName(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// Before this package, three places decided it with the same regular expression. IsName must say
// what that expression said for every string: the lengths around 40 and 64, with one position
// changed to each character that sits at the edge of a class or that a reader might trim.
func TestIsNameAnswersAsTheRegularExpressionThreeReadersHadAnswered(t *testing.T) {
	was := regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	odd := []string{"0", "9", "a", "f", "g", "A", "F", "G", "/", ":", "`", "\n", " ", "\x00", "é", "x"}
	checked := 0
	for length := 0; length <= 70; length++ {
		base := strings.Repeat("a", length)
		if got, want := IsName(base), was.MatchString(base); got != want {
			t.Fatalf("IsName(%q) = %v, the regular expression says %v", base, got, want)
		}
		checked++
		for pos := 0; pos < length; pos++ {
			for _, c := range odd {
				s := base[:pos] + c + base[pos+1:]
				if got, want := IsName(s), was.MatchString(s); got != want {
					t.Fatalf("IsName(%q) = %v, the regular expression says %v", s, got, want)
				}
				checked++
			}
		}
	}
	if checked < 10000 {
		t.Errorf("only %d strings were compared", checked)
	}
}
