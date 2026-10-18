package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_DispatchesSkillsStaleSubcommand(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "missing-engram.db")
	t.Setenv(engramDBEnvVar, dbPath)

	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"skills-stale", "--project-root", root, "--project", "fixture-project"})
	})
	if exit != exitEngramUnavailable {
		t.Fatalf("run([skills-stale ...]) = %d, want %d, proving dispatch reaches the detector command", exit, exitEngramUnavailable)
	}
	if !strings.Contains(stderr, "skills-stale") {
		t.Fatalf("skills-stale error does not name the command: %q", stderr)
	}
}

// Since C23 M2 (decision 4) a root in no git repository has a project, its directory's name, so
// skills-stale with no --project no longer refuses there for want of one (it was exit 2, "--project
// is required when the project root cannot be resolved"): it goes on to the memory it checks, here
// an Engram that cannot be opened (exit 4). With an Engram and no lock, the golden
// project-identity-73 pins the missing lock (exit 1).
func TestCmdSkillsStale_ARootInNoRepositoryIsNamedByItsDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "missing-engram.db"))

	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"skills-stale", "--project-root", root})
	})
	if exit != exitEngramUnavailable {
		t.Fatalf("skills-stale of a root in no repository = %d, want %d (it reaches Engram, not a refusal for want of a project)", exit, exitEngramUnavailable)
	}
	if strings.Contains(stderr, "--project is required") || !strings.Contains(stderr, "engram:") {
		t.Fatalf("stderr = %q, want Engram's failure and no demand for --project", stderr)
	}
}

func TestCmdSkillsStale_RequiresAbsoluteProjectRoot(t *testing.T) {
	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"skills-stale", "--project-root", "relative", "--project", "fixture-project"})
	})
	if exit != exitUsage {
		t.Fatalf("relative skills-stale project root exit = %d, want %d", exit, exitUsage)
	}
	if !strings.Contains(stderr, "absolute") {
		t.Fatalf("relative project-root refusal does not explain the absolute-path requirement: %q", stderr)
	}
}
