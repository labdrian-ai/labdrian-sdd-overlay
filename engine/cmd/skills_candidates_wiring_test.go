package main

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/memorycardcli"
)

func TestTheCandidateSourceOfTheProductionDepsIsMemorycard(t *testing.T) {
	source, ok := newSkillsDeps(deps{}).Candidates.(memorycardcli.Source)
	if !ok {
		t.Fatalf("Candidates = %T; want memorycardcli.Source", newSkillsDeps(deps{}).Candidates)
	}
	if source.Runner == nil {
		t.Error("the production source has no command runner")
	}
}

func TestUsageSaysSkillsCandidatesReadsMemorycard(t *testing.T) {
	var line string
	for _, l := range strings.Split(captureUsage(t), "\n") {
		if strings.Contains(l, "candidate and draft listing") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("usage has no candidate and draft listing line")
	}
	if !strings.Contains(line, "memorycard") || strings.Contains(line, "Engram") {
		t.Errorf("help line = %q; want it to name memorycard and not Engram", line)
	}
}
