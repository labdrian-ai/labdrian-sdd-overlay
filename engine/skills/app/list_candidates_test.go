package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

const (
	candidateTopicA = "procedural/candidates/repeated-success/probe-a"
	candidateTopicB = "procedural/candidates/failure-recovery/sqlite/override-home"
	draftTopicA     = "procedural/drafts/repeated-success/probe-a"
)

type candidateObservationsSource struct {
	observations skills.CandidateObservations
	err          error
	project      string
}

func (s *candidateObservationsSource) ListCandidateObservations(project string) (skills.CandidateObservations, error) {
	s.project = project
	return s.observations, s.err
}

func TestListCandidatesParsesSortsFiltersAndOptionallyIncludesDrafts(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{
		Candidates: []skills.CandidateObservation{
			{TopicKey: candidateTopicB, Content: "**Kind**: failure-recovery\n**Status**: emitted\n**Threshold**: 2\n"},
			{TopicKey: candidateTopicA, Content: "**Kind**: repeated-success\n**Status**: emitted\n**Threshold**: 3\n**Disposition**: extend:probe\n**Draft**: " + draftTopicA + "\n"},
		},
		Drafts: []skills.CandidateObservation{{TopicKey: draftTopicA, Content: "**Candidate**: " + candidateTopicA + "\n**Status**: open\n**ForRevision**: 2\n"}},
	}}
	got, err := ListCandidates(source, CandidateListInput{
		Project: "example/project", Statuses: []skills.CandidateStatus{skills.StatusEmitted},
		Kind: "repeated-success", IncludeDrafts: true,
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if source.project != "example/project" {
		t.Errorf("source project = %q, want example/project", source.project)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Key != candidateTopicA {
		t.Fatalf("Candidates = %+v, want only sorted/filter-matching candidate %q", got.Candidates, candidateTopicA)
	}
	if got.Candidates[0].Threshold != 3 || got.Candidates[0].OccurrenceCount != 0 || got.Candidates[0].Disposition != "extend:probe" {
		t.Errorf("candidate fields lost: %+v", got.Candidates[0])
	}
	if len(got.Drafts) != 1 || got.Drafts[0].Key != draftTopicA || got.Drafts[0].ForRevision != 2 {
		t.Errorf("Drafts = %+v, want parsed draft %q revision 2", got.Drafts, draftTopicA)
	}
	if len(got.Anomalies) != 0 {
		t.Errorf("Anomalies = %+v, want none", got.Anomalies)
	}
}

func TestListCandidatesReturnsAllCandidatesSortedAndAnomaliesWithoutDraftOutput(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{
		Candidates: []skills.CandidateObservation{
			{TopicKey: candidateTopicB, Content: "**Kind**: failure-recovery\n**Status**: registered\n"},
			{TopicKey: candidateTopicA, Content: "**Kind**: repeated-success\n**Status**: emitted\n**Draft**: " + draftTopicA + "\n"},
		},
		Drafts: []skills.CandidateObservation{{TopicKey: draftTopicA, Content: "**Candidate**: " + candidateTopicA + "\n**Status**: open\n**ForRevision**: 1\n"}},
	}}
	got, err := ListCandidates(source, CandidateListInput{Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != 2 || got.Candidates[0].Key != candidateTopicB || got.Candidates[1].Key != candidateTopicA {
		t.Errorf("Candidates are not sorted by key: %+v", got.Candidates)
	}
	if got.Drafts != nil {
		t.Errorf("Drafts = %+v; drafts should be omitted without IncludeDrafts", got.Drafts)
	}
	if len(got.Anomalies) != 0 {
		t.Errorf("Anomalies = %+v, want none", got.Anomalies)
	}
}

func TestListCandidatesReconcilesTheUnfilteredRecords(t *testing.T) {
	missingDraft := "procedural/drafts/failure-recovery/sqlite/missing"
	source := &candidateObservationsSource{observations: skills.CandidateObservations{
		Candidates: []skills.CandidateObservation{{TopicKey: candidateTopicA, Content: "**Kind**: repeated-success\n**Status**: emitted\n**Draft**: " + missingDraft + "\n"}},
		Drafts:     []skills.CandidateObservation{{TopicKey: draftTopicA, Content: "**Candidate**: procedural/candidates/failure-recovery/sqlite/missing\n**Status**: open\n**ForRevision**: 1\n"}},
	}}
	got, err := ListCandidates(source, CandidateListInput{Project: "p", Kind: "unmatched"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != 0 {
		t.Errorf("filtered candidates = %+v, want empty", got.Candidates)
	}
	if len(got.Anomalies) != 2 {
		t.Errorf("Anomalies = %+v, want missing-draft and orphan-draft diagnostics from full source", got.Anomalies)
	}
}

func TestListCandidatesPropagatesSourceFailuresAndRejectsMissingSource(t *testing.T) {
	cause := errors.New("Engram unavailable")
	if _, err := ListCandidates(&candidateObservationsSource{err: cause}, CandidateListInput{Project: "p"}); !errors.Is(err, cause) {
		t.Errorf("source error = %v, want wrapped cause", err)
	}
	if _, err := ListCandidates(nil, CandidateListInput{Project: "p"}); err == nil {
		t.Fatal("ListCandidates accepted a missing CandidateSource")
	}
}

func TestListCandidatesDoesNotMutateSourceObservations(t *testing.T) {
	observations := skills.CandidateObservations{Candidates: []skills.CandidateObservation{
		{TopicKey: candidateTopicB, Content: "**Kind**: failure-recovery\n**Status**: emitted\n"},
		{TopicKey: candidateTopicA, Content: "**Kind**: repeated-success\n**Status**: emitted\n"},
	}}
	before := skills.CandidateObservations{Candidates: append([]skills.CandidateObservation(nil), observations.Candidates...)}
	if _, err := ListCandidates(&candidateObservationsSource{observations: observations}, CandidateListInput{Project: "p"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observations, before) {
		t.Fatalf("source observations changed: %+v", observations)
	}
}
