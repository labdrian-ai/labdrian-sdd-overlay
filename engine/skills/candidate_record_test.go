package skills

import (
	"reflect"
	"strings"
	"testing"
)

const candidateKeyOK = "procedural/candidates/repeated-success/probe-engram-with-home-override"

// candidateBlock is the record of section 2 and the fields section 9 adds, as an agent writes it.
const candidateBlock = `**Kind**: repeated-success
**Candidate**: probe-engram-with-home-override
**Aliases**: probe engram with a home override; override home for the engram probe
**Status**: registered
**Threshold**: 3
**OccurrenceCount**: 3
**FirstObserved**: 2026-09-15T10:04:00Z
**LastObserved**: 2026-09-18T08:12:00Z
**Occurrences**:
- engram:3401 @ 2026-09-15T10:04:00Z — first
- engram:3407 @ 2026-09-16T08:12:00Z — second
- engram:3412 @ 2026-09-18T08:12:00Z — third
**Summary**: Override HOME, because the probe writes under it.
**Disposition**: new
**Draft**: procedural/drafts/repeated-success/probe-engram-with-home-override
**History**:
- 2026-09-18T08:20:00Z | none -> observing | agent | engram:3401
- 2026-09-18T08:21:00Z | observing -> emitted | agent | count 3/3, no registry match
- 2026-09-20T10:05:00Z | emitted -> drafted | agent | draft procedural/drafts/repeated-success/probe-engram-with-home-override; Disposition new
- 2026-09-20T10:09:00Z | drafted -> registered | agent | skill probe-engram-with-home-override rev 1
`

func TestParseCandidateRecordReadsTheFieldsOfAWellFormedRecord(t *testing.T) {
	got, anomalies := ParseCandidateRecord(candidateKeyOK, candidateBlock)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies = %v; want none", anomalies)
	}
	want := CandidateRecord{
		Key: candidateKeyOK, Kind: "repeated-success", Status: StatusRegistered,
		Aliases:   []string{"probe engram with a home override", "override home for the engram probe"},
		Threshold: 3, OccurrenceCount: 3, Occurrences: []string{"engram:3401", "engram:3407", "engram:3412"},
		Disposition: "new", Draft: "procedural/drafts/repeated-success/probe-engram-with-home-override",
		History: []HistoryEntry{
			{At: "2026-09-18T08:20:00Z", From: noStatus, To: StatusObserving, Actor: ActorAgent, Evidence: "engram:3401"},
			{At: "2026-09-18T08:21:00Z", From: StatusObserving, To: StatusEmitted, Actor: ActorAgent, Evidence: "count 3/3, no registry match"},
			{At: "2026-09-20T10:05:00Z", From: StatusEmitted, To: StatusDrafted, Actor: ActorAgent, Evidence: "draft procedural/drafts/repeated-success/probe-engram-with-home-override; Disposition new"},
			{At: "2026-09-20T10:09:00Z", From: StatusDrafted, To: StatusRegistered, Actor: ActorAgent, Evidence: "skill probe-engram-with-home-override rev 1"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("record = %+v\nwant   %+v", got, want)
	}
}

func TestParseCandidateRecordAliasesNoneIsNoAlias(t *testing.T) {
	rec, _ := ParseCandidateRecord(candidateKeyOK, "**Kind**: repeated-success\n**Status**: observing\n**Aliases**: none\n")
	if len(rec.Aliases) != 0 {
		t.Errorf("Aliases = %v; want none", rec.Aliases)
	}
}

func TestParseCandidateRecordReadsTheKeyOfAFailureRecoveryUnderAGoal(t *testing.T) {
	key := "goal:g-42/procedural/candidates/failure-recovery/sqlite-readonly/override-home"
	rec, anomalies := ParseCandidateRecord(key, "**Kind**: failure-recovery\n**Status**: observing\n")
	if len(anomalies) != 0 {
		t.Fatalf("anomalies = %v; a goal prefix names the goal, it is not an anomaly", anomalies)
	}
	if rec.Key != "procedural/candidates/failure-recovery/sqlite-readonly/override-home" || rec.Goal != "g-42" {
		t.Errorf("Key, Goal = %q, %q; want the key without its goal prefix and the goal", rec.Key, rec.Goal)
	}
}

// Every case is one thing that is wrong with a record, and what the reader says; none of them is repaired.
func TestParseCandidateRecordReportsWhatIsWrongAndRepairsNothing(t *testing.T) {
	cases := []struct {
		name, key, content, want string
	}{
		{"a key that is not a candidate key", "procedural/candidates/other/x", "**Status**: observing\n", "candidate key"},
		{"a status outside the vocabulary", candidateKeyOK, "**Kind**: repeated-success\n**Status**: open\n", "candidate status"},
		{"no status", candidateKeyOK, "**Kind**: repeated-success\n", "no Status"},
		{"a kind that is not the key's", candidateKeyOK, "**Kind**: failure-recovery\n**Status**: observing\n", "Kind"},
		{"a count that is not the occurrences listed",
			candidateKeyOK, "**Kind**: repeated-success\n**Status**: observing\n**OccurrenceCount**: 3\n**Occurrences**:\n- engram:1 @ t — a\n- engram:1 @ t — again\n", "OccurrenceCount"},
		{"a threshold that is not a number", candidateKeyOK, "**Kind**: repeated-success\n**Status**: observing\n**Threshold**: three\n", "Threshold"},
		{"a history that is not a transition",
			candidateKeyOK, "**Kind**: repeated-success\n**Status**: emitted\n**History**:\n- t1 | none -> emitted | agent | x\n", "none -> emitted"},
		{"a history whose lines do not chain",
			candidateKeyOK, "**Kind**: repeated-success\n**Status**: emitted\n**History**:\n- t1 | none -> observing | agent | x\n- t2 | drafted -> registered | agent | x\n", "chain"},
		{"a history that ends elsewhere than the status",
			candidateKeyOK, "**Kind**: repeated-success\n**Status**: drafted\n**History**:\n- t1 | none -> observing | agent | x\n", "ends at observing"},
		{"a history line of fewer than four columns",
			candidateKeyOK, "**Kind**: repeated-success\n**Status**: observing\n**History**:\n- t1 | none -> observing\n", "History line"},
		{"a field given twice", candidateKeyOK, "**Kind**: repeated-success\n**Status**: observing\n**Status**: emitted\n", "twice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, anomalies := ParseCandidateRecord(c.key, c.content)
			if len(anomalies) == 0 {
				t.Fatalf("no anomaly; want one that says %q", c.want)
			}
			found := false
			for _, a := range anomalies {
				if strings.Contains(a.Message, c.want) {
					found = true
				}
				if a.Key == "" {
					t.Errorf("anomaly %+v names no key", a)
				}
			}
			if !found {
				t.Errorf("anomalies = %v; want one that says %q", anomalies, c.want)
			}
		})
	}
}

// An agent writes these by hand: a record that cannot be understood still gets its key listed, with the anomaly.
func TestParseCandidateRecordKeepsTheKeyOfARecordItCannotRead(t *testing.T) {
	rec, anomalies := ParseCandidateRecord(candidateKeyOK, "free text, no fields at all")
	if rec.Key != candidateKeyOK || len(anomalies) == 0 {
		t.Errorf("record key %q, anomalies %v; want the key kept and an anomaly", rec.Key, anomalies)
	}
}

// splitGoalPrefix is the rule of longterm-mem's memory.TopicWithoutGoal and memory.GoalOfTopic; the engine cannot import
// that module, so these are the cases of its test (longterm-mem/internal/memory) and both must say the same.
func TestSplitGoalPrefixIsTheRuleOfTheMemoryModule(t *testing.T) {
	cases := []struct{ topic, goal, rest string }{
		{"goal:g-42/architecture/auth", "g-42", "architecture/auth"},
		{"goal:g-42/", "g-42", ""},
		{"goal:g-42", "", "goal:g-42"},
		{"goal:/x", "", "goal:/x"},
		{"goal:", "", "goal:"},
		{"architecture/auth", "", "architecture/auth"},
		{"", "", ""},
	}
	for _, c := range cases {
		if goal, rest := splitGoalPrefix(c.topic); goal != c.goal || rest != c.rest {
			t.Errorf("splitGoalPrefix(%q) = %q, %q; want %q, %q", c.topic, goal, rest, c.goal, c.rest)
		}
	}
}
