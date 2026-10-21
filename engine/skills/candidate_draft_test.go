package skills

import (
	"reflect"
	"strings"
	"testing"
)

const draftKeyOK = "procedural/drafts/repeated-success/probe-engram-with-home-override"

const draftBlock = `**Candidate**: procedural/candidates/repeated-success/probe-engram-with-home-override
**Disposition**: extend:engram-probe
**Status**: open
**ForRevision**: 2
**Lint**: hard 0, warnings 1 (body-recommended)
**History**:
- t1 | registered -> registered | agent | revision draft opened
**Body**:
` + "````diff\n**Status**: abandoned\n**ForRevision**: 0\n**History**:\n- not a history entry\n````\n"

func TestParseDraftRecordReadsTheHeaderAndNotTheBody(t *testing.T) {
	want := DraftRecord{
		Key: draftKeyOK, Candidate: candidateKeyOK, Disposition: "extend:engram-probe",
		Status: DraftOpen, ForRevision: 2,
		History: []HistoryEntry{{At: "t1", From: StatusRegistered, To: StatusRegistered, Actor: ActorAgent, Evidence: "revision draft opened"}},
	}
	for _, ending := range []string{"LF", "CRLF"} {
		t.Run(ending, func(t *testing.T) {
			content := draftBlock
			if ending == "CRLF" {
				content = strings.ReplaceAll(content, "\n", "\r\n")
			}
			got, anomalies := ParseDraftRecord(draftKeyOK, content)
			if !reflect.DeepEqual(got, want) || len(anomalies) != 0 {
				t.Fatalf("ParseDraftRecord = %+v, %v; want %+v, no anomalies", got, anomalies, want)
			}
		})
	}
}

func TestParseDraftRecordReadsBothKindsAndGoalPrefixes(t *testing.T) {
	for _, tail := range []string{"repeated-success/probe-engram-with-home-override", "failure-recovery/sqlite-readonly/override-home"} {
		t.Run(tail, func(t *testing.T) {
			key := "procedural/drafts/" + tail
			candidate := "goal:g-42/procedural/candidates/" + tail
			content := "**Candidate**: " + candidate + "\n**Disposition**: new\n**Status**: registered\n**ForRevision**: 1\n"
			rec, anomalies := ParseDraftRecord("goal:g-42/"+key, content)
			want := DraftRecord{Key: key, Goal: "g-42", Candidate: candidate, Disposition: "new", Status: DraftRegistered, ForRevision: 1}
			if !reflect.DeepEqual(rec, want) || len(anomalies) != 0 {
				t.Fatalf("ParseDraftRecord = %+v, %v; want %+v, no anomalies", rec, anomalies, want)
			}
		})
	}
}

func TestParseDraftRecordRequiresAPositiveRevision(t *testing.T) {
	cases := []struct {
		name, field, message string
		want                 int
	}{
		{"absent", "", `ForRevision "" is not a positive integer`, 0},
		{"blank", "**ForRevision**: \t\n", `ForRevision "" is not a positive integer`, 0},
		{"zero", "**ForRevision**: 0\n", `ForRevision "0" is not a positive integer`, 0},
		{"negative", "**ForRevision**: -1\n", `ForRevision "-1" is not a whole number`, 0},
		{"word", "**ForRevision**: two\n", `ForRevision "two" is not a whole number`, 0},
		{"fraction", "**ForRevision**: 1.5\n", `ForRevision "1.5" is not a whole number`, 0},
		{"too many digits", "**ForRevision**: 1000000000\n", `ForRevision "1000000000" is not a whole number`, 0},
		{"first revision", "**ForRevision**: 1\n", "", 1},
		{"existing numeric reader upper bound", "**ForRevision**: 999999999\n", "", 999999999},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			content := "**Candidate**: " + candidateKeyOK + "\n**Status**: open\n" + c.field
			rec, anomalies := ParseDraftRecord(draftKeyOK, content)
			want := DraftRecord{Key: draftKeyOK, Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: c.want}
			var wantAnomalies []Anomaly
			if c.message != "" {
				wantAnomalies = []Anomaly{{Key: draftKeyOK, Message: c.message}}
			}
			if !reflect.DeepEqual(rec, want) || !reflect.DeepEqual(anomalies, wantAnomalies) {
				t.Fatalf("ParseDraftRecord = %+v, %v; want %+v, %v", rec, anomalies, want, wantAnomalies)
			}
		})
	}
}

func TestParseDraftRecordReportsAnomaliesWithoutRepair(t *testing.T) {
	base := "**Candidate**: " + candidateKeyOK + "\n**Status**: abandoned\n**ForRevision**: 2\n"
	cases := []struct {
		name, key, content, message string
	}{
		{"wrong namespace", candidateKeyOK, base, "draft key"},
		{"invalid tail", "procedural/drafts/failure-recovery/only-one", base, "exactly 2 slug segments"},
		{"missing candidate", draftKeyOK, strings.ReplaceAll(base, "**Candidate**: "+candidateKeyOK+"\n", ""), "no Candidate"},
		{"invalid candidate", draftKeyOK, strings.ReplaceAll(base, candidateKeyOK, "not-a-key"), "validate candidate key"},
		{"empty candidate after goal", draftKeyOK, strings.ReplaceAll(base, candidateKeyOK, "goal:g-42/"), "validate candidate key"},
		{"missing status", draftKeyOK, strings.ReplaceAll(base, "**Status**: abandoned\n", ""), "no Status"},
		{"invalid status", draftKeyOK, strings.ReplaceAll(base, "abandoned", "drafted"), "draft status"},
		{"duplicate field", draftKeyOK, base + "**Status**: open\n", "the field Status is given twice"},
		{"invalid history", draftKeyOK, base + "**History**:\n- broken\n", "History line"},
		{"illegal history transition", draftKeyOK, base + "**History**:\n- t1 | none -> emitted | agent | x\n", "none -> emitted"},
		{"disconnected history", draftKeyOK, base + "**History**:\n- t1 | none -> observing | agent | x\n- t2 | emitted -> drafted | agent | x\n", "does not chain"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, anomalies := ParseDraftRecord("goal:g-42/"+c.key, c.content)
			if rec.Key != c.key || rec.Goal != "g-42" || rec.ForRevision != 2 {
				t.Fatalf("parser repaired or lost the record: %+v", rec)
			}
			if len(anomalies) != 1 || anomalies[0].Key != "goal:g-42/"+c.key || !strings.Contains(anomalies[0].Message, c.message) {
				t.Fatalf("anomalies = %v; want one naming the original topic and %q", anomalies, c.message)
			}
			if c.name == "invalid candidate" && rec.Candidate != "not-a-key" {
				t.Errorf("Candidate repaired to %q", rec.Candidate)
			}
			if c.name == "invalid status" && rec.Status != "" {
				t.Errorf("invalid Status replaced by %q", rec.Status)
			}
			if c.name == "duplicate field" && rec.Status != DraftOpen {
				t.Errorf("Status = %q; want the last readable field as in the candidate parser", rec.Status)
			}
		})
	}
}

func TestParseDraftRecordKeepsAnUnreadableRecordsIdentityAndAnomalyOrder(t *testing.T) {
	rec, anomalies := ParseDraftRecord(draftKeyOK, "free text")
	want := []Anomaly{
		{Key: draftKeyOK, Message: "no Candidate"},
		{Key: draftKeyOK, Message: "no Status"},
		{Key: draftKeyOK, Message: `ForRevision "" is not a positive integer`},
	}
	if !reflect.DeepEqual(rec, DraftRecord{Key: draftKeyOK}) || !reflect.DeepEqual(anomalies, want) {
		t.Fatalf("ParseDraftRecord = %+v, %v; want identity kept, %v", rec, anomalies, want)
	}
}
