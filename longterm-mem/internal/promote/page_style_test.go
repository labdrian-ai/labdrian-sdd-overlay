package promote

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

func memoryWithID(source, id string) memory.Memory {
	return memory.Memory{Identity: memory.Identity{Source: source, ID: id}}
}

// numericID reads the id the pages record, and says what is wrong with one that is no usable integer: the id once,
// and why. A caller can still ask errors.Is and errors.As for the strconv error behind the message.
func TestNumericID_ReadsAnIntegerIdAsItIs(t *testing.T) {
	for id, want := range map[string]int64{"7": 7, "0012": 12, "9223372036854775807": 9223372036854775807} {
		got, err := numericID(memoryWithID(memorycardSource, id))
		if err != nil || got != want {
			t.Errorf("numericID(%q) = %d, %v, want %d", id, got, err, want)
		}
	}
}

func TestNumericID_AnIdThatIsNoNumberSaysSoOnceAndKeepsItsCause(t *testing.T) {
	_, err := numericID(memoryWithID(engramSource, "x1"))
	if err == nil {
		t.Fatal("numericID(x1) = nil error")
	}
	if got := err.Error(); strings.Count(got, "x1") != 1 || !strings.Contains(got, "not a number") || strings.Contains(got, "out of range") {
		t.Errorf("error = %q, want it to name %q once and say it is not a number", got, "x1")
	}
	var numErr *strconv.NumError
	if !errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) || !errors.As(err, &numErr) {
		t.Errorf("error %v: Is(ErrSyntax) = %v, Is(ErrRange) = %v, As(*NumError) = %v, want true, false, true",
			err, errors.Is(err, strconv.ErrSyntax), errors.Is(err, strconv.ErrRange), errors.As(err, &numErr))
	}
}

func TestNumericID_AnAllDigitIdTooBigForAnInt64IsOutOfRangeNotNoNumber(t *testing.T) {
	const id = "9223372036854775808"
	_, err := numericID(memoryWithID(memorycardSource, id))
	if err == nil {
		t.Fatal("numericID(2^63) = nil error")
	}
	if got := err.Error(); strings.Count(got, id) != 1 || !strings.Contains(got, "out of range") || strings.Contains(got, "not a number") {
		t.Errorf("error = %q, want it to name %q once and say it is out of range, not that it is no number", got, id)
	}
	var numErr *strconv.NumError
	if !errors.Is(err, strconv.ErrRange) || errors.Is(err, strconv.ErrSyntax) || !errors.As(err, &numErr) {
		t.Errorf("error %v: Is(ErrRange) = %v, Is(ErrSyntax) = %v, As(*NumError) = %v, want true, false, true",
			err, errors.Is(err, strconv.ErrRange), errors.Is(err, strconv.ErrSyntax), errors.As(err, &numErr))
	}
}
