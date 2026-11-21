package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

type legacyResolverFunc func([]string) (map[string]int, error)

func (f legacyResolverFunc) ResolveLegacyOccurrences(numbers []string) (map[string]int, error) {
	return f(numbers)
}

func legacyCandidatesPorts(root, occurrences string, resolver skills.LegacyOccurrenceResolver) skills.Deps {
	return skills.Deps{
		Candidates: candidateSourceFunc(func(string) (skills.CandidateObservations, error) {
			return skills.CandidateObservations{Candidates: []skills.CandidateObservation{{
				TopicKey: "procedural/candidates/repeated-success/probe-a",
				Content:  "**Kind**: repeated-success\n**Status**: emitted\n**Threshold**: 2\n**OccurrenceCount**: 1\n**Occurrences**:\n" + occurrences,
			}}}, nil
		}),
		LegacyOccurrences: resolver,
		Identity:          &recordedIdentity{},
		Cwd:               func() (string, error) { return root, nil },
	}
}

func TestSkillsCandidatesCountsALegacyIDAndItsMemorycardIDOnce(t *testing.T) {
	root := t.TempDir()
	var asked []string
	ports := legacyCandidatesPorts(root, "- engram:3401 @ t — a\n- memorycard:77 @ t — b\n",
		legacyResolverFunc(func(numbers []string) (map[string]int, error) {
			asked = numbers
			return map[string]int{"3401": 77}, nil
		}))
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--project-root", root, "--json"}, &stdout, &stderr, func(v int) { code = v })
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"anomalies":[]`) {
		t.Fatalf("exit %d, stderr %q, stdout %q; want success and no anomaly", code, stderr.String(), stdout.String())
	}
	if len(asked) != 1 || asked[0] != "3401" {
		t.Fatalf("resolver asked %v; want [3401]", asked)
	}
}

func TestSkillsCandidatesResolverFailureExitsTwoWithTheCause(t *testing.T) {
	root := t.TempDir()
	cause := errors.New(`memorycard candidates: resolve: unknown command "resolve"`)
	ports := legacyCandidatesPorts(root, "- engram:3401 @ t — a\n",
		legacyResolverFunc(func([]string) (map[string]int, error) { return nil, cause }))
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--project-root", root}, &stdout, &stderr, func(v int) { code = v })
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), cause.Error()) {
		t.Fatalf("exit %d, stdout %q, stderr %q; want exit 2 with the cause and no listing", code, stdout.String(), stderr.String())
	}
}

func TestSkillsCandidatesWithNoLegacyIDNeverCallsTheResolver(t *testing.T) {
	root := t.TempDir()
	ports := legacyCandidatesPorts(root, "- memorycard:77 @ t — a\n",
		legacyResolverFunc(func([]string) (map[string]int, error) {
			t.Fatal("resolver called for a record with no legacy id")
			return nil, nil
		}))
	var stdout, stderr bytes.Buffer
	var code int
	runSkillsCoreWith(ports, "candidates", []string{"--project-root", root}, &stdout, &stderr, func(v int) { code = v })
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q; want success", code, stderr.String())
	}
}
