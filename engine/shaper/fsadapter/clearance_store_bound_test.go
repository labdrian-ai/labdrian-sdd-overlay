package fsadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// paddedRecordBytes is a valid clearance record of exactly size bytes: its one flag
// resolution carries a reason as long as it takes. A reason is free text with no length
// of its own, which is why the record as a whole is bounded (shaper.MaxRecordBytes).
func paddedRecordBytes(t *testing.T, key [3]string, size int) []byte {
	t.Helper()
	verified := false
	record := shaper.ClearanceRecord{
		Version: shaper.ClearanceRecordVersion,
		Subject: shaper.ClearanceSubject{
			ProjectID: key[0], GoalID: key[1],
			GoalSHA256: hex64("1"), HandoffSHA256: key[2],
			ProvenanceSHA256: hex64("2"), ViewSHA256: hex64("3"),
		},
		FlagResolutions: []shaper.FlagResolution{{FlagID: "f", Reason: "r", Evidence: "e"}},
		Decision:        shaper.DecisionAffirm,
		Channel:         shaper.ChannelProvenance{Runtime: "pi", Mode: "tui", Verified: &verified},
	}
	base, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if size < len(base) {
		t.Fatalf("cannot pad a %d byte record to %d bytes", len(base), size)
	}
	record.FlagResolutions[0].Reason += strings.Repeat("x", size-len(base))
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if len(data) != size {
		t.Fatalf("padded record is %d bytes, want %d", len(data), size)
	}
	return data
}

// placeRecord puts data at the record path of key the way the store would have left it,
// without going through the store, so a test can leave there what no Put would accept.
func placeRecord(t *testing.T, state string, key [3]string, data []byte) string {
	t.Helper()
	path := recordPath(state, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A record of exactly the bound is stored and read back whole: the bound is the largest
// record the program ever accepted, so the read must not stop one byte short of it.
func TestARecordOfExactlyTheBoundIsStoredAndReadBack(t *testing.T) {
	s, _ := newTestStore(t)
	key := [3]string{"proj", "goal-alpha", hex64("a")}
	data := paddedRecordBytes(t, key, shaper.MaxRecordBytes)

	if _, err := s.Put(data); err != nil {
		t.Fatalf("Put of a record of exactly shaper.MaxRecordBytes: %v", err)
	}
	got, err := s.Get(key[0], key[1], key[2])
	if err != nil {
		t.Fatalf("Get of a record of exactly shaper.MaxRecordBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Get returned %d bytes, want the %d stored", len(got), len(data))
	}
}

// A record over the bound is not stored: the parse that every Put starts with refuses it,
// before the store makes a directory for it.
func TestPutRefusesARecordOverTheBound(t *testing.T) {
	s, state := newTestStore(t)
	key := [3]string{"proj", "goal-alpha", hex64("a")}

	_, err := s.Put(paddedRecordBytes(t, key, shaper.MaxRecordBytes+1))
	if !errors.Is(err, shaper.ErrRecordTooLarge) {
		t.Fatalf("Put of a record one byte over the bound = %v, want shaper.ErrRecordTooLarge", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "labdrian")); !os.IsNotExist(err) {
		t.Errorf("Put created store directories for an oversized record (err %v)", err)
	}
}

// A file over the bound that something else left in the store is refused in the store's
// words, naming the size the opened file reported and the bound, and it is a store that
// cannot be read, not an absent clearance.
func TestGetRefusesARecordFileOverTheBound(t *testing.T) {
	s, state := newTestStore(t)
	key := [3]string{"proj", "goal-alpha", hex64("a")}
	path := placeRecord(t, state, key, paddedRecordBytes(t, key, shaper.MaxRecordBytes+1))

	got, err := s.Get(key[0], key[1], key[2])
	want := fmt.Sprintf("clearance store: record %q is %d bytes, exceeding the maximum of %d", path, shaper.MaxRecordBytes+1, shaper.MaxRecordBytes)
	if err == nil || err.Error() != want || errors.Is(err, shaper.ErrClearanceNotFound) {
		t.Fatalf("Get of a file one byte over the bound = %d bytes, %v, want a refusal %q", len(got), err, want)
	}
}

// The size a refusal names is the larger of what the opened file reported and what was read
// of it. They differ when the file grew between the open and the read: the descriptor
// reported a few bytes and the read then returned the bound plus one, and a refusal that
// said "is 10 bytes, exceeding the maximum" would contradict itself. The race cannot be
// staged through Get, which opens and reads in one call, so the wording is pinned where it
// is made.
func TestTheRefusalOfAnOversizedRecordNamesTheLargerOfTheSizeAndWhatWasRead(t *testing.T) {
	const path = "/state/record.json"
	for _, tc := range []struct {
		name     string
		reported int64
		read     int
		want     int64
	}{
		{"a file that reported its full size", 512 << 20, shaper.MaxRecordBytes + 1, 512 << 20},
		{"a file that grew after it was opened", 10, shaper.MaxRecordBytes + 1, shaper.MaxRecordBytes + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := recordTooLarge(path, tc.reported, tc.read)
			want := fmt.Sprintf("clearance store: record %q is %d bytes, exceeding the maximum of %d", path, tc.want, shaper.MaxRecordBytes)
			if err == nil || err.Error() != want {
				t.Errorf("recordTooLarge(%d, %d) = %v, want %q", tc.reported, tc.read, err, want)
			}
		})
	}
}

// A file far over the bound is refused without being read in full: the read stops one byte
// past the bound. A sparse file occupies almost no disk but reports its full size.
func TestGetDoesNotReadAFileFarOverTheBound(t *testing.T) {
	s, state := newTestStore(t)
	key := [3]string{"proj", "goal-alpha", hex64("a")}
	path := placeRecord(t, state, key, nil)
	const size = 512 << 20
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := s.Get(key[0], key[1], key[2])
	runtime.ReadMemStats(&after)

	want := fmt.Sprintf("clearance store: record %q is %d bytes, exceeding the maximum of %d", path, size, shaper.MaxRecordBytes)
	if err == nil || err.Error() != want {
		t.Fatalf("Get of a 512 MiB file = %v, want a refusal %q", err, want)
	}
	// Reading the bound plus one byte grows the buffer a few times over (ReadAll doubles
	// it), so the limit is eight times the bound: far more than that read needs, and 16
	// times less than reading the file whole would allocate.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8*shaper.MaxRecordBytes {
		t.Errorf("Get of a 512 MiB file allocated %d bytes, want at most %d: the file must not be read past the bound", allocated, 8*shaper.MaxRecordBytes)
	}
}
