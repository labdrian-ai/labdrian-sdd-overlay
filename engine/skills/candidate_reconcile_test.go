package skills

import (
	"reflect"
	"strings"
	"testing"
)

func TestReconcileCandidatesReportsTheThreeCrossRecordAnomalies(t *testing.T) {
	cases := []struct {
		name       string
		candidates []CandidateRecord
		drafts     []DraftRecord
		want       []Anomaly
	}{
		{"empty source", nil, nil, nil},
		{"orphan draft", nil, []DraftRecord{{Key: draftKeyOK, Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: 1}},
			[]Anomaly{{Key: draftKeyOK, Message: "orphan draft: candidate \"" + candidateKeyOK + "\" is missing"}}},
		{"missing draft", []CandidateRecord{{Key: candidateKeyOK, Draft: draftKeyOK}}, nil,
			[]Anomaly{{Key: candidateKeyOK, Message: "missing draft: \"" + draftKeyOK + "\""}}},
		{"two open drafts for one revision", []CandidateRecord{{Key: candidateKeyOK}},
			[]DraftRecord{
				{Key: draftKeyOK + "-b", Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: 2},
				{Key: draftKeyOK, Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: 2},
			},
			[]Anomaly{{Key: candidateKeyOK, Message: "multiple open drafts for candidate \"" + candidateKeyOK + "\" revision 2: " + draftKeyOK + ", " + draftKeyOK + "-b"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ReconcileCandidates(c.candidates, c.drafts); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ReconcileCandidates = %v; want %v", got, c.want)
			}
		})
	}
}

func TestReconcileCandidatesAcceptsExistingLinksAndIndependentLatches(t *testing.T) {
	for _, status := range []DraftStatus{DraftOpen, DraftRegistered, DraftAbandoned} {
		t.Run(string(status), func(t *testing.T) {
			nextRevision := 1
			if status == DraftOpen {
				nextRevision = 2
			}
			candidates := []CandidateRecord{{Key: candidateKeyOK, Draft: draftKeyOK}, {Key: candidateKeyOK + "-other"}}
			drafts := []DraftRecord{
				{Key: draftKeyOK, Candidate: candidateKeyOK, Status: status, ForRevision: 1},
				{Key: draftKeyOK + "-next", Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: nextRevision},
				{Key: draftKeyOK + "-other", Candidate: candidateKeyOK + "-other", Status: DraftOpen, ForRevision: 1},
			}
			if got := ReconcileCandidates(candidates, drafts); len(got) != 0 {
				t.Fatalf("independent revisions, candidates and closed drafts: %v", got)
			}
		})
	}
}

func TestReconcileCandidatesExcludesInvalidLatchesButStillChecksLinks(t *testing.T) {
	for _, field := range []string{"", "**ForRevision**: \n", "**ForRevision**: 0\n", "**ForRevision**: -1\n", "**ForRevision**: two\n"} {
		t.Run(field, func(t *testing.T) {
			content := "**Candidate**: " + candidateKeyOK + "\n**Status**: open\n" + field
			first, anomalies := ParseDraftRecord(draftKeyOK, content)
			second, _ := ParseDraftRecord(draftKeyOK+"-other", content)
			if len(anomalies) != 1 || !strings.Contains(anomalies[0].Message, "ForRevision") {
				t.Fatalf("invalid latch anomalies = %v", anomalies)
			}
			candidates := []CandidateRecord{{Key: candidateKeyOK, Draft: draftKeyOK}}
			if got := ReconcileCandidates(candidates, []DraftRecord{first, second}); len(got) != 0 {
				t.Fatalf("invalid latches participated in duplicate detection: %v", got)
			}
			got := ReconcileCandidates(nil, []DraftRecord{first})
			want := []Anomaly{{Key: draftKeyOK, Message: "orphan draft: candidate \"" + candidateKeyOK + "\" is missing"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("invalid latch hid orphan: %v; want %v", got, want)
			}
		})
	}
	// Direct domain callers also cannot latch a negative revision.
	if got := ReconcileCandidates([]CandidateRecord{{Key: candidateKeyOK}}, []DraftRecord{
		{Key: draftKeyOK, Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: -1},
		{Key: draftKeyOK + "-b", Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: -1},
	}); len(got) != 0 {
		t.Fatalf("negative latches participated: %v", got)
	}
}

func TestReconcileCandidatesReadsGoalPrefixedReferencesWithoutRewritingRecords(t *testing.T) {
	candidate, _ := ParseCandidateRecord("goal:g-42/"+candidateKeyOK,
		"**Kind**: repeated-success\n**Status**: drafted\n**Draft**: goal:g-42/"+draftKeyOK+"\n")
	draft, anomalies := ParseDraftRecord("goal:g-42/"+draftKeyOK,
		"**Candidate**: goal:g-42/"+candidateKeyOK+"\n**Status**: open\n**ForRevision**: 1\n")
	if len(anomalies) != 0 {
		t.Fatalf("ParseDraftRecord: %v", anomalies)
	}
	if got := ReconcileCandidates([]CandidateRecord{candidate}, []DraftRecord{draft}); len(got) != 0 {
		t.Fatalf("goal-prefixed links: %v", got)
	}
	got := ReconcileCandidates(nil, []DraftRecord{draft})
	want := []Anomaly{{Key: "goal:g-42/" + draftKeyOK, Message: "orphan draft: candidate \"goal:g-42/" + candidateKeyOK + "\" is missing"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orphan topic = %v; want %v", got, want)
	}
}

func TestReconcileCandidatesIsDeterministicAndRepairsNothing(t *testing.T) {
	candidates := []CandidateRecord{
		{Key: candidateKeyOK + "-z", Draft: draftKeyOK + "-missing"},
		{Key: candidateKeyOK},
	}
	drafts := []DraftRecord{
		{Key: draftKeyOK + "-z", Candidate: candidateKeyOK + "-missing", Status: DraftAbandoned, ForRevision: 1},
		{Key: draftKeyOK + "-c", Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: 2},
		{Key: draftKeyOK + "-a", Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: 2},
		{Key: draftKeyOK + "-b", Candidate: candidateKeyOK, Status: DraftOpen, ForRevision: 2},
	}
	beforeCandidates := append([]CandidateRecord(nil), candidates...)
	beforeDrafts := append([]DraftRecord(nil), drafts...)
	want := []Anomaly{
		{Key: candidateKeyOK, Message: "multiple open drafts for candidate \"" + candidateKeyOK + "\" revision 2: " + draftKeyOK + "-a, " + draftKeyOK + "-b, " + draftKeyOK + "-c"},
		{Key: candidateKeyOK + "-z", Message: "missing draft: \"" + draftKeyOK + "-missing\""},
		{Key: draftKeyOK + "-z", Message: "orphan draft: candidate \"" + candidateKeyOK + "-missing\" is missing"},
	}
	for i := 0; i < 8; i++ {
		got := ReconcileCandidates(candidates, drafts)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ReconcileCandidates = %v; want %v", got, want)
		}
		if !reflect.DeepEqual(candidates, beforeCandidates) || !reflect.DeepEqual(drafts, beforeDrafts) {
			t.Fatal("reconciliation modified the records or their order")
		}
	}
	for i, j := 0, len(drafts)-1; i < j; i, j = i+1, j-1 {
		drafts[i], drafts[j] = drafts[j], drafts[i]
	}
	candidates[0], candidates[1] = candidates[1], candidates[0]
	if got := ReconcileCandidates(candidates, drafts); !reflect.DeepEqual(got, want) {
		t.Fatalf("input order changed anomalies: %v; want %v", got, want)
	}
}
