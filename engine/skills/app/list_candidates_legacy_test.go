package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

type legacyResolverFake struct {
	mapped map[string]int
	err    error
	calls  [][]string
}

func (r *legacyResolverFake) ResolveLegacyOccurrences(numbers []string) (map[string]int, error) {
	r.calls = append(r.calls, append([]string(nil), numbers...))
	return r.mapped, r.err
}

func legacyCandidate(count string, ids ...string) string {
	body := "**Kind**: repeated-success\n**Status**: emitted\n**Threshold**: 2\n**OccurrenceCount**: " + count + "\n**Occurrences**:\n"
	for _, id := range ids {
		body += "- " + id + " @ 2026-09-15T10:04:00Z — seen\n"
	}
	return body
}

func TestListCandidatesCountsALegacyIDAndItsMemorycardIDOnce(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{Candidates: []skills.CandidateObservation{
		{TopicKey: candidateTopicA, Content: legacyCandidate("1", "engram:3401", "memorycard:77")},
	}}}
	resolver := &legacyResolverFake{mapped: map[string]int{"3401": 77}}
	got, err := ListCandidates(source, resolver, CandidateListInput{Project: "p"})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(got.Anomalies) != 0 {
		t.Fatalf("Anomalies = %+v; want none: one memory listed under both schemes counts once", got.Anomalies)
	}
	if got.Candidates[0].OccurrenceCount != 1 || !reflect.DeepEqual(got.Candidates[0].Occurrences, []string{"engram:3401", "memorycard:77"}) {
		t.Fatalf("record = %+v; want the stated count and the ids as written", got.Candidates[0])
	}
}

func TestListCandidatesAnUnknownLegacyIDCountsAsItself(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{Candidates: []skills.CandidateObservation{
		{TopicKey: candidateTopicA, Content: legacyCandidate("2", "engram:9999", "memorycard:77")},
	}}}
	resolver := &legacyResolverFake{mapped: map[string]int{}}
	got, err := ListCandidates(source, resolver, CandidateListInput{Project: "p"})
	if err != nil || len(got.Anomalies) != 0 {
		t.Fatalf("got anomalies %+v, err %v; want an unmapped id to count as itself", got.Anomalies, err)
	}
}

func TestListCandidatesTwoLegacyIDsOfOneMemoryCountOnce(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{Candidates: []skills.CandidateObservation{
		{TopicKey: candidateTopicA, Content: legacyCandidate("1", "engram:3401", "engram:3407")},
	}}}
	resolver := &legacyResolverFake{mapped: map[string]int{"3401": 77, "3407": 77}}
	got, err := ListCandidates(source, resolver, CandidateListInput{Project: "p"})
	if err != nil || len(got.Anomalies) != 0 {
		t.Fatalf("got anomalies %+v, err %v; want two observations merged on import to count as 1", got.Anomalies, err)
	}
}

func TestListCandidatesCallsTheResolverOnceWithTheSortedDistinctLegacyNumbers(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{
		Candidates: []skills.CandidateObservation{
			{TopicKey: candidateTopicA, Content: legacyCandidate("3", "engram:20", "engram:3", "memorycard:5")},
			{TopicKey: candidateTopicB, Content: legacyCandidate("2", "engram:3", "engram:100")},
		},
		Drafts: []skills.CandidateObservation{{TopicKey: draftTopicA, Content: "**Candidate**: " + candidateTopicA + "\n**Status**: open\n**Occurrences**:\n- engram:999\n"}},
	}}
	resolver := &legacyResolverFake{mapped: map[string]int{}}
	if _, err := ListCandidates(source, resolver, CandidateListInput{Project: "p"}); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"100", "20", "3"}}; !reflect.DeepEqual(resolver.calls, want) {
		t.Fatalf("resolver calls = %v; want one call with the sorted (as text) distinct legacy numbers of the candidates %v", resolver.calls, want)
	}
}

func TestListCandidatesNeverCallsTheResolverWhenNoLegacyIDIsListed(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{Candidates: []skills.CandidateObservation{
		{TopicKey: candidateTopicA, Content: legacyCandidate("1", "memorycard:77")},
		{TopicKey: candidateTopicB, Content: "**Kind**: failure-recovery\n**Status**: emitted\n"},
	}}}
	resolver := &legacyResolverFake{}
	if _, err := ListCandidates(source, resolver, CandidateListInput{Project: "p"}); err != nil {
		t.Fatal(err)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("resolver called %v; want no call without a legacy id", resolver.calls)
	}
	if _, err := ListCandidates(source, nil, CandidateListInput{Project: "p"}); err != nil {
		t.Fatalf("a nil resolver with no legacy id must not matter: %v", err)
	}
}

func TestListCandidatesResolverFailureIsAnErrorWithTheCause(t *testing.T) {
	source := &candidateObservationsSource{observations: skills.CandidateObservations{Candidates: []skills.CandidateObservation{
		{TopicKey: candidateTopicA, Content: legacyCandidate("1", "engram:3401")},
	}}}
	cause := errors.New(`unknown command "resolve"`)
	if _, err := ListCandidates(source, &legacyResolverFake{err: cause}, CandidateListInput{Project: "p"}); !errors.Is(err, cause) {
		t.Fatalf("error = %v; want the resolver's cause wrapped, never an empty result", err)
	}
	if _, err := ListCandidates(source, nil, CandidateListInput{Project: "p"}); err == nil || !strings.Contains(err.Error(), "resolver") {
		t.Fatalf("a legacy id with no resolver wired: error = %v; want a refusal naming the resolver", err)
	}
}
