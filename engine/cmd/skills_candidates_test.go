package main

import (
	"bytes"
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

var updateSkillsCandidatesGolden = flag.Bool("update-skills-candidates-golden", false, "rewrite skills candidates output goldens")

type candidateSourceFunc func(string) (skills.CandidateObservations, error)

func (f candidateSourceFunc) ListCandidateObservations(project string) (skills.CandidateObservations, error) {
	return f(project)
}

func TestSkillsCandidatesTextGolden(t *testing.T) {
	root := t.TempDir()
	identity := &recordedIdentity{}
	ports := skills.Deps{
		Candidates: candidateSourceFunc(func(project string) (skills.CandidateObservations, error) {
			if project != "p" {
				t.Fatalf("source project = %q, want p", project)
			}
			return skills.CandidateObservations{
				Candidates: []skills.CandidateObservation{
					{TopicKey: "procedural/candidates/repeated-success/probe-a", Content: "**Kind**: repeated-success\n**Status**: emitted\n**Threshold**: 3\n**OccurrenceCount**: 0\n**Disposition**: new\n**Draft**: procedural/drafts/repeated-success/probe-a\n"},
					{TopicKey: "procedural/candidates/failure-recovery/sqlite/override-home", Content: "**Kind**: failure-recovery\n**Status**: registered\n"},
				},
				Drafts: []skills.CandidateObservation{{TopicKey: "procedural/drafts/repeated-success/probe-a", Content: "**Candidate**: procedural/candidates/repeated-success/probe-a\n**Status**: open\n**ForRevision**: 2\n"}},
			}, nil
		}),
		Identity: identity,
		Cwd:      func() (string, error) { return root, nil },
	}
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--project-root", root, "--status", "registered", "--status", "emitted", "--drafts"}, &stdout, &stderr, func(v int) { code = v })
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q; want success", code, stderr.String())
	}
	if len(identity.queries) != 1 || identity.queries[0].Dir != root {
		t.Fatalf("identity queries = %+v, want project root %q", identity.queries, root)
	}
	checkGoldenIn(t, "skills-candidates-golden", "lists-filtered-candidates", stdout.String(), updateSkillsCandidatesGolden, "-update-skills-candidates-golden")
}

func TestSkillsCandidatesJSONGolden(t *testing.T) {
	ports := skills.Deps{
		Candidates: candidateSourceFunc(func(project string) (skills.CandidateObservations, error) {
			return skills.CandidateObservations{Candidates: []skills.CandidateObservation{{
				TopicKey: "procedural/candidates/repeated-success/probe-a",
				Content:  "**Kind**: repeated-success\n**Status**: emitted\n**Threshold**: 3\n**OccurrenceCount**: 0\n",
			}}}, nil
		}),
		Identity: &recordedIdentity{},
		Cwd:      func() (string, error) { return "/project", nil },
	}
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--json"}, &stdout, &stderr, func(v int) { code = v })
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q; want success", code, stderr.String())
	}
	checkGoldenIn(t, "skills-candidates-golden", "json-stable-shape", stdout.String(), updateSkillsCandidatesGolden, "-update-skills-candidates-golden")
}

func TestSkillsCandidatesUsageRefusalPrecedesSourceRead(t *testing.T) {
	called := false
	ports := skills.Deps{Candidates: candidateSourceFunc(func(string) (skills.CandidateObservations, error) {
		called = true
		return skills.CandidateObservations{}, nil
	})}
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--status", "unknown"}, &stdout, &stderr, func(v int) { code = v })
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "candidate status") || called {
		t.Fatalf("exit %d, stdout %q, stderr %q, source called %v", code, stdout.String(), stderr.String(), called)
	}
}

func TestSkillsCandidatesSourceFailureIsNotReportedAsEmpty(t *testing.T) {
	cause := errors.New("engram is unavailable")
	ports := skills.Deps{
		Candidates: candidateSourceFunc(func(string) (skills.CandidateObservations, error) { return skills.CandidateObservations{}, cause }),
		Identity:   &recordedIdentity{},
		Cwd:        func() (string, error) { return "/project", nil },
	}
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", nil, &stdout, &stderr, func(v int) { code = v })
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), cause.Error()) || strings.Contains(stdout.String(), "no candidates") {
		t.Fatalf("exit %d, stdout %q, stderr %q; want source failure, not empty result", code, stdout.String(), stderr.String())
	}
}

func TestSkillsCandidatesRejectsNonAbsoluteProjectRootAndPreservesStatusFilterOrder(t *testing.T) {
	ports := skills.Deps{Candidates: candidateSourceFunc(func(string) (skills.CandidateObservations, error) {
		t.Fatal("source called for a relative project root")
		return skills.CandidateObservations{}, nil
	})}
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--project-root", "relative"}, &stdout, &stderr, func(v int) { code = v })
	if code != 1 || !strings.Contains(stderr.String(), "absolute") {
		t.Fatalf("exit %d, stderr %q; want absolute-root usage refusal", code, stderr.String())
	}

	parsed, err := skillsCandidatesSpec.parse([]string{"--status", "emitted", "--status", "registered"})
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.allValues(flagCandidateStatus); !reflect.DeepEqual(got, []string{"emitted", "registered"}) {
		t.Fatalf("repeated status values = %v", got)
	}
}
