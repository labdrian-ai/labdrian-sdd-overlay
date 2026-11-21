package skills

import "testing"

func TestCandidateTopicFamilySortsTopicsWithAndWithoutAGoalPrefix(t *testing.T) {
	cases := []struct {
		topic string
		want  CandidateFamily
	}{
		{"procedural/candidates/repeated-success/one", FamilyCandidate},
		{"procedural/drafts/repeated-success/one", FamilyDraft},
		{"goal:g-42/procedural/candidates/x", FamilyCandidate},
		{"goal:g-42/procedural/drafts/x", FamilyDraft},
		{"procedural/candidates/", FamilyCandidate},
		{"project/other", FamilyNone},
		{"goal:g-42/project/other", FamilyNone},
		{"goal:g-42", FamilyNone},
		{"goal:/procedural/candidates/x", FamilyNone},
		{"goal:procedural/candidates/x", FamilyNone},
		{"", FamilyNone},
	}
	for _, tc := range cases {
		if got := CandidateTopicFamily(tc.topic); got != tc.want {
			t.Errorf("CandidateTopicFamily(%q) = %v; want %v", tc.topic, got, tc.want)
		}
	}
}
