package main

// The create-time record of memory:procedural-skills (C23 M8 decision 4): the deps hand the presence
// prober the project the process runs in, so the record stops saying a project with a lock has no
// procedural skill to read. These tests say which project that is; what the record says of its lock
// is in procedural_presence_test.go.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The project is the worktree that holds the working directory: its lock is the one asked for from a
// directory below the root, the lock of another worktree is not, and a directory in no worktree, or no
// working directory at all, is a project the probe does not know.
func TestTheProjectOfTheProceduralLockIsTheWorktreeOfTheWorkingDirectory(t *testing.T) {
	const unknown = "the project directory is unknown, so the procedural skills lock was not looked for"
	lockPath := func(root string) string { return filepath.Join(root, filepath.FromSlash(skills.ProjectLockRelPath)) }
	withLock := fixtureRepo(t, "with-lock")
	writeFixtureFile(t, lockPath(withLock), `{"version":1,"skills":[]}`)
	withoutLock := fixtureRepo(t, "without-lock")
	below := filepath.Join(withLock, "docs", "design")
	writeFixtureFile(t, filepath.Join(below, "notes.md"), "notes")

	for _, tc := range []struct {
		name       string
		getwd      func() (string, error)
		wantStatus string
		wantDetail string // "" when only the status is pinned
	}{
		{"the root of a worktree with a lock", func() (string, error) { return withLock, nil }, workflow.ObservationAvailable, ""},
		{"a directory below the root reads the lock of the root", func() (string, error) { return below, nil }, workflow.ObservationAvailable, ""},
		{"another worktree has no lock of its own", func() (string, error) { return withoutLock, nil }, workflow.ObservationUnavailable, "the procedural skills lock was not found: no such file"},
		{"a directory in no worktree", func() (string, error) { return t.TempDir(), nil }, workflow.ObservationUnavailable, unknown},
		{"no working directory", nil, workflow.ObservationUnavailable, unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDeps()
			d.getwd = tc.getwd
			got := proceduralObservation(t, d)
			if got.Status != tc.wantStatus || (tc.wantDetail != "" && got.Detail != tc.wantDetail) {
				t.Errorf("observation = %+v, want %s %q", got, tc.wantStatus, tc.wantDetail)
			}
			if got.Detail == "" {
				t.Errorf("observation %+v has no detail", got)
			}
		})
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
