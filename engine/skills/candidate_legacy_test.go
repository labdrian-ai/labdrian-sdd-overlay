package skills

import (
	"reflect"
	"strings"
	"testing"
)

// legacyBlock lists the same observation under both schemes (engram:3401 imported as memory 77)
// and a second, unrelated memory, with a stated count of 2.
const legacyBlockHead = `**Kind**: repeated-success
**Candidate**: probe-engram-with-home-override
**Status**: emitted
**Threshold**: 2
**OccurrenceCount**: %COUNT%
**Occurrences**:
`

func legacyBlock(count string, ids ...string) string {
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(legacyBlockHead, "%COUNT%", count))
	for _, id := range ids {
		b.WriteString("- " + id + " @ 2026-09-15T10:04:00Z — seen\n")
	}
	return b.String()
}

func TestLegacyOccurrenceNumberReadsOnlyEngramDigits(t *testing.T) {
	cases := []struct {
		id   string
		want string
		ok   bool
	}{
		{"engram:3401", "3401", true},
		{"engram:007", "007", true},
		{"memorycard:77", "", false},
		{"engram:", "", false},
		{"engram:12x", "", false},
		{"engram:-1", "", false},
		{"Engram:5", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := LegacyOccurrenceNumber(tc.id)
		if got != tc.want || ok != tc.ok {
			t.Errorf("LegacyOccurrenceNumber(%q) = %q, %v; want %q, %v", tc.id, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTranslateOccurrenceMapsOnlyAMappedLegacyID(t *testing.T) {
	mapped := map[string]int{"3401": 77}
	cases := map[string]string{
		"engram:3401":   "memorycard:77",
		"engram:9999":   "engram:9999",
		"memorycard:77": "memorycard:77",
		"other":         "other",
	}
	for id, want := range cases {
		if got := TranslateOccurrence(id, mapped); got != want {
			t.Errorf("TranslateOccurrence(%q) = %q; want %q", id, got, want)
		}
	}
	if got := TranslateOccurrence("engram:3401", nil); got != "engram:3401" {
		t.Errorf("TranslateOccurrence with no map = %q; want the id as written", got)
	}
}

func TestParseCandidateRecordTranslatedCountsAMappedLegacyIDWithItsMemorycardIDOnce(t *testing.T) {
	content := legacyBlock("1", "engram:3401", "memorycard:77")
	if _, anomalies := ParseCandidateRecord(candidateKeyOK, content); len(anomalies) != 1 ||
		!strings.Contains(anomalies[0].Message, "OccurrenceCount is 1 but 2 distinct") {
		t.Fatalf("untranslated anomalies = %v; want the count of 2 distinct ids the record always had", anomalies)
	}
	got, anomalies := ParseCandidateRecordTranslated(candidateKeyOK, content, map[string]int{"3401": 77})
	if len(anomalies) != 0 {
		t.Fatalf("translated anomalies = %v; want none: engram:3401 and memorycard:77 are one occurrence", anomalies)
	}
	if want := []string{"engram:3401", "memorycard:77"}; !reflect.DeepEqual(got.Occurrences, want) {
		t.Fatalf("Occurrences = %v; want the ids as written %v (the engine never rewrites a record)", got.Occurrences, want)
	}
}

func TestParseCandidateRecordTranslatedAnUnknownLegacyIDCountsAsItself(t *testing.T) {
	content := legacyBlock("2", "engram:9999", "memorycard:77")
	if _, anomalies := ParseCandidateRecordTranslated(candidateKeyOK, content, map[string]int{"3401": 77}); len(anomalies) != 0 {
		t.Fatalf("anomalies = %v; want none: an unmapped engram:9999 is one distinct id of its own", anomalies)
	}
}

func TestParseCandidateRecordTranslatedTwoLegacyIDsOfOneMemoryCountOnce(t *testing.T) {
	content := legacyBlock("1", "engram:3401", "engram:3407")
	if _, anomalies := ParseCandidateRecordTranslated(candidateKeyOK, content, map[string]int{"3401": 77, "3407": 77}); len(anomalies) != 0 {
		t.Fatalf("anomalies = %v; want none: two observations merged on import count as 1", anomalies)
	}
	if _, anomalies := ParseCandidateRecord(candidateKeyOK, content); len(anomalies) != 1 {
		t.Fatalf("untranslated anomalies = %v; want the old 2-distinct anomaly", anomalies)
	}
}

func TestParseCandidateRecordWithNoLegacyIDIsUnchangedByTheTranslation(t *testing.T) {
	content := legacyBlock("3", "memorycard:1", "memorycard:2", "memorycard:2")
	plain, plainAnomalies := ParseCandidateRecord(candidateKeyOK, content)
	translated, translatedAnomalies := ParseCandidateRecordTranslated(candidateKeyOK, content, map[string]int{"3401": 77})
	if !reflect.DeepEqual(plain, translated) || !reflect.DeepEqual(plainAnomalies, translatedAnomalies) {
		t.Fatalf("record or anomalies differ: %+v %v vs %+v %v", plain, plainAnomalies, translated, translatedAnomalies)
	}
}
