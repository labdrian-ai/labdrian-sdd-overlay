package main

// The create-time record of memory:procedural-skills (C23 M8 decision 4): the deps hand the presence
// prober the lock of the project the process runs in, so the record stops saying a project with a
// lock has no procedural skill to read.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

func TestTheProceduralLockPathIsUnderTheWorktreeRootOfTheWorkingDirectory(t *testing.T) {
	repo := fixtureRepo(t, "repo")
	d := testDeps()
	d.getwd = func() (string, error) { return repo, nil }
	want := filepath.Join(repo, filepath.FromSlash(skills.ProjectLockRelPath))
	if got := d.proceduralLockPath(); got != want {
		t.Errorf("proceduralLockPath() = %q, want %q", got, want)
	}

	d.getwd = func() (string, error) { return t.TempDir(), nil }
	if got := d.proceduralLockPath(); got != "" {
		t.Errorf("outside every worktree: proceduralLockPath() = %q, want none", got)
	}
	d.getwd = nil
	if got := d.proceduralLockPath(); got != "" {
		t.Errorf("without a working directory: proceduralLockPath() = %q, want none", got)
	}
}

func TestTheDepsProberSeesTheProceduralLockOfTheProject(t *testing.T) {
	repo := fixtureRepo(t, "repo")
	d := testDeps()
	d.getwd = func() (string, error) { return repo, nil }
	observe := func() workflow.Observation {
		t.Helper()
		got, err := d.dependencyProber().Probe(context.Background(), []string{"memory:procedural-skills"})
		if err != nil || len(got) != 1 {
			t.Fatalf("Probe = %+v, %v", got, err)
		}
		return got[0]
	}
	if got := observe(); got.Status != workflow.ObservationUnavailable {
		t.Errorf("no lock: %+v, want unavailable", got)
	}
	writeFixtureFile(t, filepath.Join(repo, filepath.FromSlash(skills.ProjectLockRelPath)), `{"version":1,"skills":[]}`)
	if got := observe(); got.Status != workflow.ObservationAvailable {
		t.Errorf("with a lock: %+v, want available", got)
	}
}
