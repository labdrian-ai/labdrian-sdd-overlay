package prespec

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"
)

// crockfordLiteral is the alphabet the IDs are written in, written out here and not taken from the
// package, so that a change to the package's constant is a change to this test.
const crockfordLiteral = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func TestCrockfordAlphabetIsTheOneTheIDsAreWrittenIn(t *testing.T) {
	if crockfordAlphabet != crockfordLiteral {
		t.Errorf("crockfordAlphabet = %q, want %q", crockfordAlphabet, crockfordLiteral)
	}
}

// referenceID makes the ID the long way, which shares nothing with NewIDFrom but the layout: the
// low 48 bits of the millisecond count and then the 80 random bits, written out as a string of
// '0' and '1', the timestamp padded with two zero bits to 50 so that it fills ten characters of
// five bits, and the whole cut into 26 groups of five, each an index into the alphabet.
func referenceID(ms uint64, random []byte) string {
	var bits strings.Builder
	for i := 47; i >= 0; i-- {
		bits.WriteByte('0' + byte((ms>>uint(i))&1))
	}
	bits.WriteString("00")
	for _, b := range random {
		fmt.Fprintf(&bits, "%08b", b)
	}
	s := bits.String()
	out := make([]byte, 0, 26)
	for i := 0; i+5 <= len(s); i += 5 {
		n, err := strconv.ParseUint(s[i:i+5], 2, 8)
		if err != nil {
			panic(err)
		}
		out = append(out, crockfordLiteral[n])
	}
	return string(out)
}

const low48 = 1<<48 - 1

// TestNewIDFromIsThePinnedEncodingOfItsTwoInputs holds NewIDFrom to the encoding it always had,
// character by character: seven IDs written down from the encoder as it was before a loop
// replaced its 26 unrolled lines (and agreeing with referenceID), and 200 pairs of inputs against
// referenceID.
func TestNewIDFromIsThePinnedEncodingOfItsTwoInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ms     int64
		random []byte
		want   string
	}{
		{"the epoch and no random bits", 0, make([]byte, 10), "00000000000000000000000000"},
		{"every bit set", 0xFFFFFFFFFFFF, bytes.Repeat([]byte{0xFF}, 10), "ZZZZZZZZZWZZZZZZZZZZZZZZZZ"},
		{"counting bytes", 0x0123456789AB, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "04HMASW9NC041061050R3GG28A"},
		{"a time of this age and a random part with every nibble", 1760000000000, []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, "06CWGB6000VTPVXVR024H36H2N"},
		{"the first and the last random bit", 1, []byte{0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}, "0000000004G000000000000001"},
		{"a count over 48 bits keeps its low 48 bits", 1<<48 + 5, make([]byte, 10), "000000000M0000000000000000"},
		{"the same count without the high bit", 5, make([]byte, 10), "000000000M0000000000000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewIDFrom(time.UnixMilli(tc.ms), bytes.NewReader(tc.random))
			if got != tc.want {
				t.Errorf("NewIDFrom(%d, %x) = %q, want %q", tc.ms, tc.random, got, tc.want)
			}
			if ref := referenceID(uint64(tc.ms)&low48, tc.random); ref != tc.want {
				t.Fatalf("setup: referenceID(%d, %x) = %q, want %q: the reference and the pinned ID disagree", tc.ms, tc.random, ref, tc.want)
			}
		})
	}

	t.Run("a time before the epoch keeps the low 48 bits of its two's complement", func(t *testing.T) {
		random := []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
		got := NewIDFrom(time.UnixMilli(-1), bytes.NewReader(random))
		if want := referenceID(low48, random); got != want {
			t.Errorf("NewIDFrom(-1, %x) = %q, want %q", random, got, want)
		}
	})

	t.Run("200 pairs against the reference encoder", func(t *testing.T) {
		r := rand.New(rand.NewSource(48))
		for i := 0; i < 200; i++ {
			ms := r.Int63()
			random := make([]byte, 10)
			r.Read(random)
			got := NewIDFrom(time.UnixMilli(ms), bytes.NewReader(random))
			if want := referenceID(uint64(ms)&low48, random); got != want {
				t.Fatalf("pair %d: NewIDFrom(%d, %x) = %q, want %q", i, ms, random, got, want)
			}
		}
	})
}

// Every one of the 80 random bits and every one of the 48 timestamp bits lands in the character
// the layout says: setting a single bit changes exactly the character that holds it.
func TestNewIDFromPutsEachBitInTheCharacterThatHoldsIt(t *testing.T) {
	base := NewIDFrom(time.UnixMilli(0), bytes.NewReader(make([]byte, 10)))
	for bit := 0; bit < 48; bit++ {
		got := NewIDFrom(time.UnixMilli(int64(1)<<uint(bit)), bytes.NewReader(make([]byte, 10)))
		wantChar := (47 - bit) / 5
		for i := range got {
			if changed := got[i] != base[i]; changed != (i == wantChar) {
				t.Errorf("timestamp bit %d: %q, character %d changed = %v, want only character %d", bit, got, i, changed, wantChar)
				break
			}
		}
	}
	for bit := 0; bit < 80; bit++ {
		random := make([]byte, 10)
		random[bit/8] = 0x80 >> uint(bit%8)
		got := NewIDFrom(time.UnixMilli(0), bytes.NewReader(random))
		wantChar := 10 + bit/5
		for i := range got {
			if changed := got[i] != base[i]; changed != (i == wantChar) {
				t.Errorf("random bit %d: %q, character %d changed = %v, want only character %d", bit, got, i, changed, wantChar)
				break
			}
		}
	}
}

// NewIDFrom takes ten bytes of the source and no more, and a source that cannot give them is a
// broken source, which it does not paper over.
func TestNewIDFromReadsTenBytesAndRefusesAShortSource(t *testing.T) {
	source := bytes.NewReader(make([]byte, 12))
	NewIDFrom(time.UnixMilli(0), source)
	if source.Len() != 2 {
		t.Errorf("a source of 12 bytes has %d left, want 2: NewIDFrom reads exactly 10", source.Len())
	}

	defer func() {
		got := recover()
		msg, _ := got.(string)
		if !strings.HasPrefix(msg, "prespec: ULID random source failed: ") {
			t.Errorf("a source of 9 bytes: recovered %v, want a panic that says the random source failed", got)
		}
	}()
	NewIDFrom(time.UnixMilli(0), bytes.NewReader(make([]byte, 9)))
	t.Error("a source of 9 bytes did not panic")
}
