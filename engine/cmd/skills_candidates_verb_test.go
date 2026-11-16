package main

import (
	"bytes"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The program hands every skills verb its arguments with the verb among them
// (runSkillsWithStdin passes args whole), so `labdrian skills candidates --json`
// reaches skillsCandidates as ["candidates", "--json"]. The verb word is not an
// unexpected argument.
func TestSkillsCandidatesAcceptsTheVerbAmongItsArguments(t *testing.T) {
	read := false
	ports := skills.Deps{
		Candidates: candidateSourceFunc(func(string) (skills.CandidateObservations, error) {
			read = true
			return skills.CandidateObservations{}, nil
		}),
		Identity: &recordedIdentity{},
		Cwd:      func() (string, error) { return "/project", nil },
	}
	args := []string{"candidates", "--json"}
	var stdout, stderr bytes.Buffer
	code := -1
	runSkillsCoreWith(ports, verbFromArgs(args), args, &stdout, &stderr, func(v int) { code = v })
	if code > 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q; want success with the verb among the arguments", code, stderr.String())
	}
	if !read {
		t.Fatal("the candidate source was not read")
	}
}

// A second positional word after the verb is still refused.
func TestSkillsCandidatesRefusesAWordAfterTheVerb(t *testing.T) {
	ports := skills.Deps{Candidates: candidateSourceFunc(func(string) (skills.CandidateObservations, error) {
		t.Fatal("the source was read although the arguments were refused")
		return skills.CandidateObservations{}, nil
	})}
	args := []string{"candidates", "extra"}
	var stdout, stderr bytes.Buffer
	code := 0
	runSkillsCoreWith(ports, verbFromArgs(args), args, &stdout, &stderr, func(v int) { code = v })
	if code == 0 || !bytes.Contains(stderr.Bytes(), []byte(`"extra"`)) {
		t.Fatalf("exit %d, stderr %q; want a refusal naming \"extra\"", code, stderr.String())
	}
}
