package sha256hex

import (
	"strings"
	"testing"
)

// The expected digests were made with sha256sum, not with this package.
func TestSumIsTheLowercaseHexSHA256OfTheExactBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", []byte{}, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"nil is empty", nil, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", []byte("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"bytes that are not text", []byte{0x00, 0xff}, "06eb7d6a69ee19e5fbdf749018d3d2abfa04bcbd1365db312eb86dc7169389b8"},
		{"a trailing newline is part of the bytes", []byte("abc\n"), "edeaaff3f1774ad2888673770c6d64097e391bc362d7d6fb34982ddf0efd18cb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sum(tc.in); got != tc.want {
				t.Errorf("Sum(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSumIsAlwaysAValidDigest(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("a"), []byte(strings.Repeat("x", 4096))} {
		if got := Sum(in); !IsValid(got) {
			t.Errorf("IsValid(Sum(%d bytes)) = false for %q", len(in), got)
		}
	}
}

func TestLengthIsTheWidthOfAHexDigest(t *testing.T) {
	if Length != 64 {
		t.Errorf("Length = %d, want 64", Length)
	}
	if got := len(Sum([]byte("x"))); got != Length {
		t.Errorf("len(Sum(...)) = %d, want Length %d", got, Length)
	}
}

// IsValid is the one digest rule the records check their digests by, so its boundary is pinned
// at the function itself: exactly 64 lowercase hexadecimal characters, nothing else.
func TestIsValid(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want bool
	}{
		{"64 lowercase hex digits", strings.Repeat("a1", 32), true},
		{"all digits", strings.Repeat("0", 64), true},
		{"every hex digit", strings.Repeat("0123456789abcdef", 4), true},
		{"empty", "", false},
		{"one short", strings.Repeat("a", 63), false},
		{"one long", strings.Repeat("a", 65), false},
		{"uppercase", strings.Repeat("A", 64), false},
		{"one uppercase among lowercase", strings.Repeat("a", 63) + "F", false},
		{"the character after f", strings.Repeat("a", 63) + "g", false},
		{"the character before 0", strings.Repeat("a", 63) + "/", false},
		{"the character after 9", strings.Repeat("a", 63) + ":", false},
		{"the character before a", strings.Repeat("a", 63) + "`", false},
		{"embedded traversal", strings.Repeat("a", 30) + "/../" + strings.Repeat("b", 30), false},
		{"trailing newline", strings.Repeat("a", 64) + "\n", false},
		{"trailing newline in place of the last digit", strings.Repeat("a", 63) + "\n", false},
		{"leading space", " " + strings.Repeat("a", 63), false},
		{"0x prefix", "0x" + strings.Repeat("a", 62), false},
		{"a multi-byte character that makes 64 bytes", strings.Repeat("a", 62) + string(rune(0xE9)), false},
		{"a NUL byte", strings.Repeat("a", 63) + "\x00", false},
		{"a git object name of 40 digits", strings.Repeat("a", 40), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValid(tc.in); got != tc.want {
				t.Errorf("IsValid(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
